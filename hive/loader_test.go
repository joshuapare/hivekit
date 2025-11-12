package hive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// writeMinimalHive creates a *real-looking* hive:
//
// 0x0000 - 0x0FFF : REGF / base block
// 0x1000 - 0x1FFF : 1 HBIN (minimal header, rest zero)
//
// Header says: data size = 0x1000 (one HBIN), so total hive length = 0x2000.
// We actually write 0x2000 bytes to disk, so ValidateSanity should pass.
func writeMinimalHive(t *testing.T, path string) {
	t.Helper()

	// 1) make full 8 KiB file
	//    0x0000..0x0FFF = REGF
	//    0x1000..0x1FFF = HBIN
	buf := make([]byte, format.HeaderSize+format.HBINAlignment) // 4096 + 4096 = 8192

	// ------------------------------------------------------------------
	// REGF (base block) at 0x0000
	// ------------------------------------------------------------------
	// magic
	copy(
		buf[format.REGFSignatureOffset:format.REGFSignatureOffset+format.REGFSignatureSize],
		format.REGFSignature,
	)

	// sequence numbers
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)

	// root CELL offset (relative to first HBIN at 0x1000).
	// Real hives often put NK at 0x20 inside the first HBIN.
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)

	// data size = exactly one HBIN (4096)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(format.HBINAlignment))

	// versions
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// ------------------------------------------------------------------
	// HBIN at 0x1000
	// ------------------------------------------------------------------
	hbinOff := format.HeaderSize // 0x1000
	hbin := buf[hbinOff : hbinOff+format.HBINHeaderSize]

	// "hbin"
	copy(hbin[0:4], format.HBINSignature)

	// HBIN "file offset" field (at 0x04) = where this HBIN starts in the file
	format.PutU32(buf, hbinOff+format.HBINFileOffsetField, uint32(hbinOff))

	// HBIN size (at 0x08) = full 4 KiB
	format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(format.HBINAlignment))

	// rest of HBIN can stay zero

	err := os.WriteFile(path, buf, 0o644)
	require.NoError(t, err)
}

func TestOpen_MinimalHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "minimal.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	// file is 8 KiB
	require.Equal(t, int64(format.HeaderSize+format.HBINAlignment), h.Size())

	// ABSOLUTE root = 0x1000 (first HBIN) + 0x20 (relative NK)
	require.Equal(t, uint32(format.HeaderSize+0x20), h.RootOffset())

	gotMagic := string(h.Bytes()[0:4])
	require.Equal(t, "regf", gotMagic)
	require.Positive(t, h.FD())
}

func TestOpen_InvalidMagic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.hiv")

	buf := make([]byte, format.HeaderSize)
	copy(buf, []byte("xxxx"))
	err := os.WriteFile(p, buf, 0o644)
	require.NoError(t, err)

	h, err := Open(p)
	require.Error(t, err)
	if h != nil {
		_ = h.Close()
	}
}
