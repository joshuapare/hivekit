package hive

import (
	"testing"
	"time"

	"github.com/joshuapare/hivekit/internal/format"
)

// CellSpec describes ONE cell we want to place into the HBIN for tests.
type CellSpec struct {
	// Allocated = true  → write NEGATIVE size (in-use)
	// Allocated = false → write POSITIVE size (free)
	Allocated bool

	// Size is the TOTAL cell size in bytes, INCLUDING the 4-byte cell header.
	// This is the number that will be written (negated if Allocated=true).
	Size int

	// Payload is what goes right after the 4-byte size header.
	// If len(payload) < Size-4, we pad with zeros.
	// If len(payload) > Size-4, we truncate to fit.
	Payload []byte
}

// buildHBINFromSpec builds a single HBIN using the Windows layout:
//
//	Offset  Size  Field
//	--------------------------------
//	0x00    4     Signature "hbin"
//	0x04    4     FileOffset (where this bin lives in the file)
//	0x08    4     Size (total HBIN size, usually 0x1000 for tests)
//	0x0C    8     Reserved
//	0x14    8     TimeStamp (we'll just zero or fake it)
//	0x1C    4     Spare
//
// Cells start at 0x20 and are 8-byte aligned.
//
// For tests we default to a 4 KiB HBIN.
func buildHBINFromSpec(t *testing.T, cells []CellSpec) []byte {
	t.Helper()

	const hbinSize = format.HBINAlignment  // 0x1000 = 4096
	const fileOffset = format.HiveDataBase // Always 4096

	hb := make([]byte, hbinSize)

	// Write HBIN header per your spec
	// +0x000 Signature        : Uint4B
	copy(hb[0:4], format.HBINSignature)

	// +0x004 FileOffset       : Uint4B
	format.PutU32(hb, format.HBINFileOffsetField, fileOffset)

	// +0x008 Size             : Uint4B
	format.PutU32(hb, format.HBINSizeOffset, hbinSize)

	// +0x00c Reserved1        : [2] Uint4B → leave zeros

	// +0x014 TimeStamp        : _LARGE_INTEGER
	// we can put a fake FILETIME if we want determinism
	// let's just write current unix nanos truncated, purely for realism
	ft := fakeFILETIME(time.Now())
	format.PutU32(hb, 0x14, uint32(ft))       // low
	format.PutU32(hb, 0x14+4, uint32(ft>>32)) // high

	// +0x01c Spare            : Uint4B → zero

	// Now write cells right after 0x20
	off := int(format.HBINHeaderSize) // 0x20

	for _, cs := range cells {
		if cs.Size < format.CellHeaderSize {
			t.Fatalf("cell size %d too small, must be >= %d", cs.Size, format.CellHeaderSize)
		}

		// If the cell would overflow the HBIN, stop writing.
		if off+cs.Size > len(hb) {
			// we could fail hard, but for tests it's useful to just stop
			break
		}

		// write size (allocated → negative)
		if cs.Allocated {
			format.PutI32(hb, off, int32(-cs.Size))
		} else {
			format.PutI32(hb, off, int32(cs.Size))
		}

		// write payload
		payloadStart := off + format.CellHeaderSize
		maxPayload := cs.Size - format.CellHeaderSize

		if len(cs.Payload) > 0 {
			n := min(len(cs.Payload), maxPayload)
			copy(hb[payloadStart:payloadStart+n], cs.Payload[:n])
		}

		// advance
		next := off + cs.Size

		// align to 8 bytes
		if rem := next % format.CellAlignment; rem != 0 {
			next += format.CellAlignment - rem
		}

		off = next
		// if we filled the HBIN, stop
		if off >= len(hb) {
			break
		}
	}

	return hb
}

// fakeFILETIME just gives us something deterministic-ish for tests.
// FILETIME = 100-ns intervals since Jan 1, 1601. We'll just return unix * 10^7.
func fakeFILETIME(ti time.Time) uint64 {
	// doesn't have to be exact for tests
	const winToUnixSeconds = 11644473600 // seconds between 1601 and 1970
	secs := uint64(ti.Unix() + winToUnixSeconds)
	return secs * 10_000_000
}
