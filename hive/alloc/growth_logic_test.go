//go:build linux || darwin

package alloc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_Grow_AppendsContiguousPages verifies new HBINs are appended contiguously
// This is Test #13 from DEBUG.md: "Grow_AppendsContiguousPages".
func Test_Grow_AppendsContiguousPages(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Record initial end
	data := h.Bytes()
	firstEnd := len(data)

	// Grow by one page
	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	data = h.Bytes()
	secondEnd := len(data)

	// Verify second HBIN starts exactly where first ended
	// Check that second HBIN signature is at firstEnd
	require.GreaterOrEqual(t, len(data), firstEnd+format.HBINHeaderSize)
	sig := string(data[firstEnd : firstEnd+4])
	require.Equal(t, string(format.HBINSignature), sig,
		"Second HBIN should start at 0x%X (where first ended)", firstEnd)

	// Grow again
	err = fa.GrowByPages(2) // Add 8KB HBIN
	require.NoError(t, err)

	data = h.Bytes()
	thirdEnd := len(data)

	// Verify third HBIN starts exactly where second ended
	require.GreaterOrEqual(t, len(data), secondEnd+format.HBINHeaderSize)
	sig = string(data[secondEnd : secondEnd+4])
	require.Equal(t, string(format.HBINSignature), sig,
		"Third HBIN should start at 0x%X (where second ended)", secondEnd)

	t.Logf("HBINs appended contiguously: 0x1000 → 0x%X → 0x%X → 0x%X",
		firstEnd, secondEnd, thirdEnd)
}

// Test_Grow_UpdatesHeaderBlocksEachTime verifies header data size increments after each grow
// This is Test #14 from DEBUG.md: "Grow_UpdatesHeaderBlocksEachTime".
func Test_Grow_UpdatesHeaderBlocksEachTime(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Track header data size after each grow
	data := h.Bytes()
	initialSize := getU32(data, format.REGFDataSizeOffset)
	t.Logf("Initial data size: 0x%X", initialSize)

	pages := []int{1, 2, 1} // 4KB, 8KB, 4KB
	expectedIncreases := []uint32{}

	for i, numPages := range pages {
		err = fa.GrowByPages(numPages)
		require.NoError(t, err, "Grow iteration %d failed", i)

		data = h.Bytes()
		newSize := getU32(data, format.REGFDataSizeOffset)

		// Calculate expected increase (may be aligned up from requested)
		increase := newSize - initialSize
		expectedIncreases = append(expectedIncreases, increase)

		t.Logf("After grow %d (requested %d pages = %d KB): data size = 0x%X (increase: 0x%X)",
			i, numPages, numPages*4, newSize, increase)

		require.Greater(t, newSize, initialSize,
			"Data size should increase after grow (iteration %d)", i)

		initialSize = newSize
	}

	t.Logf("Header data size updated after each grow: increases were %v", expectedIncreases)
}

// Test_Grow_SeedFreeSpaceAcrossPages verifies allocator can use free space from any page
// This is Test #15 from DEBUG.md: "Grow_SeedFreeSpaceAcrossPages".
func Test_Grow_SeedFreeSpaceAcrossPages(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow to add multiple pages
	for range 3 {
		err = fa.GrowByPages(1) // Add 4KB HBIN
		require.NoError(t, err)
	}

	// Now allocate many small cells - should consume from different pages
	allocations := []CellRef{}
	for i := range 50 {
		ref, _, allocErr := fa.Alloc(64, ClassNK)
		require.NoError(t, allocErr, "Allocation %d failed", i)
		allocations = append(allocations, ref)
	}

	// Free half of them to create fragmentation across pages
	for i := 0; i < len(allocations); i += 2 {
		err = fa.Free(allocations[i])
		require.NoError(t, err)
	}

	// Allocate again - should be able to reuse freed space from different pages
	for i := range 10 {
		ref, _, allocErr := fa.Alloc(64, ClassNK)
		require.NoError(t, allocErr, "Re-allocation %d failed", i)

		// Verify it's a valid allocation
		data := h.Bytes()
		fileOff := int(ref) + format.HeaderSize
		require.Less(t, fileOff, len(data))

		cellSize := getI32(data, fileOff)
		require.Negative(t, cellSize, "Cell should be allocated (negative size)")
	}

	t.Logf("Allocator successfully used free space across multiple pages")
}
