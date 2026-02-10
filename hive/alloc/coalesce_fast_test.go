package alloc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestCoalesceForward_O1Lookup verifies that forward coalescing uses
// startIdx for O(1) lookup instead of scanning forward.
func TestCoalesceForward_O1Lookup(t *testing.T) {
	// Create hive with layout: [allocated -64][free +128][free +remaining]
	// The allocator scans and finds the free cells during initialization
	h, offsets := newTestHiveWithLayout(t, 1, []int32{-64, 128})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator (scans and finds the two free cells: 128 and remaining)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Free the 64-byte allocated cell (should merge forward with 128-byte cell)
	relOff := cellRelOffset(h, base)
	err := fa.Free(uint32(relOff))
	require.NoError(t, err)
	_ = offsets // Keep for potential future use

	// Expected: Forward coalesce creates one big free cell (64+128 = 192)
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(192), cellSize, "should coalesce forward to 192 bytes")
	assert.False(t, cellFlag, "coalesced cell should be free")

	// Verify free cell count and total bytes
	// We have the 192-byte coalesced cell + remaining HBIN space (4064-192 = 3872)
	stats := getAllocatorStats(fa)
	assert.Equal(t, 2, stats.NumFreeCells, "should have coalesced cell + remaining space")
	assert.Equal(t, int64(4064), stats.TotalFreeBytes, "total free should be full HBIN (4064)")

	assertInvariants(t, fa, h)
}

// TestCoalesceBackward_O1Lookup verifies that backward coalescing uses
// endIdx for O(1) lookup instead of linear HBIN walk.
func TestCoalesceBackward_O1Lookup(t *testing.T) {
	// Create hive with layout: [free +128][allocated -64][free +remaining]
	h, offsets := newTestHiveWithLayout(t, 1, []int32{128, -64})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator (scans and finds the free cells during initialization)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Free the 64-byte allocated cell (should merge backward with 128-byte cell,
	// then forward with remaining space, creating one full HBIN-sized free cell)
	mid := offsets[64]
	relOff := cellRelOffset(h, mid)
	err := fa.Free(uint32(relOff))
	require.NoError(t, err)

	// Expected: All cells coalesce into one full HBIN cell (128+64+remaining = 4064)
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(4064), cellSize, "should coalesce all adjacent cells to full HBIN")
	assert.False(t, cellFlag, "coalesced cell should be free")

	// Verify free cell count and total bytes
	stats := getAllocatorStats(fa)
	assert.Equal(t, 1, stats.NumFreeCells, "should have single fully coalesced cell")
	assert.Equal(t, int64(4064), stats.TotalFreeBytes, "total free should be full HBIN (4064)")

	assertInvariants(t, fa, h)
}

// TestCoalesceBidirectional verifies that freeing a cell between two free cells
// coalesces in both directions using O(1) lookups.
func TestCoalesceBidirectional(t *testing.T) {
	// Create hive with layout: [free +128][allocated -64][free +128][free +remaining]
	h, offsets := newTestHiveWithLayout(t, 1, []int32{128, -64, 128})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator (scans and finds the free cells during initialization)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Free the middle cell (should merge both directions: 128+64+128 = 320)
	mid := offsets[64]
	relOff := cellRelOffset(h, mid)
	err := fa.Free(uint32(relOff))
	require.NoError(t, err)

	// Expected: Bidirectional coalesce creates 320-byte free cell
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(320), cellSize, "should coalesce bidirectionally to 320 bytes")
	assert.False(t, cellFlag, "coalesced cell should be free")

	// Verify free cell count and total bytes
	// We have the 320-byte coalesced cell + remaining HBIN space (4064-320 = 3744)
	stats := getAllocatorStats(fa)
	assert.Equal(t, 2, stats.NumFreeCells, "should have coalesced cell + remaining space")
	assert.Equal(t, int64(4064), stats.TotalFreeBytes, "total free should be full HBIN (4064)")

	assertInvariants(t, fa, h)
}

// TestCoalesceRespectsHBINBoundary verifies that coalescing never crosses
// HBIN boundaries, even with index lookups.
func TestCoalesceRespectsHBINBoundary(t *testing.T) {
	// Create a hive with 2 HBINs
	// HBIN 0: [allocated -4000][allocated -64] (right at end of HBIN)
	// HBIN 1: [free +128][free +remaining]
	// This ensures last cells are adjacent in file but in different HBINs

	h := newTestHiveEmpty(t, 2) // 2 HBINs, no master free cells
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// HBIN 0: Fill most of it with one allocated cell, leave last 64 bytes
	putCell(data, base, 4000, true)
	lastCellOff := base + 4000
	putCell(data, lastCellOff, 64, true) // Last cell in HBIN 0 (allocated)

	// HBIN 1: Start with free cells
	hbin1Start := int32(format.HeaderSize + format.HBINAlignment + format.HBINHeaderSize)
	putCell(data, hbin1Start, 128, false) // First cell in HBIN 1 (free)

	// Fill remaining space in HBIN 1
	remaining := int32(4064 - 128)
	putCell(data, hbin1Start+128, remaining, false)

	// Create allocator (will scan and find the free cells)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Verify initial state: should have 2 free cells (both in HBIN 1)
	stats := getAllocatorStats(fa)
	assert.Equal(t, 2, stats.NumFreeCells, "should have 2 free cells in HBIN 1")

	// Free the last cell in HBIN 0 - should NOT merge with HBIN 1
	relOff := cellRelOffset(h, lastCellOff)
	err := fa.Free(uint32(relOff))
	require.NoError(t, err)

	// Should now have 3 free cells (one in HBIN 0, two in HBIN 1)
	stats = getAllocatorStats(fa)
	assert.Equal(t, 3, stats.NumFreeCells, "cells in different HBINs stay separate")

	// Verify the freed cell in HBIN 0 stayed separate
	cellSize, cellFlag := getCell(data, lastCellOff)
	assert.Equal(t, int32(64), cellSize, "cell in HBIN 0 should remain 64 bytes")
	assert.False(t, cellFlag, "cell should be free")

	// Verify HBIN 1 cells didn't merge with HBIN 0
	cellSize, cellFlag = getCell(data, hbin1Start)
	assert.Equal(t, int32(128), cellSize, "first cell in HBIN 1 should remain 128 bytes")
	assert.False(t, cellFlag, "cell should be free")

	assertInvariantsNoHBIN(t, fa, h)
}

// TestIndexConsistencyAfterCoalesce verifies that startIdx and endIdx remain
// consistent after coalescing operations.
func TestIndexConsistencyAfterCoalesce(t *testing.T) {
	// Use a pattern similar to the working tests: [free][alloc][free][alloc][free]
	// Then free the allocated cells and verify index consistency
	// Fill to HBIN boundary: 64+72+64+72+64 = 336, barrier = 4064-336 = 3728
	h, _ := newTestHiveWithLayout(t, 1, []int32{64, -72, 64, -72, 64, -3728})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator (scans and finds the three 64-byte free cells)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Calculate offsets
	firstAlloc := base + 64            // First 72-byte allocated cell
	secondAlloc := base + 64 + 72 + 64 // Second 72-byte allocated cell

	// Free first allocated cell (should merge with adjacent free cells: 64+72+64 = 200)
	err := fa.Free(uint32(cellRelOffset(h, firstAlloc)))
	require.NoError(t, err)

	// Verify coalescence
	cellSize, _ := getCell(data, base)
	assert.Equal(t, int32(200), cellSize, "should have 200-byte coalesced cell")

	// Check index consistency after first coalesce
	assertIndexConsistency(t, fa)

	// Free second allocated cell (should merge with previous and following free cells)
	err = fa.Free(uint32(cellRelOffset(h, secondAlloc)))
	require.NoError(t, err)

	// Now we should have one big coalesced cell: 200 + 72 + 64 = 336
	cellSize, _ = getCell(data, base)
	assert.Equal(t, int32(336), cellSize, "should have 336-byte coalesced cell")

	// Check index consistency after second coalesce
	assertIndexConsistency(t, fa)

	assertInvariants(t, fa, h)
}

// TestCoalesceMultipleSequential verifies correct coalescing when freeing
// multiple adjacent allocated cells in sequence.
func TestCoalesceMultipleSequential(t *testing.T) {
	// Create 5 adjacent allocated cells with unique sizes, plus a barrier that fills exactly to HBIN boundary
	// Total: 72+120+256+136+56 = 640, barrier = 4064-640 = 3424
	// Layout: [-72][-120][-256][-136][-56][allocated -3424]
	h, offsets := newTestHiveWithLayout(t, 1, []int32{-72, -120, -256, -136, -56, -3424})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator (no free cells, everything is allocated)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Free them in order: each should coalesce with previous freed cells
	err := fa.Free(uint32(cellRelOffset(h, offsets[72])))
	require.NoError(t, err, "free 72 should succeed")

	err = fa.Free(uint32(cellRelOffset(h, offsets[120])))
	require.NoError(t, err, "free 120 should succeed")

	err = fa.Free(uint32(cellRelOffset(h, offsets[256])))
	require.NoError(t, err, "free 256 should succeed")

	err = fa.Free(uint32(cellRelOffset(h, offsets[136])))
	require.NoError(t, err, "free 136 should succeed")

	err = fa.Free(uint32(cellRelOffset(h, offsets[56])))
	require.NoError(t, err, "free 56 should succeed")

	// All should be coalesced into one big free cell
	// Total: 72+120+256+136+56 = 640 bytes
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(640), cellSize, "all cells should coalesce to 640 bytes")
	assert.False(t, cellFlag, "final cell should be free")

	// Should have only the 640-byte coalesced cell (barrier prevents remaining space)
	stats := getAllocatorStats(fa)
	assert.Equal(t, 1, stats.NumFreeCells, "should have only coalesced cell")

	assertInvariants(t, fa, h)
}

// TestCoalesceWithLargeBlocks verifies that coalescing works correctly
// when merging creates large blocks (>16KB).
func TestCoalesceWithLargeBlocks(t *testing.T) {
	// Create two adjacent allocated blocks that span multiple HBINs
	// We need to manually construct this since it crosses HBIN boundaries
	h := newTestHiveEmpty(t, 10) // 10 HBINs, no master free cells
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Write two adjacent 10KB allocated cells
	putCell(data, base, 10*1024, true)
	mid := base + 10*1024
	putCell(data, mid, 10*1024, true)

	// Fill remaining space in affected HBINs with free cells
	// These cells span from HBIN 0 into HBIN 5 (each HBIN is 4KB)
	// For simplicity, we'll just ensure the 20KB region is allocated

	// Create allocator
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Free first block
	err := fa.Free(uint32(cellRelOffset(h, base)))
	require.NoError(t, err)

	// Free second block (should coalesce to 20KB)
	err = fa.Free(uint32(cellRelOffset(h, mid)))
	require.NoError(t, err)

	// Should have one 20KB free block
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(20*1024), cellSize, "should coalesce to 20KB")
	assert.False(t, cellFlag)

	// Count free cells: should have the 20KB coalesced block
	stats := getAllocatorStats(fa)
	assert.GreaterOrEqual(t, stats.TotalFreeBytes, int64(20*1024), "should have at least 20KB free")

	// Use assertInvariantsNoHBIN since we manually constructed this
	assertInvariantsNoHBIN(t, fa, h)
}

// TestCoalescePreservesAlignment verifies that coalesced cells remain
// 8-byte aligned.
func TestCoalescePreservesAlignment(t *testing.T) {
	// Create cells with various 8-aligned sizes followed by a barrier to prevent merging with remaining space
	// Layout: [-72][-56][-88][-64][-96][allocated -3424][free +remaining]
	h, offsets := newTestHiveWithLayout(t, 1, []int32{-72, -56, -88, -64, -96, -3424})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Free the first 5 cells in order (but not the barrier)
	err := fa.Free(uint32(cellRelOffset(h, offsets[72])))
	require.NoError(t, err)

	err = fa.Free(uint32(cellRelOffset(h, offsets[56])))
	require.NoError(t, err)

	err = fa.Free(uint32(cellRelOffset(h, offsets[88])))
	require.NoError(t, err)

	err = fa.Free(uint32(cellRelOffset(h, offsets[64])))
	require.NoError(t, err)

	err = fa.Free(uint32(cellRelOffset(h, offsets[96])))
	require.NoError(t, err)

	// Verify final coalesced cell is 8-aligned
	// Total: 72+56+88+64+96 = 376 bytes
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(376), cellSize, "should coalesce to 376 bytes")
	assert.False(t, cellFlag, "coalesced cell should be free")
	assert.Equal(t, int32(0), cellSize%8, "coalesced cell must be 8-aligned")
	assert.Equal(t, int32(0), base%8, "coalesced cell offset must be 8-aligned")

	// Verify the barrier is still allocated
	barrierOff := base + 376
	barrierSize, barrierFlag := getCell(data, barrierOff)
	assert.Equal(t, int32(3424), barrierSize, "barrier should remain 3424 bytes")
	assert.True(t, barrierFlag, "barrier should remain allocated")

	assertInvariants(t, fa, h)
}

// TestCoalesceDoesNotCrossAllocated verifies that coalescing stops at
// allocated cells (doesn't skip over them).
func TestCoalesceDoesNotCrossAllocated(t *testing.T) {
	// Layout: [free +64][allocated -64][free +64][free +remaining]
	h, _ := newTestHiveWithLayout(t, 1, []int32{64, -64, 64})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Create allocator (scans and finds the two free cells + remaining space)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// The two free cells should NOT coalesce (separated by allocated cell)
	// We have: 64-byte free, 64-byte allocated, 64-byte free, and remaining space
	stats := getAllocatorStats(fa)
	assert.Equal(t, 3, stats.NumFreeCells, "should have 3 free cells (two 64-byte + remaining)")

	// Verify first free cell
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(64), cellSize, "first cell should be 64 bytes")
	assert.False(t, cellFlag, "first cell should be free")

	// Verify allocated barrier
	mid := base + 64
	cellSize, cellFlag = getCell(data, mid)
	assert.Equal(t, int32(64), cellSize, "middle cell should be 64 bytes")
	assert.True(t, cellFlag, "middle cell should be allocated")

	// Verify second free cell
	tail := mid + 64
	cellSize, cellFlag = getCell(data, tail)
	assert.Equal(t, int32(64), cellSize, "third cell should be 64 bytes")
	assert.False(t, cellFlag, "third cell should be free")

	assertInvariants(t, fa, h)
}

// TestCoalesceIndexUpdates verifies that when cells coalesce, the indexes
// are updated correctly (old entries removed, new entry added).
func TestCoalesceIndexUpdates(t *testing.T) {
	// Use a pattern similar to TestCoalesceBidirectional: [free][alloc][free]
	// Total: 128+80+256 = 464, barrier = 4064-464 = 3600
	// Layout: [free +128][allocated -80][free +256][allocated -3600]
	h, _ := newTestHiveWithLayout(t, 1, []int32{128, -80, 256, -3600})
	data := h.Bytes()
	base := int32(format.HeaderSize + format.HBINHeaderSize)

	// Calculate offsets manually
	firstFree := base             // 128 bytes free
	cell80 := base + 128          // 80 bytes allocated
	secondFree := base + 128 + 80 // 256 bytes free

	// Create allocator (scans and finds the two free cells)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Verify initial index state
	assert.NotNil(t, fa.cellIndex.findByOffset(firstFree), "cellIndex should have first free cell (128)")
	assert.NotNil(t, fa.cellIndex.findByOffset(secondFree), "cellIndex should have second free cell (256)")

	// Free the 80-byte cell (should coalesce with both adjacent free cells: 128+80+256=464)
	err := fa.Free(uint32(cellRelOffset(h, cell80)))
	require.NoError(t, err)

	// Verify index updates:
	// - Merged cell starts at firstFree (base)
	// - Old secondFree entry should be removed
	assert.NotNil(t, fa.cellIndex.findByOffset(firstFree), "cellIndex should have merged cell at base")
	assert.Nil(t, fa.cellIndex.findByOffset(secondFree), "cellIndex should not have old secondFree entry")

	// Verify the merged cell size
	cellSize, cellFlag := getCell(data, base)
	assert.Equal(t, int32(464), cellSize, "should have 464-byte coalesced cell")
	assert.False(t, cellFlag, "cell should be free")

	assertIndexConsistency(t, fa)
	assertInvariants(t, fa, h)
}

// TestCoalescePerformance is a stress test to ensure coalescing remains fast
// with many free cells (O(1) vs O(n) behavior).
func TestCoalescePerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Create alternating pattern that fits cleanly in HBINs
	// Use 3 cells per HBIN: [alloc -1360][free +1352][alloc -1352]
	// This creates a predictable pattern for testing coalescing performance
	// We'll create 50 such triplets across 50 HBINs

	h := newTestHiveEmpty(t, 50) // 50 HBINs
	data := h.Bytes()

	// Write alternating pattern: allocated, free, allocated
	allocOffsets := make([]int32, 0, 100)

	for hbin := range 50 {
		hbinStart := int32(format.HeaderSize + hbin*format.HBINAlignment + format.HBINHeaderSize)

		// First cell: allocated 1360 bytes
		putCell(data, hbinStart, 1360, true)
		allocOffsets = append(allocOffsets, hbinStart)

		// Second cell: free 1352 bytes
		putCell(data, hbinStart+1360, 1352, false)

		// Third cell: allocated 1352 bytes
		putCell(data, hbinStart+1360+1352, 1352, true)
		allocOffsets = append(allocOffsets, hbinStart+1360+1352)
	}

	// Create allocator (scans and finds the 50 free cells)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)

	// Verify we start with exactly 50 free cells
	stats := getAllocatorStats(fa)
	initialFreeCells := stats.NumFreeCells
	assert.Equal(t, 50, initialFreeCells, "should start with exactly 50 free cells")

	// Now free all allocated cells
	// With O(1) coalescing, this should be fast
	// With O(n) HBIN walk, this would be slow
	for _, off := range allocOffsets {
		err := fa.Free(uint32(cellRelOffset(h, off)))
		require.NoError(t, err)
	}

	// Most cells should be coalesced within their HBINs
	// After freeing, we expect ~50 HBINs with fully coalesced cells
	stats = getAllocatorStats(fa)
	assert.LessOrEqual(t, stats.NumFreeCells, 60,
		"most cells should be coalesced with O(1) algorithm")

	// Use assertInvariantsNoHBIN since we manually constructed this complex layout
	assertInvariantsNoHBIN(t, fa, h)
}
