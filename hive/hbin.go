package hive

import (
	"bytes"
	"fmt"
	"io"
	"math"

	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// maxHiveSize is the maximum size of a registry hive file (4GB).
	// Registry hives use uint32 offsets, limiting them to 4GB.
	maxHiveSize = math.MaxUint32
)

type HBIN struct {
	Data   []byte // full HBIN bytes (header + payload), zero-copy view
	Offset uint32 // absolute file offset of this HBIN (must be 0x1000-aligned)
	Size   uint32 // total size of this HBIN (must be 0x1000-aligned)
}

type HBINIterator struct {
	h     *Hive
	next  uint32 // absolute offset of the next HBIN to try
	limit uint32 // hard stop (usually file length); 0 => len(h.data)
	done  bool
}

// NewHBINIterator returns an iterator positioned at the first HBIN
// (immediately after the 4KiB REGF base block).
func (h *Hive) NewHBINIterator() HBINIterator {
	return HBINIterator{
		h:     h,
		next:  uint32(format.HeaderSize), // HBINs start at 0x1000
		limit: 0,                         // 0 means use len(data) on first Next() call
	}
}

// Next returns the next HBIN or io.EOF.
// Non-"hbin" bytes encountered at a 0x1000 boundary are treated as end-of-bins
// (Windows hives may have zero padding after the last HBIN).
func (it *HBINIterator) Next() (HBIN, error) {
	if it.done {
		return HBIN{}, io.EOF
	}
	data := it.h.data
	if it.limit == 0 {
		dataLen := len(data)
		// Validate conversion safety: hive files are limited to 4GB by format spec.
		// Registry uses uint32 offsets, so files larger than 4GB are malformed.
		if dataLen > maxHiveSize {
			return HBIN{}, fmt.Errorf("hive: file too large (%d bytes, max 4GB)", dataLen)
		}
		// Safe conversion: validated dataLen <= maxHiveSize (0xFFFFFFFF)
		it.limit = uint32(dataLen)
	}

	// No room for an HBIN header.
	if it.next > it.limit || it.next+uint32(format.HBINHeaderSize) > it.limit {
		it.done = true
		return HBIN{}, io.EOF
	}

	// Treat non-"hbin" as end-of-stream (common trailing padding).
	if string(data[it.next:it.next+4]) != string(format.HBINSignature) {
		it.done = true
		return HBIN{}, io.EOF
	}

	// Delegate to the strict single-HBIN parser (ensures size/alignment/bounds).
	hb, err := ParseHBINAt(data, it.next)
	if err != nil {
		// Surface the precise error (corruption, overflow, etc.).
		it.done = true
		return HBIN{}, err
	}

	// Advance to the next aligned HBIN.
	next := it.next + hb.Size
	if next >= it.limit {
		it.done = true
	} else {
		it.next = next
	}

	return hb, nil
}

// fast, zero-alloc check.
func isHBIN(b []byte) bool {
	// caller must have ensured len(b) >= format.HeaderSize, but be defensive
	const off = format.HBINSignatureOffset
	const n = format.HBINSignatureSize
	if len(b) < off+n {
		return false
	}
	return bytes.Equal(b[off:off+n], format.HBINSignature)
}

// ParseHBINAt parses one HBIN at absolute file offset `abs` and returns
// a zero-copy view over the backing hive buffer.
func ParseHBINAt(hive []byte, abs uint32) (HBIN, error) {
	hiveLen := len(hive)
	// Validate conversion safety: hive files are limited to 4GB by format spec.
	// Registry uses uint32 offsets, so files larger than 4GB are malformed.
	if hiveLen > maxHiveSize {
		return HBIN{}, fmt.Errorf("hive: file too large (%d bytes, max 4GB)", hiveLen)
	}
	// Safe conversion: validated hiveLen <= maxHiveSize (0xFFFFFFFF)
	end := uint32(hiveLen)

	// Must have room for header.
	if abs+uint32(format.HBINHeaderSize) > end {
		return HBIN{}, fmt.Errorf("hive: HBIN header truncated at 0x%X", abs)
	}

	hdr := hive[abs : abs+uint32(format.HBINHeaderSize)]

	// Signature "hbin".
	if !isHBIN(hdr[0:4]) {
		return HBIN{}, fmt.Errorf("hive: HBIN bad signature at 0x%X", abs)
	}

	// Offset echo at +0x04 (per spec it should match our absolute offset).
	// echo := format.ReadU32(hdr, format.HBINOffsetEchoOffset)
	//
	// TODO: when we hook up diagnostic reporting, we can use this.
	// if echo != abs {
	// 	// Not fatal in the wild, but extremely useful for corruption detection.
	// 	// return HBIN{}, fmt.Errorf("hive: HBIN offset echo 0x%X != actual 0x%X", echo, abs)
	// }

	// Total HBIN size at +0x08.
	sz := format.ReadU32(hdr, format.HBINSizeOffset)
	if sz == 0 {
		return HBIN{}, fmt.Errorf("hive: HBIN at 0x%X has size 0", abs)
	}

	// Start/size alignment: bins are 4KiB-aligned.
	if abs%uint32(format.HeaderSize) != 0 {
		return HBIN{}, fmt.Errorf("hive: HBIN start 0x%X not 4KiB-aligned", abs)
	}
	if sz%uint32(format.HeaderSize) != 0 {
		return HBIN{}, fmt.Errorf("hive: HBIN size 0x%X not 4KiB-aligned", sz)
	}

	// Bounds.
	hend := abs + sz
	if hend > end {
		return HBIN{}, fmt.Errorf(
			"hive: HBIN at 0x%X (size 0x%X) exceeds file (0x%X)",
			abs,
			sz,
			end,
		)
	}

	return HBIN{
		Data:   hive[abs:hend], // zero-copy slice
		Offset: abs,
		Size:   sz,
	}, nil
}

/* ---------- Zero-copy HBIN helpers ---------- */

// Header returns the fixed-size HBIN header bytes (zero-copy).
func (h *HBIN) Header() []byte { return h.Data[:format.HBINHeaderSize] }

// Payload returns the HBIN payload region where cells reside (zero-copy).
func (h *HBIN) Payload() []byte { return h.Data[format.HBINHeaderSize:] }

// FirstCellAbs returns the absolute file offset of the first cell in this HBIN.
func (h *HBIN) FirstCellAbs() uint32 { return h.Offset + uint32(format.HBINHeaderSize) }

// EndAbs returns the absolute file offset right after this HBIN.
func (h *HBIN) EndAbs() uint32 { return h.Offset + h.Size }
