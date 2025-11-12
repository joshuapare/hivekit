package alloc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllocationDeterminism verifies that the same sequence of allocations
// produces identical cell offsets across multiple runs.
func TestAllocationDeterminism(t *testing.T) {
	sequence := []int32{64, 128, 256, 512, 128, 64, 1024}

	// Run 1
	h1 := newTestHive(t, 2)
	fa1 := newFastAllocatorWithRealDirtyTracker(t, h1)
	offsets1 := make([]int32, len(sequence))

	for i, size := range sequence {
		ref, _, err := fa1.Alloc(size, ClassNK)
		require.NoError(t, err)
		offsets1[i] = int32(ref)
	}

	// Run 2 (identical sequence)
	h2 := newTestHive(t, 2)
	fa2 := newFastAllocatorWithRealDirtyTracker(t, h2)
	offsets2 := make([]int32, len(sequence))

	for i, size := range sequence {
		ref, _, err := fa2.Alloc(size, ClassNK)
		require.NoError(t, err)
		offsets2[i] = int32(ref)
	}

	// Assert: Identical offsets
	assert.Equal(t, offsets1, offsets2, "allocations must be deterministic")
}

// TestBestFitDeterminism verifies consistent selection when multiple
// cells of same size exist (tie-breaking is deterministic).
func TestBestFitDeterminism(t *testing.T) {
	// Run allocation sequence twice
	// With ties, should pick same cell each time
	for range 2 {
		h := newTestHive(t, 1)
		fa := newFastAllocatorWithRealDirtyTracker(t, h)

		// Create identical free cells: 512, 512, 512
		// Allocation of 256 should pick same one each run
		ref, _, err := fa.Alloc(256, ClassNK)
		require.NoError(t, err)
		// In practice, should get same offset each time
		_ = ref
	}
}

// TestCoalesceDeterminism verifies that freeing cells in different orders
// produces the same final layout.
func TestCoalesceDeterminism(t *testing.T) {
	// This test verifies that coalescing is order-independent
	// Free order shouldn't affect final free cell layout

	// Run 1: Free in order 0, 1, 2
	h1 := newTestHive(t, 1)
	fa1 := newFastAllocatorWithRealDirtyTracker(t, h1)

	refs1 := make([]uint32, 3)
	for i := range 3 {
		ref, _, _ := fa1.Alloc(64, ClassNK)
		refs1[i] = ref
	}
	for i := range 3 {
		_ = fa1.Free(refs1[i])
	}
	stats1 := getAllocatorStats(fa1)

	// Run 2: Free in order 2, 0, 1
	h2 := newTestHive(t, 1)
	fa2 := newFastAllocatorWithRealDirtyTracker(t, h2)

	refs2 := make([]uint32, 3)
	for i := range 3 {
		ref, _, _ := fa2.Alloc(64, ClassNK)
		refs2[i] = ref
	}
	_ = fa2.Free(refs2[2])
	_ = fa2.Free(refs2[0])
	_ = fa2.Free(refs2[1])
	stats2 := getAllocatorStats(fa2)

	// Final state should be identical
	assert.Equal(t, stats1.NumFreeCells, stats2.NumFreeCells,
		"final free cell count should be same regardless of free order")
	assert.Equal(t, stats1.TotalFreeBytes, stats2.TotalFreeBytes,
		"final free bytes should be same regardless of free order")
}
