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

// Test_Alloc_DoesNotCrossPage verifies that cell allocations never cross HBIN boundaries
// This is Test #10 from DEBUG.md: "Alloc_DoesNotCrossPage".
func Test_Alloc_DoesNotCrossPage(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create a hive with some free cells to start with
	createHiveWithFreeCells(t, hivePath, []int{128, 256, 512, 1024})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Try to allocate cells near page boundaries
	// The allocator should either:
	// 1. Allocate from a different page
	// 2. Grow and allocate from new page
	// But NEVER allocate a cell that crosses an HBIN boundary

	// Allocate many cells to fragment the hive
	refs := make([]CellRef, 0, 100)
	for i := range 100 {
		// Vary sizes to create fragmentation
		size := 64 + (i%10)*32
		ref, _, allocErr := fa.Alloc(int32(size), ClassRD)
		require.NoError(t, allocErr)
		refs = append(refs, ref)
	}

	// Free some to create gaps near boundaries
	for i := 0; i < len(refs); i += 2 {
		freeErr := fa.Free(refs[i])
		require.NoError(t, freeErr)
	}

	// Now allocate a large cell - this should NOT cross page boundaries
	ref, buf, err := fa.Alloc(4000, ClassRD)
	require.NoError(t, err)

	// Verify the allocation is within a single HBIN
	fileOff := int(ref) + format.HeaderSize
	cellEnd := fileOff + len(buf) + format.CellHeaderSize

	// Find which HBIN contains this cell
	hbinStart, hbinSize, found := fa.findHBINBounds(fileOff)
	require.True(t, found, "Cell not found in any HBIN")

	hbinEnd := hbinStart + hbinSize

	// CRITICAL: Cell must not extend past HBIN boundary
	require.LessOrEqual(t, cellEnd, hbinEnd, "Cell extends beyond HBIN boundary")
	t.Logf("Cell allocated at 0x%X (size %d) stays within HBIN bounds [0x%X - 0x%X]",
		fileOff, len(buf)+format.CellHeaderSize, hbinStart, hbinEnd)
}

// Test_DataCell_FitsWithinPage verifies data cells for large values stay within page boundaries
// This is Test #16 from DEBUG.md: "DataCell_FitsWithinPage".
func Test_DataCell_FitsWithinPage(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create a hive with a large free cell
	createHiveWithFreeCells(t, hivePath, []int{8192})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Allocate cells of various sizes near page ends
	sizes := []int{
		format.CellHeaderSize + 16,   // Small cell
		format.CellHeaderSize + 256,  // Medium cell
		format.CellHeaderSize + 1024, // Large cell
		format.CellHeaderSize + 4000, // Very large cell
	}

	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			ref, buf, allocErr := fa.Alloc(int32(size), ClassRD)
			require.NoError(t, allocErr)

			fileOff := int(ref) + format.HeaderSize
			cellEnd := fileOff + len(buf) + format.CellHeaderSize

			// Find HBIN containing this cell
			hbinStart, hbinSize, found := fa.findHBINBounds(fileOff)
			require.True(t, found)

			hbinEnd := hbinStart + hbinSize

			// Verify cell stays within HBIN
			require.GreaterOrEqual(t, fileOff, hbinStart+format.HBINHeaderSize,
				"Cell starts before HBIN data area")
			require.LessOrEqual(t, cellEnd, hbinEnd,
				"Cell extends past HBIN end")
		})
	}
}
