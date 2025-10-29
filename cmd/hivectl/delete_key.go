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
	deleteKeyRecursive bool
	deleteKeyForce     bool
	deleteKeyBackup    bool
	deleteKeyDryRun    bool
	deleteKeyDefrag    bool
)

func init() {
	cmd := newDeleteKeyCmd()
	cmd.Flags().BoolVarP(&deleteKeyRecursive, "recursive", "r", false, "Delete subkeys too (required if has subkeys)")
	cmd.Flags().BoolVarP(&deleteKeyForce, "force", "f", false, "Don't prompt for confirmation")
	cmd.Flags().BoolVar(&deleteKeyBackup, "backup", true, "Create backup")
	cmd.Flags().BoolVar(&deleteKeyDryRun, "dry-run", false, "Show what would be deleted")
	cmd.Flags().BoolVar(&deleteKeyDefrag, "defrag", false, "Defragment after operation")
	rootCmd.AddCommand(cmd)
}

func newDeleteKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-key <hive> <path>",
		Short: "Delete a registry key",
		Long: `The delete-key command deletes a registry key from a hive.

Example:
  hivectl delete-key system.hive "Software\\OldApp"
  hivectl delete-key system.hive "Software\\OldApp" --recursive --force
  hivectl delete-key system.hive "Software\\OldApp" --dry-run`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteKey(args)
		},
	}
	return cmd
}

func runDeleteKey(args []string) error {
	hivePath := args[0]
	keyPath := args[1]

	printVerbose("Opening hive: %s\n", hivePath)

	// Get key info for confirmation
	keys, err := hive.ListKeys(hivePath, keyPath, false, 1)
	if err != nil {
		return fmt.Errorf("failed to get key info: %w", err)
	}

	var subkeyCount, valueCount int
	if len(keys) > 0 {
		// The key itself might not be in the list if we're at root
		// Get values to show in confirmation
		values, _ := hive.ListValues(hivePath, keyPath)
		valueCount = len(values)
	}

	// Count subkeys
	allKeys, err := hive.ListKeys(hivePath, keyPath, true, 0)
	if err == nil {
		subkeyCount = len(allKeys)
	}

	// Check if key has subkeys and recursive flag
	if subkeyCount > 0 && !deleteKeyRecursive && !deleteKeyDryRun {
		return fmt.Errorf("key has %d subkeys; use --recursive to delete them", subkeyCount)
	}

	// Confirm deletion (unless forced or dry-run)
	if !deleteKeyForce && !deleteKeyDryRun && !quiet {
		printInfo("\nDeleting key from %s:\n", hivePath)
		printInfo("  Path: %s\n", keyPath)
		if subkeyCount > 0 {
			printInfo("  Subkeys: %d\n", subkeyCount)
		}
		if valueCount > 0 {
			printInfo("  Values: %d\n", valueCount)
		}
		printInfo("\n⚠ This will delete the key")
		if deleteKeyRecursive && subkeyCount > 0 {
			printInfo(" and all subkeys")
		}
		printInfo(".\n")

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
		CreateBackup: deleteKeyBackup,
		DryRun:       deleteKeyDryRun,
		Defragment:   deleteKeyDefrag,
	}

	// Delete key
	if err := hive.DeleteKey(hivePath, keyPath, deleteKeyRecursive, opts); err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	// Output as JSON if requested
	if jsonOut {
		result := map[string]interface{}{
			"hive":         hivePath,
			"path":         keyPath,
			"subkeys":      subkeyCount,
			"values":       valueCount,
			"success":      true,
			"dry_run":      deleteKeyDryRun,
		}
		return printJSON(result)
	}

	// Text output
	if deleteKeyDryRun {
		printInfo("\n✓ Would delete:\n")
		printInfo("  Key: %s\n", keyPath)
		if subkeyCount > 0 {
			printInfo("  Subkeys: %d\n", subkeyCount)
		}
		if valueCount > 0 {
			printInfo("  Values: %d\n", valueCount)
		}
		printInfo("\n(dry-run mode, no changes made)\n")
	} else {
		printInfo("\n✓ Key deleted successfully\n")
		if deleteKeyBackup {
			printInfo("Backup created: %s.bak\n", hivePath)
		}
	}

	return nil
}
