package alloc

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestHBINAccountingInvariant performs random alloc/free operations and
// verifies that sum(allocated) + sum(free) == usable for each HBIN.
func TestHBINAccountingInvariant(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	rng := rand.New(rand.NewSource(42))
	allocated := make(map[int32]int32) // offset -> size

	// Perform 100 random operations
	for range 100 {
		if rng.Float64() < 0.6 || len(allocated) == 0 {
			// Allocate
			size := int32(rng.Intn(512) + 8)
			size = format.Align8I32(size)

			ref, _, err := fa.Alloc(size, ClassNK)
			if err == nil {
				absOff := int32(ref) + int32(format.HeaderSize)
				allocated[absOff] = size
			}
		} else {
			// Free random cell
			var off int32
			for off = range allocated {
				break
			}
			_ = allocated[off] // size not used in Free
			relOff := cellRelOffset(h, off)
			_ = fa.Free(uint32(relOff))
			delete(allocated, off)
		}

		// Verify accounting after each operation
		assertHBINAccounting(t, h, 0, getHBINStart(h, 0))
	}

	assertInvariants(t, fa, h)
}

// TestAllCells8Aligned verifies that all cells (allocated and free)
// are 8-byte aligned in both offset and size.
func TestAllCells8Aligned(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Perform random operations
	for i := range 50 {
		size := int32((i%64 + 1) * 8)
		_, _, _ = fa.Alloc(size, ClassNK)
	}

	// Scan all cells and verify alignment
	cells := scanHBINs(h)
	for _, cell := range cells {
		require.Equal(t, int32(0), cell.Off%8, "cell offset must be 8-aligned at 0x%x", cell.Off)
		require.Equal(t, int32(0), cell.Size%8, "cell size must be 8-aligned at 0x%x", cell.Off)
	}

	assertInvariants(t, fa, h)
}

// TestNoIllegalFreeCells verifies that no free cells smaller than
// 8 bytes exist after any sequence of operations.
func TestNoIllegalFreeCells(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Allocate and free various sizes
	refs := make([]uint32, 0, 20)
	for i := range 20 {
		size := int32((i%32 + 1) * 8)
		ref, _, err := fa.Alloc(size, ClassNK)
		if err == nil {
			refs = append(refs, ref)
		}
	}

	// Free half of them
	for i := 0; i < len(refs); i += 2 {
		_ = fa.Free(refs[i])
	}

	// Verify no illegal free cells
	cells := scanHBINs(h)
	for _, cell := range cells {
		if !cell.IsAllocated {
			require.GreaterOrEqual(t, cell.Size, int32(8),
				"free cell at 0x%x must be >= 8 bytes, got %d", cell.Off, cell.Size)
		}
	}

	assertInvariants(t, fa, h)
}

// TestNoGapsInHBIN verifies that HBINs have no gaps - every byte
// is accounted for in a cell.
func TestNoGapsInHBIN(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Do some allocations
	for i := range 10 {
		_, _, _ = fa.Alloc(int32((i+1)*64), ClassNK)
	}

	// Scan HBIN and verify no gaps
	hbinOff := getHBINStart(h, 0)
	cells := scanHBIN(h.Bytes(), hbinOff)

	expectedOff := hbinOff + format.HBINHeaderSize
	for _, cell := range cells {
		require.Equal(t, expectedOff, cell.Off, "no gap should exist before cell at 0x%x", cell.Off)
		expectedOff = cell.Off + format.Align8I32(cell.Size)
	}

	assertInvariants(t, fa, h)
}
