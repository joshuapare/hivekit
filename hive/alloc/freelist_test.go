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

// Test_FreeCellIntegrity verifies that free cells don't corrupt adjacent cells.
func Test_FreeCellIntegrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.hiv")

	// Create hive with multiple free cells
	createHiveWithFreeCells(t, path, []int{256, 512, 1024})

	h, err := hive.Open(path)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate from first cell (200 bytes payload + 4 byte header = 204 total, aligned to 208)
	ref1, data1, err := fa.Alloc(200+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)
	require.NotNil(t, data1)
	// After 8-byte alignment: 208 total - 4 header = 204 bytes payload
	require.GreaterOrEqual(t, len(data1), 200, "Should get at least 200 bytes of payload")

	// Write pattern to verify it doesn't corrupt neighbors
	for i := range data1 {
		data1[i] = 0xAA
	}

	// Verify size fields of neighboring cells are intact
	data := h.Bytes()

	// Check the allocated cell's size field (should be negative)
	off1 := int(ref1) + format.HeaderSize
	size1 := getI32(data, off1)
	require.Negative(t, size1, "Allocated cell should have negative size")
	require.GreaterOrEqual(
		t,
		-size1,
		int32(200+format.CellHeaderSize),
		"Size should cover allocation",
	)

	// Allocate from second cell (400 bytes payload + 4 byte header = 404 total, aligned to 408)
	_, data2, err := fa.Alloc(400+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data2), 400, "Should get at least 400 bytes of payload")

	// Verify first cell's data wasn't corrupted
	for i := range data1 {
		require.Equal(t, byte(0xAA), data1[i], "Cell 1 data corrupted at offset %d", i)
	}

	// Write different pattern to second cell
	for i := range data2 {
		data2[i] = 0xBB
	}

	// Free first cell
	err = fa.Free(ref1)
	require.NoError(t, err)

	// Verify second cell's data wasn't corrupted by freeing first
	for i := range data2 {
		require.Equal(
			t,
			byte(0xBB),
			data2[i],
			"Cell 2 data corrupted at offset %d after freeing cell 1",
			i,
		)
	}
}

// Test_ForwardCoalescing verifies that freeing adjacent cells merges them correctly.
func Test_ForwardCoalescing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.hiv")

	// Create hive with EXACTLY enough space for two 264-byte cells (no extra free space before them)
	// This ensures they'll be allocated adjacently at the start of the HBIN
	createHiveWithFreeCells(t, path, []int{264, 264, 1000})

	h, err := hive.Open(path)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate two adjacent cells
	ref1, _, err := fa.Alloc(256+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	ref2, _, err := fa.Alloc(256+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	data := h.Bytes()
	off1 := int(ref1) + format.HeaderSize
	off2 := int(ref2) + format.HeaderSize

	// Ensure they are in order (swap if needed)
	if off1 > off2 {
		off1, off2 = off2, off1
		ref1, ref2 = ref2, ref1
	}

	// Verify they are adjacent
	size1Abs := int(getI32(data, off1))
	if size1Abs < 0 {
		size1Abs = -size1Abs
	}
	expectedOff2 := off1 + format.Align8(size1Abs)
	require.Equal(t, expectedOff2, off2, "Cells should be adjacent")

	// Free first cell
	err = fa.Free(ref1)
	require.NoError(t, err)

	// Verify first cell is marked free (positive size)
	size1After := getI32(data, off1)
	require.Positive(t, size1After, "Freed cell should have positive size")

	// Free second cell (should coalesce with first)
	err = fa.Free(ref2)
	require.NoError(t, err)

	// Verify coalescing happened - first cell should now be larger
	sizeCoalesced := getI32(data, off1)
	require.Greater(t, sizeCoalesced, size1After, "Should have coalesced into larger cell")

	// Verify the coalesced size is correct (sum of both cells)
	size2Abs := int(getI32(data, off2))
	if size2Abs < 0 {
		size2Abs = -size2Abs
	}
	expectedCoalesced := int32(size1Abs + size2Abs)
	require.Equal(t, expectedCoalesced, sizeCoalesced, "Coalesced size should be sum of both cells")

	// Critical: Verify the size field at off2 location wasn't left with stale data
	// It should either be part of the coalesced cell's data area, or have valid data
	// Let's verify we can reallocate and it doesn't read stale size from off2
	ref3, _, err := fa.Alloc(400+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	off3 := int(ref3) + format.HeaderSize
	require.Equal(t, off1, off3, "Should reallocate from coalesced cell at original offset")
}

// Test_BackwardCoalescing verifies backward coalescing works correctly.
func Test_BackwardCoalescing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.hiv")

	createHiveWithFreeCells(t, path, []int{264, 264, 1000})

	h, err := hive.Open(path)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate two adjacent cells
	ref1, _, err := fa.Alloc(256+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	ref2, _, err := fa.Alloc(256+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	data := h.Bytes()
	off1 := int(ref1) + format.HeaderSize
	off2 := int(ref2) + format.HeaderSize

	// Ensure they are in order (swap if needed)
	if off1 > off2 {
		off1, off2 = off2, off1
		ref1, ref2 = ref2, ref1
	}

	// Free in reverse order (second then first)
	err = fa.Free(ref2)
	require.NoError(t, err)

	// Verify second cell is free
	size2 := getI32(data, off2)
	require.Positive(t, size2, "Second cell should be free (positive size)")

	// Free first cell (should coalesce forward with second)
	err = fa.Free(ref1)
	require.NoError(t, err)

	// Verify first cell now contains the coalesced size
	sizeCoalesced := getI32(data, off1)
	require.Greater(t, sizeCoalesced, int32(256+format.CellHeaderSize), "Should have coalesced")

	// Verify we can reallocate a large chunk from the coalesced cell
	ref3, _, err := fa.Alloc(400+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)
	off3 := int(ref3) + format.HeaderSize
	require.Equal(t, off1, off3, "Should reallocate from first cell after coalescing")
}

// Test_FreeCellSizeFieldIntegrity specifically tests that free operations don't corrupt size fields.
func Test_FreeCellSizeFieldIntegrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.hiv")

	// Create hive with space for multiple allocations
	createHiveWithFreeCells(t, path, []int{4096})

	h, err := hive.Open(path)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate 5 cells
	refs := make([]CellRef, 5)
	for i := range 5 {
		ref, _, allocErr := fa.Alloc(200+format.CellHeaderSize, ClassNK)
		require.NoError(t, allocErr)
		refs[i] = ref
	}

	data := h.Bytes()

	// Verify all cells have valid size fields
	for i, ref := range refs {
		off := int(ref) + format.HeaderSize
		size := getI32(data, off)
		require.Negative(t, size, "Cell %d should be allocated (negative size)", i)

		sizeAbs := -size
		require.GreaterOrEqual(t, sizeAbs, int32(200+format.CellHeaderSize),
			"Cell %d size %d should be at least %d", i, sizeAbs, 200+format.CellHeaderSize)
		require.LessOrEqual(t, sizeAbs, int32(4096),
			"Cell %d size %d should not exceed HBIN size", i, sizeAbs)
	}

	// Free cells 1, 3 (non-adjacent)
	err = fa.Free(refs[1])
	require.NoError(t, err)
	err = fa.Free(refs[3])
	require.NoError(t, err)

	// Verify freed cells have positive sizes
	for _, idx := range []int{1, 3} {
		off := int(refs[idx]) + format.HeaderSize
		size := getI32(data, off)
		require.Positive(t, size, "Freed cell %d should have positive size", idx)
	}

	// Verify allocated cells still have negative sizes
	for _, idx := range []int{0, 2, 4} {
		off := int(refs[idx]) + format.HeaderSize
		size := getI32(data, off)
		require.Negative(t, size, "Allocated cell %d should still have negative size", idx)

		// CRITICAL: Verify size value is reasonable, not corrupted with signature bytes
		sizeAbs := -size
		require.Less(t, sizeAbs, int32(1000),
			"Cell %d size %d looks corrupted (too large)", idx, sizeAbs)

		// Check if size looks like it contains ASCII signature bytes
		if sizeAbs > 10000 {
			t.Errorf(
				"Cell %d size 0x%X may be corrupted with signature bytes",
				idx,
				uint32(sizeAbs),
			)
		}
	}

	// Free remaining cells
	for _, idx := range []int{0, 2, 4} {
		err = fa.Free(refs[idx])
		require.NoError(t, err)
	}

	// Verify the first cell is now free (positive size)
	// Note: We can't assert a specific coalesced size since the allocator doesn't
	// guarantee cells are allocated adjacently - they may be in different size classes
	off0 := int(refs[0]) + format.HeaderSize
	sizeFinal := getI32(data, off0)
	require.Positive(t, sizeFinal, "Cell should be free (positive size)")
	require.Greater(
		t,
		sizeFinal,
		int32(200),
		"Cell size should be at least the original allocation",
	)
}

// Test_GrowAndFree tests that cells in newly grown HBINs work correctly with Free().
func Test_GrowAndFree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.hiv")

	// Create small hive
	createHiveWithFreeCells(t, path, []int{512})

	h, err := hive.Open(path)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate from existing space
	_, _, err = fa.Alloc(256+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	// Allocate more than available - should trigger Grow()
	ref2, _, err := fa.Alloc(3000+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)

	data := h.Bytes()
	off2 := int(ref2) + format.HeaderSize

	// Verify the cell in the new HBIN has a valid size
	size2 := getI32(data, off2)
	require.Negative(t, size2, "Cell in new HBIN should be allocated")

	size2Abs := -size2
	require.GreaterOrEqual(t, size2Abs, int32(3000+format.CellHeaderSize),
		"Cell size should cover allocation")

	// CRITICAL: Free the cell in the new HBIN
	freeErr := fa.Free(ref2)
	require.NoError(t, freeErr)

	// Verify size field after free - this is where corruption happens!
	size2After := getI32(data, off2)
	t.Logf("Size after free: 0x%X (%d)", uint32(size2After), size2After)
	t.Logf("Size before free: 0x%X (%d)", uint32(size2Abs), size2Abs)

	require.Positive(t, size2After, "Freed cell should have positive size")
	// Size after free may be larger due to forward coalescing with trailing free space
	require.GreaterOrEqual(
		t,
		size2After,
		size2Abs,
		"Size should be at least the original allocation",
	)

	// Verify size value is reasonable, not corrupted
	if size2After > int32(10000) || size2After < int32(3000) {
		t.Errorf("Size 0x%X looks corrupted! Expected around %d", uint32(size2After), size2Abs)

		// Decode to see if it's ASCII
		b0 := byte(size2After)
		b1 := byte(size2After >> 8)
		b2 := byte(size2After >> 16)
		b3 := byte(size2After >> 24)
		t.Errorf("Size bytes: 0x%02X 0x%02X 0x%02X 0x%02X ('%c%c%c%c')",
			b0, b1, b2, b3, b0, b1, b2, b3)
	}

	// Try to reallocate - should succeed without corruption
	// Note: We may not get the same cell back due to coalescing and allocator ordering,
	// but we should be able to allocate successfully, proving Free() worked correctly
	ref3, _, err := fa.Alloc(3000+format.CellHeaderSize, ClassNK)
	require.NoError(t, err)
	require.NotEqual(t, CellRef(0), ref3, "Should successfully reallocate")
}
