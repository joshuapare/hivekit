package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	deleteValueForce  bool
	deleteValueBackup bool
	deleteValueDefrag bool
)

func init() {
	cmd := newDeleteValueCmd()
	cmd.Flags().BoolVarP(&deleteValueForce, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().BoolVar(&deleteValueBackup, "backup", true, "Create backup")
	cmd.Flags().BoolVar(&deleteValueDefrag, "defrag", false, "Defragment after operation")
	rootCmd.AddCommand(cmd)
}

func newDeleteValueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-value <hive> <path> <name>",
		Short: "Delete a value from a registry key",
		Long: `The delete-value command deletes a value from a registry key.

Example:
  hivectl delete-value system.hive "Software\\MyApp" "OldSetting"
  hivectl delete-value system.hive "Software\\MyApp" "Debug" --force`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteValue(args)
		},
	}
	return cmd
}

func runDeleteValue(args []string) error {
	hivePath := args[0]
	keyPath := args[1]
	valueName := args[2]

	printVerbose("Opening hive: %s\n", hivePath)

	// Get value info for confirmation
	value, err := hive.GetValue(hivePath, keyPath, valueName)
	if err != nil {
		return fmt.Errorf("failed to get value info: %w", err)
	}

	// Confirm deletion (unless forced)
	if !deleteValueForce && !quiet {
		printInfo("\nDeleting value from %s:\n", hivePath)
		printInfo("  Path: %s\n", keyPath)
		printInfo("  Name: %s\n", valueName)
		printInfo("  Type: %s\n", value.Type)
		printInfo("  Size: %d bytes\n", value.Size)
		printInfo("\n⚠ This will delete the value.\n")

		printInfo("Proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			printInfo("Aborted.\n")
			return nil
		}
	}

	// Prepare options
	opts := &hive.OperationOptions{
		CreateBackup: deleteValueBackup,
		Defragment:   deleteValueDefrag,
	}

	// Delete value
	if err := hive.DeleteValue(hivePath, keyPath, valueName, opts); err != nil {
		return fmt.Errorf("failed to delete value: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":    hivePath,
			"path":    keyPath,
			"name":    valueName,
			"type":    value.Type,
			"success": true,
		}
		return printJSON(result)
	}

	// Text output
	printInfo("\n✓ Value deleted successfully\n")
	if deleteValueBackup {
		printInfo("Backup created: %s.bak\n", hivePath)
	}

	return nil
}
