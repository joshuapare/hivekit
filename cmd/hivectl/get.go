package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
	"github.com/spf13/cobra"
)

var (
	getShowType bool
	getHex      bool
)

func init() {
	cmd := newGetCmd()
	cmd.Flags().BoolVar(&getShowType, "type", false, "Show type information")
	cmd.Flags().BoolVar(&getHex, "hex", false, "Output numeric values as hex")
	rootCmd.AddCommand(cmd)
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <hive> <path> <name>",
		Short: "Get a specific registry value",
		Long: `The get command retrieves and displays a specific value from a registry key.

Example:
  hivectl get system.hive "ControlSet001" "MyValue"
  hivectl get system.hive "Software" "Version" --type
  hivectl get system.hive "Software" "Data" --hex`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(args)
		},
	}
	return cmd
}

func runGet(args []string) error {
	hivePath := args[0]
	keyPath := args[1]
	valueName := args[2]

	printVerbose("Opening hive: %s\n", hivePath)

	// Open hive with new backend
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive: %w", err)
	}
	defer h.Close()

	// Configure printer options
	opts := printer.DefaultOptions()
	opts.ShowValueTypes = getShowType

	// Handle JSON output
	if jsonOut {
		opts.Format = printer.FormatJSON
		opts.ShowValueTypes = true
		if err := h.PrintValue(os.Stdout, keyPath, valueName, opts); err != nil {
			return fmt.Errorf("failed to get value: %w", err)
		}
		return nil
	}

	// Text output
	opts.Format = printer.FormatText

	// Note: --hex flag is not yet supported with new printer
	if getHex {
		printVerbose("Warning: --hex flag not yet supported with new backend\n")
	}

	if err := h.PrintValue(os.Stdout, keyPath, valueName, opts); err != nil {
		return fmt.Errorf("failed to get value: %w", err)
	}

	return nil
}
