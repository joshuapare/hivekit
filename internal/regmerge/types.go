package regmerge

// OptimizerOptions controls query optimizer behavior.
//
// The optimizer operates on []types.EditOp (from parsed .reg files) and produces
// an optimized, execution-ready operation list by:
//   - Applying right-hand priority (last-write-wins)
//   - Removing dead code (ops shadowed by deletes)
//   - Reordering for I/O efficiency (grouped by key)
//
// This is a preprocessing step that happens BEFORE merge execution, reducing
// work for the merge engine.
type OptimizerOptions struct {
	// EnableDedup removes duplicate SetValue operations using last-write-wins.
	// Example: Two SetValue ops on same (key, value) → keep only the last one.
	// Default: true
	EnableDedup bool

	// EnableDeleteOpt removes operations shadowed by DeleteKey.
	// Example: SetValue under a deleted subtree → removed entirely.
	// Default: true
	EnableDeleteOpt bool

	// EnableOrdering reorders operations for execution efficiency:
	//   - Groups by key path (reduces handle opens)
	//   - Parents before children (shallow paths first)
	//   - Within key: CreateKey, DeleteValue, SetValue
	// Default: true
	EnableOrdering bool

	// EnableSubtreeOpt optimizes subtree delete operations (future Phase 2).
	// When implemented, this will:
	//   - Remove redundant child deletes under parent deletes
	//   - Detect "replace subtree" patterns
	// Default: false (not implemented in Phase 1)
	EnableSubtreeOpt bool
}

// DefaultOptimizerOptions returns recommended optimizer settings for Phase 1.
func DefaultOptimizerOptions() OptimizerOptions {
	return OptimizerOptions{
		EnableDedup:      true,
		EnableDeleteOpt:  true,
		EnableOrdering:   true,
		EnableSubtreeOpt: false, // Phase 2 feature
	}
}

// Stats tracks optimizer performance metrics.
//
// These statistics help understand what the optimizer did and measure
// its effectiveness.
type Stats struct {
	// InputOps is the number of operations before optimization.
	InputOps int

	// OutputOps is the number of operations after optimization.
	OutputOps int

	// DedupedSetValue is the count of duplicate SetValue ops removed.
	// These are SetValue operations that were overwritten by later ops
	// on the same (key, value) pair.
	DedupedSetValue int

	// ShadowedByDelete is the count of ops removed because they were
	// under a deleted subtree.
	// Example: SetValue("Software\\Test\\Child", ...) removed when
	// DeleteKey("Software\\Test") exists.
	ShadowedByDelete int

	// SubtreeOptimized is the count of redundant subtree deletes removed
	// (Phase 2 feature, always 0 in Phase 1).
	SubtreeOptimized int
}

// ReductionPercent returns the percentage of operations eliminated.
func (s Stats) ReductionPercent() float64 {
	if s.InputOps == 0 {
		return 0
	}
	return float64(s.InputOps-s.OutputOps) / float64(s.InputOps) * 100
}
