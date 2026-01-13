package hive

import (
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// Cell is a zero-cost view over a single hive cell that lives inside an HBIN.
// A cell on disk looks like:
//
//	int32  size     // NEGATIVE = allocated, POSITIVE = free
//	...    payload
//
// Size is ALWAYS relative to the start of this cell header.
type Cell struct {
	// Buf is the full HBIN data (or even the whole hive) backing this cell.
	Buf []byte
	// Off is the offset into Buf where THIS cell starts.
	Off int
}

// newCellAt creates a cell view at the given offset, doing basic bounds checks.
func newCellAt(buf []byte, off int) (Cell, error) {
	if off+format.CellHeaderSize > len(buf) {
		return Cell{}, fmt.Errorf("hive: cell header at %d truncated (len=%d)", off, len(buf))
	}
	return Cell{Buf: buf, Off: off}, nil
}

// RawSize returns the int32 size field as stored on disk.
// <0 → allocated, >0 → free.
func (c Cell) RawSize() int32 {
	return format.ReadI32(c.Buf, c.Off)
}

// SizeAbs returns the absolute size of the cell (header + payload).
func (c Cell) SizeAbs() int {
	sz := c.RawSize()
	if sz < 0 {
		sz = -sz
	}
	return int(sz)
}

// IsAllocated reports whether the cell is in-use (negative size on disk).
func (c Cell) IsAllocated() bool {
	return c.RawSize() < 0
}

// Payload returns the bytes AFTER the header (i.e. the NK/VK/... body).
func (c Cell) Payload() []byte {
	start := c.Off + format.CellHeaderSize
	end := min(c.Off+c.SizeAbs(), len(c.Buf))
	return c.Buf[start:end]
}

// Signature2 returns the first 2 bytes of the payload (NK, VK, LF, LH, LI, RI, SK, DB ...).
// If the cell is too small, it returns empty.
func (c Cell) Signature2() []byte {
	pl := c.Payload()
	if len(pl) < format.SignatureSize {
		return nil
	}
	return pl[:format.SignatureSize]
}
