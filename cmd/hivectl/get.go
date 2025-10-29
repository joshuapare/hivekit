package main

import (
	"encoding/hex"
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
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

	// Get value using public API
	value, err := hive.GetValue(hivePath, keyPath, valueName)
	if err != nil {
		return fmt.Errorf("failed to get value: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"name": value.Name,
			"type": value.Type,
			"size": value.Size,
		}

		// Add decoded value
		if value.StringVal != "" {
			result["value"] = value.StringVal
		} else if len(value.StringVals) > 0 {
			result["value"] = value.StringVals
		} else if value.Type == "REG_DWORD" || value.Type == "REG_DWORD_LE" || value.Type == "REG_DWORD_BE" {
			result["value"] = value.DWordVal
		} else if value.Type == "REG_QWORD" {
			result["value"] = value.QWordVal
		} else if len(value.Data) > 0 {
			result["value_hex"] = hex.EncodeToString(value.Data)
		}

		return printJSON(result)
	}

	// Text output
	if getShowType {
		printInfo("Name: %s\n", value.Name)
		printInfo("Type: %s\n", value.Type)
		printInfo("Size: %d bytes\n", value.Size)
		printInfo("Value: ")
	}

	// Print value
	if value.StringVal != "" {
		printInfo("%s\n", value.StringVal)
	} else if len(value.StringVals) > 0 {
		for _, s := range value.StringVals {
			printInfo("%s\n", s)
		}
	} else if value.Type == "REG_DWORD" || value.Type == "REG_DWORD_LE" || value.Type == "REG_DWORD_BE" {
		if getHex {
			printInfo("0x%08x\n", value.DWordVal)
		} else {
			printInfo("%d\n", value.DWordVal)
		}
	} else if value.Type == "REG_QWORD" {
		if getHex {
			printInfo("0x%016x\n", value.QWordVal)
		} else {
			printInfo("%d\n", value.QWordVal)
		}
	} else if len(value.Data) > 0 {
		printInfo("%s\n", hex.EncodeToString(value.Data))
	}

	return nil
}
