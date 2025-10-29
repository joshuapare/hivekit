package main

import (
	"encoding/hex"
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var (
	diffKey         string
	diffValuesOnly  bool
	diffIgnoreCase  bool
	diffOutput      string
	diffFormat      string
)

func init() {
	cmd := newDiffCmd()
	cmd.Flags().StringVar(&diffKey, "key", "", "Compare only specific subtree")
	cmd.Flags().BoolVar(&diffValuesOnly, "values-only", false, "Compare only values (ignore structure)")
	cmd.Flags().BoolVar(&diffIgnoreCase, "ignore-case", false, "Case-insensitive comparison")
	cmd.Flags().StringVar(&diffOutput, "output", "", "Save diff to file")
	cmd.Flags().StringVar(&diffFormat, "format", "text", "Output format (text, json, unified)")
	rootCmd.AddCommand(cmd)
}

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <hive1> <hive2>",
		Short: "Compare two hives and show differences",
		Long: `The diff command compares two hives and shows the differences
in keys and values.

Example:
  hivectl diff before.hive after.hive
  hivectl diff before.hive after.hive --key "ControlSet001\\Services"
  hivectl diff before.hive after.hive --values-only
  hivectl diff before.hive after.hive --format unified`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(args)
		},
	}
	return cmd
}

type DiffResult struct {
	AddedKeys      []string
	DeletedKeys    []string
	ModifiedValues []ValueDiff
}

type ValueDiff struct {
	Path     string
	Name     string
	Type     string
	Action   string // "added", "deleted", "modified"
	OldValue string
	NewValue string
}

func runDiff(args []string) error {
	hive1Path := args[0]
	hive2Path := args[1]

	printVerbose("Comparing %s and %s...\n", hive1Path, hive2Path)

	// Load keys from both hives
	keys1, err := hive.ListKeys(hive1Path, diffKey, true, 0)
	if err != nil {
		return fmt.Errorf("failed to list keys from hive1: %w", err)
	}

	keys2, err := hive.ListKeys(hive2Path, diffKey, true, 0)
	if err != nil {
		return fmt.Errorf("failed to list keys from hive2: %w", err)
	}

	// Build key sets
	keys1Set := make(map[string]bool)
	for _, key := range keys1 {
		keys1Set[key.Path] = true
	}

	keys2Set := make(map[string]bool)
	for _, key := range keys2 {
		keys2Set[key.Path] = true
	}

	result := DiffResult{
		AddedKeys:      make([]string, 0),
		DeletedKeys:    make([]string, 0),
		ModifiedValues: make([]ValueDiff, 0),
	}

	// Find added keys
	if !diffValuesOnly {
		for _, key := range keys2 {
			if !keys1Set[key.Path] {
				result.AddedKeys = append(result.AddedKeys, key.Path)
			}
		}

		// Find deleted keys
		for _, key := range keys1 {
			if !keys2Set[key.Path] {
				result.DeletedKeys = append(result.DeletedKeys, key.Path)
			}
		}
	}

	// Compare values in common keys
	commonKeys := make([]string, 0)
	for _, key := range keys1 {
		if keys2Set[key.Path] {
			commonKeys = append(commonKeys, key.Path)
		}
	}

	for _, keyPath := range commonKeys {
		values1, err := hive.ListValues(hive1Path, keyPath)
		if err != nil {
			printVerbose("Warning: failed to list values from hive1 for %s: %v\n", keyPath, err)
			continue
		}

		values2, err := hive.ListValues(hive2Path, keyPath)
		if err != nil {
			printVerbose("Warning: failed to list values from hive2 for %s: %v\n", keyPath, err)
			continue
		}

		// Build value maps
		values1Map := make(map[string]hive.ValueInfo)
		for _, val := range values1 {
			values1Map[val.Name] = val
		}

		values2Map := make(map[string]hive.ValueInfo)
		for _, val := range values2 {
			values2Map[val.Name] = val
		}

		// Find added values
		for _, val := range values2 {
			if _, exists := values1Map[val.Name]; !exists {
				result.ModifiedValues = append(result.ModifiedValues, ValueDiff{
					Path:     keyPath,
					Name:     val.Name,
					Type:     val.Type,
					Action:   "added",
					NewValue: formatValue(val),
				})
			}
		}

		// Find deleted values
		for _, val := range values1 {
			if _, exists := values2Map[val.Name]; !exists {
				result.ModifiedValues = append(result.ModifiedValues, ValueDiff{
					Path:     keyPath,
					Name:     val.Name,
					Type:     val.Type,
					Action:   "deleted",
					OldValue: formatValue(val),
				})
			}
		}

		// Find modified values
		for _, val1 := range values1 {
			if val2, exists := values2Map[val1.Name]; exists {
				// Compare values
				if !valuesEqual(val1, val2) {
					result.ModifiedValues = append(result.ModifiedValues, ValueDiff{
						Path:     keyPath,
						Name:     val1.Name,
						Type:     val1.Type,
						Action:   "modified",
						OldValue: formatValue(val1),
						NewValue: formatValue(val2),
					})
				}
			}
		}
	}

	// Output as JSON if requested
	if jsonOut || diffFormat == "json" {
		return printJSON(result)
	}

	// Text output
	if diffFormat == "unified" {
		return printUnifiedDiff(hive1Path, hive2Path, result)
	}

	printInfo("\nComparing %s and %s...\n\n", hive1Path, hive2Path)

	// Added keys
	if len(result.AddedKeys) > 0 {
		printInfo("Added Keys (%d):\n", len(result.AddedKeys))
		for _, key := range result.AddedKeys {
			printInfo("  + %s\n", key)
		}
		printInfo("\n")
	}

	// Deleted keys
	if len(result.DeletedKeys) > 0 {
		printInfo("Deleted Keys (%d):\n", len(result.DeletedKeys))
		for _, key := range result.DeletedKeys {
			printInfo("  - %s\n", key)
		}
		printInfo("\n")
	}

	// Modified values
	if len(result.ModifiedValues) > 0 {
		printInfo("Modified Values (%d):\n", len(result.ModifiedValues))
		for _, vd := range result.ModifiedValues {
			valName := vd.Name
			if valName == "" {
				valName = "(Default)"
			}
			switch vd.Action {
			case "added":
				printInfo("  + %s\\%s: %s\n", vd.Path, valName, vd.NewValue)
			case "deleted":
				printInfo("  - %s\\%s: %s\n", vd.Path, valName, vd.OldValue)
			case "modified":
				printInfo("  ~ %s\\%s: %s → %s\n", vd.Path, valName, vd.OldValue, vd.NewValue)
			}
		}
		printInfo("\n")
	}

	// Summary
	addedCount := 0
	deletedCount := 0
	modifiedCount := 0
	for _, vd := range result.ModifiedValues {
		switch vd.Action {
		case "added":
			addedCount++
		case "deleted":
			deletedCount++
		case "modified":
			modifiedCount++
		}
	}

	printInfo("Summary:\n")
	printInfo("  Keys:   +%d -%d\n", len(result.AddedKeys), len(result.DeletedKeys))
	printInfo("  Values: +%d -%d ~%d\n", addedCount, deletedCount, modifiedCount)

	return nil
}

func formatValue(val hive.ValueInfo) string {
	switch val.Type {
	case "REG_SZ", "REG_EXPAND_SZ":
		return fmt.Sprintf("\"%s\"", val.StringVal)
	case "REG_DWORD", "REG_DWORD_BE":
		return fmt.Sprintf("%d", val.DWordVal)
	case "REG_QWORD":
		return fmt.Sprintf("%d", val.QWordVal)
	case "REG_BINARY":
		if len(val.Data) > 16 {
			return hex.EncodeToString(val.Data[:16]) + "..."
		}
		return hex.EncodeToString(val.Data)
	default:
		return fmt.Sprintf("(%d bytes)", val.Size)
	}
}

func valuesEqual(v1, v2 hive.ValueInfo) bool {
	if v1.Type != v2.Type {
		return false
	}
	if v1.Size != v2.Size {
		return false
	}

	switch v1.Type {
	case "REG_SZ", "REG_EXPAND_SZ":
		return v1.StringVal == v2.StringVal
	case "REG_DWORD", "REG_DWORD_BE":
		return v1.DWordVal == v2.DWordVal
	case "REG_QWORD":
		return v1.QWordVal == v2.QWordVal
	default:
		// Compare raw bytes
		if len(v1.Data) != len(v2.Data) {
			return false
		}
		for i := range v1.Data {
			if v1.Data[i] != v2.Data[i] {
				return false
			}
		}
		return true
	}
}

func printUnifiedDiff(hive1, hive2 string, result DiffResult) error {
	printInfo("--- %s\n", hive1)
	printInfo("+++ %s\n", hive2)

	for _, vd := range result.ModifiedValues {
		printInfo("@@ %s @@\n", vd.Path)
		valName := vd.Name
		if valName == "" {
			valName = "(Default)"
		}

		switch vd.Action {
		case "deleted":
			printInfo("-%s=%s\n", valName, vd.OldValue)
		case "added":
			printInfo("+%s=%s\n", valName, vd.NewValue)
		case "modified":
			printInfo("-%s=%s\n", valName, vd.OldValue)
			printInfo("+%s=%s\n", valName, vd.NewValue)
		}
	}

	return nil
}
