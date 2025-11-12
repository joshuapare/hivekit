package hivexval

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// IsHivexshAvailable checks if the hivexsh command is available on the system.
//
// Example:
//
//	if !hivexval.IsHivexshAvailable() {
//	    t.Skip("hivexsh not available")
//	}
func IsHivexshAvailable() bool {
	_, err := exec.LookPath("hivexsh")
	return err == nil
}

// ValidateWithHivexsh runs hivexsh -d on the hive file.
//
// This uses the authoritative hivexsh implementation to validate
// hive structure, checksums, offsets, and cell boundaries.
//
// Returns nil if hivexsh successfully parses the hive.
// Returns error with hivexsh output if validation fails.
//
// Example:
//
//	if err := v.ValidateWithHivexsh(); err != nil {
//	    t.Errorf("Hivexsh validation failed: %v", err)
//	}
func (v *Validator) ValidateWithHivexsh() error {
	// Check if hivexsh is available
	if !IsHivexshAvailable() {
		if v.opts.SkipIfHivexshUnavailable {
			return nil // Skip silently
		}
		return errors.New("hivexsh command not found")
	}

	// Ensure we have a file path
	path, err := v.ensurePath()
	if err != nil {
		return fmt.Errorf("get hive path: %w", err)
	}

	// Run hivexsh -d to dump the hive (with 30s timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "hivexsh", "-d", path)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return &HivexshError{
			exitCode: cmd.ProcessState.ExitCode(),
			output:   string(output),
		}
	}

	// Check for known error patterns in output
	outStr := string(output)
	if err := checkHivexshErrors(outStr); err != nil {
		return err
	}

	return nil
}

// ValidateStructure checks if the hive can be opened and parsed.
//
// This validates basic structure using the primary backend.
//
// Example:
//
//	if err := v.ValidateStructure(); err != nil {
//	    t.Fatalf("Structure invalid: %v", err)
//	}
func (v *Validator) ValidateStructure() error {
	// Try to get root node
	_, err := v.Root()
	if err != nil {
		return fmt.Errorf("cannot access root: %w", err)
	}

	return nil
}

// Validate performs comprehensive validation using all enabled backends.
//
// Returns a ValidationResult with detailed results from each check.
//
// Example:
//
//	result, err := v.Validate()
//	if err != nil {
//	    t.Fatal(err)
//	}
//	if !result.StructureValid {
//	    t.Errorf("Validation errors: %v", result.Errors)
//	}
func (v *Validator) Validate() (*ValidationResult, error) {
	result := &ValidationResult{
		Errors:   make([]string, 0),
		Warnings: make([]string, 0),
	}

	// Check structure
	if err := v.ValidateStructure(); err != nil {
		result.StructureValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("structure: %v", err))
	} else {
		result.StructureValid = true
	}

	// Count keys and values
	if keyCount, valueCount, err := v.CountTree(); err == nil {
		result.KeyCount = keyCount
		result.ValueCount = valueCount
	} else {
		result.Warnings = append(result.Warnings, fmt.Sprintf("count tree: %v", err))
	}

	// Validate with hivexsh if enabled
	if v.opts.UseHivexsh {
		if err := v.ValidateWithHivexsh(); err != nil {
			result.HivexshPassed = false
			result.Errors = append(result.Errors, fmt.Sprintf("hivexsh: %v", err))
		} else {
			result.HivexshPassed = true
		}
	}

	// Cross-validate if requested
	if v.opts.CompareAll {
		// TODO: Implement cross-backend comparison
		result.Warnings = append(result.Warnings, "cross-validation not yet implemented")
	}

	return result, nil
}

// HivexshError represents an error from hivexsh validation.
type HivexshError struct {
	exitCode int
	output   string
}

// Error implements the error interface.
func (e *HivexshError) Error() string {
	return fmt.Sprintf("hivexsh validation failed (exit code %d): %s", e.exitCode, e.output)
}

// Output returns the hivexsh output.
func (e *HivexshError) Output() string {
	return e.output
}

// ExitCode returns the hivexsh exit code.
func (e *HivexshError) ExitCode() int {
	return e.exitCode
}

// checkHivexshErrors scans hivexsh output for known error patterns.
func checkHivexshErrors(output string) error {
	// Convert to lowercase for case-insensitive matching
	lower := strings.ToLower(output)

	// Known ACTUAL error patterns from hivexsh (not debug output)
	// These are messages that indicate the hive is corrupted or invalid
	errorPatterns := []struct {
		pattern string
		message string
	}{
		{"trailing garbage", "hive has trailing garbage after last page"},
		{"bad registry", "hive has structural errors (bad registry)"},
		{"extends beyond", "hive data extends beyond declared bounds"},
		{"does not match computed offset", "HBIN page offset mismatch"},
		{"returning enotsup", "hivex returned ENOTSUP (unsupported operation)"},
		{"error", "hivex reported an error"},
	}

	for _, ep := range errorPatterns {
		if strings.Contains(lower, ep.pattern) {
			return &HivexshError{
				exitCode: 1,
				output:   fmt.Sprintf("%s\n\nFull output:\n%s", ep.message, output),
			}
		}
	}

	return nil
}
