package flush

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/merge/v2/write"
	"github.com/joshuapare/hivekit/internal/format"
)

// buildTestHive creates a minimal single-HBIN hive written to a temp file
// and returns the opened Hive. The caller must call h.Close() when done.
func buildTestHive(t testing.TB) *hive.Hive {
	t.Helper()

	path := filepath.Join(t.TempDir(), fmt.Sprintf("flush_test_%s.hive", t.Name()))

	const numHBINs = 1
	hbinDataSize := numHBINs * format.HBINAlignment
	totalSize := format.HeaderSize + hbinDataSize
	buf := make([]byte, totalSize)

	// REGF header.
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinDataSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// HBIN.
	hbinOff := format.HeaderSize
	copy(buf[hbinOff:], format.HBINSignature)
	format.PutU32(buf, hbinOff+format.HBINFileOffsetField, 0)
	format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(format.HBINAlignment))

	// Master free cell.
	freeCellOff := hbinOff + format.HBINHeaderSize
	freeCellSize := format.HBINAlignment - format.HBINHeaderSize
	format.PutI32(buf, freeCellOff, int32(freeCellSize))

	// Checksum.
	var cs uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		cs ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, cs)

	require.NoError(t, os.WriteFile(path, buf, 0o600))

	h, err := hive.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	return h
}

// buildTestAllocator creates a FastAllocator for the given hive with a nil
// dirty tracker (sufficient for flush unit tests that don't need disk sync).
func buildTestAllocator(t testing.TB, h *hive.Hive) *alloc.FastAllocator {
	t.Helper()
	fa, err := alloc.NewFast(h, nil, nil)
	require.NoError(t, err)
	return fa
}

// TestApply_WritesUpdates verifies that Apply writes update bytes at the
// correct offsets in the hive and that the checksum in the header is valid
// after the call.
func TestApply_WritesUpdates(t *testing.T) {
	h := buildTestHive(t)
	fa := buildTestAllocator(t, h)

	data := h.Bytes()

	// Pick an offset inside the HBIN data area (past the HBIN header).
	// We write into the free cell payload (after the 4-byte cell header).
	targetOff := int32(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)

	payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	updates := []write.InPlaceUpdate{
		{Offset: targetOff, Data: payload},
	}

	err := Apply(h, updates, fa, nil) // nil dirty tracker in tests
	require.NoError(t, err, "Apply should not return an error")

	// Verify the bytes were written.
	got := data[targetOff : int(targetOff)+len(payload)]
	require.Equal(t, payload, got, "Apply should write the update bytes at the correct offset")

	// Verify header checksum is valid after apply.
	checksum := ComputeFullChecksum(data[:508])
	stored := readU32(data, format.REGFCheckSumOffset)
	require.Equal(t, checksum, stored, "header checksum should be valid after Apply")
}

// TestApply_EmptyUpdates verifies that Apply with no updates still finalizes
// the header without error and leaves the checksum valid.
func TestApply_EmptyUpdates(t *testing.T) {
	h := buildTestHive(t)
	fa := buildTestAllocator(t, h)

	data := h.Bytes()

	err := Apply(h, nil, fa, nil) // nil dirty tracker in tests
	require.NoError(t, err, "Apply with no updates should not error")

	// Checksum must remain valid.
	checksum := ComputeFullChecksum(data[:508])
	stored := readU32(data, format.REGFCheckSumOffset)
	require.Equal(t, checksum, stored, "header checksum should be valid after Apply with no updates")
}
