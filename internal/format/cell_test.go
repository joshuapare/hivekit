package format

import (
	"encoding/binary"
	"testing"
)

func TestNextCellAllocated(t *testing.T) {
	buf := make([]byte, HeaderSize+HBINAlignment)
	hOff := HeaderSize
	copy(buf[hOff:], HBINSignature)
	binary.LittleEndian.PutUint32(buf[hOff+0x04:], uint32(hOff))
	binary.LittleEndian.PutUint32(buf[hOff+0x08:], HBINAlignment)

	cellOff := hOff + HBINHeaderSize
	size := 0x30
	binary.LittleEndian.PutUint32(buf[cellOff:], uint32(-size))
	buf[cellOff+4] = 'n'
	buf[cellOff+5] = 'k'

	h := HBIN{FileOffset: uint32(hOff), Size: HBINAlignment}
	cell, next, err := NextCell(buf, h, cellOff)
	if err != nil {
		t.Fatalf("NextCell: %v", err)
	}
	if cell.Free {
		t.Fatalf("expected allocated cell")
	}
	if cell.Size != size || cell.Tag != [2]byte{'n', 'k'} {
		t.Fatalf("unexpected cell: %+v", cell)
	}
	if next != cellOff+size {
		t.Fatalf("next offset mismatch: %d", next)
	}
}

func TestNextCellFree(t *testing.T) {
	buf := make([]byte, HeaderSize+HBINAlignment)
	hOff := HeaderSize
	copy(buf[hOff:], HBINSignature)
	binary.LittleEndian.PutUint32(buf[hOff+0x04:], uint32(hOff))
	binary.LittleEndian.PutUint32(buf[hOff+0x08:], HBINAlignment)

	cellOff := hOff + HBINHeaderSize
	size := 0x20
	binary.LittleEndian.PutUint32(buf[cellOff:], uint32(size))

	h := HBIN{FileOffset: uint32(hOff), Size: HBINAlignment}
	cell, _, err := NextCell(buf, h, cellOff)
	if err != nil {
		t.Fatalf("NextCell: %v", err)
	}
	if !cell.Free {
		t.Fatalf("expected free cell")
	}
}
