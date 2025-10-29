package format

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// HBIN describes a hive bin. Each HBIN begins with a 0x20-byte header with the
// following structure (little-endian):
//
//	Offset  Size  Field
//	0x00    4     'h' 'b' 'i' 'n'
//	0x04    4     File offset of this HBIN (relative to start of hive)
//	0x08    4     Size of HBIN, multiple of 0x1000
//	0x0C    4     Reserved / unknown
//	...
//	0x1C    4     Next HBIN offset (often equal to size)
//
// We only retain the fields necessary to iterate over cells safely.
type HBIN struct {
	FileOffset uint32
	Size       uint32
}

// NextHBIN validates the HBIN header located at off within b and returns the
// header along with the offset of the subsequent HBIN.
func NextHBIN(b []byte, off int) (HBIN, int, error) {
	if off < 0 || off+HBINHeaderSize > len(b) {
		return HBIN{}, 0, fmt.Errorf("hbin: %w", ErrTruncated)
	}
	head := b[off : off+HBINHeaderSize]
	if !bytes.Equal(head[:4], HBINSignature) {
		return HBIN{}, 0, fmt.Errorf("hbin: %w", ErrSignatureMismatch)
	}
	fileOff := buf.U32LE(head[HBINFileOffsetField:])
	size := buf.U32LE(head[HBINSizeOffset:])
	if size == 0 || size%HBINAlignment != 0 {
		return HBIN{}, 0, fmt.Errorf("hbin: invalid size %d", size)
	}
	next := off + int(size)
	if next > len(b) {
		return HBIN{}, 0, fmt.Errorf("hbin: %w", ErrTruncated)
	}
	return HBIN{FileOffset: fileOff, Size: size}, next, nil
}
