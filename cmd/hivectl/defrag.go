package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var noBackup bool

func init() {
	cmd := newDefragCmd()
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "Don't create backup before defragmenting")
	rootCmd.AddCommand(cmd)
}

func newDefragCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "defrag <hive>",
		Short: "Defragment and compact a hive file",
		Long: `The defrag command optimizes a Windows registry hive file by removing
fragmentation and compacting the structure. This can reduce file size and
improve performance.

By default, a backup is created before defragmentation.

Example:
  hivectl defrag system.hive
  hivectl defrag system.hive --no-backup`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDefrag(args)
		},
	}
	return cmd
}

func runDefrag(args []string) error {
	hivePath := args[0]

	printVerbose("Defragmenting hive: %s\n", hivePath)

	// Get original file size
	origStat, err := os.Stat(hivePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	origSize := origStat.Size()

	printInfo("\nDefragmenting %s...\n", hivePath)
	if !noBackup {
		printInfo("  Creating backup...\n")
	}

	// Defragment the hive
	if err := hive.Defragment(hivePath); err != nil {
		return fmt.Errorf("defragmentation failed: %w", err)
	}

	// Get new file size
	newStat, err := os.Stat(hivePath)
	if err != nil {
		return fmt.Errorf("failed to stat file after defrag: %w", err)
	}
	newSize := newStat.Size()

	saved := origSize - newSize
	savedPercent := 0.0
	if origSize > 0 {
		savedPercent = float64(saved) * 100.0 / float64(origSize)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"file":          hivePath,
			"original_size": origSize,
			"new_size":      newSize,
			"saved_bytes":   saved,
			"saved_percent": savedPercent,
			"backup":        !noBackup,
		}
		return printJSON(result)
	}

	// Text output
	printInfo("  Original size: ")
	if origSize < 1024 {
		printInfo("%d bytes\n", origSize)
	} else if origSize < 1024*1024 {
		printInfo("%.1f KB\n", float64(origSize)/1024)
	} else {
		printInfo("%.1f MB\n", float64(origSize)/(1024*1024))
	}

	printInfo("  Compacted size: ")
	if newSize < 1024 {
		printInfo("%d bytes\n", newSize)
	} else if newSize < 1024*1024 {
		printInfo("%.1f KB\n", float64(newSize)/1024)
	} else {
		printInfo("%.1f MB\n", float64(newSize)/(1024*1024))
	}

	if saved > 0 {
		printInfo("  Saved: ")
		if saved < 1024 {
			printInfo("%d bytes", saved)
		} else if saved < 1024*1024 {
			printInfo("%.1f KB", float64(saved)/1024)
		} else {
			printInfo("%.1f MB", float64(saved)/(1024*1024))
		}
		printInfo(" (%.1f%%)\n", savedPercent)
	}

	if !noBackup {
		printInfo("\nBackup: %s.bak\n", hivePath)
	}
	printInfo("✓ Defragmentation complete\n")

	return nil
}
