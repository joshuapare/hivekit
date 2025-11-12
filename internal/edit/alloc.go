package edit

import "github.com/joshuapare/hivekit/internal/format"

// allocator manages cell offset allocation for the new types.
// It assigns offsets sequentially starting from the first HBIN data area.
// Cells must not span HBIN boundaries per Windows registry spec.
type allocator struct {
	nextOffset int32
}

// newAllocator creates a new allocator for cell buffer (starts at 0).
func newAllocator() *allocator {
	// Start at 0; cells will be placed at format.HBINHeaderSize in HBIN (after header) during packing.
	return &allocator{
		nextOffset: 0,
	}
}

// alloc assigns an offset for a cell of the given size and advances the allocator.
// Cells in hives must be 8-byte aligned per spec.
// CRITICAL: Cells must NOT span HBIN boundaries. Each HBIN has format.HBINDataSize bytes of data.
// size must be non-negative and fit in int32 range (validated by caller).
func (a *allocator) alloc(size int32) int32 {
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
	if posInHBIN+alignedSize > hbinDataSize {
		// Cell doesn't fit, skip to next HBIN
		// The remaining space in current HBIN will be wasted (or filled with free cell)
		a.nextOffset = (currentHBIN + 1) * hbinDataSize
	}

	offset := a.nextOffset
	a.nextOffset += alignedSize
	return offset
}
