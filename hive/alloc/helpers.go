package alloc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// defaultRootCellOffset is the standard offset for the root NK cell.
	defaultRootCellOffset = 0x20

	// nkCellMinSize is the minimum size for an NK cell (header + fixed fields).
	nkCellMinSize = 80

	// regfMinorVersion is the default minor version for REGF headers.
	regfMinorVersion = 5
)

// ============================================================================
// Test Helpers
// ============================================================================

// createHiveWithFreeCells creates a test hive with specified free cell sizes.
// For internal FastAlloc tests that don't need hivexsh validation.
func createHiveWithFreeCells(t testing.TB, path string, cellSizes []int) {
	t.Helper()
	createHiveWithFreeCellsInternal(t, path, cellSizes, false)
}

// createHiveWithFreeCellsAndRoot creates a test hive with specified free cell sizes
// and a proper NK root cell, suitable for hivexsh validation.
func createHiveWithFreeCellsAndRoot(t testing.TB, path string, cellSizes []int) {
	t.Helper()
	createHiveWithFreeCellsInternal(t, path, cellSizes, true)
}

// createHiveWithFreeCellsInternal is the internal implementation.
func createHiveWithFreeCellsInternal(
	t testing.TB,
	path string,
	cellSizes []int,
	includeRootNK bool,
) {
	t.Helper()

	// Calculate total HBIN size needed
	totalCellSize := 0
	if includeRootNK {
		totalCellSize += nkCellMinSize // NK cell size
	}
	for _, sz := range cellSizes {
		totalCellSize += sz
	}
	hbinSize := format.HBINHeaderSize + totalCellSize
	// Round to 4KB alignment
	if hbinSize%format.HBINAlignment != 0 {
		hbinSize = ((hbinSize / format.HBINAlignment) + 1) * format.HBINAlignment
	}

	buf := make([]byte, format.HeaderSize+hbinSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, defaultRootCellOffset)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, regfMinorVersion)

	// Write HBIN header
	hbinOff := format.HeaderSize
	copy(buf[hbinOff:hbinOff+4], format.HBINSignature)
	// HBIN offset field is relative to 0x1000 (after REGF header)
	// First HBIN at 0x1000 (absolute) has offset 0 (relative)
	format.PutU32(buf, hbinOff+format.HBINFileOffsetField, uint32(hbinOff-format.HeaderSize))
	format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(hbinSize))

	cellOff := hbinOff + format.HBINHeaderSize

	// Optionally write a minimal NK cell at offset 0x20 (root) for hivexsh validation
	if includeRootNK {
		nkCellSize := nkCellMinSize                     // NK cell size: 4 (header) + 0x4C (NK fixed header)
		format.PutI32(buf, cellOff, -int32(nkCellSize)) // Allocated cell (negative size)

		// Write NK structure (all fields must be initialized)
		nkPayload := cellOff + format.CellHeaderSize
		copy(buf[nkPayload:nkPayload+format.NKSignatureLen], format.NKSignature) // "nk"
		format.PutU32(
			buf,
			nkPayload+format.NKParentOffset,
			format.InvalidOffset,
		) // parent: none
		format.PutU32(
			buf,
			nkPayload+format.NKSubkeyListOffset,
			format.InvalidOffset,
		) // subkey list: none
		format.PutU32(
			buf,
			nkPayload+format.NKVolSubkeyListOffset,
			format.InvalidOffset,
		) // volatile subkey list: none
		format.PutU32(
			buf,
			nkPayload+format.NKValueListOffset,
			format.InvalidOffset,
		) // value list: none
		format.PutU32(
			buf,
			nkPayload+format.NKSecurityOffset,
			format.InvalidOffset,
		) // security: none
		format.PutU32(
			buf,
			nkPayload+format.NKClassNameOffset,
			format.InvalidOffset,
		) // class name: none
		// All other fields default to 0 (already zeroed by make())

		cellOff += nkCellSize
	}

	// Write free cells
	for _, sz := range cellSizes {
		format.PutI32(buf, cellOff, int32(sz)) // Positive size = free
		cellOff += sz
	}

	// Fill remaining HBIN space with one large free cell to avoid uninitialized data
	// that would cause hivexsh to fail
	hbinEnd := hbinOff + hbinSize
	if cellOff < hbinEnd {
		remainingSize := hbinEnd - cellOff
		if remainingSize >= format.CellAlignment { // Minimum cell size
			format.PutI32(buf, cellOff, int32(remainingSize)) // Positive = free
		}
	}

	// Calculate and write header checksum (hivexsh validates this)
	// Checksum = XOR of first 508 dwords
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += format.CellHeaderSize {
		checksum ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	err := os.WriteFile(path, buf, 0o600)
	require.NoError(t, err)
}

// cellAbsSize returns the absolute size of a cell (removes sign bit).
func cellAbsSize(buf []byte, off int) int {
	raw := format.ReadI32(buf, off)
	if raw < 0 {
		raw = -raw
	}
	return int(raw)
}

// Note: format.PutU32 and format.PutI32 are defined in fastalloc.go (using unsafe.Pointer)
