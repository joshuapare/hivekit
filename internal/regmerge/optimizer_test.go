package regmerge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/pkg/types"
)

// Test basic deduplication - last write wins.
func TestDeduplication_LastWriteWins(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Type: types.REG_SZ, Data: []byte("v1\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Type: types.REG_SZ, Data: []byte("v2\x00")},
		types.OpSetValue{
			Path: "HKLM\\Software\\Test",
			Name: "Value",
			Type: types.REG_SZ,
			Data: []byte("v3\x00"),
		}, // Should win
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	require.Len(t, result, 1, "Should keep only last operation")
	assert.Equal(t, 2, stats.DedupedSetValue, "Should mark 2 as duplicates")

	// Verify the winner is v3 (last write)
	sv, ok := result[0].(types.OpSetValue)
	require.True(t, ok, "Result should be OpSetValue")
	assert.Equal(t, []byte("v3\x00"), sv.Data, "Last write should win")
}

// Test deduplication across different paths.
func TestDeduplication_DifferentPaths(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test1", Name: "Value", Data: []byte("v1\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test2", Name: "Value", Data: []byte("v2\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test1", Name: "Value", Data: []byte("v3\x00")}, // Duplicate of #1
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	assert.Len(t, result, 2, "Should keep ops for different paths")
	assert.Equal(t, 1, stats.DedupedSetValue, "Should only dedup Test1")
}

// Test deduplication with different value names.
func TestDeduplication_DifferentValueNames(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value1", Data: []byte("v1\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value2", Data: []byte("v2\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value1", Data: []byte("v3\x00")}, // Duplicate
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	assert.Len(t, result, 2, "Should keep ops for different value names")
	assert.Equal(t, 1, stats.DedupedSetValue)
}

// Test delete shadowing - ops under deleted subtree are removed.
func TestDeleteShadowing_RemovesChildOps(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test\\Child", Name: "Value", Data: []byte("v1\x00")},
		types.OpDeleteKey{Path: "HKLM\\Software\\Test", Recursive: true}, // Shadows above
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	require.Len(t, result, 1, "Should only keep delete")
	assert.Equal(t, 1, stats.ShadowedByDelete)

	_, ok := result[0].(types.OpDeleteKey)
	assert.True(t, ok, "Result should be OpDeleteKey")
}

// Test delete shadowing with multiple levels.
func TestDeleteShadowing_MultipleLevels(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test\\A\\B\\C", Name: "Value", Data: []byte("v1\x00")},
		types.OpCreateKey{Path: "HKLM\\Software\\Test\\A\\B"},
		types.OpDeleteKey{Path: "HKLM\\Software\\Test", Recursive: true},                       // Shadows all above
		types.OpSetValue{Path: "HKLM\\Software\\Other", Name: "Value", Data: []byte("v2\x00")}, // Not shadowed
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	assert.Len(t, result, 2, "Should keep delete + non-shadowed op")
	assert.Equal(t, 2, stats.ShadowedByDelete, "Should shadow 2 ops")
}

// Test case-insensitive path matching.
func TestCaseInsensitive_Paths(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\SOFTWARE\\Test", Name: "Value", Data: []byte("v1\x00")},
		types.OpSetValue{
			Path: "HKLM\\Software\\Test",
			Name: "Value",
			Data: []byte("v2\x00"),
		}, // Same path, different case
		types.OpSetValue{
			Path: "HKLM\\software\\test",
			Name: "Value",
			Data: []byte("v3\x00"),
		}, // Same path, all lowercase
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	assert.Len(t, result, 1, "Should treat as same path (case-insensitive)")
	assert.Equal(t, 2, stats.DedupedSetValue)
}

// Test HKEY prefix normalization.
func TestPathNormalization_HKEYPrefixes(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKEY_LOCAL_MACHINE\\Software\\Test", Name: "Value", Data: []byte("v1\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v2\x00")}, // Same path, short form
		types.OpSetValue{Path: "Software\\Test", Name: "Value", Data: []byte("v3\x00")},       // Same path, no prefix
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	assert.Len(t, result, 1, "Should normalize all to same path")
	assert.Equal(t, 2, stats.DedupedSetValue)

	sv := result[0].(types.OpSetValue)
	assert.Equal(t, []byte("v3\x00"), sv.Data, "Last write should win")
}

// Test ordering - operations grouped by key.
func TestOrdering_GroupedByKey(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test\\Child", Name: "A", Data: []byte("v1\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "B", Data: []byte("v2\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test\\Child", Name: "C", Data: []byte("v3\x00")},
	}

	result, _ := Optimize(ops, DefaultOptimizerOptions())

	require.Len(t, result, 3)

	// Shallower path (Software\Test) should come first
	sv1 := result[0].(types.OpSetValue)
	assert.Contains(t, sv1.Path, "Test")
	assert.NotContains(t, sv1.Path, "Child")

	// Deeper path ops should be grouped together
	sv2 := result[1].(types.OpSetValue)
	sv3 := result[2].(types.OpSetValue)
	assert.Contains(t, sv2.Path, "Child")
	assert.Contains(t, sv3.Path, "Child")
}

// Test ordering - CreateKey before SetValue.
func TestOrdering_CreateBeforeSet(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v1\x00")},
		types.OpCreateKey{Path: "HKLM\\Software\\Test"},
	}

	result, _ := Optimize(ops, DefaultOptimizerOptions())

	require.Len(t, result, 2)

	// CreateKey should come first
	_, ok := result[0].(types.OpCreateKey)
	assert.True(t, ok, "CreateKey should come before SetValue")

	_, ok = result[1].(types.OpSetValue)
	assert.True(t, ok, "SetValue should come after CreateKey")
}

// Test empty input.
func TestOptimize_EmptyInput(t *testing.T) {
	ops := []types.EditOp{}
	result, stats := Optimize(ops, DefaultOptimizerOptions())

	assert.Empty(t, result)
	assert.Equal(t, 0, stats.InputOps)
	assert.Equal(t, 0, stats.OutputOps)
}

// Test with optimization disabled.
func TestOptimize_Disabled(t *testing.T) {
	ops := []types.EditOp{
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v1\x00")},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v2\x00")}, // Duplicate
	}

	opts := OptimizerOptions{
		EnableDedup:     false, // Disable dedup
		EnableDeleteOpt: false,
		EnableOrdering:  false,
	}

	result, stats := Optimize(ops, opts)

	assert.Len(t, result, 2, "Should keep all ops when optimization disabled")
	assert.Equal(t, 0, stats.DedupedSetValue, "No dedup should occur")
}

// Test complex scenario: multiple files with overlapping changes.
func TestComplexScenario_MultiFileOverlap(t *testing.T) {
	// Simulates:
	//   base.reg: Sets Value=v1
	//   patch1.reg: Deletes parent key
	//   patch2.reg: Recreates and sets Value=v3
	ops := []types.EditOp{
		// base.reg
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v1\x00")},

		// patch1.reg
		types.OpDeleteKey{Path: "HKLM\\Software", Recursive: true},

		// patch2.reg
		types.OpCreateKey{Path: "HKLM\\Software\\Test"},
		types.OpSetValue{Path: "HKLM\\Software\\Test", Name: "Value", Data: []byte("v3\x00")},
	}

	result, stats := Optimize(ops, DefaultOptimizerOptions())

	// Should optimize to:
	//   1. DeleteKey (Software)
	//   2. CreateKey (Software\Test) - recreates after delete
	//   3. SetValue (v3)
	// The first SetValue (v1) is removed by dedup (same key/value, v3 wins)

	assert.Len(t, result, 3, "Should have delete + recreate + set")
	assert.Equal(t, 1, stats.DedupedSetValue, "First SetValue deduplicated (same key/value)")

	// Verify order: delete should come first (shallower path)
	_, ok := result[0].(types.OpDeleteKey)
	assert.True(t, ok, "Delete should come first")
}

// Test ReductionPercent calculation.
func TestStats_ReductionPercent(t *testing.T) {
	stats := Stats{
		InputOps:  100,
		OutputOps: 25,
	}

	reduction := stats.ReductionPercent()
	assert.InDelta(t, 75.0, reduction, 0.001, "Should calculate 75% reduction")
}

// Test ReductionPercent with zero input.
func TestStats_ReductionPercentZero(t *testing.T) {
	stats := Stats{
		InputOps:  0,
		OutputOps: 0,
	}

	reduction := stats.ReductionPercent()
	assert.InDelta(t, 0.0, reduction, 0.001, "Should return 0 for empty input")
}
