package regmerge

import (
	"strings"

	"github.com/joshuapare/hivekit/pkg/types"
)

// Optimize applies all enabled optimizations to a list of EditOps.
//
// This is the main entry point for the query optimizer. It takes raw parsed
// operations (from one or more .reg files) and produces an optimized plan by:
//
//  1. Scanning right-to-left to apply last-write-wins (most recent op wins)
//  2. Removing operations shadowed by subtree deletes
//  3. Deduplicating identical operations
//  4. Reordering for execution efficiency (if EnableOrdering is true)
//
// The optimizer is hive-agnostic - it operates purely on operation semantics
// without needing to read the hive file. This makes it very fast.
//
// Example:
//
//	ops := []types.EditOp{
//	    types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v1")},
//	    types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v2")},
//	}
//	optimized, stats := Optimize(ops, DefaultOptimizerOptions())
//	// Result: Single OpSetValue with Data="v2" (last-write-wins)
//	// stats.DedupedSetValue == 1
//
// Complexity: O(N × D) where N = number of ops, D = average path depth.
func Optimize(ops []types.EditOp, opts OptimizerOptions) ([]types.EditOp, Stats) {
	stats := Stats{InputOps: len(ops)}

	// Early exit for empty input
	if len(ops) == 0 {
		return ops, stats
	}

	// Step 1: Apply R2L optimization (dedup + delete shadowing)
	var optimized []types.EditOp
	if opts.EnableDedup || opts.EnableDeleteOpt {
		optimized, stats = applyR2LOptimization(ops, opts, stats)
	} else {
		optimized = ops
	}

	// Step 2: Reorder for execution efficiency (optional, from ordering.go)
	if opts.EnableOrdering {
		optimized = orderOps(optimized)
	}

	stats.OutputOps = len(optimized)
	return optimized, stats
}

// applyR2LOptimization implements the core right-to-left sweep algorithm.
//
// Key insight: By scanning operations from right-to-left (newest to oldest),
// the first occurrence we see is the "winner" for last-write-wins semantics.
//
// Algorithm:
//  1. Scan ops from right to left (index len-1 down to 0)
//  2. Track which (key, value) pairs we've already seen
//  3. Track which key paths have been deleted
//  4. Skip duplicate or shadowed operations
//  5. Reverse the result (since we built it backwards)
//
// This handles:
//   - Deduplication: Only keep the rightmost (newest) operation per (key, value)
//   - Delete shadowing: Skip operations under deleted subtrees
//
// Example:
//
//	Input (left-to-right order):
//	  1. SetValue(Software\Test\Child, "Value", "data1")
//	  2. DeleteKey(Software\Test)
//	  3. SetValue(Software\Test\Child, "Value", "data2")
//
//	R2L scan sees:
//	  - Op 3: Keep (first seen for Software\Test\Child:Value)
//	  - Op 2: Keep (DeleteKey always kept unless shadowed)
//	  - Op 1: Skip (Software\Test\Child is under deleted Software\Test)
//
//	Output: [DeleteKey, SetValue] (ops 2 and 3 only)
func applyR2LOptimization(ops []types.EditOp, opts OptimizerOptions, stats Stats) ([]types.EditOp, Stats) {
	// opKey uniquely identifies a (key path, value name) pair
	type opKey struct {
		path      string // Normalized path (lowercased, no HKEY_ prefix)
		valueName string // Value name (for SetValue/DeleteValue), or "" for key ops
	}

	// Track what we've kept (first occurrence from right wins)
	kept := make(map[opKey]bool)

	// Track which key paths have been deleted
	// Map key = normalized path, value = true if deleted
	deletedPaths := make(map[string]bool)

	// Build result in reverse order (we're scanning R2L)
	result := make([]types.EditOp, 0, len(ops))

	// Scan right-to-left (newest operations first)
	for i := len(ops) - 1; i >= 0; i-- {
		op := ops[i]

		switch o := op.(type) {
		case types.OpSetValue:
			path := normalizePath(o.Path)
			key := opKey{path, o.Name}

			// Check if we've already seen this (key, value) from the right
			if kept[key] && opts.EnableDedup {
				stats.DedupedSetValue++
				continue // Skip duplicate (later write wins)
			}

			// Check if this value is under a deleted subtree
			if isUnderDeleted(path, deletedPaths) && opts.EnableDeleteOpt {
				stats.ShadowedByDelete++
				continue // Skip (will be deleted anyway)
			}

			// Keep this operation
			kept[key] = true
			result = append(result, op)

		case types.OpDeleteValue:
			path := normalizePath(o.Path)
			key := opKey{path, o.Name}

			// Check for duplicates
			if kept[key] && opts.EnableDedup {
				continue // Skip duplicate delete
			}

			// Check if under deleted subtree
			if isUnderDeleted(path, deletedPaths) && opts.EnableDeleteOpt {
				stats.ShadowedByDelete++
				continue // Parent delete will handle it
			}

			// Keep this operation
			kept[key] = true
			result = append(result, op)

		case types.OpDeleteKey:
			path := normalizePath(o.Path)

			// Track this deletion for shadowing other ops
			deletedPaths[path] = true

			// Always keep delete operations (they might be redundant but that's
			// for Phase 2 subtree optimization to handle)
			result = append(result, op)

		case types.OpCreateKey:
			path := normalizePath(o.Path)
			key := opKey{path, ""} // Use empty value name for key ops

			// Check if under deleted subtree
			if isUnderDeleted(path, deletedPaths) && opts.EnableDeleteOpt {
				stats.ShadowedByDelete++
				continue // Will be deleted anyway
			}

			// Check for duplicates (multiple CreateKey on same path)
			if kept[key] && opts.EnableDedup {
				continue // Skip duplicate create
			}

			// Keep this operation
			kept[key] = true
			result = append(result, op)

		default:
			// Unknown operation type - keep it to be safe
			result = append(result, op)
		}
	}

	// Reverse result (we built it backwards during R2L scan)
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return result, stats
}

// isUnderDeleted checks if a path is a descendant of any deleted path.
//
// Example:
//
//	deletedPaths = {"software\\test": true}
//	isUnderDeleted("software\\test\\child", deletedPaths) → true
//	isUnderDeleted("software\\other", deletedPaths) → false
//	isUnderDeleted("software\\test", deletedPaths) → true (exact match)
//
// Algorithm: Walk up the path hierarchy checking each ancestor.
// Complexity: O(D) where D = path depth (typically 5-10).
func isUnderDeleted(path string, deletedPaths map[string]bool) bool {
	// Check if the path itself is deleted
	if deletedPaths[path] {
		return true
	}

	// Check all ancestors by removing segments from the right
	for {
		// Find last backslash
		idx := strings.LastIndex(path, "\\")
		if idx <= 0 {
			break // Reached root or no more segments
		}

		// Move to parent
		path = path[:idx]

		// Check if this ancestor is deleted
		if deletedPaths[path] {
			return true
		}
	}

	return false
}

// normalizePath converts a registry path to canonical form for comparison.
//
// Normalization steps:
//  1. Strip common hive root prefixes (HKEY_LOCAL_MACHINE\, HKLM\, etc.)
//  2. Convert to lowercase (registry paths are case-insensitive)
//
// This ensures that paths like:
//   - "HKEY_LOCAL_MACHINE\\Software\\Test"
//   - "HKLM\\Software\\Test"
//   - "hklm\\software\\test"
//   - "Software\\Test"
//
// All normalize to: "software\\test"
//
// Example:
//
//	normalizePath("HKEY_LOCAL_MACHINE\\Software\\Microsoft") → "software\\microsoft"
//	normalizePath("HKLM\\System\\CurrentControlSet")        → "system\\currentcontrolset"
//	normalizePath("Software\\Test")                         → "software\\test"
func normalizePath(path string) string {
	// Strip common hive root prefixes (case-insensitive)
	upper := strings.ToUpper(path)

	prefixes := []string{
		"HKEY_LOCAL_MACHINE\\",
		"HKLM\\",
		"HKEY_CURRENT_USER\\",
		"HKCU\\",
		"HKEY_USERS\\",
		"HKU\\",
		"HKEY_CLASSES_ROOT\\",
		"HKCR\\",
		"HKEY_CURRENT_CONFIG\\",
		"HKCC\\",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(upper, prefix) {
			path = path[len(prefix):]
			break
		}
	}

	// Convert to lowercase for case-insensitive comparison
	return strings.ToLower(path)
}
