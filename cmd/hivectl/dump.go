package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
	"github.com/spf13/cobra"
)

var (
	dumpKey        string
	dumpDepth      int
	dumpValuesOnly bool
	dumpCompact    bool
)

func init() {
	cmd := newDumpCmd()
	cmd.Flags().StringVar(&dumpKey, "key", "", "Dump only specific subtree")
	cmd.Flags().IntVar(&dumpDepth, "depth", 0, "Maximum depth (0 = unlimited)")
	cmd.Flags().BoolVar(&dumpValuesOnly, "values-only", false, "Show only values")
	cmd.Flags().BoolVar(&dumpCompact, "compact", false, "Compact output")
	rootCmd.AddCommand(cmd)
}

func newDumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dump <hive>",
		Short: "Human-readable dump of hive contents",
		Long: `The dump command creates a human-readable dump of all keys and values in a hive.

Example:
  hivectl dump system.hive
  hivectl dump system.hive --key "ControlSet001\\Services"
  hivectl dump system.hive --depth 2 --compact
  hivectl dump system.hive --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDump(args)
		},
	}
	return cmd
}

func runDump(args []string) error {
	hivePath := args[0]
	keyPath := dumpKey

	printVerbose("Opening hive: %s\n", hivePath)

	// Open hive with new backend
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive: %w", err)
	}
	defer h.Close()

	// Configure printer options
	opts := printer.DefaultOptions()
	opts.ShowValues = true
	opts.ShowValueTypes = true
	opts.MaxDepth = dumpDepth
	opts.PrintMetadata = true // Dump shows full metadata

	// Handle JSON output
	if jsonOut {
		opts.Format = printer.FormatJSON
		if err := h.PrintTree(os.Stdout, keyPath, opts); err != nil {
			return fmt.Errorf("dump failed: %w", err)
		}
		return nil
	}

	// Text output
	opts.Format = printer.FormatText

	// Adjust indentation for compact mode
	if dumpCompact {
		opts.IndentSize = 0
	}

	// Note: --values-only flag is not yet supported with new printer
	// Would require printer enhancement
	if dumpValuesOnly {
		printVerbose("Warning: --values-only flag not yet supported with new backend\n")
	}

	// Dump using printer
	if err := h.PrintTree(os.Stdout, keyPath, opts); err != nil {
		return fmt.Errorf("dump failed: %w", err)
	}

	return nil
}
