package regmerge

import (
	"fmt"

	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
)

// ParseAndOptimize parses multiple .reg files and returns optimized operations.
//
// This is the main entry point for multi-file registry merging. It:
//  1. Parses all .reg files in order (left-to-right)
//  2. Combines operations into a single list
//  3. Runs the query optimizer to remove redundancy
//  4. Returns optimized ops ready for merge execution
//
// The optimizer applies:
//   - Last-write-wins deduplication
//   - Delete shadowing elimination
//   - I/O-efficient ordering
//
// Example:
//
//	// Parse base.reg and patch.reg
//	files := [][]byte{baseRegData, patchRegData}
//	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())
//	if err != nil {
//	    return err
//	}
//
//	// ops now contains optimized operations ready to execute
//	fmt.Printf("Reduced from %d to %d ops (%.1f%% savings)\n",
//	    stats.InputOps, stats.OutputOps, stats.ReductionPercent())
//
// This function is hive-agnostic - it operates purely on .reg file contents
// without needing access to the hive file. This makes it very fast.
//
// Complexity: O(N × D) where N = total ops across all files, D = avg path depth.
func ParseAndOptimize(files [][]byte, opts OptimizerOptions) ([]types.EditOp, Stats, error) {
	// Step 1: Parse all .reg files (preserves left-to-right order)
	var allOps []types.EditOp

	for i, fileData := range files {
		// Parse this .reg file
		ops, err := regtext.ParseReg(fileData, types.RegParseOptions{
			InputEncoding: "UTF-8", // Most .reg files are UTF-8
		})
		if err != nil {
			return nil, Stats{}, fmt.Errorf("parse file %d: %w", i, err)
		}

		// Append to combined list (maintains file order)
		allOps = append(allOps, ops...)
	}

	// Step 2: Optimize combined operation list
	// The optimizer handles:
	//   - Deduplication across files (last file wins)
	//   - Delete shadowing
	//   - Operation ordering
	optimized, stats := Optimize(allOps, opts)

	return optimized, stats, nil
}

// ParseAndOptimizeSingle is a convenience for single-file optimization.
//
// Even single .reg files can benefit from optimization if they contain:
//   - Redundant operations (same key/value set multiple times)
//   - Operations under deleted subtrees
//   - Poorly ordered operations
//
// Example:
//
//	regData, _ := os.ReadFile("changes.reg")
//	ops, stats, err := ParseAndOptimizeSingle(regData, DefaultOptimizerOptions())
//	if stats.DedupedSetValue > 0 {
//	    fmt.Printf("Removed %d redundant ops\n", stats.DedupedSetValue)
//	}
func ParseAndOptimizeSingle(file []byte, opts OptimizerOptions) ([]types.EditOp, Stats, error) {
	return ParseAndOptimize([][]byte{file}, opts)
}

// ParseFiles parses multiple .reg files without optimization.
//
// This is useful when you want to inspect the raw operations before optimization,
// or when you need to apply custom optimization logic.
//
// Example:
//
//	files := [][]byte{file1, file2}
//	ops, err := ParseFiles(files)
//	// ops contains raw unoptimized operations
//
//	// Apply custom filtering
//	filtered := filterOps(ops)
//
//	// Then optimize
//	optimized, stats := Optimize(filtered, DefaultOptimizerOptions())
func ParseFiles(files [][]byte) ([]types.EditOp, error) {
	var allOps []types.EditOp

	for i, fileData := range files {
		ops, err := regtext.ParseReg(fileData, types.RegParseOptions{
			InputEncoding: "UTF-8",
		})
		if err != nil {
			return nil, fmt.Errorf("parse file %d: %w", i, err)
		}

		allOps = append(allOps, ops...)
	}

	return allOps, nil
}
