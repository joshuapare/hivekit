package bigdata

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// createTestHive creates a minimal test hive with one large free cell.
func createTestHive(t testing.TB, path string, freeSpace int) {
	t.Helper()

	// Round free space to 8-byte alignment
	freeSpace = format.Align8(freeSpace)

	// Calculate HBIN size
	hbinSize := format.HBINHeaderSize + freeSpace
	// Round to 4KB alignment
	if hbinSize%format.HBINAlignment != 0 {
		hbinSize = ((hbinSize / format.HBINAlignment) + 1) * format.HBINAlignment
	}

	buf := make([]byte, format.HeaderSize+hbinSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write HBIN header
	hbinOff := format.HeaderSize
	copy(buf[hbinOff:hbinOff+4], format.HBINSignature)
	format.PutU32(buf, hbinOff+format.HBINFileOffsetField, uint32(hbinOff))
	format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(hbinSize))

	// Write one large free cell
	cellOff := hbinOff + format.HBINHeaderSize
	format.PutI32(buf, cellOff, int32(freeSpace)) // Positive size = free

	err := os.WriteFile(path, buf, 0644)
	if err != nil {
		t.Fatal(err)
	}
}
