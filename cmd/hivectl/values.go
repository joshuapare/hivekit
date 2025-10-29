package main

import (
	"encoding/hex"
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	valuesShowData bool
	valuesShowType bool
	valuesHex      bool
)

func init() {
	cmd := newValuesCmd()
	cmd.Flags().BoolVar(&valuesShowData, "show-data", true, "Show full value data")
	cmd.Flags().BoolVar(&valuesShowType, "show-type", true, "Show registry type")
	cmd.Flags().BoolVar(&valuesHex, "hex", false, "Show numeric values as hex")
	rootCmd.AddCommand(cmd)
}

func newValuesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "values <hive> <path>",
		Short: "List all values at a registry key",
		Long: `The values command lists all values at a specific registry key.

Example:
  hivectl values system.hive "ControlSet001"
  hivectl values system.hive "Software" --json
  hivectl values system.hive "ControlSet001\\Services"`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValues(args)
		},
	}
	return cmd
}

func runValues(args []string) error {
	hivePath := args[0]
	keyPath := args[1]

	printVerbose("Opening hive: %s\n", hivePath)

	// List values using public API
	values, err := hive.ListValues(hivePath, keyPath)
	if err != nil {
		return fmt.Errorf("failed to list values: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":   hivePath,
			"path":   keyPath,
			"values": values,
			"count":  len(values),
		}
		return printJSON(result)
	}

	// Text output
	printInfo("\nValues in %s:\n", keyPath)

	if len(values) == 0 {
		printInfo("  (no values)\n")
	} else {
		for _, val := range values {
			valueName := val.Name
			if valueName == "" {
				valueName = "(Default)"
			}

			// Print name
			printInfo("  %-20s", valueName)

			// Print type if requested
			if valuesShowType {
				printInfo("  %-15s", val.Type)
			}

			// Print data if requested
			if valuesShowData {
				printInfo("  ")
				// Print based on type
				if val.StringVal != "" {
					printInfo("\"%s\"", val.StringVal)
				} else if len(val.StringVals) > 0 {
					printInfo("[")
					for i, s := range val.StringVals {
						if i > 0 {
							printInfo(", ")
						}
						printInfo("\"%s\"", s)
					}
					printInfo("]")
				} else if val.Type == "REG_DWORD" || val.Type == "REG_DWORD_LE" || val.Type == "REG_DWORD_BE" {
					if valuesHex {
						printInfo("0x%08x", val.DWordVal)
					} else {
						printInfo("%d", val.DWordVal)
					}
				} else if val.Type == "REG_QWORD" {
					if valuesHex {
						printInfo("0x%016x", val.QWordVal)
					} else {
						printInfo("%d", val.QWordVal)
					}
				} else if len(val.Data) > 0 {
					// Binary data
					if len(val.Data) > 32 {
						printInfo("%s... (%d bytes)", hex.EncodeToString(val.Data[:32]), len(val.Data))
					} else {
						printInfo("%s", hex.EncodeToString(val.Data))
					}
				}
			}

			printInfo("\n")
		}
	}

	printInfo("\nTotal: %d values\n", len(values))

	return nil
}
