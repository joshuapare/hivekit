package values

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// Read reads the value list from an NK cell.
// Returns ErrNoValueList if the NK has no values.
func Read(h *hive.Hive, nk hive.NK) (*List, error) {
	count := nk.ValueCount()
	if count == 0 {
		return nil, ErrNoValueList
	}

	valueListOffset := nk.ValueListOffsetRel()
	if valueListOffset == format.InvalidOffset {
		return nil, ErrNoValueList
	}

	// Resolve the value list cell
	payload, err := resolveCell(h, valueListOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve value list cell: %w", err)
	}

	// Parse the value list (flat array of uint32 VK offsets)
	vkRefs, err := parseValueList(payload, count)
	if err != nil {
		return nil, err
	}

	return &List{VKRefs: vkRefs}, nil
}

// parseValueList parses a value list payload.
// Value lists are just flat arrays of HCELL_INDEX (uint32) references to VK cells.
func parseValueList(payload []byte, count uint32) ([]uint32, error) {
	need := int(count) * format.DWORDSize // Each entry is a DWORD (uint32)

	if len(payload) < need {
		return nil, ErrTruncated
	}

	vkRefs := make([]uint32, count)
	for i := range count {
		offset := int(i) * format.DWORDSize
		vkRefs[i] = format.ReadU32(payload, offset)
	}

	return vkRefs, nil
}

// resolveCell resolves a cell reference to its payload.
func resolveCell(h *hive.Hive, ref uint32) ([]byte, error) {
	data := h.Bytes()
	offset := format.HeaderSize + int(ref)

	if offset < 0 || offset+4 > len(data) {
		return nil, fmt.Errorf("cell offset out of bounds: 0x%X", ref)
	}

	// Read cell size (4 bytes, little-endian, signed)
	sizeRaw := format.ReadI32(data, offset)

	if sizeRaw >= 0 {
		return nil, fmt.Errorf("cell is free (positive size): 0x%X", ref)
	}

	size := int(-sizeRaw)
	if size < format.CellHeaderSize {
		return nil, fmt.Errorf("cell size too small: %d", size)
	}

	payloadOffset := offset + format.CellHeaderSize
	payloadEnd := offset + size
	if payloadEnd > len(data) {
		return nil, fmt.Errorf("cell payload out of bounds: 0x%X", ref)
	}

	return data[payloadOffset:payloadEnd], nil
}
