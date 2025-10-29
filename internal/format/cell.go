package format

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// Cell represents a single allocation (free or in-use) within an HBIN.
//
// Cell header layout (little-endian):
//
//	Offset  Size  Description
//	0x00    4     Signed size. Negative => allocated, positive => free.
//	              The absolute value includes the 4-byte header.
//	0x04    ...   Payload. First two bytes form the record tag when allocated.
type Cell struct {
	Offset int  // Offset relative to the start of the hive data slice
	Size   int  // Total size including header
	Free   bool // True when the cell is marked as free
	Tag    [SignatureSize]byte
	Data   []byte // Payload bytes (alias of underlying buffer)
}

// NextCell decodes the cell at offset within the HBIN and returns the cell plus
// the offset of the following cell within the same HBIN. The caller must ensure
// offset points to the start of a cell header.
func NextCell(b []byte, h HBIN, off int) (Cell, int, error) {
	if off < 0 || off+CellHeaderSize > len(b) {
		return Cell{}, 0, fmt.Errorf("cell: %w", ErrTruncated)
	}
	if off < int(h.FileOffset)+HBINHeaderSize || off >= int(h.FileOffset)+int(h.Size) {
		return Cell{}, 0, fmt.Errorf("cell: offset %d outside hbin", off)
	}
	raw := buf.I32LE(b[off:])
	if raw == 0 {
		return Cell{}, 0, errors.New("cell: zero length")
	}
	allocated := raw < 0
	size := int(raw)
	if allocated {
		size = -size
	}
	if size < CellHeaderSize {
		return Cell{}, 0, fmt.Errorf("cell: declared size too small (%d)", size)
	}
	next := off + size
	if next > int(h.FileOffset)+int(h.Size) {
		return Cell{}, 0, fmt.Errorf("cell: %w", ErrTruncated)
	}
	payload := b[off+CellHeaderSize : off+size]
	var tag [SignatureSize]byte
	if len(payload) >= SignatureSize {
		tag[0], tag[1] = payload[0], payload[1]
	}
	return Cell{
		Offset: off,
		Size:   size,
		Free:   !allocated,
		Tag:    tag,
		Data:   payload,
	}, next, nil
}

// ParseCell is a convenience wrapper that decodes the first cell in b. It is
// retained for callers that operate on individual cells without iterating an
// entire HBIN.
func ParseCell(b []byte) (Cell, error) {
	if len(b) < CellHeaderSize {
		return Cell{}, fmt.Errorf("cell: %w", ErrTruncated)
	}
	raw := buf.I32LE(b)
	if raw == 0 {
		return Cell{}, errors.New("cell: zero length")
	}
	allocated := raw < 0
	size := int(raw)
	if allocated {
		size = -size
	}
	if size < CellHeaderSize || size > len(b) {
		return Cell{}, fmt.Errorf("cell: %w", ErrTruncated)
	}
	payload := b[CellHeaderSize:size]
	var tag [SignatureSize]byte
	if len(payload) >= SignatureSize {
		tag[0], tag[1] = payload[0], payload[1]
	}
	return Cell{
		Offset: 0,
		Size:   size,
		Free:   !allocated,
		Tag:    tag,
		Data:   payload,
	}, nil
}
