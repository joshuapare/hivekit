package regmerge

import (
	"sort"
	"strings"

	"github.com/joshuapare/hivekit/pkg/types"
)

// orderOps reorders operations for execution efficiency.
//
// The goal is to minimize I/O operations during merge execution by:
//  1. Grouping operations by key path (all ops on same key together)
//  2. Ordering paths parent-before-child (shallow paths first)
//  3. Within each key: CreateKey → DeleteValue → SetValue
//
// Why this matters:
//   - Grouping by key reduces handle opens from N to 1 per unique key
//   - Parent-before-child ensures parents exist when children are processed
//   - Operation order within key ensures correct semantics
//
// Example:
//
//	Input:
//	  SetValue(Software\Test\Child, "A", ...)
//	  SetValue(Software\Test, "B", ...)
//	  CreateKey(Software\Test\Child)
//	  SetValue(Software\Test\Child, "C", ...)
//
//	Output (grouped and ordered):
//	  SetValue(Software\Test, "B", ...)           # Shallower path first
//	  CreateKey(Software\Test\Child)              # Parent before child
//	  SetValue(Software\Test\Child, "A", ...)     # All child ops grouped
//	  SetValue(Software\Test\Child, "C", ...)
//
// This ordering is safe because the optimizer has already handled:
//   - Deduplication (no conflicting ops on same key/value)
//   - Delete shadowing (no ops under deleted subtrees)
//
// Complexity: O(N log N) where N = number of operations.
func orderOps(ops []types.EditOp) []types.EditOp {
	if len(ops) <= 1 {
		return ops // Nothing to reorder
	}

	// Step 1: Group operations by normalized key path
	type opGroup struct {
		path  string         // Normalized path
		depth int            // Path depth (for sorting)
		ops   []types.EditOp // Operations on this path
	}

	groups := make(map[string]*opGroup)

	for _, op := range ops {
		path := getOpPath(op)

		if g, ok := groups[path]; ok {
			// Add to existing group
			g.ops = append(g.ops, op)
		} else {
			// Create new group
			depth := strings.Count(path, "\\")
			groups[path] = &opGroup{
				path:  path,
				depth: depth,
				ops:   []types.EditOp{op},
			}
		}
	}

	// Step 2: Convert map to slice for sorting
	sorted := make([]*opGroup, 0, len(groups))
	for _, g := range groups {
		sorted = append(sorted, g)
	}

	// Step 3: Sort groups by path (parents before children, then lexicographic)
	sort.Slice(sorted, func(i, j int) bool {
		// Primary sort: depth (shallower paths first)
		if sorted[i].depth != sorted[j].depth {
			return sorted[i].depth < sorted[j].depth
		}

		// Secondary sort: lexicographic (for deterministic output)
		return sorted[i].path < sorted[j].path
	})

	// Step 4: Sort operations within each group
	for _, group := range sorted {
		if len(group.ops) > 1 {
			sort.Slice(group.ops, func(i, j int) bool {
				// Order within key: CreateKey → DeleteKey → DeleteValue → SetValue
				return opPriority(group.ops[i]) < opPriority(group.ops[j])
			})
		}
	}

	// Step 5: Flatten back to single slice
	result := make([]types.EditOp, 0, len(ops))
	for _, group := range sorted {
		result = append(result, group.ops...)
	}

	return result
}

// opPriority assigns execution priority within a key.
//
// Lower numbers execute first. This ensures:
//   - Keys are created before values are set
//   - Deletes happen before sets (avoids conflicts)
//
// Priority order:
//  0. CreateKey   - Must exist before setting values
//  1. DeleteKey   - Deletes should happen early
//  2. DeleteValue - Remove old values before setting new ones
//  3. SetValue    - Set values last
func opPriority(op types.EditOp) int {
	switch op.(type) {
	case types.OpCreateKey:
		return 0
	case types.OpDeleteKey:
		return 1
	case types.OpDeleteValue:
		return 2
	case types.OpSetValue:
		return 3
	default:
		return 4 // Unknown ops go last
	}
}

// getOpPath extracts the normalized path from any EditOp.
//
// For value operations (SetValue, DeleteValue), returns the key path.
// For key operations (CreateKey, DeleteKey), returns the key path.
//
// The path is normalized using normalizePath() for consistent grouping.
func getOpPath(op types.EditOp) string {
	switch o := op.(type) {
	case types.OpCreateKey:
		return normalizePath(o.Path)
	case types.OpDeleteKey:
		return normalizePath(o.Path)
	case types.OpSetValue:
		return normalizePath(o.Path)
	case types.OpDeleteValue:
		return normalizePath(o.Path)
	default:
		return "" // Unknown operation type
	}
}
