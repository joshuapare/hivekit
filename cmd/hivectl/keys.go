package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
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

	// Open hive with new backend
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive: %w", err)
	}
	defer h.Close()

	// Configure printer options
	opts := printer.DefaultOptions()
	opts.ShowValues = false    // Keys only, no values
	opts.PrintMetadata = false // No metadata, just names

	// Set depth: non-recursive shows immediate children (depth 2), recursive uses user depth
	if keysRecursive {
		opts.MaxDepth = keysDepth
	} else {
		opts.MaxDepth = 2 // Root + 1 level of children
	}

	// Set format
	if jsonOut {
		opts.Format = printer.FormatJSON
	} else {
		opts.Format = printer.FormatText
	}

	return h.PrintTree(os.Stdout, keyPath, opts)
}
