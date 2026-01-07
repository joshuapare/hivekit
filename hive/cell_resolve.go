package hive

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
	"github.com/joshuapare/hivekit/internal/format"
)

// Sentinel errors for cell resolution failures.
var (
	// ErrCellOffsetZero indicates a cell offset of 0, which is invalid.
	ErrCellOffsetZero = errors.New("rel cell: offset is zero")

	// ErrCellOutOfRange indicates a cell offset that exceeds hive bounds.
	ErrCellOutOfRange = errors.New("rel cell: offset out of range")

	// ErrCellTruncated indicates a cell that extends beyond available data.
	ErrCellTruncated = errors.New("rel cell: truncated")
)

// resolveRelCell returns the slice of hiveBuf starting at the absolute
// position for the given relative HCELL offset (header + payload).
func resolveRelCell(hiveBuf []byte, relOff uint32) ([]byte, error) {
	if relOff == 0 {
		return nil, ErrCellOffsetZero
	}
	abs := int(format.HiveDataBase) + int(relOff)
	if abs < 0 || abs > len(hiveBuf) {
		return nil, fmt.Errorf("%w: abs=%d, len=%d", ErrCellOutOfRange, abs, len(hiveBuf))
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
		return nil, fmt.Errorf("%w: header", ErrCellTruncated)
	}

	// decode little-endian int32 without allocations
	size := buf.I32LE(cell)
	if size == 0 {
		return nil, ErrCellOffsetZero
	}
	total := int(size)
	if total < 0 {
		total = -total // allocated cells store negative size
	}
	if total < format.CellHeaderSize {
		return nil, fmt.Errorf("%w: size too small: %d", ErrCellTruncated, total)
	}
	if total > len(cell) {
		return nil, fmt.Errorf("%w: declared size %d > available %d", ErrCellOutOfRange, total, len(cell))
	}

	// Return only the payload (exclude 4-byte size header).
	return cell[format.CellHeaderSize:total], nil
}
