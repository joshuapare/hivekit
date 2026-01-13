package hive

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// NK Resolver Method Tests
// ============================================================================

func TestNK_ResolveSubkeyList_OnRealHive(t *testing.T) {
	// Why this test: Validates that ResolveSubkeyList works on a real hive file.
	// Uses windows-xp-system which has a well-formed structure with LH lists.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Get root NK
	rootOffset := h.RootCellOffset()
	payload, err := resolveRelCellPayload(h.Bytes(), rootOffset)
	require.NoError(t, err)

	rootNK, err := ParseNK(payload)
	require.NoError(t, err)

	// Root should have subkeys
	require.Positive(t, rootNK.SubkeyCount(), "root should have subkeys")

	// Resolve subkey list
	result, err := rootNK.ResolveSubkeyList(h)
	require.NoError(t, err)

	// Should be LH for modern hives
	require.Equal(t, ListLH, result.Kind, "root subkey list should be LH")

	// Validate we can access the list
	require.Positive(t, result.LH.Count(), "LH list should have entries")
}

func TestNK_ResolveSubkeyList_NoSubkeys(t *testing.T) {
	// Why this test: Validates error handling when NK has no subkeys.
	// Need to find an NK with 0 subkeys in a real hive.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Walk the tree to find an NK with no subkeys (leaf key)
	rootOffset := h.RootCellOffset()
	leafNK := findLeafNK(t, h, rootOffset)
	require.NotNil(t, leafNK, "should find at least one leaf NK")

	// Try to resolve subkey list (should fail)
	_, err = leafNK.ResolveSubkeyList(h)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no subkeys")
}

func TestNK_ResolveValueList_OnRealHive(t *testing.T) {
	// Why this test: Validates that ResolveValueList works on a real hive file.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Find an NK with values
	rootOffset := h.RootCellOffset()
	nkWithValues := findNKWithValues(t, h, rootOffset)
	require.NotNil(t, nkWithValues, "should find at least one NK with values")

	// Resolve value list
	vl, err := nkWithValues.ResolveValueList(h)
	require.NoError(t, err)

	// Validate the list
	require.Positive(t, vl.Count(), "value list should have entries")

	// Verify we can access VK offsets (at least some should be valid)
	validOffsets := 0
	for i := range vl.Count() {
		vkOffset, vkErr := vl.VKOffsetAt(i)
		require.NoError(t, vkErr)
		if vkOffset != 0 && vkOffset != 0xFFFFFFFF {
			validOffsets++
		}
	}
	require.Positive(t, validOffsets, "should have at least one valid VK offset")
}

func TestNK_ResolveValueList_NoValues(t *testing.T) {
	// Why this test: Validates error handling when NK has no values.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Find an NK without values
	rootOffset := h.RootCellOffset()
	nkNoValues := findNKWithoutValues(t, h, rootOffset)
	require.NotNil(t, nkNoValues, "should find at least one NK without values")

	// Try to resolve value list (should fail)
	_, err = nkNoValues.ResolveValueList(h)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no values")
}

func TestNK_ResolveSecurity_OnRealHive(t *testing.T) {
	// Why this test: Validates that ResolveSecurity works on a real hive file.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Get root NK
	rootOffset := h.RootCellOffset()
	payload, err := resolveRelCellPayload(h.Bytes(), rootOffset)
	require.NoError(t, err)

	rootNK, err := ParseNK(payload)
	require.NoError(t, err)

	// Resolve security
	sk, err := rootNK.ResolveSecurity(h)
	require.NoError(t, err)

	// Validate SK structure
	require.Positive(t, sk.DescriptorLength(), "SK should have descriptor data")
	require.Positive(t, sk.ReferenceCount(), "SK should be referenced")
}

func TestNK_ResolveClassName_OnRealHive(t *testing.T) {
	// Why this test: Validates ResolveClassName when a class name exists.
	// Note: Class names are rare in real hives. This test may not find one.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Walk the tree to find an NK with a class name
	rootOffset := h.RootCellOffset()
	nkWithClass := findNKWithClassName(t, h, rootOffset)

	if nkWithClass == nil {
		t.Skip("No NK with class name found in this hive")
	}

	// Resolve class name
	className, err := nkWithClass.ResolveClassName(h)
	require.NoError(t, err)
	require.NotEmpty(t, className, "class name should not be empty")
}

func TestNK_ResolveClassName_NoClassName(t *testing.T) {
	// Why this test: Validates error handling when NK has no class name.
	h, err := Open(filepath.Join("..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Most NKs don't have class names
	rootOffset := h.RootCellOffset()
	payload, err := resolveRelCellPayload(h.Bytes(), rootOffset)
	require.NoError(t, err)

	rootNK, err := ParseNK(payload)
	require.NoError(t, err)

	// Root typically doesn't have a class name
	if rootNK.ClassLength() == 0 {
		_, err = rootNK.ResolveClassName(h)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no class name")
	}
}

// ============================================================================
// Helper Functions for Finding Specific NK Types
// ============================================================================

// findLeafNK finds the first NK with no subkeys (leaf node).
func findLeafNK(t *testing.T, h *Hive, startOffset uint32) *NK {
	t.Helper()

	visited := make(map[uint32]bool)
	var result *NK

	var walk func(offset uint32) bool
	walk = func(offset uint32) bool {
		if visited[offset] || result != nil {
			return result != nil
		}
		visited[offset] = true

		payload, err := resolveRelCellPayload(h.Bytes(), offset)
		if err != nil {
			return false
		}

		nk, err := ParseNK(payload)
		if err != nil {
			return false
		}

		// Check if this is a leaf (no subkeys)
		if nk.SubkeyCount() == 0 {
			result = &nk
			return true
		}

		// Recurse to subkeys
		if nk.SubkeyCount() > 0 && nk.SubkeyListOffsetRel() != 0xFFFFFFFF {
			listResult, subkeyErr := nk.ResolveSubkeyList(h)
			if subkeyErr == nil {
				switch listResult.Kind {
				case ListLH:
					for i := range listResult.LH.Count() {
						entry := listResult.LH.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLF:
					for i := range listResult.LF.Count() {
						entry := listResult.LF.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLI:
					for i := range listResult.LI.Count() {
						childOffset := listResult.LI.CellIndexAt(i)
						if walk(childOffset) {
							return true
						}
					}
				case ListRI:
					for i := range listResult.RI.Count() {
						leafOffset := listResult.RI.LeafCellAt(i)
						if walk(leafOffset) {
							return true
						}
					}
				case ListUnknown:
					// Skip unknown list types
				}
			}
		}

		return false
	}

	walk(startOffset)
	return result
}

// findNKWithValues finds the first NK that has values.
func findNKWithValues(t *testing.T, h *Hive, startOffset uint32) *NK {
	t.Helper()

	visited := make(map[uint32]bool)
	var result *NK

	var walk func(offset uint32) bool
	walk = func(offset uint32) bool {
		if visited[offset] || result != nil {
			return result != nil
		}
		visited[offset] = true

		payload, err := resolveRelCellPayload(h.Bytes(), offset)
		if err != nil {
			return false
		}

		nk, err := ParseNK(payload)
		if err != nil {
			return false
		}

		// Check if this NK has values
		if nk.ValueCount() > 0 {
			result = &nk
			return true
		}

		// Recurse to subkeys
		if nk.SubkeyCount() > 0 && nk.SubkeyListOffsetRel() != 0xFFFFFFFF {
			listResult, subkeyErr := nk.ResolveSubkeyList(h)
			if subkeyErr == nil {
				switch listResult.Kind {
				case ListLH:
					for i := range listResult.LH.Count() {
						entry := listResult.LH.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLF:
					for i := range listResult.LF.Count() {
						entry := listResult.LF.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLI:
					for i := range listResult.LI.Count() {
						childOffset := listResult.LI.CellIndexAt(i)
						if walk(childOffset) {
							return true
						}
					}
				case ListRI:
					for i := range listResult.RI.Count() {
						leafOffset := listResult.RI.LeafCellAt(i)
						if walk(leafOffset) {
							return true
						}
					}
				case ListUnknown:
					// Skip unknown list types
				}
			}
		}

		return false
	}

	walk(startOffset)
	return result
}

// findNKWithoutValues finds the first NK that has no values.
func findNKWithoutValues(t *testing.T, h *Hive, startOffset uint32) *NK {
	t.Helper()

	visited := make(map[uint32]bool)
	var result *NK

	var walk func(offset uint32) bool
	walk = func(offset uint32) bool {
		if visited[offset] || result != nil {
			return result != nil
		}
		visited[offset] = true

		payload, err := resolveRelCellPayload(h.Bytes(), offset)
		if err != nil {
			return false
		}

		nk, err := ParseNK(payload)
		if err != nil {
			return false
		}

		// Check if this NK has no values
		if nk.ValueCount() == 0 {
			result = &nk
			return true
		}

		// Recurse to subkeys
		if nk.SubkeyCount() > 0 && nk.SubkeyListOffsetRel() != 0xFFFFFFFF {
			listResult, subkeyErr := nk.ResolveSubkeyList(h)
			if subkeyErr == nil {
				switch listResult.Kind {
				case ListLH:
					for i := range listResult.LH.Count() {
						entry := listResult.LH.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLF:
					for i := range listResult.LF.Count() {
						entry := listResult.LF.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLI:
					for i := range listResult.LI.Count() {
						childOffset := listResult.LI.CellIndexAt(i)
						if walk(childOffset) {
							return true
						}
					}
				case ListRI:
					for i := range listResult.RI.Count() {
						leafOffset := listResult.RI.LeafCellAt(i)
						if walk(leafOffset) {
							return true
						}
					}
				case ListUnknown:
					// Skip unknown list types
				}
			}
		}

		return false
	}

	walk(startOffset)
	return result
}

// findNKWithClassName finds the first NK that has a class name.
func findNKWithClassName(t *testing.T, h *Hive, startOffset uint32) *NK {
	t.Helper()

	visited := make(map[uint32]bool)
	var result *NK

	var walk func(offset uint32) bool
	walk = func(offset uint32) bool {
		if visited[offset] || result != nil {
			return result != nil
		}
		visited[offset] = true

		payload, err := resolveRelCellPayload(h.Bytes(), offset)
		if err != nil {
			return false
		}

		nk, err := ParseNK(payload)
		if err != nil {
			return false
		}

		// Check if this NK has a class name
		if nk.ClassLength() > 0 && nk.ClassNameOffsetRel() != 0xFFFFFFFF {
			result = &nk
			return true
		}

		// Recurse to subkeys (limit depth for performance)
		if nk.SubkeyCount() > 0 && nk.SubkeyListOffsetRel() != 0xFFFFFFFF {
			listResult, subkeyErr := nk.ResolveSubkeyList(h)
			if subkeyErr == nil {
				switch listResult.Kind {
				case ListLH:
					for i := range min(listResult.LH.Count(), 100) { // Limit for perf
						entry := listResult.LH.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLF:
					for i := range min(listResult.LF.Count(), 100) {
						entry := listResult.LF.Entry(i)
						if walk(entry.Cell()) {
							return true
						}
					}
				case ListLI:
					for i := range min(listResult.LI.Count(), 100) {
						childOffset := listResult.LI.CellIndexAt(i)
						if walk(childOffset) {
							return true
						}
					}
				case ListRI:
					for i := range min(listResult.RI.Count(), 100) {
						leafOffset := listResult.RI.LeafCellAt(i)
						if walk(leafOffset) {
							return true
						}
					}
				case ListUnknown:
					// Skip unknown list types
				}
			}
		}

		return false
	}

	walk(startOffset)
	return result
}
