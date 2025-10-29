package main

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/pkg/hive"
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

	// List keys recursively
	keys, err := hive.ListKeys(hivePath, keyPath, true, dumpDepth)
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	// Collect all key-value data
	type KeyData struct {
		Path   string
		Values []hive.ValueInfo
	}

	var allData []KeyData

	// Add root key if it exists
	if keyPath != "" {
		values, err := hive.ListValues(hivePath, keyPath)
		if err == nil {
			allData = append(allData, KeyData{
				Path:   keyPath,
				Values: values,
			})
		}
	}

	// Add all subkeys
	for _, key := range keys {
		values, err := hive.ListValues(hivePath, key.Path)
		if err != nil {
			printVerbose("Warning: failed to list values for %s: %v\n", key.Path, err)
			continue
		}

		allData = append(allData, KeyData{
			Path:   key.Path,
			Values: values,
		})
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive": hivePath,
			"path": keyPath,
			"data": allData,
		}
		return printJSON(result)
	}

	// Text output
	if !dumpCompact {
		printInfo("\nRegistry Hive Dump: %s\n", hivePath)
		printInfo("%s\n\n", strings.Repeat("═", 40))
	}

	for _, kd := range allData {
		// Print key header
		if !dumpValuesOnly {
			printInfo("[%s]\n", kd.Path)
		}

		// Print values
		if len(kd.Values) == 0 {
			if !dumpCompact {
				printInfo("  (no values)\n")
			}
		} else {
			for _, val := range kd.Values {
				valueName := val.Name
				if valueName == "" {
					valueName = "(Default)"
				}

				// Format value based on type
				var valueStr string
				switch val.Type {
				case "REG_SZ", "REG_EXPAND_SZ":
					valueStr = fmt.Sprintf("REG_SZ \"%s\"", val.StringVal)
				case "REG_MULTI_SZ":
					valueStr = fmt.Sprintf("REG_MULTI_SZ [%s]", strings.Join(val.StringVals, ", "))
				case "REG_DWORD", "REG_DWORD_BE":
					valueStr = fmt.Sprintf("REG_DWORD 0x%08x", val.DWordVal)
				case "REG_QWORD":
					valueStr = fmt.Sprintf("REG_QWORD 0x%016x", val.QWordVal)
				case "REG_BINARY":
					if len(val.Data) > 32 {
						valueStr = fmt.Sprintf("REG_BINARY %s... (%d bytes)", hex.EncodeToString(val.Data[:32]), len(val.Data))
					} else {
						valueStr = fmt.Sprintf("REG_BINARY %s", hex.EncodeToString(val.Data))
					}
				default:
					if len(val.Data) > 32 {
						valueStr = fmt.Sprintf("%s %s... (%d bytes)", val.Type, hex.EncodeToString(val.Data[:32]), len(val.Data))
					} else {
						valueStr = fmt.Sprintf("%s %s", val.Type, hex.EncodeToString(val.Data))
					}
				}

				if dumpValuesOnly {
					printInfo("%s\\%s = %s\n", kd.Path, valueName, valueStr)
				} else {
					printInfo("  %s = %s\n", valueName, valueStr)
				}
			}
		}

		if !dumpCompact && !dumpValuesOnly {
			printInfo("\n")
		}
	}

	return nil
}
