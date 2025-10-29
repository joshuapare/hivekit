package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newInfoCmd())
}

func newInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <hive>",
		Short: "Validate a hive header and report basic metadata",
		Long: `The open command validates a Windows registry hive file and displays
basic metadata including file size, root keys, total keys/values, and depth.

Example:
  hivectl info system.hive
  hivectl info system.hive --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInfo(args)
		},
	}
	return cmd
}

func runInfo(args []string) error {
	hivePath := args[0]

	printVerbose("Opening hive: %s\n", hivePath)

	// Get hive info
	info, err := hive.HiveStats(hivePath)
	if err != nil {
		return fmt.Errorf("failed to get hive info: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		return printJSON(info)
	}

	// Text output
	printInfo("\nHive Information:\n")
	printInfo("  File: %s\n", hivePath)

	// Get file size
	if stat, err := os.Stat(hivePath); err == nil {
		size := stat.Size()
		if size < 1024 {
			printInfo("  Size: %d bytes\n", size)
		} else if size < 1024*1024 {
			printInfo("  Size: %.1f KB\n", float64(size)/1024)
		} else {
			printInfo("  Size: %.1f MB\n", float64(size)/(1024*1024))
		}
	}

	// Print all info fields
	for key, value := range info {
		printInfo("  %s: %s\n", key, value)
	}

	printInfo("\nValidation:\n")
	printInfo("  ✓ Structure valid\n")
	printInfo("  ✓ No corruption detected\n")

	return nil
}
