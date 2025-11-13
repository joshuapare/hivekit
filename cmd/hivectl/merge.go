package main

import (
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	mergeBackup bool
	mergeDefrag bool
	mergeLimits string
)

func init() {
	cmd := newMergeCmd()
	cmd.Flags().BoolVarP(&mergeBackup, "backup", "b", true, "Create backup before merging")
	cmd.Flags().BoolVar(&mergeDefrag, "defrag", false, "Defragment after merge")
	cmd.Flags().StringVar(&mergeLimits, "limits", "default", "Limits preset (default, strict, relaxed)")
	rootCmd.AddCommand(cmd)
}

func newMergeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge <hive> <regfile>...",
		Short: "Merge one or more .reg files into a hive",
		Long: `The merge command applies registry changes from .reg files to a Windows
registry hive. Multiple .reg files can be merged in a single operation.

By default, a backup is created before merging.

Example:
  hivectl merge system.hive changes.reg
  hivectl merge system.hive base.reg patch1.reg patch2.reg
  hivectl merge system.hive changes.reg --limits strict`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMerge(args)
		},
	}
	return cmd
}

func runMerge(args []string) error {
	hivePath := args[0]
	regFiles := args[1:]

	printVerbose("Merging into hive: %s\n", hivePath)
	printVerbose("Reg files: %v\n", regFiles)

	// Get the limits based on the preset
	var limits *hive.Limits
	switch mergeLimits {
	case "default":
		l := hive.DefaultLimits()
		limits = &l
	case "strict":
		l := hive.StrictLimits()
		limits = &l
	case "relaxed":
		l := hive.RelaxedLimits()
		limits = &l
	default:
		return fmt.Errorf("unknown limits preset: %s", mergeLimits)
	}

	// Prepare options
	opts := &hive.MergeOptions{
		Limits:       limits,
		CreateBackup: mergeBackup,
		Defragment:   mergeDefrag,
	}

	printInfo("\nMerging into %s:\n", hivePath)

	// Merge each file
	for _, regFile := range regFiles {
		printInfo("  Processing %s...\n", regFile)

		if err := hive.MergeRegFile(hivePath, regFile, opts); err != nil {
			return fmt.Errorf("failed to merge %s: %w", regFile, err)
		}

		printInfo("  ✓ %s merged\n", regFile)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":       hivePath,
			"reg_files":  regFiles,
			"backup":     mergeBackup,
			"defragment": mergeDefrag,
			"success":    true,
		}
		return printJSON(result)
	}

	// Text output
	if mergeBackup {
		printInfo("\nBackup: %s.bak\n", hivePath)
	}
	printInfo("✓ Merge complete\n")

	return nil
}
