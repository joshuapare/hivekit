package edit

import (
	"encoding/binary"
	"github.com/joshuapare/hivekit/internal/format"
)

// hbinInfo tracks information about each HBIN
type hbinInfo struct {
	offset     int32 // Offset in cell buffer where this HBIN starts
	size       int32 // Size of this HBIN (multiple of 4KB: 4096, 8192, 12288, or 16384)
	fileOffset int32 // Cumulative file offset where this HBIN starts (cached for O(1) lookup)
}

// allocator manages cell offset allocation for the new types.
// It assigns offsets sequentially starting from the first HBIN data area.
// Cells must not span HBIN boundaries per Windows registry spec.
// Supports dynamic HBIN sizing: 4KB, 8KB, 12KB, or 16KB as needed.
type allocator struct {
	nextOffset int32
	cellBuf    []byte     // Reference to cell buffer for writing free cells
	hbins      []hbinInfo // Track HBIN boundaries and sizes
}

// newAllocator creates a new allocator for cell buffer (starts at 0).
func newAllocator() *allocator {
	// Start at 0; cells will be placed at format.HBINHeaderSize in HBIN (after header) during packing.
	// Start with a default 4KB HBIN
	return &allocator{
		nextOffset: 0,
		hbins: []hbinInfo{
			{offset: 0, size: format.HBINAlignment, fileOffset: 0}, // Initial 4KB HBIN
		},
	}
}

// setCellBuffer sets the reference to the cell buffer for free cell writing
func (a *allocator) setCellBuffer(buf []byte) {
	a.cellBuf = buf
}

// alloc assigns an offset for a cell of the given size and advances the allocator.
// Cells in hives must be 8-byte aligned per spec.
// CRITICAL: Cells must NOT span HBIN boundaries.
// CRITICAL: We must always leave at least 8 bytes for a free cell marker, or skip to next HBIN.
// Dynamically chooses HBIN size (4KB, 8KB, 12KB, or 16KB) based on cell size.
func (a *allocator) alloc(size int) int32 {
	const minFreeCellSize = 8 // Minimum size for a valid free cell
	const hbinHeaderSize = format.HBINHeaderSize

	// Round size up to 8-byte boundary
	alignedSize := size
	if size%8 != 0 {
		alignedSize = size + (8 - size%8)
	}

	// Determine minimum HBIN size needed for this cell (multiple of 4KB)
	// HBIN has header (32 bytes), leaving (hbinSize - 32) for data
	minHBINSize := int32(format.HBINAlignment) // Start with 4KB
	for minHBINSize < 16*1024 {
		dataSize := minHBINSize - int32(hbinHeaderSize)
		if int32(alignedSize) <= dataSize-int32(minFreeCellSize) {
			break // Cell fits with room for free cell
		}
		minHBINSize += int32(format.HBINAlignment) // Try next size (8KB, 12KB, 16KB)
	}

	// Get current HBIN
	currentHBINIdx := len(a.hbins) - 1
	currentHBIN := a.hbins[currentHBINIdx]
	currentDataSize := currentHBIN.size - hbinHeaderSize

	// Calculate position within current HBIN
	posInHBIN := a.nextOffset - currentHBIN.offset

	// Check if cell fits in current HBIN with room for a free cell afterward
	spaceAfter := currentDataSize - posInHBIN - int32(alignedSize)
	if spaceAfter < 0 || (spaceAfter > 0 && spaceAfter < minFreeCellSize) {
		// Either cell doesn't fit, or it would leave a gap too small for a free cell
		// Write a free cell in the remaining space
		remaining := currentDataSize - posInHBIN
		if remaining >= minFreeCellSize && a.cellBuf != nil && int(a.nextOffset) < len(a.cellBuf) {
			freeSize := (remaining / 8) * 8
			if freeSize >= minFreeCellSize {
				binary.LittleEndian.PutUint32(a.cellBuf[a.nextOffset:], uint32(freeSize))
			}
		}

		// Move to next HBIN, choosing appropriate size
		nextHBINSize := minHBINSize
		if nextHBINSize < int32(format.HBINAlignment) {
			nextHBINSize = int32(format.HBINAlignment)
		}

		nextOffset := currentHBIN.offset + currentDataSize
		nextFileOffset := currentHBIN.fileOffset + currentHBIN.size // Cumulative file offset
		a.hbins = append(a.hbins, hbinInfo{
			offset:     nextOffset,
			size:       nextHBINSize,
			fileOffset: nextFileOffset,
		})
		a.nextOffset = nextOffset
	}

	offset := a.nextOffset
	a.nextOffset += int32(alignedSize)
	return offset
}

// getHBINs returns the HBIN layout information for packing
func (a *allocator) getHBINs() []hbinInfo {
	return a.hbins
}

// cellBufOffsetToHBINOffset converts a cellBuf offset to an HBIN-relative file offset.
// This accounts for variable-sized HBIN headers when cells span multiple HBINs.
func (a *allocator) cellBufOffsetToHBINOffset(cellBufOff int32) int32 {
	const hbinHeaderSize = format.HBINHeaderSize

	// Binary search for the HBIN containing this offset
	// Find the rightmost HBIN whose start offset is <= cellBufOff
	n := len(a.hbins)
	if n == 0 {
		// Fallback for empty allocator
		hbinDataSize := int32(format.HBINDataSize)
		hbinIndex := cellBufOff / hbinDataSize
		posInHBIN := cellBufOff % hbinDataSize
		return hbinIndex*int32(format.HBINAlignment) + int32(hbinHeaderSize) + posInHBIN
	}

	// Binary search: find largest i where hbins[i].offset <= cellBufOff
	left, right := 0, n
	for left < right {
		mid := (left + right) / 2
		if a.hbins[mid].offset <= cellBufOff {
			left = mid + 1
		} else {
			right = mid
		}
	}

	// left-1 is the HBIN that should contain cellBufOff
	if left == 0 {
		// Before first HBIN - shouldn't happen, but fallback
		hbinDataSize := int32(format.HBINDataSize)
		hbinIndex := cellBufOff / hbinDataSize
		posInHBIN := cellBufOff % hbinDataSize
		return hbinIndex*int32(format.HBINAlignment) + int32(hbinHeaderSize) + posInHBIN
	}

	// Calculate file offset by summing all HBIN sizes before this one
	hbin := a.hbins[left-1]
	hbinDataSize := hbin.size - int32(hbinHeaderSize)
	hbinDataEnd := hbin.offset + hbinDataSize

	if cellBufOff >= hbin.offset && cellBufOff < hbinDataEnd {
		// Cell is in this HBIN - use cached file offset (O(1) lookup)
		posInHBIN := cellBufOff - hbin.offset
		return hbin.fileOffset + int32(hbinHeaderSize) + posInHBIN
	}

	// Shouldn't reach here if cellBufOff is valid - fallback
	hbinDataSizeFallback := int32(format.HBINDataSize)
	hbinIndex := cellBufOff / hbinDataSizeFallback
	posInHBIN := cellBufOff % hbinDataSizeFallback
	return hbinIndex*int32(format.HBINAlignment) + int32(hbinHeaderSize) + posInHBIN
}
