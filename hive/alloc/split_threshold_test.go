package alloc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestSplitKeeps8ByteTail verifies that allocating 56 bytes from a 64-byte free cell
// creates an 8-byte tail that is kept as a free cell (not absorbed).
// This is the CRITICAL test for the 8-byte minimum cell size fix.
func TestSplitKeeps8ByteTail(t *testing.T) {
	// Setup: Create hive with a single 64-byte free cell
	h, cellOff := newTestHiveWithSingleCell(t, 64)
	dt := newMockDirtyTracker()
	fa := newFastAllocatorForTest(t, h, dt)
	data := h.Bytes()

	// Allocate 56 bytes (should leave exactly 8-byte tail)
	ref, payload, err := fa.Alloc(56, ClassNK)
	require.NoError(t, err, "Alloc should succeed")
	require.NotEqual(t, int32(0), ref, "ref should be non-zero")
	require.NotNil(t, payload, "payload should not be nil")

	absOff := int32(ref) + int32(format.HeaderSize)

	// Verify allocation came from the expected cell
	assert.Equal(t, cellOff, absOff, "allocation should come from the 64-byte cell we created")

	// Assert 1: Allocated cell header is -56 (negative = allocated)
	allocSize, allocFlag := getCell(data, absOff)
	assert.Equal(t, int32(56), allocSize, "allocated cell size should be 56")
	assert.True(t, allocFlag, "allocated cell should be marked allocated")

	// Assert 2: Tail free cell at offset absOff+56 should be +8 (positive = free)
	tailOff := absOff + 56
	tailSize, tailFlag := getCell(data, tailOff)
	assert.Equal(t, int32(8), tailSize, "tail should be exactly 8 bytes")
	assert.False(t, tailFlag, "tail should be free (not allocated)")

	// Assert 3: Tail is in free list (plus the remaining HBIN space from newTestHiveWithSingleCell)
	stats := getAllocatorStats(fa)
	assert.GreaterOrEqual(t, stats.NumFreeCells, 1, "tail should exist as a free cell in free list")
	// Total free should be the 8-byte tail + remaining HBIN space (4064-64 = 4000)
	assert.Equal(t, int64(4008), stats.TotalFreeBytes, "total free bytes should be 8 (tail) + 4000 (remaining)")

	// Assert 4: Overall invariants hold
	assertInvariants(t, fa, h)
}

// TestSplitAbsorbs4ByteTail verifies that allocating 60 bytes from a 64-byte free cell
// absorbs the 4-byte remainder (because 4 < 8 minimum cell size).
func TestSplitAbsorbs4ByteTail(t *testing.T) {
	// Setup: 64-byte free cell
	h, cellOff := newTestHiveWithSingleCell(t, 64)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)
	data := h.Bytes()

	// Allocate 60 bytes (remainder = 4, must be absorbed)
	ref, _, err := fa.Alloc(60, ClassNK)
	require.NoError(t, err)

	absOff := int32(ref) + int32(format.HeaderSize)
	assert.Equal(t, cellOff, absOff, "allocation should come from the 64-byte cell")

	// Assert: Allocated cell is full 64 bytes (absorbed 4-byte tail)
	allocSize, allocFlag := getCell(data, absOff)
	assert.Equal(t, int32(64), allocSize, "should absorb 4-byte tail, allocating full 64 bytes")
	assert.True(t, allocFlag, "should be allocated")

	// Assert: No 4-byte tail (absorbed), only remaining HBIN space
	stats := getAllocatorStats(fa)
	assert.Equal(t, 1, stats.NumFreeCells, "only remaining HBIN space, no tail")
	assert.Equal(t, int64(4000), stats.TotalFreeBytes, "should have remaining HBIN space (4064-64=4000)")

	assertInvariants(t, fa, h)
}

// TestSplitAbsorbs6ByteTail verifies that allocating 58 bytes from a 64-byte free cell
// absorbs the 6-byte remainder (because 6 < 8 minimum cell size).
func TestSplitAbsorbs6ByteTail(t *testing.T) {
	h, cellOff := newTestHiveWithSingleCell(t, 64)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)
	data := h.Bytes()

	// Allocate 58 bytes (remainder = 6, must be absorbed)
	ref, _, err := fa.Alloc(58, ClassNK)
	require.NoError(t, err)

	absOff := int32(ref) + int32(format.HeaderSize)
	assert.Equal(t, cellOff, absOff, "allocation should come from the 64-byte cell")

	// Assert: Full 64 bytes allocated (absorbed 6-byte tail)
	allocSize, allocFlag := getCell(data, absOff)
	assert.Equal(t, int32(64), allocSize, "should absorb 6-byte tail")
	assert.True(t, allocFlag)

	stats := getAllocatorStats(fa)
	assert.Equal(t, 1, stats.NumFreeCells, "only remaining HBIN space, no 6-byte tail")
	assert.Equal(t, int64(4000), stats.TotalFreeBytes, "should have remaining HBIN space")

	assertInvariants(t, fa, h)
}

// TestNoSplitExactFit verifies that allocating exactly 64 bytes from a 64-byte cell
// results in no split and no tail.
func TestNoSplitExactFit(t *testing.T) {
	h, cellOff := newTestHiveWithSingleCell(t, 64)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)
	data := h.Bytes()

	// Allocate exactly 64 bytes (exact fit, no split)
	ref, _, err := fa.Alloc(64, ClassNK)
	require.NoError(t, err)

	absOff := int32(ref) + int32(format.HeaderSize)
	assert.Equal(t, cellOff, absOff, "allocation should come from the 64-byte cell")

	// Assert: Allocated exactly 64 bytes
	allocSize, allocFlag := getCell(data, absOff)
	assert.Equal(t, int32(64), allocSize, "exact fit allocation")
	assert.True(t, allocFlag)

	// Assert: No tail from exact fit, only remaining HBIN space
	stats := getAllocatorStats(fa)
	assert.Equal(t, 1, stats.NumFreeCells, "exact fit should not create tail, only remaining space")

	assertInvariants(t, fa, h)
}

// TestSplitMultipleBoundaries is a table-driven test that verifies split behavior
// across various size combinations, testing the boundary conditions around the
// 8-byte minimum cell size.
func TestSplitMultipleBoundaries(t *testing.T) {
	testCases := []struct {
		name           string
		freeSize       int32
		allocSize      int32
		expectTail     bool
		expectTailSize int32
		expectAllocSz  int32 // actual allocated size (may absorb)
	}{
		{
			name:           "64 alloc 56 -> tail 8",
			freeSize:       64,
			allocSize:      56,
			expectTail:     true,
			expectTailSize: 8,
			expectAllocSz:  56,
		},
		{
			name:          "64 alloc 60 -> absorb 4",
			freeSize:      64,
			allocSize:     60,
			expectTail:    false,
			expectAllocSz: 64, // absorbed 4-byte tail
		},
		{
			name:          "64 alloc 58 -> absorb 6",
			freeSize:      64,
			allocSize:     58,
			expectTail:    false,
			expectAllocSz: 64, // absorbed 6-byte tail
		},
		{
			name:          "64 alloc 64 -> exact fit",
			freeSize:      64,
			allocSize:     64,
			expectTail:    false,
			expectAllocSz: 64,
		},
		{
			name:           "128 alloc 112 -> tail 16",
			freeSize:       128,
			allocSize:      112,
			expectTail:     true,
			expectTailSize: 16,
			expectAllocSz:  112,
		},
		{
			name:           "128 alloc 120 -> tail 8",
			freeSize:       128,
			allocSize:      120,
			expectTail:     true,
			expectTailSize: 8,
			expectAllocSz:  120,
		},
		{
			name:          "128 alloc 124 -> absorb 4",
			freeSize:      128,
			allocSize:     124,
			expectTail:    false,
			expectAllocSz: 128, // absorbed 4-byte tail
		},
		{
			name:          "128 alloc 122 -> absorb 6",
			freeSize:      128,
			allocSize:     122,
			expectTail:    false,
			expectAllocSz: 128, // absorbed 6-byte tail
		},
		{
			name:           "16 alloc 8 -> tail 8",
			freeSize:       16,
			allocSize:      8,
			expectTail:     true,
			expectTailSize: 8,
			expectAllocSz:  8,
		},
		{
			name:          "16 alloc 12 -> absorb 4",
			freeSize:      16,
			allocSize:     12,
			expectTail:    false,
			expectAllocSz: 16, // absorbed 4-byte tail
		},
		{
			name:          "16 alloc 10 -> absorb 6",
			freeSize:      16,
			allocSize:     10,
			expectTail:    false,
			expectAllocSz: 16, // absorbed 6-byte tail
		},
		{
			name:           "256 alloc 240 -> tail 16",
			freeSize:       256,
			allocSize:      240,
			expectTail:     true,
			expectTailSize: 16,
			expectAllocSz:  240,
		},
		{
			name:           "256 alloc 248 -> tail 8",
			freeSize:       256,
			allocSize:      248,
			expectTail:     true,
			expectTailSize: 8,
			expectAllocSz:  248,
		},
		{
			name:          "256 alloc 252 -> absorb 4",
			freeSize:      256,
			allocSize:     252,
			expectTail:    false,
			expectAllocSz: 256,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h, cellOff := newTestHiveWithSingleCell(t, tc.freeSize)
			fa := newFastAllocatorWithRealDirtyTracker(t, h)
			data := h.Bytes()

			// Allocate
			ref, _, err := fa.Alloc(tc.allocSize, ClassNK)
			require.NoError(t, err, "Alloc should succeed")

			absOff := int32(ref) + int32(format.HeaderSize)
			assert.Equal(t, cellOff, absOff, "allocation should come from the cell we created")

			// Verify allocated size
			allocSize, allocFlag := getCell(data, absOff)
			assert.Equal(t, tc.expectAllocSz, allocSize,
				"allocated cell size mismatch")
			assert.True(t, allocFlag, "cell should be allocated")

			// Verify tail
			if tc.expectTail {
				tailOff := absOff + tc.expectAllocSz
				tailSize, tailFlag := getCell(data, tailOff)
				assert.Equal(t, tc.expectTailSize, tailSize,
					"tail size mismatch")
				assert.False(t, tailFlag, "tail should be free")

				stats := getAllocatorStats(fa)
				assert.GreaterOrEqual(t, stats.NumFreeCells, 1,
					"tail should exist as free cell")
			} else {
				stats := getAllocatorStats(fa)
				assert.Equal(t, 1, stats.NumFreeCells,
					"no tail should exist (absorbed or exact fit), only remaining space")
			}

			assertInvariants(t, fa, h)
		})
	}
}

// TestSplitAlignmentRequirement verifies that splits only occur on 8-byte boundaries
// and that cell sizes are always 8-aligned.
func TestSplitAlignmentRequirement(t *testing.T) {
	h, cellOff := newTestHiveWithSingleCell(t, 128)
	fa := newFastAllocatorWithRealDirtyTracker(t, h)
	data := h.Bytes()

	// Allocate 63 bytes (will be rounded up to 64 due to 8-byte alignment)
	ref, _, err := fa.Alloc(63, ClassNK)
	require.NoError(t, err)

	absOff := int32(ref) + int32(format.HeaderSize)
	assert.Equal(t, cellOff, absOff, "allocation should come from the 128-byte cell")

	// Actual allocation should be 64 (aligned), leaving 64-byte tail
	allocSize, _ := getCell(data, absOff)
	assert.Equal(t, int32(0), allocSize%8, "allocated size must be 8-aligned")

	// Verify tail exists and is 8-aligned
	tailOff := absOff + allocSize
	tailSize, tailFlag := getCell(data, tailOff)
	assert.False(t, tailFlag, "tail should be free")
	assert.Equal(t, int32(0), tailSize%8, "tail size must be 8-aligned")
	assert.GreaterOrEqual(t, tailSize, int32(8), "tail must be >= 8 bytes")

	assertInvariants(t, fa, h)
}
