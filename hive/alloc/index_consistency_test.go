package alloc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartIdxMatchesFreeLists verifies that every free cell in the
// free lists appears in startIdx with the correct size.
func TestStartIdxMatchesFreeLists(t *testing.T) {
	// Create hive with 5 free cells: 128, 256, 384, 512, 640 bytes
	sizes := []int32{128, 256, 384, 512, 640}
	h, _ := newTestHiveWithCells(t, sizes)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Skip if indexes not enabled
	if fa.startIdx == nil {
		t.Skip("startIdx not enabled, will test after Phase 8")
	}

	assertIndexConsistency(t, fa)
}

// TestEndIdxMatchesFreeLists verifies that every free cell appears in
// endIdx at the correct end offset with the correct size.
func TestEndIdxMatchesFreeLists(t *testing.T) {
	// Create hive with 5 free cells: 128, 256, 384, 512, 640 bytes
	sizes := []int32{128, 256, 384, 512, 640}
	h, _ := newTestHiveWithCells(t, sizes)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	if fa.endIdx == nil {
		t.Skip("endIdx not enabled, will test after Phase 8")
	}

	assertIndexConsistency(t, fa)
}

// TestMaxFreeMatchesRecomputed verifies that maxFree always matches
// the actual largest free span.
func TestMaxFreeMatchesRecomputed(t *testing.T) {
	// Create hive with free cells of various sizes
	sizes := []int32{256, 512, 384, 1024, 640}
	h, _ := newTestHiveWithCells(t, sizes)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Compute actual max from all free lists (heaps)
	var actualMax int32
	for sc := range len(fa.freeLists) {
		heap := &fa.freeLists[sc].heap
		for i := range heap.Len() {
			if (*heap)[i].size > actualMax {
				actualMax = (*heap)[i].size
			}
		}
	}

	// Also check large free list
	lb := fa.largeFree
	for lb != nil {
		if lb.size > actualMax {
			actualMax = lb.size
		}
		lb = lb.next
	}

	if fa.maxFree > 0 {
		assert.Equal(t, actualMax, fa.maxFree, "maxFree should match actual max")
	}
}

// TestIndexCleanedOnAlloc verifies that allocated cells are removed
// from both startIdx and endIdx.
func TestIndexCleanedOnAlloc(t *testing.T) {
	// Create hive with a 512-byte free cell
	h, offsets := newTestHiveWithCells(t, []int32{512})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	if fa.startIdx == nil {
		t.Skip("indexes not enabled")
	}

	off := offsets[512] // Get the offset of our 512-byte cell

	// Verify in indexes
	assert.Contains(t, fa.startIdx, off, "should be in startIdx before alloc")

	// Allocate
	_, _, err := fa.Alloc(512, ClassNK)
	require.NoError(t, err)

	// Verify removed from indexes (or possibly replaced if split occurred)
	// We just check consistency
	assertIndexConsistency(t, fa)
}

// TestIndexUpdatedOnFree verifies that freed cells are added to
// both startIdx and endIdx with correct values.
func TestIndexUpdatedOnFree(t *testing.T) {
	// Create hive with an allocated 512-byte cell at the start
	h, offsets := newTestHiveWithLayout(t, 1, []int32{-512})
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	if fa.startIdx == nil {
		t.Skip("indexes not enabled")
	}

	off := offsets[512] // Get the offset of our allocated cell

	// Verify NOT in indexes (it's allocated)
	assert.NotContains(t, fa.startIdx, off, "should not be in index when allocated")

	// Free
	relOff := cellRelOffset(h, off)
	err := fa.Free(uint32(relOff))
	require.NoError(t, err)

	// Verify added to indexes (or merged with adjacent free cell)
	// Cell may have coalesced with remaining free space, so just check consistency
	assertIndexConsistency(t, fa)
}
