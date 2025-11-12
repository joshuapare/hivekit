package regmerge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/pkg/types"
)

// loadTestFile loads a test .reg file from testdata/.
func loadTestFile(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to load test file %s", filename)
	return data
}

// TestParseAndOptimize_SingleFile tests single file parsing with optimization.
func TestParseAndOptimize_SingleFile(t *testing.T) {
	baseReg := loadTestFile(t, "base.reg")

	files := [][]byte{baseReg}
	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")
	assert.Equal(t, stats.InputOps, stats.OutputOps, "Single clean file should have no reduction")
	assert.Equal(t, 0, stats.DedupedSetValue, "No deduplication expected")
	assert.Equal(t, 0, stats.ShadowedByDelete, "No shadowing expected")
}

// TestParseAndOptimize_MultipleFiles tests multi-file merging with cross-file deduplication.
func TestParseAndOptimize_MultipleFiles(t *testing.T) {
	baseReg := loadTestFile(t, "base.reg")
	patch1Reg := loadTestFile(t, "patch1.reg")
	patch2Reg := loadTestFile(t, "patch2.reg")

	files := [][]byte{baseReg, patch1Reg, patch2Reg}
	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")

	// Multi-file merge should have cross-file deduplication
	// base.reg and patch1.reg update some of the same values
	// patch2.reg updates values from patch1.reg
	assert.Less(t, stats.OutputOps, stats.InputOps, "Should have reduced operations")

	// Should have some deduplication (patches override base values)
	assert.Positive(t, stats.DedupedSetValue, "Should have deduplicated some SetValue operations")

	// patch2.reg deletes Config subtree, shadowing operations from base/patch1
	assert.Positive(t, stats.ShadowedByDelete, "Should have shadowed operations under deleted subtrees")
}

// TestParseAndOptimize_WithDuplicates tests deduplication with duplicates.reg
// This file has 100 operations that should deduplicate to ~10 operations (90% reduction).
func TestParseAndOptimize_WithDuplicates(t *testing.T) {
	duplicatesReg := loadTestFile(t, "duplicates.reg")

	files := [][]byte{duplicatesReg}
	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")

	// 100 SetValue operations (10 values × 10 rounds) + 1 CreateKey
	assert.Equal(t, 101, stats.InputOps, "Should have 101 input operations")

	// Should deduplicate to ~11 operations (1 CreateKey + 10 final SetValues)
	assert.LessOrEqual(t, stats.OutputOps, 15, "Should deduplicate to ~11 operations")
	assert.GreaterOrEqual(t, stats.OutputOps, 10, "Should keep at least 10 unique operations")

	// Should have ~90 deduplicated SetValue operations
	assert.GreaterOrEqual(t, stats.DedupedSetValue, 80, "Should deduplicate ~90 operations")

	// Verify high reduction percentage
	reductionPct := stats.ReductionPercent()
	assert.Greater(t, reductionPct, 80.0, "Should have >80%% reduction")

	t.Logf("Deduplication: %d → %d ops (%.1f%% reduction)",
		stats.InputOps, stats.OutputOps, reductionPct)
}

// TestParseAndOptimize_WithDeletions tests delete shadowing with deletions.reg
// This file creates a deep hierarchy and deletes a subtree, shadowing ~30 operations.
func TestParseAndOptimize_WithDeletions(t *testing.T) {
	deletionsReg := loadTestFile(t, "deletions.reg")

	files := [][]byte{deletionsReg}
	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")

	// Should have many operations shadowed by the Level2 deletion
	assert.Greater(t, stats.ShadowedByDelete, 25, "Should shadow at least 25 operations")

	// Output should be significantly less than input
	assert.Less(t, stats.OutputOps, stats.InputOps, "Should have reduced operations")

	// Verify reduction
	reductionPct := stats.ReductionPercent()
	assert.Greater(t, reductionPct, 30.0, "Should have >30%% reduction from shadowing")

	t.Logf("Delete shadowing: %d → %d ops (%d shadowed, %.1f%% reduction)",
		stats.InputOps, stats.OutputOps, stats.ShadowedByDelete, reductionPct)
}

// TestParseAndOptimize_MixedCase tests case-insensitive path normalization
// mixed_case.reg has ~30 operations with different case variations that should deduplicate.
func TestParseAndOptimize_MixedCase(t *testing.T) {
	mixedCaseReg := loadTestFile(t, "mixed_case.reg")

	files := [][]byte{mixedCaseReg}
	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")

	// Should have some input operations
	assert.Greater(t, stats.InputOps, 30, "Should have >30 input operations")

	// Should deduplicate significantly due to case-insensitive path normalization
	assert.Less(t, stats.OutputOps, stats.InputOps, "Should have reduction")

	// Should have deduplicated some operations
	assert.Greater(t, stats.DedupedSetValue, 10, "Should deduplicate many operations")

	// Verify reduction percentage
	reductionPct := stats.ReductionPercent()
	assert.Greater(t, reductionPct, 50.0, "Should have >50%% reduction")

	t.Logf("Case normalization: %d → %d ops (%d deduped, %.1f%% reduction)",
		stats.InputOps, stats.OutputOps, stats.DedupedSetValue, reductionPct)
}

// TestParseAndOptimizeSingle_Convenience tests the convenience wrapper.
func TestParseAndOptimizeSingle_Convenience(t *testing.T) {
	baseReg := loadTestFile(t, "base.reg")

	ops, stats, err := ParseAndOptimizeSingle(baseReg, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")
	assert.Equal(t, stats.InputOps, stats.OutputOps, "Single clean file should have no reduction")
}

// TestParseAndOptimize_WithOptimizationDisabled tests parsing without optimization.
func TestParseAndOptimize_WithOptimizationDisabled(t *testing.T) {
	duplicatesReg := loadTestFile(t, "duplicates.reg")

	// Disable all optimization
	opts := OptimizerOptions{
		EnableDedup:      false,
		EnableDeleteOpt:  false,
		EnableOrdering:   false,
		EnableSubtreeOpt: false,
	}

	files := [][]byte{duplicatesReg}
	ops, stats, err := ParseAndOptimize(files, opts)

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")

	// With optimization disabled, input and output should be equal
	assert.Equal(t, stats.InputOps, stats.OutputOps, "Should have no reduction")
	assert.Equal(t, 0, stats.DedupedSetValue, "Should have no deduplication")
	assert.Equal(t, 0, stats.ShadowedByDelete, "Should have no shadowing")
}

// TestParseAndOptimize_EmptyFile tests parsing an empty .reg file.
func TestParseAndOptimize_EmptyFile(t *testing.T) {
	emptyReg := []byte("Windows Registry Editor Version 5.00\n\n")

	files := [][]byte{emptyReg}
	ops, stats, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)
	assert.Empty(t, ops, "Should have no operations")
	assert.Equal(t, 0, stats.InputOps, "Should have 0 input operations")
	assert.Equal(t, 0, stats.OutputOps, "Should have 0 output operations")
}

// TestParseAndOptimize_InvalidRegFile tests error handling for invalid .reg files.
func TestParseAndOptimize_InvalidRegFile(t *testing.T) {
	invalidReg := []byte("This is not a valid .reg file")

	files := [][]byte{invalidReg}
	_, _, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	assert.Error(t, err, "Should fail to parse invalid .reg file")
}

// TestParseFiles_WithoutOptimization tests raw parsing without optimization.
func TestParseFiles_WithoutOptimization(t *testing.T) {
	baseReg := loadTestFile(t, "base.reg")
	patch1Reg := loadTestFile(t, "patch1.reg")

	files := [][]byte{baseReg, patch1Reg}
	ops, err := ParseFiles(files)

	require.NoError(t, err)
	assert.NotEmpty(t, ops, "Should have parsed operations")

	// ParseFiles does not optimize, so we should have all operations
	// (cannot easily count exact operations without parsing, but should be substantial)
	assert.Greater(t, len(ops), 50, "Should have many operations from both files")
}

// TestParseAndOptimize_VerifyOperationTypes tests that all operation types are preserved.
func TestParseAndOptimize_VerifyOperationTypes(t *testing.T) {
	baseReg := loadTestFile(t, "base.reg")

	files := [][]byte{baseReg}
	ops, _, err := ParseAndOptimize(files, DefaultOptimizerOptions())

	require.NoError(t, err)

	// Verify we get expected operation types
	hasCreateKey := false
	hasSetValue := false

	for _, op := range ops {
		switch op.(type) {
		case types.OpCreateKey:
			hasCreateKey = true
		case types.OpSetValue:
			hasSetValue = true
		}
	}

	assert.True(t, hasCreateKey, "Should have OpCreateKey operations")
	assert.True(t, hasSetValue, "Should have OpSetValue operations")
}
