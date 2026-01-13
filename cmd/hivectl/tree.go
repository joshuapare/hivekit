package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
	"github.com/spf13/cobra"
)

var (
	treeDepth   int
	treeValues  bool
	treeCompact bool
	treeASCII   bool
)

func init() {
	cmd := newTreeCmd()
	cmd.Flags().IntVar(&treeDepth, "depth", 3, "Maximum depth")
	cmd.Flags().BoolVar(&treeValues, "values", false, "Show values too")
	cmd.Flags().BoolVar(&treeCompact, "compact", false, "Compact output")
	cmd.Flags().BoolVar(&treeASCII, "ascii", false, "ASCII-only characters")
	rootCmd.AddCommand(cmd)
}

func newTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree <hive> [path]",
		Short: "Display tree structure",
		Long: `The tree command displays a hierarchical tree view of registry keys.

Example:
  hivectl tree system.hive
  hivectl tree system.hive "ControlSet001\\Services" --depth 2
  hivectl tree system.hive --values --depth 1`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTree(args)
		},
	}
	return cmd
}

func runTree(args []string) error {
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
	opts.ShowValues = treeValues
	opts.ShowValueTypes = true
	opts.MaxDepth = treeDepth
	opts.PrintMetadata = false // Tree command shows clean structure without metadata counts

	// Handle JSON output
	if jsonOut {
		opts.Format = printer.FormatJSON
		return h.PrintTree(os.Stdout, keyPath, opts)
	}

	// Text output
	opts.Format = printer.FormatText

	// Adjust indentation for compact mode
	if treeCompact {
		opts.IndentSize = 1
	}

	// Note: ASCII tree characters (├── └── │) not yet supported
	if treeASCII {
		printVerbose("Note: --ascii flag uses simplified output (ASCII tree chars not yet supported)\n")
	}

	// Print tree
	if err := h.PrintTree(os.Stdout, keyPath, opts); err != nil {
		return fmt.Errorf("failed to display tree: %w", err)
	}

	return nil
}
