package builder

import (
	"fmt"
	"os"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// createMinimalHive creates a new minimal valid hive file from scratch.
//
// The created hive contains:
//   - REGF header (4KB) with proper signature, version, sequences, timestamp, and checksum
//   - First HBIN (4KB) with header
//   - Root NK cell (empty name, no values/subkeys)
//   - Master free cell for remaining HBIN space
//
// Total file size: 8KB (4KB header + 4KB first HBIN)
//
// The returned *hive.Hive is opened read-write and ready for building operations.
func createMinimalHive(path string, version HiveVersion) (*hive.Hive, error) {
	// Calculate total file size: REGF header + one HBIN
	totalSize := format.HeaderSize + format.HBINAlignment // 4096 + 4096 = 8192
	buf := make([]byte, totalSize)

	// =========================================================================
	// REGF Header (0x0000 - 0x0FFF, 4KB)
	// =========================================================================

	// Signature: "regf" (0x0000)
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)

	// Sequence numbers: both start at 1 (clean hive)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)

	// Timestamp: current time in Windows FILETIME format
	nowFiletime := format.TimeToFiletime(time.Now())
	format.PutU64(buf, format.REGFTimeStampOffset, nowFiletime)

	// Version: convert HiveVersion to major.minor (major is always 1)
	const major = 1
	minor := version.toMinorVersion()
	format.PutU32(buf, format.REGFMajorVersionOffset, major)
	format.PutU32(buf, format.REGFMinorVersionOffset, minor)

	// Type: 0 (normal hive)
	format.PutU32(buf, format.REGFTypeOffset, 0)

	// Format: 1 (direct memory)
	format.PutU32(buf, format.REGFFormatOffset, 1)

	// Root cell offset: 0x20 (32 bytes into first HBIN, standard location)
	// This is relative to 0x1000 (start of data section)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)

	// Data size: total size of all HBINs (just one 4KB HBIN)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(format.HBINAlignment))

	// Cluster: 1 (default)
	format.PutU32(buf, format.REGFClusterOffset, 1)

	// File name: leave as zeros (optional field)

	// Checksum: calculated over first 508 bytes (after writing all above fields)
	checksum := calculateChecksum(buf)
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	// =========================================================================
	// HBIN Header (0x1000 - 0x101F, 32 bytes)
	// =========================================================================

	hbinOffset := format.HeaderSize // 0x1000

	// Signature: "hbin" (0x1000)
	copy(buf[hbinOffset+format.HBINSignatureOffset:], format.HBINSignature)

	// HBIN offset field: relative to 0x1000 (after REGF header)
	// First HBIN at file offset 0x1000 has relative offset 0
	// This is CRITICAL for hivexsh compatibility!
	format.PutU32(buf, hbinOffset+format.HBINFileOffsetField, uint32(hbinOffset-format.HeaderSize))

	// Size: total HBIN size including header (4096 bytes)
	format.PutU32(buf, hbinOffset+format.HBINSizeOffset, uint32(format.HBINAlignment))

	// Rest of HBIN header fields can remain zero

	// =========================================================================
	// Root NK Cell (0x1020 - 0x104B, 76 bytes minimum)
	// =========================================================================

	nkOffset := hbinOffset + format.HBINHeaderSize // 0x1020
	nkPayloadSize := format.NKFixedHeaderSize      // 76 bytes (0x4C) for empty name

	// Cell header: size field (negative for allocated)
	// Size includes 4-byte cell header itself
	cellSize := -int32(nkPayloadSize + format.CellHeaderSize)
	format.PutI32(buf, nkOffset, cellSize)

	// NK payload starts after 4-byte cell header
	nkPayload := nkOffset + format.CellHeaderSize

	// Signature: "nk"
	buf[nkPayload+format.NKSignatureOffset] = 'n'
	buf[nkPayload+format.NKSignatureOffset+1] = 'k'

	// Flags: KEY_COMP_NAME (0x20) - compressed (ASCII) name
	flags := uint16(format.NKFlagCompressedName)
	format.PutU16(buf, nkPayload+format.NKFlagsOffset, flags)

	// Last write time: current time
	format.PutU64(buf, nkPayload+format.NKLastWriteOffset, nowFiletime)

	// Access bits: 0 (not used in older versions)
	format.PutU32(buf, nkPayload+format.NKAccessBitsOffset, 0)

	// Parent offset: InvalidOffset (root has no parent)
	format.PutU32(buf, nkPayload+format.NKParentOffset, format.InvalidOffset)

	// Subkey count: 0
	format.PutU32(buf, nkPayload+format.NKSubkeyCountOffset, 0)

	// Volatile subkey count: 0
	format.PutU32(buf, nkPayload+format.NKVolSubkeyCountOffset, 0)

	// Subkey list offset: InvalidOffset (no subkeys)
	format.PutU32(buf, nkPayload+format.NKSubkeyListOffset, format.InvalidOffset)

	// Volatile subkey list offset: InvalidOffset
	format.PutU32(buf, nkPayload+format.NKVolSubkeyListOffset, format.InvalidOffset)

	// Value count: 0
	format.PutU32(buf, nkPayload+format.NKValueCountOffset, 0)

	// Value list offset: InvalidOffset (no values)
	format.PutU32(buf, nkPayload+format.NKValueListOffset, format.InvalidOffset)

	// Security offset: InvalidOffset (no security descriptor)
	format.PutU32(buf, nkPayload+format.NKSecurityOffset, format.InvalidOffset)

	// Class name offset: InvalidOffset (no class name)
	format.PutU32(buf, nkPayload+format.NKClassNameOffset, format.InvalidOffset)

	// Max lengths: all 0 (no subkeys/values yet)
	format.PutU32(buf, nkPayload+format.NKMaxNameLenOffset, 0)
	format.PutU32(buf, nkPayload+format.NKMaxClassLenOffset, 0)
	format.PutU32(buf, nkPayload+format.NKMaxValueNameOffset, 0)
	format.PutU32(buf, nkPayload+format.NKMaxValueDataOffset, 0)

	// Work var: 0
	format.PutU32(buf, nkPayload+format.NKWorkVarOffset, 0)

	// Name length: 0 (empty name for root)
	format.PutU16(buf, nkPayload+format.NKNameLenOffset, 0)

	// Class length: 0
	format.PutU16(buf, nkPayload+format.NKClassLenOffset, 0)

	// Name bytes: none (length = 0)

	// =========================================================================
	// Master Free Cell (remaining space in HBIN)
	// =========================================================================

	// Calculate offset and size of master free cell
	// NK cell total size (with header): 76 + 4 = 80 bytes
	// Align to 8-byte boundary
	nkTotalSize := nkPayloadSize + format.CellHeaderSize                                     // 80
	nkTotalAligned := (nkTotalSize + format.CellAlignment - 1) &^ (format.CellAlignment - 1) // 80 is already aligned

	freeCellOffset := nkOffset + nkTotalAligned
	hbinEnd := hbinOffset + format.HBINAlignment
	freeCellSize := hbinEnd - freeCellOffset

	// Write free cell header (positive size = free)
	format.PutI32(buf, freeCellOffset, int32(freeCellSize))

	// =========================================================================
	// Write file to disk
	// =========================================================================

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return nil, fmt.Errorf("write hive file: %w", err)
	}

	// =========================================================================
	// Open the newly created hive
	// =========================================================================

	h, err := hive.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open created hive: %w", err)
	}

	return h, nil
}

// calculateChecksum computes the REGF header checksum by XORing the first 127 DWORDs (508 bytes).
func calculateChecksum(data []byte) uint32 {
	if len(data) < format.REGFChecksumRegionLen {
		return 0
	}

	var checksum uint32

	// XOR the first 127 dwords (508 bytes)
	for i := range format.REGFChecksumDwords {
		offset := i * format.DWORDSize
		dword := format.ReadU32(data, offset)
		checksum ^= dword
	}

	return checksum
}
