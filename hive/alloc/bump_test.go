package alloc

import (
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBumpAllocator_SimpleAlloc tests basic bump allocation.
func TestBumpAllocator_SimpleAlloc(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err, "NewBump should not error")

	// Allocate a small cell
	ref, payload, err := ba.Alloc(64, ClassNK)
	require.NoError(t, err, "Alloc should succeed")
	require.NotZero(t, ref, "Ref should be non-zero")
	require.Len(t, payload, 60, "Payload should be 64-4 = 60 bytes")

	// Verify cell is marked as allocated (negative size)
	data := h.Bytes()
	absOff := int(ref) + format.HeaderSize
	raw := format.ReadI32(data, absOff)
	assert.Less(t, raw, int32(0), "Cell should have negative size (allocated)")
	assert.Equal(t, int32(-64), raw, "Cell size should be -64")
}

// TestBumpAllocator_MultipleAllocs tests sequential allocations.
func TestBumpAllocator_MultipleAllocs(t *testing.T) {
	h := newTestHive(t, 2)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate multiple cells
	var refs []CellRef
	for i := range 10 {
		size := int32(32 + (i * 8)) // 32, 40, 48, ... 104 bytes
		size = format.Align8I32(size)

		ref, payload, err := ba.Alloc(size, ClassVK)
		require.NoError(t, err, "Alloc %d should succeed", i)
		require.Len(t, payload, int(size-format.CellHeaderSize))
		refs = append(refs, ref)
	}

	// Verify all refs are different and increasing
	for i := 1; i < len(refs); i++ {
		assert.Greater(t, refs[i], refs[i-1], "Refs should be monotonically increasing")
	}
}

// TestBumpAllocator_Alignment tests 8-byte alignment.
func TestBumpAllocator_Alignment(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate various sizes that require alignment
	sizes := []int32{5, 7, 9, 13, 17, 25}
	for _, size := range sizes {
		ref, _, err := ba.Alloc(size, ClassNK)
		require.NoError(t, err, "Alloc(%d) should succeed", size)

		// Verify ref is 8-byte aligned
		absOff := int(ref) + format.HeaderSize
		assert.Equal(t, 0, absOff%8, "Offset should be 8-byte aligned for size %d", size)

		// Verify cell size is 8-byte aligned
		data := h.Bytes()
		rawSize := format.ReadI32(data, absOff)
		actualSize := -rawSize // Allocated = negative
		assert.Equal(t, int32(0), actualSize%8, "Cell size should be 8-byte aligned for request %d", size)
	}
}

// TestBumpAllocator_Grow tests automatic HBIN growth.
func TestBumpAllocator_Grow(t *testing.T) {
	h := newTestHive(t, 1) // Start with 1 HBIN

	initialSize := h.Size()

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate more than what fits in 1 HBIN (4064 usable bytes)
	// Each allocation is 512 bytes, so 10 allocations = 5120 bytes > 4064
	for range 10 {
		_, _, err := ba.Alloc(512, ClassDB)
		require.NoError(t, err, "Alloc should succeed with automatic growth")
	}

	// Verify hive grew
	assert.Greater(t, h.Size(), initialSize, "Hive should have grown")
}

// TestBumpAllocator_Free tests that Free flips the sign bit.
func TestBumpAllocator_Free(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate a cell
	ref, _, err := ba.Alloc(64, ClassNK)
	require.NoError(t, err)

	// Verify it's allocated (negative)
	data := h.Bytes()
	absOff := int(ref) + format.HeaderSize
	rawBefore := format.ReadI32(data, absOff)
	assert.Less(t, rawBefore, int32(0), "Cell should be allocated (negative)")

	// Free the cell
	err = ba.Free(ref)
	require.NoError(t, err, "Free should succeed")

	// Verify it's now free (positive)
	data = h.Bytes()
	rawAfter := format.ReadI32(data, absOff)
	assert.Greater(t, rawAfter, int32(0), "Cell should be free (positive)")
	assert.Equal(t, -rawBefore, rawAfter, "Size magnitude should be preserved")
}

// TestBumpAllocator_FreeIdempotent tests that Free on an already-free cell is idempotent.
func TestBumpAllocator_FreeIdempotent(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate and free a cell
	ref, _, err := ba.Alloc(64, ClassNK)
	require.NoError(t, err)

	err = ba.Free(ref)
	require.NoError(t, err)

	// Free again - should be no-op
	err = ba.Free(ref)
	require.NoError(t, err, "Double-free should be a no-op")

	// Verify still free
	data := h.Bytes()
	absOff := int(ref) + format.HeaderSize
	raw := format.ReadI32(data, absOff)
	assert.Greater(t, raw, int32(0), "Cell should still be free")
}

// TestBumpAllocator_NoReuseAfterFree tests that freed cells are NOT reused (append-only).
func TestBumpAllocator_NoReuseAfterFree(t *testing.T) {
	h := newTestHive(t, 2)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate a cell
	ref1, _, err := ba.Alloc(64, ClassNK)
	require.NoError(t, err)

	// Free it
	err = ba.Free(ref1)
	require.NoError(t, err)

	// Allocate another cell - should NOT reuse the freed cell
	ref2, _, err := ba.Alloc(64, ClassNK)
	require.NoError(t, err)

	// ref2 should be AFTER ref1 (append-only)
	assert.Greater(t, ref2, ref1, "New allocation should be after freed cell (no reuse)")
}

// TestBumpAllocator_GrowByPages tests explicit page growth.
func TestBumpAllocator_GrowByPages(t *testing.T) {
	h := newTestHive(t, 1)

	initialSize := h.Size()

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Grow by 2 pages (8KB)
	err = ba.GrowByPages(2)
	require.NoError(t, err, "GrowByPages should succeed")

	// Verify hive grew by exactly 8KB
	assert.Equal(t, initialSize+8192, h.Size(), "Hive should grow by exactly 8KB")
}

// TestBumpAllocator_MinCellSize tests the minimum cell size constraint.
func TestBumpAllocator_MinCellSize(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Try to allocate less than 4 bytes (header size)
	_, _, err = ba.Alloc(2, ClassNK)
	assert.Error(t, err, "Alloc(<4) should error")
	assert.ErrorIs(t, err, ErrNeedSmall)
}

// TestBumpAllocator_DirtyTracking tests that dirty tracking works.
func TestBumpAllocator_DirtyTracking(t *testing.T) {
	h := newTestHive(t, 1)

	dt := &bumpMockDirtyTracker{}
	ba, err := NewBump(h, dt)
	require.NoError(t, err)

	// Allocate should mark dirty
	_, _, err = ba.Alloc(64, ClassNK)
	require.NoError(t, err)

	assert.Greater(t, len(dt.regions), 0, "Should have dirty regions after alloc")
}

// TestBumpAllocator_HBINHeader tests HBIN header correctness after growth.
func TestBumpAllocator_HBINHeader(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Force growth by allocating more than fits
	for range 15 {
		_, _, err := ba.Alloc(512, ClassDB)
		require.NoError(t, err)
	}

	data := h.Bytes()

	// Find and verify all HBIN headers
	hbinCount := 0
	for off := format.HeaderSize; off < len(data); {
		// Check for HBIN signature
		if off+4 <= len(data) && string(data[off:off+4]) == "hbin" {
			hbinCount++

			// Verify HBIN size field
			hbinSize := format.ReadU32(data, off+format.HBINSizeOffset)
			assert.Equal(t, uint32(0), hbinSize%4096, "HBIN size should be 4KB aligned")
			assert.GreaterOrEqual(t, hbinSize, uint32(4096), "HBIN should be at least 4KB")

			off += int(hbinSize)
		} else {
			break
		}
	}

	assert.GreaterOrEqual(t, hbinCount, 2, "Should have created multiple HBINs")
}

// TestBumpAllocator_HeaderDataSize tests that REGF data size is updated correctly.
func TestBumpAllocator_HeaderDataSize(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	data := h.Bytes()
	initialDataSize := format.ReadU32(data, format.REGFDataSizeOffset)

	// Grow by 2 pages
	err = ba.GrowByPages(2)
	require.NoError(t, err)

	data = h.Bytes()
	newDataSize := format.ReadU32(data, format.REGFDataSizeOffset)

	assert.Equal(t, initialDataSize+8192, newDataSize, "Data size should increase by 8KB")
}

// TestBumpAllocator_Close tests that Close is safe.
func TestBumpAllocator_Close(t *testing.T) {
	h := newTestHive(t, 1)

	ba, err := NewBump(h, nil)
	require.NoError(t, err)

	// Allocate some cells
	for range 5 {
		_, _, err := ba.Alloc(64, ClassNK)
		require.NoError(t, err)
	}

	// Close should not panic
	require.NotPanics(t, func() {
		ba.Close()
	})
}

// bumpMockDirtyTracker is a simple DirtyTracker for testing BumpAllocator.
type bumpMockDirtyTracker struct {
	regions []bumpDirtyRegion
}

type bumpDirtyRegion struct {
	off, length int
}

func (m *bumpMockDirtyTracker) Add(off, length int) {
	m.regions = append(m.regions, bumpDirtyRegion{off, length})
}
