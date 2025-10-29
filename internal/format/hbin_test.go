package format

import (
	"encoding/binary"
	"testing"
)

func TestNextHBIN(t *testing.T) {
	buf := make([]byte, HeaderSize+HBINAlignment*2)
	off := HeaderSize
	copy(buf[off:], HBINSignature)
	binary.LittleEndian.PutUint32(buf[off+HBINFileOffsetField:], uint32(off))
	binary.LittleEndian.PutUint32(buf[off+HBINSizeOffset:], HBINAlignment)

	h, next, err := NextHBIN(buf, off)
	if err != nil {
		t.Fatalf("NextHBIN: %v", err)
	}
	if h.FileOffset != uint32(off) || h.Size != HBINAlignment {
		t.Fatalf("unexpected HBIN: %+v", h)
	}
	if next != off+HBINAlignment {
		t.Fatalf("next offset mismatch: %d", next)
	}
}

func TestNextHBINErrors(t *testing.T) {
	buf := make([]byte, HeaderSize+HBINHeaderSize)
	if _, _, err := NextHBIN(buf, HeaderSize); err == nil {
		t.Fatalf("expected signature error")
	}
	copy(buf[HeaderSize:], HBINSignature)
	binary.LittleEndian.PutUint32(buf[HeaderSize+HBINSizeOffset:], 123) // not aligned
	if _, _, err := NextHBIN(buf, HeaderSize); err == nil {
		t.Fatalf("expected size error")
	}
}
