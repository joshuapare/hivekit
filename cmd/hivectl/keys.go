package main

import (
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	keysRecursive bool
	keysDepth     int
)

func init() {
	cmd := newKeysCmd()
	cmd.Flags().BoolVarP(&keysRecursive, "recursive", "r", false, "List all subkeys recursively")
	cmd.Flags().IntVar(&keysDepth, "depth", 1, "Maximum recursion depth (0 = unlimited)")
	rootCmd.AddCommand(cmd)
}

func newKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys <hive> [path]",
		Short: "List keys at a given path",
		Long: `The keys command lists all subkeys at a given path in a registry hive.
If no path is specified, lists keys at the root.

Example:
  hivectl keys system.hive
  hivectl keys system.hive "ControlSet001\\Services"
  hivectl keys system.hive --recursive --depth 2
  hivectl keys system.hive "Software" --json`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKeys(args)
		},
	}
	return cmd
}

func runKeys(args []string) error {
	hivePath := args[0]
	var keyPath string
	if len(args) > 1 {
		keyPath = args[1]
	}

	printVerbose("Opening hive: %s\n", hivePath)

	// List keys using public API
	keys, err := hive.ListKeys(hivePath, keyPath, keysRecursive, keysDepth)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":  hivePath,
			"path":  keyPath,
			"keys":  keys,
			"count": len(keys),
		}
		return printJSON(result)
	}

	// Text output
	if keyPath != "" {
		printInfo("\nKeys in %s:\n", keyPath)
	} else {
		printInfo("\nKeys at root:\n")
	}

	for _, key := range keys {
		if keysRecursive {
			printInfo("  %s\n", key.Path)
		} else {
			printInfo("  %s\n", key.Name)
		}
	}

	printInfo("\nTotal: %d keys\n", len(keys))

	return nil
}
