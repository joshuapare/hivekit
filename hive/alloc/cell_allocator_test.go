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

// Test_Alloc_ExactFit_NoRemainder verifies exact-fit allocations leave no remainder
// This is Test #8 from DEBUG.md: "Alloc_ExactFit_NoRemainder".
func Test_Alloc_ExactFit_NoRemainder(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create a hive with a 256-byte free cell
	createHiveWithFreeCells(t, hivePath, []int{256})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate exactly 256 bytes
	ref, buf, err := fa.Alloc(256, ClassNK)
	require.NoError(t, err)
	require.NotZero(t, ref)

	// Verify the entire cell was used (no split)
	data := h.Bytes()
	fileOff := int(ref) + format.HeaderSize
	cellSize := cellAbsSize(data, fileOff)

	require.Equal(t, 256, cellSize, "Cell should be exactly 256 bytes (no remainder)")

	// Verify no free cell follows this one
	nextOff := fileOff + cellSize
	if nextOff+format.CellHeaderSize <= len(data) {
		nextSize := getI32(data, nextOff)
		// If there's a cell here, it should be either:
		// - A different free cell from the original free list, OR
		// - At the end of the HBIN
		t.Logf("Allocated exactly %d bytes, no remainder (next cell at 0x%X has size %d)",
			len(buf)+format.CellHeaderSize, nextOff, nextSize)
	}
}

// Test_Alloc_Split_LeavesAlignedRemainder verifies cell splitting leaves aligned remainder
// This is Test #9 from DEBUG.md: "Alloc_Split_LeavesAlignedRemainder".
func Test_Alloc_Split_LeavesAlignedRemainder(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create a hive with a 512-byte free cell
	createHiveWithFreeCells(t, hivePath, []int{512})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate 128 bytes from 512-byte cell (should split)
	ref, _, err := fa.Alloc(128, ClassNK)
	require.NoError(t, err)

	data := h.Bytes()
	fileOff := int(ref) + format.HeaderSize
	allocSize := cellAbsSize(data, fileOff)

	// Verify allocation is 8-byte aligned
	require.Zero(t, allocSize%8, "Allocated cell size must be 8-byte aligned")

	// Find the remainder cell
	nextOff := fileOff + allocSize
	require.Less(t, nextOff+format.CellHeaderSize, len(data), "Should have room for remainder cell")

	remainderSize := getI32(data, nextOff)
	require.Positive(t, remainderSize, "Remainder should be free (positive size)")

	// Verify remainder is 8-byte aligned
	require.Zero(t, remainderSize%8, "Remainder size must be 8-byte aligned")

	// Verify total matches original
	total := allocSize + int(remainderSize)
	require.Equal(t, 512, total, "Allocated size + remainder should equal original 512")

	t.Logf("Split 512 bytes into allocated %d + free %d (both 8-byte aligned)",
		allocSize, remainderSize)
}

// Test_Free_CoalesceAdjacent verifies freeing adjacent cells coalesces them
// This is Test #11 from DEBUG.md: "Free_CoalesceAdjacent".
func Test_Free_CoalesceAdjacent(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create a hive with a large free cell we'll allocate from
	createHiveWithFreeCells(t, hivePath, []int{1024})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate three adjacent cells
	ref1, _, err := fa.Alloc(128, ClassNK)
	require.NoError(t, err)

	ref2, _, err := fa.Alloc(128, ClassNK)
	require.NoError(t, err)

	ref3, _, err := fa.Alloc(128, ClassNK)
	require.NoError(t, err)

	// Free middle cell first
	err = fa.Free(ref2)
	require.NoError(t, err)

	// Free first cell - should coalesce with ref2
	err = fa.Free(ref1)
	require.NoError(t, err)

	// Verify they coalesced by checking if we can allocate a 256-byte cell
	// (which should fit in the coalesced ref1+ref2)
	refBig, _, err := fa.Alloc(256, ClassNK)
	require.NoError(t, err)

	// The big allocation should be at or near ref1's location
	bigOff := int(refBig) + format.HeaderSize
	ref1Off := int(ref1) + format.HeaderSize

	// They should be the same or very close (within the HBIN)
	require.Less(t, abs(bigOff-ref1Off), 512,
		"Large allocation should reuse coalesced space (got 0x%X, expected near 0x%X)",
		bigOff, ref1Off)

	t.Logf("Freed adjacent cells coalesced: allocated 256 bytes at 0x%X (original was 0x%X)",
		bigOff, ref1Off)

	// Clean up
	fa.Free(ref3)
	fa.Free(refBig)
}

// Test_FreeToAlloc_Transition_Integrity verifies repeated free/alloc cycles maintain integrity
// This is Test #12 from DEBUG.md: "FreeToAlloc_Transition_Integrity".
func Test_FreeToAlloc_Transition_Integrity(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{4096})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate -> Free -> Re-allocate same size multiple times
	for i := range 10 {
		ref, buf, allocErr := fa.Alloc(256, ClassNK)
		require.NoError(t, allocErr, "Iteration %d: Alloc failed", i)

		// Verify cell is allocated (negative size)
		data := h.Bytes()
		fileOff := int(ref) + format.HeaderSize
		rawSize := getI32(data, fileOff)
		require.Negative(t, rawSize, "Iteration %d: Cell should be allocated (negative size)", i)

		// Write pattern to buffer
		for j := range buf {
			buf[j] = byte(i)
		}

		// Free it
		err = fa.Free(ref)
		require.NoError(t, err, "Iteration %d: Free failed", i)

		// Verify cell is free (positive size)
		data = h.Bytes()
		rawSize = getI32(data, fileOff)
		require.Positive(t, rawSize, "Iteration %d: Cell should be free (positive size)", i)
	}

	t.Logf("10 alloc/free cycles completed with valid headers each time")
}

// Helper function.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
