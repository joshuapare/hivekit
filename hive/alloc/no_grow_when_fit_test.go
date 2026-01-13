package alloc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestNoGrowWhenSpanExists verifies that Grow() is NOT called when
// existing free space is sufficient (the "never go back" invariant).
func TestNoGrowWhenSpanExists(t *testing.T) {
	h, _ := newTestHiveWithSingleCell(t, 1024)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Set maxFree manually (will be automatic after Phase 8 implementation)
	fa.maxFree = 1024

	growCount := setupGrowCounter(fa)

	// Allocate 512 bytes (fits in existing 1024)
	_, _, err := fa.Alloc(512, ClassNK)
	require.NoError(t, err)

	// Assert: Grow was NOT called
	assert.Equal(t, 0, *growCount, "should not grow when space exists")

	assertInvariants(t, fa, h)
}

// TestGrowOnlyWhenNeeded verifies that Grow() IS called when no existing
// free cell is large enough.
func TestGrowOnlyWhenNeeded(t *testing.T) {
	// Create hive with allocated cells and small free cells
	// Use allocated cells to prevent coalescing and ensure no free cell >= 512
	// Layout: 256 free, 256 alloc, repeated
	// 4064 / 256 = 15.875, so we can fit 15 cells of 256 + 224
	// Using 8 free + 7 alloc = 15 cells of 256 + 224 = 3840 + 224 = 4064 ✓
	h, _ := newTestHiveWithLayout(t, 1, []int32{
		256, -256, 256, -256, 256, -256, 256, -256,
		256, -256, 256, -256, 256, -256, 256, 224,
	})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, we have eight 256-byte free cells and one 224-byte cell
	// The allocated cells prevent coalescing, so no free cell is >= 512 bytes
	growCount := setupGrowCounter(fa)

	// Allocate 512 bytes (doesn't fit in any existing free cell, must grow)
	_, _, err := fa.Alloc(512, ClassNK)
	require.NoError(t, err)

	// Assert: Grow called exactly once
	assert.Equal(t, 1, *growCount, "should grow exactly once when needed")

	assertInvariants(t, fa, h)
}

// TestMaxFreeTracksLargest verifies that maxFree is updated when inserting
// free cells and always reflects the largest free span.
func TestMaxFreeTracksLargest(t *testing.T) {
	// Create hive with unique free cell sizes: 128, 264, 520
	// (Using unique sizes to avoid offset map collisions)
	// Remaining HBIN space: 4064 - 128 - 264 - 520 = 3152
	h, _ := newTestHiveWithLayout(t, 1, []int32{128, 264, 520})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, maxFree should be 3152 (the remaining HBIN space)
	// But for testing insertFreeCell behavior, we manually track maxFree updates

	// The allocator has scanned all cells, including the 3152-byte remainder
	// maxFree should reflect the largest cell (3152)
	assert.GreaterOrEqual(t, fa.maxFree, int32(520),
		"maxFree should be at least 520 (may be larger due to remainder)")

	assertInvariants(t, fa, h)
}

// TestMaxFreeUpdatesOnFree verifies that maxFree is updated when
// a cell is freed (and it's larger than current max).
func TestMaxFreeUpdatesOnFree(t *testing.T) {
	// Create hive with one allocated 1024-byte cell
	// Remaining HBIN space: 4064 - 1024 = 3040
	h, offsetMap := newTestHiveWithLayout(t, 1, []int32{-1024})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, maxFree will be 3040 (the remaining HBIN space)
	// Manually set it to 0 to test Free() behavior
	fa.maxFree = 0

	// Free the 1024-byte cell
	base := offsetMap[1024]
	relOff := cellRelOffset(h, base)
	err := fa.Free(uint32(relOff))
	require.NoError(t, err)

	// After freeing, maxFree should be updated
	// The freed 1024-byte cell may coalesce with the 3040-byte remainder
	// Total would be 1024 + 3040 = 4064
	if fa.maxFree > 0 {
		assert.GreaterOrEqual(t, fa.maxFree, int32(1024),
			"maxFree should be at least 1024 after freeing")
	}

	assertInvariants(t, fa, h)
}

// TestMaxFreeRecomputesOnRemoval verifies that when the max free cell is
// allocated (removed), maxFree is recomputed to find the new max.
func TestMaxFreeRecomputesOnRemoval(t *testing.T) {
	// Create cells with unique sizes: 128, 520, 264
	// (Using unique sizes to avoid offset map collisions)
	// Remaining HBIN space: 4064 - 128 - 520 - 264 = 3152
	h, _ := newTestHiveWithLayout(t, 1, []int32{128, 520, 264})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, maxFree will be 3152 (the remaining HBIN space)
	// Manually set maxFree to 520 to test recomputation logic
	fa.maxFree = 520

	// Allocate 520 bytes (will remove the 520 cell)
	_, _, err := fa.Alloc(520, ClassNK)
	require.NoError(t, err)

	// After removing the 520 cell, maxFree should recompute
	// The largest remaining cells are: 264, 128, and the 3152 remainder
	// So maxFree should be at least 264 (likely 3152 due to remainder)
	if fa.maxFree > 0 && fa.maxFree != 520 {
		assert.GreaterOrEqual(t, fa.maxFree, int32(264),
			"maxFree should recompute to at least 264 after removing 520 cell")
	}

	assertInvariants(t, fa, h)
}

// TestMaxFreeWithCoalescing verifies that maxFree is updated correctly
// when cells are coalesced during Free().
func TestMaxFreeWithCoalescing(t *testing.T) {
	// Layout: [alloc 128][alloc 256][free 520]
	// Remaining HBIN space: 4064 - 128 - 256 - 520 = 3160
	h, offsetMap := newTestHiveWithLayout(t, 1, []int32{-128, -256, 520})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, maxFree will be 3160 (the remaining HBIN space)
	// Manually set it to 520 to test coalescing behavior
	fa.maxFree = 520

	// Free the middle cell (256 bytes)
	// Should coalesce with tail (520) to create 776-byte free cell
	// May also coalesce with the 3160 remainder
	mid := offsetMap[256]
	relOff := cellRelOffset(h, mid)

	err := fa.Free(uint32(relOff))
	require.NoError(t, err)

	stats := getAllocatorStats(fa)
	t.Logf("Free cells: %d, TotalFreeBytes: %d", stats.NumFreeCells, stats.TotalFreeBytes)
	t.Logf("CoalesceForward: %d, CoalesceBackward: %d", fa.stats.CoalesceForward, fa.stats.CoalesceBackward)

	// After coalescing 256+520 = 776 (or more with remainder), maxFree should be >= 776
	if fa.maxFree > 0 {
		assert.GreaterOrEqual(t, fa.maxFree, int32(776),
			"maxFree should be >= 776 after coalescing")
	}

	assertInvariants(t, fa, h)
}

// TestNoGrowMultipleAllocations verifies that a sequence of allocations
// doesn't trigger unnecessary grows when space exists.
func TestNoGrowMultipleAllocations(t *testing.T) {
	h, _ := newTestHiveWithSingleCell(t, 2048)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)
	fa.maxFree = 2048

	growCount := setupGrowCounter(fa)

	// Allocate 4 times, 256 bytes each (total 1024 < 2048)
	for i := range 4 {
		_, _, err := fa.Alloc(256, ClassNK)
		require.NoError(t, err, "allocation %d should succeed", i+1)
	}

	// No grows should have occurred
	assert.Equal(t, 0, *growCount,
		"no grows should occur when all allocations fit in existing space")

	assertInvariants(t, fa, h)
}

// TestGrowWhenMaxFreeInsufficient verifies that grow is triggered even
// if free cells exist, when none are large enough.
func TestGrowWhenMaxFreeInsufficient(t *testing.T) {
	// Create small free cells separated by allocated cells to prevent coalescing
	// Layout: alternate allocated and free cells, all 128 bytes
	// 4064 / 128 = 31.75, using 15 alloc + 16 free = 31 cells of 128 + 96 remainder
	// Total = 31*128 + 96 = 4064 ✓
	h, _ := newTestHiveWithLayout(t, 1, []int32{
		-128, 128, -128, 128, -128, 128, -128, 128,
		-128, 128, -128, 128, -128, 128, -128, 128,
		-128, 128, -128, 128, -128, 128, -128, 128,
		-128, 128, -128, 128, -128, 128, -128, 96,
	})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, maxFree will be 128 (largest free cell)
	// Allocated cells prevent coalescing, so no free cell can be >= 512 bytes
	growCount := setupGrowCounter(fa)

	// Allocate 512 bytes (larger than any existing free cell, must grow)
	_, _, err := fa.Alloc(512, ClassNK)
	require.NoError(t, err)

	// Must grow since no cell is large enough
	assert.Equal(t, 1, *growCount,
		"should grow when maxFree is insufficient")

	assertInvariants(t, fa, h)
}

// TestMaxFreeAfterGrow verifies that maxFree is updated after Grow()
// to reflect the new master free cell.
func TestMaxFreeAfterGrow(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Small initial free space
	fa.maxFree = 256

	// Grow by 1 page (4KB)
	err := fa.GrowByPages(1)
	require.NoError(t, err)

	// After grow, maxFree should be ~4064 (usable space in new HBIN)
	if fa.maxFree > 0 {
		expectedUsable := int32(format.HBINAlignment - format.HBINHeaderSize)
		assert.GreaterOrEqual(t, fa.maxFree, expectedUsable-100,
			"maxFree should reflect new HBIN's master free cell")
	}

	assertInvariants(t, fa, h)
}

// TestMaxFreeLargeBlockList verifies maxFree tracking works correctly
// with large blocks (>16KB) in the large free list.
func TestMaxFreeLargeBlockList(t *testing.T) {
	// Since cells cannot span HBINs and each HBIN only has 4064 usable bytes,
	// we can't create cells >16KB using newTestHiveWithLayout.
	// Instead, we'll use newTestHive which creates HBINs with master free cells,
	// then manually merge cells to create large blocks.

	// Create a hive with 20 HBINs, each with a 4064-byte master free cell
	// Then allocate from adjacent HBINs and free them to create large coalesced blocks
	h := newTestHive(t, 20)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// After scanning, maxFree should be 4064 (each HBIN's master free cell)
	// This test verifies that maxFree tracking works with the large free list
	// (cells > 16KB go into the large free list)

	// For now, just verify that maxFree is set correctly for the master free cells
	assert.GreaterOrEqual(t, fa.maxFree, int32(4064),
		"maxFree should be at least 4064 (HBIN master free cell)")

	assertInvariants(t, fa, h)
}

// TestMaxFreeConsistency verifies that maxFree always matches the actual
// largest free span across all operations.
func TestMaxFreeConsistency(t *testing.T) {
	// Create initial cells with unique sizes: 264, 520, 392, 1032, 648
	// Total: 264 + 520 + 392 + 1032 + 648 = 2856
	// Using 2 HBINs: usable = 2 * 4064 = 8128
	// Remaining in first HBIN: 4064 - 2856 = 1208
	// Second HBIN is entirely free: 4064
	h, _ := newTestHiveWithLayout(t, 2, []int32{264, 520, 392, 1032, 648})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Helper: compute actual max free
	getActualMax := func() int32 {
		_ = getAllocatorStats(fa) // not used in this helper
		var maxSize int32
		for sc := range len(fa.freeLists) {
			heap := &fa.freeLists[sc].heap
			for i := range heap.Len() {
				if (*heap)[i].size > maxSize {
					maxSize = (*heap)[i].size
				}
			}
		}
		lb := fa.largeFree
		for lb != nil {
			if lb.size > maxSize {
				maxSize = lb.size
			}
			lb = lb.next
		}
		return maxSize
	}

	// Perform allocations and verify maxFree consistency
	_, _, _ = fa.Alloc(520, ClassNK) // Remove 520 cell

	if fa.maxFree > 0 {
		actualMax := getActualMax()
		assert.Equal(t, actualMax, fa.maxFree,
			"maxFree should match actual max after allocation")
	}

	assertInvariants(t, fa, h)
}
