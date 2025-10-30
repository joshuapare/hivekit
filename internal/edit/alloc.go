package edit

import (
	"encoding/binary"
	"github.com/joshuapare/hivekit/internal/format"
)

// allocator manages cell offset allocation for the new types.
// It assigns offsets sequentially starting from the first HBIN data area.
// Cells must not span HBIN boundaries per Windows registry spec.
type allocator struct {
	nextOffset int32
	cellBuf    []byte // Reference to cell buffer for writing free cells
}

// newAllocator creates a new allocator for cell buffer (starts at 0).
func newAllocator() *allocator {
	// Start at 0; cells will be placed at format.HBINHeaderSize in HBIN (after header) during packing.
	return &allocator{
		nextOffset: 0,
	}
}

// setCellBuffer sets the reference to the cell buffer for free cell writing
func (a *allocator) setCellBuffer(buf []byte) {
	a.cellBuf = buf
}

// alloc assigns an offset for a cell of the given size and advances the allocator.
// Cells in hives must be 8-byte aligned per spec.
// CRITICAL: Cells must NOT span HBIN boundaries. Each HBIN has format.HBINDataSize bytes of data.
func (a *allocator) alloc(size int) int32 {
	const hbinDataSize = format.HBINDataSize

	// Round size up to 8-byte boundary
	alignedSize := size
	if size%8 != 0 {
		alignedSize = size + (8 - size%8)
	}

	// Calculate current position within the current HBIN
	currentHBIN := a.nextOffset / hbinDataSize
	posInHBIN := a.nextOffset % hbinDataSize

	// Check if cell fits in current HBIN
	if posInHBIN+int32(alignedSize) > hbinDataSize {
		// Cell doesn't fit, skip to next HBIN
		// Write a free cell marker in the remaining space
		remaining := hbinDataSize - posInHBIN
		if remaining >= 8 && a.cellBuf != nil && int(a.nextOffset) < len(a.cellBuf) {
			// Write free cell size (positive = free, negative = allocated)
			// Only write free cells if we have at least 8 bytes (minimum valid cell size)
			// Smaller gaps are left as padding, which is allowed by the hive format
			freeSize := (remaining / 8) * 8
			if freeSize >= 8 {
				binary.LittleEndian.PutUint32(a.cellBuf[a.nextOffset:], uint32(freeSize))
			}
		}

		a.nextOffset = (currentHBIN + 1) * hbinDataSize
	}

	offset := a.nextOffset
	a.nextOffset += int32(alignedSize)
	return offset
}
