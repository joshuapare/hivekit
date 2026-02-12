package alloc

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// BumpAllocator is a high-performance, append-only allocator that matches hivex's
// allocation strategy exactly. It uses a simple bump-pointer approach for O(1)
// initialization and O(1) allocation.
//
// Key characteristics matching hivex:
//   - O(1) initialization: Only reads endPages from header, no cell scanning
//   - O(1) allocation: Pure bump pointer, no heap operations
//   - Zero memory overhead: No free lists, no indexes, no maps
//   - Free() is a no-op for allocation tracking (only flips sign bit)
//   - HBIN growth uses h.Append() then writes HBIN header manually
//
// This allocator is ideal for single-pass merge operations (StrategyAppend)
// where cell reuse is not needed.
type BumpAllocator struct {
	h  *hive.Hive
	dt DirtyTracker

	// endBlocks is the current bump pointer - the absolute file offset where
	// the next allocation will occur. This corresponds to hivex's endblocks.
	// Remains 0 until first allocation (lazy init per INV-6).
	endBlocks int32

	// endPages is the current end of allocated HBINs (absolute file offset).
	// This corresponds to hivex's endpages.
	endPages int32

	// lastHBINStart is the start of the most recently created HBIN (absolute offset).
	// Used to write the remainder free block within the same HBIN (INV-15).
	lastHBINStart int32
}

// NewBump creates a new BumpAllocator with O(1) initialization.
// Unlike FastAllocator, this does NOT scan existing cells to build a free list.
// It simply reads the current hive size and is ready for append-only allocation.
//
// Parameters:
//   - h: The hive to allocate from
//   - dt: Dirty tracker for marking header changes (can be nil for read-only use)
func NewBump(h *hive.Hive, dt DirtyTracker) (*BumpAllocator, error) {
	data := h.Bytes()

	// Read current data size from header (offset 0x28)
	dataSize := int32(format.ReadU32(data, format.REGFDataSizeOffset))

	// endPages = REGF header (4096) + data size
	// This is the absolute file offset of the end of all HBINs
	endPages := int32(format.HeaderSize) + dataSize

	// endBlocks starts at 0 (lazy init per INV-6)
	// It will be set on first allocation to the next available slot

	return &BumpAllocator{
		h:             h,
		dt:            dt,
		endBlocks:     0, // Lazy init on first alloc
		endPages:      endPages,
		lastHBINStart: 0, // Will be set when we create an HBIN
	}, nil
}

// Alloc allocates a cell using bump-pointer allocation.
// This matches hivex's allocation strategy exactly:
//   - INV-9: 8-byte alignment for all allocations
//   - INV-11: Bump pointer allocation at endBlocks
//   - INV-14: Advance pointer after allocation
//   - INV-15: Mark remainder as free block within HBIN
func (ba *BumpAllocator) Alloc(need int32, cls Class) (CellRef, []byte, error) {
	if need < format.CellHeaderSize {
		return 0, nil, ErrNeedSmall
	}

	// INV-9: 8-byte alignment for all allocations
	need = format.Align8I32(need)

	// Ensure we have space for this allocation
	for ba.endBlocks == 0 || ba.endBlocks+need > ba.endPages {
		if err := ba.grow(need); err != nil {
			return 0, nil, err
		}
	}

	// INV-11: Bump pointer allocation at endBlocks
	cellOff := ba.endBlocks

	// INV-14: Advance pointer after allocation
	ba.endBlocks += need

	data := ba.h.Bytes()

	// Write cell header with negative size (indicating allocated/used)
	// INV-13: Negative size indicates allocated cell
	putI32(data, cellOff, -need)

	// Mark cell as dirty
	if ba.dt != nil {
		ba.dt.Add(int(cellOff), int(need))
	}

	// INV-15: Mark remainder as free block within HBIN
	// If there's space left in the current HBIN after this allocation,
	// we write a free cell marker for the remainder
	remainder := ba.endPages - ba.endBlocks
	if remainder >= 8 { // Minimum cell size is 8 bytes
		// Write free cell header for remainder (positive size)
		putI32(data, ba.endBlocks, remainder)
		if ba.dt != nil {
			ba.dt.Add(int(ba.endBlocks), format.CellHeaderSize)
		}
	}

	// Return CellRef relative to 0x1000 and payload slice
	ref := CellRef(cellOff - format.HeaderSize)
	payload := data[cellOff+format.CellHeaderSize : cellOff+need]

	return ref, payload, nil
}

// Free marks a cell as free by flipping its size to positive.
// Per hivex's strategy (INV-18), Free() is a no-op for allocation tracking.
// We just flip the sign bit - blocks become dead space forever.
// This is acceptable for append-only workloads.
func (ba *BumpAllocator) Free(ref CellRef) error {
	// Convert CellRef to absolute file offset
	off := int32(ref) + format.HeaderSize

	data := ba.h.Bytes()
	if int(off) >= len(data) {
		return ErrBadRef
	}

	// Read current size
	sz := getI32(data, int(off))
	if sz >= 0 {
		// Already free (or invalid) - no-op
		return nil
	}

	// INV-18: Just flip the sign bit to mark as free
	// The block becomes dead space - no tracking, no coalescing
	putI32(data, off, -sz)

	if ba.dt != nil {
		ba.dt.Add(int(off), format.CellHeaderSize)
	}

	return nil
}

// grow appends a new HBIN to the hive file.
// This matches hivex's growth strategy:
//   - INV-19: Page growth rounds up to 4KB boundaries
func (ba *BumpAllocator) grow(need int32) error {
	// Calculate HBIN size needed
	// HBIN must contain header (32 bytes) + requested allocation
	minSize := format.HBINHeaderSize + need

	// INV-19: Round up to 4KB boundary
	hbinSize := format.AlignHBINI32(minSize)

	data := ba.h.Bytes()
	hbinOff := int32(len(data))

	// Check 2GB limit
	newSize := int64(hbinOff) + int64(hbinSize)
	if newSize > maxHiveSize {
		return fmt.Errorf("cannot grow hive beyond 2GB limit (current=%d, requested=%d)",
			hbinOff, hbinSize)
	}

	// Append to the file
	if err := ba.h.Append(int64(hbinSize)); err != nil {
		return ErrGrowFail
	}

	// Get fresh data slice after append
	data = ba.h.Bytes()

	// Write HBIN header
	copy(data[hbinOff:hbinOff+4], format.HBINSignature)
	// HBIN offset field is relative to 0x1000
	putU32(data, int(hbinOff)+format.HBINFileOffsetField, uint32(hbinOff-format.HeaderSize))
	putU32(data, int(hbinOff)+format.HBINSizeOffset, uint32(hbinSize))

	// Update allocator state
	ba.lastHBINStart = hbinOff
	ba.endPages = hbinOff + hbinSize

	// INV-6: Set endBlocks on first allocation (lazy init)
	// Point to the first usable byte after the HBIN header
	if ba.endBlocks == 0 || ba.endBlocks < hbinOff+format.HBINHeaderSize {
		ba.endBlocks = hbinOff + format.HBINHeaderSize
	}

	// Create initial free cell for the entire usable space
	freeOff := hbinOff + format.HBINHeaderSize
	freeSize := hbinSize - format.HBINHeaderSize
	putI32(data, freeOff, freeSize)

	// Update REGF header data size
	ba.h.BumpDataSize(uint32(hbinSize))

	// Update header checksum
	updateHeaderChecksum(data)

	// Mark dirty regions
	if ba.dt != nil {
		ba.dt.Add(0, format.HeaderSize)                 // REGF header
		ba.dt.Add(int(hbinOff), int(hbinSize))          // New HBIN
	}

	return nil
}

// GrowByPages adds a new HBIN of exactly (numPages * 4KB) size.
func (ba *BumpAllocator) GrowByPages(numPages int) error {
	if numPages <= 0 {
		return ErrNeedSmall
	}

	hbinSize := int32(numPages * format.HBINAlignment)

	data := ba.h.Bytes()
	hbinOff := int32(len(data))

	// Check 2GB limit
	newSize := int64(hbinOff) + int64(hbinSize)
	if newSize > maxHiveSize {
		return fmt.Errorf("cannot grow hive beyond 2GB limit (current=%d, requested=%d)",
			hbinOff, hbinSize)
	}

	// Append to the file
	if err := ba.h.Append(int64(hbinSize)); err != nil {
		return ErrGrowFail
	}

	// Get fresh data slice after append
	data = ba.h.Bytes()

	// Write HBIN header
	copy(data[hbinOff:hbinOff+4], format.HBINSignature)
	putU32(data, int(hbinOff)+format.HBINFileOffsetField, uint32(hbinOff-format.HeaderSize))
	putU32(data, int(hbinOff)+format.HBINSizeOffset, uint32(hbinSize))

	// Update allocator state
	ba.lastHBINStart = hbinOff
	ba.endPages = hbinOff + hbinSize

	// Set endBlocks if not yet set
	if ba.endBlocks == 0 {
		ba.endBlocks = hbinOff + format.HBINHeaderSize
	}

	// Create initial free cell for the entire usable space
	freeOff := hbinOff + format.HBINHeaderSize
	freeSize := hbinSize - format.HBINHeaderSize
	putI32(data, freeOff, freeSize)

	// Update REGF header data size
	ba.h.BumpDataSize(uint32(hbinSize))

	// Update header checksum
	updateHeaderChecksum(data)

	// Mark dirty regions
	if ba.dt != nil {
		ba.dt.Add(0, format.HeaderSize)
		ba.dt.Add(int(hbinOff), int(hbinSize))
	}

	return nil
}

// TruncatePages is a no-op for BumpAllocator.
// Bump allocation is append-only and doesn't support truncation.
func (ba *BumpAllocator) TruncatePages(numPages int) error {
	// No-op - bump allocation doesn't support truncation
	return nil
}

// Grow expands the hive by adding a new HBIN of at least the given size.
// Deprecated: Use GrowByPages() for explicit control.
func (ba *BumpAllocator) Grow(need int32) error {
	return ba.grow(need)
}

// Close releases any resources. BumpAllocator has no pooled resources to release.
func (ba *BumpAllocator) Close() {
	// No resources to release - BumpAllocator has no pools
}

// Compile-time interface check
var _ Allocator = (*BumpAllocator)(nil)
