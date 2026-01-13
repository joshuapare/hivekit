package values

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// bitsPerByte is the number of bits in a byte.
	bitsPerByte = 8

	// bitsPerUint16 is the bit position for the 3rd byte (16 bits).
	bitsPerUint16 = 16

	// bitsPerUint24 is the bit position for the 4th byte (24 bits).
	bitsPerUint24 = 24
)

// Write writes a value list to the hive and returns the cell reference.
// Returns InvalidOffset for empty lists.
func Write(_ *hive.Hive, allocator alloc.Allocator, list *List) (uint32, error) {
	if list == nil || list.Len() == 0 {
		return format.InvalidOffset, nil // No values
	}

	// Calculate size: count * DWORDSize (each VK ref is uint32)
	payloadSize := int32(list.Len() * format.DWORDSize)
	// CRITICAL: Allocator expects total cell size (including 4-byte header)
	totalSize := payloadSize + format.CellHeaderSize

	// Allocate cell (use ClassVK as this is a VK reference list)
	ref, buf, err := allocator.Alloc(totalSize, alloc.ClassVK)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate value list: %w", err)
	}

	// Verify we got enough space for the payload
	if len(buf) < int(payloadSize) {
		return 0, fmt.Errorf("allocator returned buffer of size %d, need %d", len(buf), payloadSize)
	}

	// Write the VK references as flat array of uint32
	for i, vkRef := range list.VKRefs {
		offset := i * format.DWORDSize
		buf[offset] = byte(vkRef)
		buf[offset+1] = byte(vkRef >> bitsPerByte)
		buf[offset+2] = byte(vkRef >> bitsPerUint16)
		buf[offset+3] = byte(vkRef >> bitsPerUint24)
	}

	return ref, nil
}

// UpdateNK updates an NK cell with the new value list reference and count.
// This should be called after Write() to update the NK fields.
func UpdateNK(h *hive.Hive, nkRef uint32, valueListRef uint32, count uint32) error {
	data := h.Bytes()
	nkOffset := format.HeaderSize + int(nkRef)

	// Verify NK cell is within bounds
	if nkOffset < 0 || nkOffset+format.NKFixedHeaderSize > len(data) {
		return fmt.Errorf("NK offset out of bounds: 0x%X", nkRef)
	}

	// Skip cell header to get to NK payload
	nkPayload := nkOffset + format.CellHeaderSize

	// Update value count (offset 0x24 in NK structure)
	valueCountOffset := nkPayload + int(format.NKValueCountOffset)
	data[valueCountOffset] = byte(count)
	data[valueCountOffset+1] = byte(count >> bitsPerByte)
	data[valueCountOffset+2] = byte(count >> bitsPerUint16)
	data[valueCountOffset+3] = byte(count >> bitsPerUint24)

	// Update value list offset (offset 0x28 in NK structure)
	valueListOffsetPos := nkPayload + int(format.NKValueListOffset)
	data[valueListOffsetPos] = byte(valueListRef)
	data[valueListOffsetPos+1] = byte(valueListRef >> bitsPerByte)
	data[valueListOffsetPos+2] = byte(valueListRef >> bitsPerUint16)
	data[valueListOffsetPos+3] = byte(valueListRef >> bitsPerUint24)

	return nil
}
