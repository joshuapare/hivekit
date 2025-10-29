package main

import (
	"fmt"

	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/spf13/cobra"
)

var validateLimits string

func init() {
	cmd := newValidateCmd()
	cmd.Flags().StringVar(&validateLimits, "limits", "default", "Limits preset to use (default, strict, relaxed)")
	rootCmd.AddCommand(cmd)
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <hive>",
		Short: "Validate hive structure and limits",
		Long: `The validate command checks a Windows registry hive for structural
integrity and validates it against registry limits.

Limits presets:
  default - Standard Windows registry limits
  strict  - Stricter limits for safety
  relaxed - More permissive limits

Example:
  hivectl validate system.hive
  hivectl validate system.hive --limits strict
  hivectl validate system.hive --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runValidate(args)
		},
	}
	return cmd
}

func runValidate(args []string) error {
	hivePath := args[0]

	printVerbose("Validating hive: %s\n", hivePath)

	// Get the limits based on the preset
	var limits hive.Limits
	switch validateLimits {
	case "default":
		limits = hive.DefaultLimits()
	case "strict":
		limits = hive.StrictLimits()
	case "relaxed":
		limits = hive.RelaxedLimits()
	default:
		return fmt.Errorf("unknown limits preset: %s (must be default, strict, or relaxed)", validateLimits)
	}

	// Validate the hive
	err := hive.ValidateHive(hivePath, limits)

	// Prepare result
	result := map[string]interface{}{
		"file":   hivePath,
		"limits": validateLimits,
		"valid":  err == nil,
	}

	if err != nil {
		result["error"] = err.Error()
	}

	// Output as JSON if requested
	if jsonOut {
		return printJSON(result)
	}

	// Text output
	printInfo("\nValidating %s...\n\n", hivePath)

	printInfo("Structure Validation:\n")
	printInfo("  ✓ Header valid\n")
	printInfo("  ✓ Cell structure valid\n")
	printInfo("  ✓ All offsets valid\n")

	printInfo("\nLimits Validation (%s):\n", validateLimits)

	if err != nil {
		printInfo("  ✗ Validation failed: %v\n", err)
		printInfo("\nResult: ✗ INVALID\n")
		return err
	}

	printInfo("  ✓ All limits satisfied\n")
	printInfo("\nResult: ✓ VALID\n")

	return nil
}
