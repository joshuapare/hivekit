package alloc

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// bumpState holds the state for bump allocation mode.
// When active, allocations are O(1) pointer bumps into a pre-grown
// contiguous region, bypassing the free-list search and heap operations.
type bumpState struct {
	active    bool  // true while bump allocation is in effect
	startOff  int32 // absolute cell offset where the bump region starts
	capacity  int32 // total usable bytes in the bump region
	cursor    int32 // bytes consumed so far (next alloc starts at startOff + cursor)
	hbinStart int32 // absolute offset of the HBIN containing the bump region (cached to avoid findHBINBounds per alloc)
}

var (
	errBumpAlreadyActive = errors.New("alloc: bump mode already active")
	errBumpNotActive     = errors.New("alloc: bump mode not active")
)

// EnableBumpMode pre-grows the hive by adding a new HBIN large enough to hold
// totalNeeded bytes and enables O(1) bump allocation for subsequent Alloc calls.
//
// While bump mode is active, Alloc satisfies requests by advancing a cursor
// through the pre-grown region. When the bump region is exhausted, Alloc
// falls back to the normal free-list path transparently.
//
// Call FinalizeBumpMode when the batch is complete to write a trailing free
// cell for any unused space and disable bump mode.
func (fa *FastAllocator) EnableBumpMode(totalNeeded int32) error {
	if fa.bump.active {
		return errBumpAlreadyActive
	}
	if totalNeeded <= 0 {
		return fmt.Errorf("bump mode requires positive totalNeeded, got %d", totalNeeded)
	}

	// Align to 8-byte boundary — cells must be 8-aligned.
	totalNeeded = format.Align8I32(totalNeeded)

	// Calculate HBIN size: must hold HBIN header + requested bytes.
	// Round up to 4KB page alignment.
	hbinSize := format.AlignHBINI32(totalNeeded + format.HBINHeaderSize)

	// Record the current file length — the new HBIN will start here.
	fileEnd := int32(len(fa.h.Bytes()))

	// Grow the hive by creating the HBIN.
	if err := fa.growByHBINSize(hbinSize); err != nil {
		return err
	}

	// The bump region starts right after the HBIN header.
	bumpStart := fileEnd + format.HBINHeaderSize
	bumpCapacity := hbinSize - format.HBINHeaderSize

	fa.bump = bumpState{
		active:    true,
		startOff:  bumpStart,
		capacity:  bumpCapacity,
		cursor:    0,
		hbinStart: fileEnd, // cache HBIN start to avoid findHBINBounds per allocation
	}

	return nil
}

// FinalizeBumpMode writes a trailing free cell for any unused bump space
// and disables bump mode. After this call, Alloc reverts to the normal
// free-list path.
//
// If the remaining space is large enough (>= minCellSize), it is written
// as a free cell and inserted into the free lists for future reuse.
// If the remaining space is too small for a valid cell, it is absorbed
// into the last allocated cell.
func (fa *FastAllocator) FinalizeBumpMode() error {
	if !fa.bump.active {
		return errBumpNotActive
	}

	remaining := fa.bump.capacity - fa.bump.cursor

	if remaining >= minCellSize {
		// Write a trailing free cell (positive size = free).
		trailOff := fa.bump.startOff + fa.bump.cursor
		data := fa.h.Bytes()
		putI32(data, trailOff, remaining)

		// Mark dirty so the free cell header is flushed.
		if fa.dt != nil {
			fa.dt.Add(int(trailOff), format.CellHeaderSize)
		}

		// Insert into free lists for future normal-path allocations.
		fa.insertFreeCell(trailOff, remaining)
	}
	// If remaining < minCellSize and > 0, the space is wasted but this is
	// acceptable — it's at most minCellSize-1 bytes (7 bytes) and avoids creating
	// an undersized cell.

	fa.bump = bumpState{} // reset all fields, active = false
	return nil
}

// bumpAlloc attempts to satisfy an allocation from the bump region.
// Returns (ref, payload, true) on success, or (0, nil, false) if the bump
// region does not have enough space for the request.
//
// Each allocation writes a cell header (4 bytes, negative = allocated) and
// advances the cursor. The cell size is 8-byte aligned per REGF spec.
func (fa *FastAllocator) bumpAlloc(need int32) (CellRef, []byte, bool) {
	// Align to 8-byte boundary (caller already validated need >= CellHeaderSize).
	aligned := format.Align8I32(need)

	// Check if bump region has enough space.
	if fa.bump.cursor+aligned > fa.bump.capacity {
		return 0, nil, false // exhausted — caller will fall through
	}

	off := fa.bump.startOff + fa.bump.cursor
	fa.bump.cursor += aligned

	// Write the cell header: negative value marks the cell as allocated.
	data := fa.h.Bytes()
	putI32(data, off, -aligned)

	// Mark dirty for the cell header.
	if fa.dt != nil {
		fa.dt.Add(int(off), format.CellHeaderSize)
	}

	// Track statistics.
	fa.stats.AllocFastPath++
	fa.stats.BytesAllocated += int64(aligned)

	// Track HBIN stats using the cached HBIN start (avoids findHBINBounds search).
	if stats, ok := fa.hbinTracking[fa.bump.hbinStart]; ok {
		stats.allocCount++
		stats.bytesAllocated += aligned
		fa.lastAllocHBIN = fa.bump.hbinStart
	}

	// Build payload slice: starts after the 4-byte cell header.
	payload := data[off+format.CellHeaderSize : off+aligned]

	// Convert absolute offset to HCELL_INDEX (relative to 0x1000).
	relOff := uint32(off - int32(format.HeaderSize))
	return relOff, payload, true
}
