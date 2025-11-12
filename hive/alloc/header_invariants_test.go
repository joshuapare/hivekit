//go:build linux || darwin

package alloc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/tx"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_Header_Blocks_TracksGrowth verifies the header's blocks field updates after growth
// This is Test #2 from DEBUG.md: "Header_Blocks_TracksGrowth".
func Test_Header_Blocks_TracksGrowth(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	// Create minimal hive (1 page)
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Initial state: 1 page = 4KB
	data := h.Bytes()
	initialBlocks := getU32(data, format.REGFDataSizeOffset)
	require.Equal(t, uint32(4096), initialBlocks, "Initial data size should be 4096")

	// Grow by 1 page
	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	data = h.Bytes()
	afterFirstGrow := getU32(data, format.REGFDataSizeOffset)
	require.Equal(t, uint32(8192), afterFirstGrow, "After first grow, data size should be 8192")

	// Grow by 2 more pages
	err = fa.GrowByPages(2) // Add 8KB HBIN
	require.NoError(t, err)

	data = h.Bytes()
	afterSecondGrow := getU32(data, format.REGFDataSizeOffset)
	require.Equal(t, uint32(16384), afterSecondGrow, "After second grow, data size should be 16384")

	t.Logf("Header blocks field tracked growth: 4096 → 8192 → 16384")
}

// Test_EOF_Exact verifies file size matches header blocks field
// This is Test #18 from DEBUG.md: "EOF_Exact".
func Test_EOF_Exact(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow and verify EOF matches header
	err = fa.GrowByPages(2) // Add 8KB HBIN
	require.NoError(t, err)

	data := h.Bytes()
	dataSize := int(getU32(data, format.REGFDataSizeOffset))
	expectedFileSize := format.HeaderSize + dataSize
	actualFileSize := len(data)

	require.Equal(t, expectedFileSize, actualFileSize,
		"File size (0x%X) should equal header (0x1000) + data size (0x%X)",
		actualFileSize, dataSize)

	t.Logf("File size 0x%X matches header (0x1000) + data size (0x%X)",
		actualFileSize, dataSize)
}

// Test_Header_SequenceNumbers_IncrementOnGrow verifies sequence numbers increment after Grow()
// Per REGF spec: Primary Sequence (0x04) and Secondary Sequence (0x08) must increment on writes
// Consistency invariant: When Seq1 == Seq2, hive is "clean" (no pending writes)
//
// ARCHITECTURAL NOTE: Grow() itself does NOT update sequences. The tx.Manager owns
// sequence protocol state. This test wraps Grow() in a transaction to verify the
// complete transaction lifecycle updates sequences correctly.
func Test_Header_SequenceNumbers_IncrementOnGrow(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Create transaction manager
	txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

	// Record initial sequence numbers (should be equal - consistent state)
	data := h.Bytes()
	initialSeq1 := getU32(data, format.REGFPrimarySeqOffset)
	initialSeq2 := getU32(data, format.REGFSecondarySeqOffset)

	t.Logf("Initial sequences: Seq1=0x%X, Seq2=0x%X", initialSeq1, initialSeq2)
	require.Equal(t, initialSeq1, initialSeq2, "Initial sequences should be equal (clean state)")

	// Begin transaction, perform Grow(), and commit
	err = txMgr.Begin()
	require.NoError(t, err)

	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	err = txMgr.Commit()
	require.NoError(t, err)

	// Check sequence numbers after Grow()
	data = h.Bytes()
	afterSeq1 := getU32(data, format.REGFPrimarySeqOffset)
	afterSeq2 := getU32(data, format.REGFSecondarySeqOffset)

	t.Logf("After Grow: Seq1=0x%X, Seq2=0x%X", afterSeq1, afterSeq2)

	// Verify both sequences incremented
	require.Greater(t, afterSeq1, initialSeq1,
		"Primary sequence (0x04) must increment after Grow() operation\n"+
			"Initial: 0x%X, After: 0x%X\n"+
			"This is required per REGF spec for write tracking",
		initialSeq1, afterSeq1)

	require.Greater(t, afterSeq2, initialSeq2,
		"Secondary sequence (0x08) must increment after Grow() operation\n"+
			"Initial: 0x%X, After: 0x%X\n"+
			"This is required per REGF spec for write tracking",
		initialSeq2, afterSeq2)

	// Verify consistency maintained (Seq1 == Seq2 after complete operation)
	require.Equal(t, afterSeq1, afterSeq2,
		"After Grow() completes, sequences must be equal (clean state)\n"+
			"Seq1=0x%X, Seq2=0x%X\n"+
			"If Seq1 != Seq2, Windows will mark hive as needing recovery",
		afterSeq1, afterSeq2)

	t.Logf("Sequence numbers incremented correctly: %d → %d", initialSeq1, afterSeq1)
}

// Test_Header_Timestamp_UpdatesOnGrow verifies Last Written timestamp updates after Grow()
// Per REGF spec: Last Written Timestamp (0x0C) is 8-byte FILETIME that must update on modifications
//
// ARCHITECTURAL NOTE: Grow() itself does NOT update timestamp. The tx.Manager owns
// timestamp state and updates it during Commit(). This test wraps Grow() in a transaction
// to verify the complete transaction lifecycle updates timestamp correctly.
func Test_Header_Timestamp_UpdatesOnGrow(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Create transaction manager
	txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

	// Record initial timestamp
	data := h.Bytes()
	initialTimestamp := getU64(data, format.REGFTimeStampOffset)

	t.Logf("Initial timestamp: 0x%016X", initialTimestamp)

	// Wait a moment to ensure timestamp will be different
	// (Windows FILETIME has 100ns resolution, but we'll wait 1ms to be safe)
	// Note: Timestamp update happens in tx.Commit(), not in Grow()
	time.Sleep(2 * time.Millisecond)

	// Begin transaction, perform Grow(), and commit
	err = txMgr.Begin()
	require.NoError(t, err)

	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	err = txMgr.Commit()
	require.NoError(t, err)

	// Check timestamp after Grow()
	data = h.Bytes()
	afterTimestamp := getU64(data, format.REGFTimeStampOffset)

	t.Logf("After Grow timestamp: 0x%016X", afterTimestamp)

	// Verify timestamp was updated
	require.Greater(t, afterTimestamp, initialTimestamp,
		"Last Written Timestamp (0x0C) must update after Grow() operation\n"+
			"Initial: 0x%016X, After: 0x%016X\n"+
			"This is required per REGF spec for change tracking and forensics",
		initialTimestamp, afterTimestamp)

	t.Logf("Timestamp updated correctly: 0x%016X → 0x%016X", initialTimestamp, afterTimestamp)
}

// createMinimalHive creates a minimal valid hive with specified HBIN size.
func createMinimalHive(t testing.TB, path string, hbinSize int) {
	t.Helper()

	// Align to 4KB
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
	format.PutU32(buf, hbinOff+format.HBINFileOffsetField, 0)
	format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(hbinSize))

	// Write a minimal NK cell at offset 0x20 (root)
	cellOff := hbinOff + format.HBINHeaderSize
	// NK cell size: 4 (cell header) + 0x4C (NK fixed header) = 0x50 (80 bytes)
	cellSize := 80
	format.PutI32(buf, cellOff, -int32(cellSize)) // Allocated cell (negative size)

	// Write NK structure (all fields must be initialized for hivexsh)
	nkPayload := cellOff + 4
	copy(buf[nkPayload:nkPayload+2], format.NKSignature)                   // "nk"
	format.PutU16(buf, nkPayload+format.NKFlagsOffset, 0)                  // flags: 0
	format.PutU64(buf, nkPayload+format.NKLastWriteOffset, 0)              // timestamp: 0
	format.PutU32(buf, nkPayload+format.NKAccessBitsOffset, 0)             // access bits: 0
	format.PutU32(buf, nkPayload+format.NKParentOffset, 0xFFFFFFFF)        // parent: none
	format.PutU32(buf, nkPayload+format.NKSubkeyCountOffset, 0)            // subkey count: 0
	format.PutU32(buf, nkPayload+format.NKVolSubkeyCountOffset, 0)         // volatile subkey count: 0
	format.PutU32(buf, nkPayload+format.NKSubkeyListOffset, 0xFFFFFFFF)    // subkey list: none
	format.PutU32(buf, nkPayload+format.NKVolSubkeyListOffset, 0xFFFFFFFF) // volatile subkey list: none
	format.PutU32(buf, nkPayload+format.NKValueCountOffset, 0)             // value count: 0
	format.PutU32(buf, nkPayload+format.NKValueListOffset, 0xFFFFFFFF)     // value list: none
	format.PutU32(buf, nkPayload+format.NKSecurityOffset, 0xFFFFFFFF)      // security: none
	format.PutU32(buf, nkPayload+format.NKClassNameOffset, 0xFFFFFFFF)     // class name: none
	format.PutU32(buf, nkPayload+format.NKMaxNameLenOffset, 0)             // max name len: 0
	format.PutU32(buf, nkPayload+format.NKMaxClassLenOffset, 0)            // max class len: 0
	format.PutU32(buf, nkPayload+format.NKMaxValueNameOffset, 0)           // max value name len: 0
	format.PutU32(buf, nkPayload+format.NKMaxValueDataOffset, 0)           // max value data len: 0
	format.PutU32(buf, nkPayload+format.NKWorkVarOffset, 0)                // work var: 0
	format.PutU16(buf, nkPayload+format.NKNameLenOffset, 0)                // name len: 0 (root has no name)
	format.PutU16(buf, nkPayload+format.NKClassLenOffset, 0)               // class len: 0

	// Write a free cell for the remaining space
	nextCellOff := cellOff + cellSize
	freeSize := hbinSize - format.HBINHeaderSize - cellSize
	if freeSize > format.CellHeaderSize {
		format.PutI32(buf, nextCellOff, int32(freeSize))
	}

	// Calculate and write header checksum (hivexsh validates this)
	// Checksum = XOR of first 508 dwords (0x1FC bytes)
	var checksum uint32
	for i := 0; i < 0x1FC; i += 4 {
		checksum ^= getU32(buf, i)
	}
	format.PutU32(buf, 0x1FC, checksum)

	// Write to file
	err := os.WriteFile(path, buf, 0644)
	require.NoError(t, err)
}

// getU64 reads a uint64 in little-endian format from data at given offset.
func getU64(data []byte, off int) uint64 {
	return uint64(data[off]) |
		uint64(data[off+1])<<8 |
		uint64(data[off+2])<<16 |
		uint64(data[off+3])<<24 |
		uint64(data[off+4])<<32 |
		uint64(data[off+5])<<40 |
		uint64(data[off+6])<<48 |
		uint64(data[off+7])<<56
}
