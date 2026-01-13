package regmerge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/pkg/types"
)

// TestCorrectness_LastWriteWins verifies that optimization preserves last-write-wins semantics.
func TestCorrectness_LastWriteWins(t *testing.T) {
	// Create operations where same key/value is written multiple times
	regData := []byte(`Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Test]
"Value1"="first"
"Value1"="second"
"Value1"="third"
`)

	// Parse WITH optimization
	opsOptimized, statsOptimized, err := ParseAndOptimizeSingle(regData, DefaultOptimizerOptions())
	require.NoError(t, err)

	// Parse WITHOUT optimization
	opsUnoptimized, _, err := ParseAndOptimizeSingle(regData, OptimizerOptions{
		EnableDedup:      false,
		EnableDeleteOpt:  false,
		EnableOrdering:   false,
		EnableSubtreeOpt: false,
	})
	require.NoError(t, err)

	// Verify optimization reduced operations
	assert.Less(t, len(opsOptimized), len(opsUnoptimized), "Optimization should reduce operations")
	assert.Positive(t, statsOptimized.DedupedSetValue, "Should have deduplicated SetValue operations")

	// Build final state for both
	optimizedState := buildFinalState(t, opsOptimized)
	unoptimizedState := buildFinalState(t, opsUnoptimized)

	// Verify final states are identical
	assert.Equal(t, unoptimizedState, optimizedState, "Final states should be identical")

	// Verify the final value is "third" (last write wins)
	// Note: Registry strings are UTF-16LE encoded
	key := normalizePath("Software\\Test")
	valueName := "Value1"
	assert.Contains(t, optimizedState.values, key+"::"+valueName, "Should have the value")

	// Convert UTF-16LE to string for comparison
	data := optimizedState.values[key+"::"+valueName]
	decodedValue := decodeUTF16LE(data)
	assert.Equal(t, "third", decodedValue, "Should have last write")
}

// TestCorrectness_DeleteShadowing verifies that delete shadowing doesn't change semantics.
func TestCorrectness_DeleteShadowing(t *testing.T) {
	// Create operations where a subtree is deleted after being populated
	regData := []byte(`Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Test\SubKey]
"Value1"="data1"
"Value2"="data2"

[HKEY_LOCAL_MACHINE\Software\Test\SubKey\DeepKey]
"Value3"="data3"

[-HKEY_LOCAL_MACHINE\Software\Test\SubKey]

[HKEY_LOCAL_MACHINE\Software\Test]
"RootValue"="root"
`)

	// Parse WITH optimization
	opsOptimized, statsOptimized, err := ParseAndOptimizeSingle(regData, DefaultOptimizerOptions())
	require.NoError(t, err)

	// Parse WITHOUT optimization
	opsUnoptimized, _, err := ParseAndOptimizeSingle(regData, OptimizerOptions{
		EnableDedup:      false,
		EnableDeleteOpt:  false,
		EnableOrdering:   false,
		EnableSubtreeOpt: false,
	})
	require.NoError(t, err)

	// Verify optimization removed shadowed operations
	assert.Less(t, len(opsOptimized), len(opsUnoptimized), "Optimization should reduce operations")
	assert.Positive(t, statsOptimized.ShadowedByDelete, "Should have shadowed operations")

	// Build final state for both (simulating execution)
	optimizedState := buildFinalState(t, opsOptimized)
	unoptimizedState := buildFinalState(t, opsUnoptimized)

	// Verify final states have same keys and values (not necessarily same deletedKeys tracking)
	assert.Equal(t, unoptimizedState.keys, optimizedState.keys, "Keys should be identical")
	assert.Equal(t, unoptimizedState.values, optimizedState.values, "Values should be identical")

	// Verify SubKey is deleted (not in keys)
	subKey := normalizePath("Software\\Test\\SubKey")
	assert.False(t, optimizedState.keys[subKey], "SubKey should not exist")

	// Verify RootValue exists
	rootKey := normalizePath("Software\\Test")
	assert.Contains(t, optimizedState.values, rootKey+"::RootValue", "RootValue should exist")
}

// TestCorrectness_MultiFileEquivalence verifies multi-file merge produces same result.
func TestCorrectness_MultiFileEquivalence(t *testing.T) {
	baseReg := loadTestFile(t, "base.reg")
	patch1Reg := loadTestFile(t, "patch1.reg")
	patch2Reg := loadTestFile(t, "patch2.reg")

	// Parse WITH optimization
	files := [][]byte{baseReg, patch1Reg, patch2Reg}
	opsOptimized, _, err := ParseAndOptimize(files, DefaultOptimizerOptions())
	require.NoError(t, err)

	// Parse WITHOUT optimization
	opsUnoptimized, _, err := ParseAndOptimize(files, OptimizerOptions{
		EnableDedup:      false,
		EnableDeleteOpt:  false,
		EnableOrdering:   false,
		EnableSubtreeOpt: false,
	})
	require.NoError(t, err)

	// Build final state for both
	optimizedState := buildFinalState(t, opsOptimized)
	unoptimizedState := buildFinalState(t, opsUnoptimized)

	// Verify final states are identical
	assert.Len(t, optimizedState.keys, len(unoptimizedState.keys), "Should have same number of keys")
	assert.Len(t, optimizedState.values, len(unoptimizedState.values), "Should have same number of values")

	// Verify all keys match
	for key := range unoptimizedState.keys {
		assert.True(t, optimizedState.keys[key], "Key %s should exist in optimized state", key)
	}

	// Verify all values match
	for valuePath, data := range unoptimizedState.values {
		assert.Contains(t, optimizedState.values, valuePath, "Value %s should exist", valuePath)
		assert.Equal(t, data, optimizedState.values[valuePath], "Value data should match for %s", valuePath)
	}
}

// TestCorrectness_CaseNormalization verifies case normalization doesn't break semantics.
func TestCorrectness_CaseNormalization(t *testing.T) {
	mixedCaseReg := loadTestFile(t, "mixed_case.reg")

	// Parse WITH optimization (case normalization enabled)
	opsOptimized, _, err := ParseAndOptimizeSingle(mixedCaseReg, DefaultOptimizerOptions())
	require.NoError(t, err)

	// Parse WITHOUT optimization
	opsUnoptimized, _, err := ParseAndOptimizeSingle(mixedCaseReg, OptimizerOptions{
		EnableDedup:      false,
		EnableDeleteOpt:  false,
		EnableOrdering:   false,
		EnableSubtreeOpt: false,
	})
	require.NoError(t, err)

	// Build final state for both (using case-insensitive comparison)
	optimizedState := buildFinalState(t, opsOptimized)
	unoptimizedState := buildFinalState(t, opsUnoptimized)

	// Both should have the same normalized keys
	// (unoptimized will have multiple case variations, but they normalize to the same key)
	for key := range optimizedState.keys {
		found := false
		for unoptKey := range unoptimizedState.keys {
			if normalizePath(unoptKey) == normalizePath(key) {
				found = true
				break
			}
		}
		assert.True(t, found, "Normalized key %s should exist", key)
	}
}

// TestCorrectness_DuplicateElimination verifies deduplication preserves semantics.
func TestCorrectness_DuplicateElimination(t *testing.T) {
	duplicatesReg := loadTestFile(t, "duplicates.reg")

	// Parse WITH optimization
	opsOptimized, stats, err := ParseAndOptimizeSingle(duplicatesReg, DefaultOptimizerOptions())
	require.NoError(t, err)

	// Should have high reduction
	assert.Greater(t, stats.DedupedSetValue, 80, "Should deduplicate many operations")
	assert.Less(t, len(opsOptimized), 20, "Should have very few operations after dedup")

	// Parse WITHOUT optimization
	opsUnoptimized, _, err := ParseAndOptimizeSingle(duplicatesReg, OptimizerOptions{
		EnableDedup:      false,
		EnableDeleteOpt:  false,
		EnableOrdering:   false,
		EnableSubtreeOpt: false,
	})
	require.NoError(t, err)

	// Build final state for both
	optimizedState := buildFinalState(t, opsOptimized)
	unoptimizedState := buildFinalState(t, opsUnoptimized)

	// Verify final states are identical
	assert.Equal(t, unoptimizedState, optimizedState, "Final states should be identical despite deduplication")

	// Verify string values have "FINAL-*"
	// Note: Even-numbered values are dwords, odd-numbered are strings
	for valuePath, data := range optimizedState.values {
		// Try to decode as UTF-16LE string
		decoded := decodeUTF16LE(data)
		if len(decoded) > 0 && decoded[0] != 0 {
			// It's a valid string, check for FINAL
			if len(decoded) >= 5 {
				assert.Contains(t, decoded, "FINAL", "String value %s should have FINAL data", valuePath)
			}
		}
	}
}

// registryState represents the final state of a registry after applying operations.
type registryState struct {
	keys        map[string]bool   // normalized key path → exists
	values      map[string][]byte // "key::valueName" → data
	deletedKeys map[string]bool   // normalized key path → deleted
}

// buildFinalState simulates applying operations and builds the final registry state.
func buildFinalState(t *testing.T, ops []types.EditOp) registryState {
	t.Helper()

	state := registryState{
		keys:        make(map[string]bool),
		values:      make(map[string][]byte),
		deletedKeys: make(map[string]bool),
	}

	for _, op := range ops {
		switch o := op.(type) {
		case types.OpCreateKey:
			key := normalizePath(o.Path)
			state.keys[key] = true
			// Undelete if it was previously deleted
			delete(state.deletedKeys, key)

		case types.OpSetValue:
			key := normalizePath(o.Path)
			valuePath := key + "::" + o.Name
			state.values[valuePath] = o.Data
			// Ensure parent key exists
			state.keys[key] = true

		case types.OpDeleteValue:
			key := normalizePath(o.Path)
			valuePath := key + "::" + o.Name
			delete(state.values, valuePath)

		case types.OpDeleteKey:
			key := normalizePath(o.Path)
			// Mark key as deleted
			state.deletedKeys[key] = true
			delete(state.keys, key)

			// Delete all child keys and values
			keyPrefix := key + "\\"
			for k := range state.keys {
				if k == key || len(k) > len(keyPrefix) && k[:len(keyPrefix)] == keyPrefix {
					delete(state.keys, k)
					state.deletedKeys[k] = true
				}
			}
			for vp := range state.values {
				// valuePath format is "key::valueName"
				if len(vp) > len(key) && vp[:len(key)] == key {
					delete(state.values, vp)
				}
			}
		}
	}

	return state
}

// decodeUTF16LE decodes UTF-16LE byte array to Go string.
func decodeUTF16LE(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// UTF-16LE: every 2 bytes is a character
	runes := make([]rune, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		// Little endian: low byte first
		r := rune(data[i]) | rune(data[i+1])<<8
		if r == 0 {
			// Null terminator
			break
		}
		runes = append(runes, r)
	}

	return string(runes)
}
