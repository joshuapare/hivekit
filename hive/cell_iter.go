package hive

import (
	"fmt"
	"io"

	"github.com/joshuapare/hivekit/internal/format"
)

type CellIterator struct {
	hbin *HBIN
	off  int
	done bool
}

func (h *HBIN) Cells() *CellIterator {
	return &CellIterator{
		hbin: h,
		off:  int(format.HBINHeaderSize),
	}
}

func (it *CellIterator) Next() (Cell, error) {
	if it.done {
		return Cell{}, io.EOF
	}

	buf := it.hbin.Data

	// reached / passed HBIN
	if it.off >= len(buf) {
		it.done = true
		return Cell{}, io.EOF
	}

	cell, err := newCellAt(buf, it.off)
	if err != nil {
		it.done = true
		return Cell{}, err
	}

	size := cell.SizeAbs()
	if size == 0 {
		it.done = true
		// If this is the first cell, it's corruption. Otherwise, it's end of cells (padding).
		if it.off == int(format.HBINHeaderSize) {
			return Cell{}, fmt.Errorf("hive: cell at %d has zero size", it.off)
		}
		return Cell{}, io.EOF
	}

	nextOff := it.off + size
	if rem := nextOff % format.CellAlignment; rem != 0 {
		nextOff += format.CellAlignment - rem
	}

	// if this cell itself overflows → corruption or truncation
	if it.off+size > len(buf) {
		it.done = true
		// If this is the first cell, it's corruption. Otherwise, it's truncation (EOF).
		if it.off == int(format.HBINHeaderSize) {
			return Cell{}, fmt.Errorf("hive: cell at %d exceeds HBIN (len=%d)", it.off, len(buf))
		}
		return Cell{}, io.EOF
	}

	// if the next offset is beyond HBIN, this was the last cell → mark done
	if nextOff > len(buf) {
		it.done = true
	} else {
		it.off = nextOff
	}

	return cell, nil
}
