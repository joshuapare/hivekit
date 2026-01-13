package main

import (
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <reg-file> <output-hive>",
	Short: "Build a new hive from a .reg file",
	Long: `Build a new registry hive file from a Windows .reg file.

This command parses a .reg file (Registry Editor format) and creates a new
hive file from scratch. All standard .reg operations are supported including
key creation, value setting, and deletions.

The .reg file must have a valid header (e.g., "Windows Registry Editor Version 5.00").
Hive root prefixes (HKEY_LOCAL_MACHINE\, HKLM\, etc.) are automatically stripped
since hive files don't contain root key information.

Examples:
  # Create a new hive from a .reg file
  hivectl import config.reg output.hive

  # Use verbose output to see progress
  hivectl import -v changes.reg system.hive

Note: This creates a NEW hive file. To merge changes into an existing hive,
use 'hivectl merge' instead.`,
	Args: cobra.ExactArgs(2),
	RunE: runImport,
}

var (
	importVerbose bool
)

func init() {
	importCmd.Flags().BoolVarP(&importVerbose, "verbose", "v", false, "Show detailed progress")
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	regPath := args[0]
	hivePath := args[1]

	// Check if .reg file exists
	if _, err := os.Stat(regPath); os.IsNotExist(err) {
		return fmt.Errorf(".reg file not found: %s", regPath)
	}

	// Check if output hive already exists
	if _, err := os.Stat(hivePath); err == nil {
		return fmt.Errorf("output hive file already exists: %s (refusing to overwrite)", hivePath)
	}

	printInfo("Building hive from .reg file...\n")
	if importVerbose {
		printVerbose("  Input:  %s\n", regPath)
		printVerbose("  Output: %s\n", hivePath)
	}

	// Build the hive from the .reg file
	err := builder.BuildFromRegFile(hivePath, regPath, nil)
	if err != nil {
		return fmt.Errorf("failed to build hive: %w", err)
	}

	// Get file size for reporting
	info, statErr := os.Stat(hivePath)
	if statErr == nil {
		printInfo("✓ Hive created successfully: %s (%d bytes)\n", hivePath, info.Size())
	} else {
		printInfo("✓ Hive created successfully: %s\n", hivePath)
	}

	return nil
}
