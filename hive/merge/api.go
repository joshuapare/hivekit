package merge

import (
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/regmerge"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
)

// PlanFromRegText parses Windows .reg file text and converts it to a merge Plan.
//
// This is the most efficient way to convert .reg text to operations:
//  1. Uses optimized zero-alloc regtext.ParseReg() parser
//  2. Converts types.EditOp → merge.Op in a single pass
//  3. Handles hive root prefix stripping automatically
//
// The .reg text must have a valid header (e.g., "Windows Registry Editor Version 5.00").
// Hive root prefixes (HKEY_LOCAL_MACHINE\, HKLM\, etc.) are automatically stripped.
//
// Example:
//
//	regText := `Windows Registry Editor Version 5.00
//	[HKEY_LOCAL_MACHINE\Software\Test]
//	"Version"="1.0"
//	`
//	plan, err := merge.PlanFromRegText(regText)
//	if err != nil {
//	    return err
//	}
//	// plan now contains: EnsureKey(["Software", "Test"]) + SetValue(...)
//
// Returns an error if:
//   - .reg text is malformed
//   - Value data cannot be parsed
//   - Unsupported operation types are encountered
func PlanFromRegText(regText string) (*Plan, error) {
	// Parse .reg text into operations
	ops, err := regtext.ParseReg([]byte(regText), types.RegParseOptions{
		InputEncoding: "UTF-8", // Most .reg files are UTF-8
	})
	if err != nil {
		return nil, fmt.Errorf("parse .reg text: %w", err)
	}

	// Convert types.EditOp → merge.Op
	plan := NewPlan()
	for _, editOp := range ops {
		switch op := editOp.(type) {
		case types.OpCreateKey:
			// Strip hive root prefix and convert to path segments
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddEnsureKey(keyPath)

		case types.OpSetValue:
			// Strip hive root prefix and convert to path segments
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddSetValue(keyPath, op.Name, uint32(op.Type), op.Data)

		case types.OpDeleteValue:
			// Strip hive root prefix and convert to path segments
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddDeleteValue(keyPath, op.Name)

		case types.OpDeleteKey:
			// Strip hive root prefix and convert to path segments
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddDeleteKey(keyPath)

		default:
			return nil, fmt.Errorf("unknown operation type: %T", editOp)
		}
	}

	return plan, nil
}

// PlanFromRegTexts parses and optimizes multiple .reg file texts into a single plan.
//
// This function provides the query optimizer benefits for multi-file merges:
//  1. Parses all .reg files in order
//  2. Applies query optimization (dedup, delete shadowing, ordering)
//  3. Returns a single optimized plan ready for execution
//
// The optimizer eliminates redundancy across files using:
//   - Last-write-wins deduplication (if file1 and file2 both set same value, file2 wins)
//   - Delete shadowing (removes ops under deleted subtrees)
//   - I/O-efficient ordering (groups by key, parents before children)
//
// Example:
//
//	regTexts := []string{baseReg, patch1Reg, patch2Reg}
//	plan, stats, err := PlanFromRegTexts(regTexts)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Optimized: %d → %d ops (%.1f%% reduction)\n",
//	    stats.InputOps, stats.OutputOps, stats.ReductionPercent())
//
//	applied, err := MergePlan("/path/to/hive", plan, nil)
//
// This is much faster than applying files sequentially because:
//   - Hive is opened once (not N times)
//   - Index is built once (not N times)
//   - Single transaction (not N transactions)
//   - Redundant ops are eliminated before execution
//
// Returns the optimized plan, optimizer statistics, and any error.
func PlanFromRegTexts(regTexts []string) (*Plan, regmerge.Stats, error) {
	// Convert strings to bytes for regmerge API
	files := make([][]byte, len(regTexts))
	for i, text := range regTexts {
		files[i] = []byte(text)
	}

	// Parse and optimize at EditOp level
	editOps, stats, err := regmerge.ParseAndOptimize(files, regmerge.DefaultOptimizerOptions())
	if err != nil {
		return nil, stats, fmt.Errorf("parse and optimize: %w", err)
	}

	// Convert optimized types.EditOp → merge.Op
	plan := NewPlan()
	for _, editOp := range editOps {
		switch op := editOp.(type) {
		case types.OpCreateKey:
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddEnsureKey(keyPath)

		case types.OpSetValue:
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddSetValue(keyPath, op.Name, uint32(op.Type), op.Data)

		case types.OpDeleteValue:
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddDeleteValue(keyPath, op.Name)

		case types.OpDeleteKey:
			keyPath := stripHiveRootAndSplit(op.Path)
			plan.AddDeleteKey(keyPath)

		default:
			return nil, stats, fmt.Errorf("unknown operation type: %T", editOp)
		}
	}

	return plan, stats, nil
}

// stripHiveRootAndSplit removes common hive root prefixes and splits path into segments.
//
// Common prefixes stripped:
//   - HKEY_LOCAL_MACHINE\
//   - HKLM\
//   - HKEY_CURRENT_USER\
//   - HKCU\
//   - HKEY_USERS\
//   - HKU\
//   - HKEY_CLASSES_ROOT\
//   - HKCR\
//
// Example:
//
//	"HKEY_LOCAL_MACHINE\Software\Microsoft\Windows" → ["Software", "Microsoft", "Windows"]
//	"Software\Microsoft\Windows" → ["Software", "Microsoft", "Windows"]
func stripHiveRootAndSplit(path string) []string {
	// Strip common hive root prefixes
	prefixes := []string{
		"HKEY_LOCAL_MACHINE\\",
		"HKLM\\",
		"HKEY_CURRENT_USER\\",
		"HKCU\\",
		"HKEY_USERS\\",
		"HKU\\",
		"HKEY_CLASSES_ROOT\\",
		"HKCR\\",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			path = path[len(prefix):]
			break
		}
	}

	// Split on backslash (Windows path separator)
	segments := strings.Split(path, "\\")

	// Filter out empty segments (from trailing/leading backslashes)
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}

	return result
}

// MergePlan applies a merge plan to a hive file in a single transaction.
//
// This is a convenience wrapper that:
//  1. Opens the hive file
//  2. Creates a merge session with specified options
//  3. Applies the plan in a transaction
//  4. Cleans up resources
//
// All operations in the plan are applied atomically - if any operation fails,
// the entire transaction is rolled back.
//
// Example:
//
//	plan := merge.NewPlan()
//	plan.AddEnsureKey([]string{"Software", "Test"})
//	plan.AddSetValue([]string{"Software", "Test"}, "Version", format.REGSZ, []byte("1.0\x00"))
//
//	applied, err := merge.MergePlan("/path/to/system.hive", plan, merge.DefaultOptions())
//	if err != nil {
//	    return fmt.Errorf("merge failed: %w", err)
//	}
//	fmt.Printf("Created %d keys, set %d values\n", applied.KeysCreated, applied.ValuesSet)
//
// Options can be nil to use defaults (StrategyHybrid with FlushAuto).
//
// Returns Applied statistics about what changed, or an error if the operation failed.
func MergePlan(hivePath string, plan *Plan, opts *Options) (Applied, error) {
	if opts == nil {
		defaultOpts := DefaultOptions()
		opts = &defaultOpts
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		return Applied{}, fmt.Errorf("open hive: %w", err)
	}
	defer h.Close()

	// Create session (builds index automatically)
	session, err := NewSession(h, *opts)
	if err != nil {
		return Applied{}, fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	// Apply plan in transaction
	applied, err := session.ApplyWithTx(plan)
	if err != nil {
		return Applied{}, fmt.Errorf("apply plan: %w", err)
	}

	return applied, nil
}

// MergeRegText parses .reg file text and applies it to a hive file.
//
// This is a convenience wrapper that combines PlanFromRegText() + MergePlan().
// Perfect for simple cases where you have .reg text and want to apply it in one call.
//
// Example:
//
//	regText := `Windows Registry Editor Version 5.00
//	[HKEY_LOCAL_MACHINE\Software\Test]
//	"Version"="1.0"
//	"Enabled"=dword:00000001
//	`
//
//	applied, err := merge.MergeRegText("/path/to/system.hive", regText, nil)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Applied: %d keys created, %d values set\n",
//	    applied.KeysCreated, applied.ValuesSet)
//
// Options can be nil to use defaults.
//
// Returns Applied statistics or an error.
func MergeRegText(hivePath string, regText string, opts *Options) (Applied, error) {
	// Parse .reg text into plan
	plan, err := PlanFromRegText(regText)
	if err != nil {
		return Applied{}, fmt.Errorf("parse .reg text: %w", err)
	}

	// Apply plan to hive
	return MergePlan(hivePath, plan, opts)
}

// WithSession opens a hive and provides a session for multiple operations.
//
// This callback pattern is useful when you need to perform multiple separate
// operations but want to share the same session (and thus the same index build).
//
// The session is automatically closed after the callback returns, ensuring
// proper cleanup even if the callback panics.
//
// Example:
//
//	err := merge.WithSession("/path/to/system.hive", merge.DefaultOptions(),
//	    func(s *merge.Session) error {
//	        // Operation 1: Apply a plan
//	        plan1 := merge.NewPlan()
//	        plan1.AddEnsureKey([]string{"Software", "Test1"})
//	        if _, err := s.ApplyWithTx(plan1); err != nil {
//	            return err
//	        }
//
//	        // Operation 2: Apply another plan
//	        plan2 := merge.NewPlan()
//	        plan2.AddEnsureKey([]string{"Software", "Test2"})
//	        if _, err := s.ApplyWithTx(plan2); err != nil {
//	            return err
//	        }
//
//	        return nil
//	    })
//
// Options can be nil to use defaults.
//
// Returns an error if session creation fails or if the callback returns an error.
func WithSession(hivePath string, opts *Options, fn func(*Session) error) error {
	if opts == nil {
		defaultOpts := DefaultOptions()
		opts = &defaultOpts
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("open hive: %w", err)
	}
	defer h.Close()

	// Create session
	session, err := NewSession(h, *opts)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	// Execute callback
	return fn(session)
}
