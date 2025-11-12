package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
	"github.com/joshuapare/hivekit/internal/format"
)

// resolveRelCell returns the slice of hiveBuf starting at the absolute
// position for the given relative HCELL offset (header + payload).
func resolveRelCell(hiveBuf []byte, relOff uint32) ([]byte, error) {
	if relOff == 0 {
		return nil, errors.New("rel cell: offset is zero")
	}
	abs := int(format.HiveDataBase) + int(relOff)
	if abs < 0 || abs > len(hiveBuf) {
		return nil, fmt.Errorf("rel cell: abs=%d out of range (len=%d)", abs, len(hiveBuf))
	}
	return hiveBuf[abs:], nil
}

// resolveRelCellPayload resolves a relative HCELL offset and returns just
// the payload bytes (skipping the 4-byte size header), bounds-checked.
//
// Cell header layout:
//
//	int32 Size  (negative => allocated; positive => free; absolute value includes header)
//	...payload...
func resolveRelCellPayload(hiveBuf []byte, relOff uint32) ([]byte, error) {
	cell, err := resolveRelCell(hiveBuf, relOff)
	if err != nil {
		return nil, err
	}
	if len(cell) < format.CellHeaderSize {
		return nil, errors.New("rel cell: truncated header")
	}

	// decode little-endian int32 without allocations
	size := buf.I32LE(cell)
	if size == 0 {
		return nil, errors.New("rel cell: zero size")
	}
	total := int(size)
	if total < 0 {
		total = -total // allocated cells store negative size
	}
	if total < format.CellHeaderSize {
		return nil, fmt.Errorf("rel cell: size too small: %d", total)
	}
	if total > len(cell) {
		return nil, fmt.Errorf("rel cell: declared size %d > available %d", total, len(cell))
	}

	// Return only the payload (exclude 4-byte size header).
	return cell[format.CellHeaderSize:total], nil
}
