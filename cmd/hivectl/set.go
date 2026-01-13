package main

import (
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	setType      string
	setCreateKey bool
	setBackup    bool
	setDefrag    bool
)

func init() {
	cmd := newSetCmd()
	cmd.Flags().StringVar(&setType, "type", "sz", "Value type (sz, dword, qword, binary, expand_sz)")
	cmd.Flags().BoolVar(&setCreateKey, "create-key", false, "Create key if it doesn't exist")
	cmd.Flags().BoolVar(&setBackup, "backup", true, "Create backup")
	cmd.Flags().BoolVar(&setDefrag, "defrag", false, "Defragment after operation")
	rootCmd.AddCommand(cmd)
}

func newSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <hive> <path> <name> <value>",
		Short: "Set a registry value",
		Long: `The set command sets a registry value at the specified key path.

Example:
  hivectl set system.hive "Software\\MyApp" "Version" "1.0.0"
  hivectl set system.hive "Software\\MyApp" "Enabled" "1" --type dword
  hivectl set system.hive "Software\\MyApp" "Data" "0102030405" --type binary
  hivectl set system.hive "Software\\NewApp" "Name" "Test" --create-key`,
		Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSet(args)
		},
	}
	return cmd
}

func runSet(args []string) error {
	hivePath := args[0]
	keyPath := args[1]
	valueName := args[2]
	valueStr := args[3]

	printVerbose("Opening hive: %s\n", hivePath)

	// Parse value type and data
	valueType, valueData, err := hive.ParseValueString(valueStr, setType)
	if err != nil {
		return fmt.Errorf("failed to parse value: %w", err)
	}

	// Prepare options
	opts := &hive.OperationOptions{
		CreateKey:    setCreateKey,
		CreateBackup: setBackup,
		Defragment:   setDefrag,
	}

	// Set value
	if err := hive.SetValue(hivePath, keyPath, valueName, valueType, valueData, opts); err != nil {
		return fmt.Errorf("failed to set value: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":    hivePath,
			"path":    keyPath,
			"name":    valueName,
			"type":    valueType.String(),
			"success": true,
		}
		return printJSON(result)
	}

	// Text output
	printInfo("\nSetting value in %s:\n", hivePath)
	printInfo("  Path: %s\n", keyPath)
	printInfo("  Name: %s\n", valueName)
	printInfo("  Type: %s\n", valueType.String())
	printInfo("  Value: %s\n", valueStr)
	printInfo("\n✓ Value set successfully\n")

	if setBackup {
		printInfo("Backup created: %s.bak\n", hivePath)
	}

	return nil
}
