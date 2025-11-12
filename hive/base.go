package hive

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// dwordBitShift is the bit shift to convert index to byte offset for DWORD arrays (i << 2 == i * 4).
	dwordBitShift = 2

	// regfChecksumAllOnes is the special checksum value when XOR results in all 1s.
	regfChecksumAllOnes = 0xFFFFFFFF

	// regfChecksumAllOnesReplacement is the replacement value for all-ones checksum.
	regfChecksumAllOnesReplacement = 0xFFFFFFFE

	// regfChecksumAllZeros is the special checksum value when XOR results in all 0s.
	regfChecksumAllZeros = 0x00000000

	// regfChecksumAllZerosReplacement is the replacement value for all-zeros checksum.
	regfChecksumAllZerosReplacement = 0x00000001
)

// BaseBlock represents the 4KiB REGF / HBASE_BLOCK header at the start of the hive.
// Zero-copy: all accessors read directly from b.raw.
type BaseBlock struct {
	raw []byte // len >= 4096
}

// isREGF is a fast, zero-alloc check for REGF signature.
func isREGF(b []byte) bool {
	// caller must have ensured len(b) >= format.HeaderSize, but be defensive
	const off = format.REGFSignatureOffset
	const n = format.REGFSignatureSize
	if len(b) < off+n {
		return false
	}
	return bytes.Equal(b[off:off+n], format.REGFSignature)
}

// ParseBaseBlock validates signature and returns a header view.
func ParseBaseBlock(b []byte) (*BaseBlock, error) {
	if len(b) < format.HeaderSize {
		return nil, fmt.Errorf("hive: file too small for REGF header (%d)", len(b))
	}
	if !isREGF(b) {
		return nil, errors.New("hive: bad REGF signature")
	}

	return &BaseBlock{raw: b[:format.HeaderSize]}, nil
}

// ---- Primitive field readers (no alloc) ----

// Raw returns the raw bytes of the base block.
func (bb *BaseBlock) Raw() []byte { return bb.raw }

// Signature returns the "regf" signature bytes.
func (bb *BaseBlock) Signature() []byte {
	return bb.raw[format.REGFSignatureOffset : format.REGFSignatureOffset+format.REGFSignatureSize]
}

// Sequence1 returns the primary sequence number.
func (bb *BaseBlock) Sequence1() uint32 { return format.ReadU32(bb.raw, format.REGFPrimarySeqOffset) }

// Sequence2 returns the secondary sequence number.
func (bb *BaseBlock) Sequence2() uint32 { return format.ReadU32(bb.raw, format.REGFSecondarySeqOffset) }

// IsClean returns true if Sequence1 equals Sequence2, indicating no pending writes.
func (bb *BaseBlock) IsClean() bool { return bb.Sequence1() == bb.Sequence2() }

// TimeStampFILETIME returns the header FILETIME at 0x0C, raw 64-bit (100ns since 1601).
func (bb *BaseBlock) TimeStampFILETIME() uint64 {
	return format.ReadU64(bb.raw, format.REGFTimeStampOffset)
}

// Major returns the major version number.
func (bb *BaseBlock) Major() uint32 { return format.ReadU32(bb.raw, format.REGFMajorVersionOffset) }

// Minor returns the minor version number.
func (bb *BaseBlock) Minor() uint32 { return format.ReadU32(bb.raw, format.REGFMinorVersionOffset) }

// VersionU32 returns the version as a uint32 using kernel format: Minor + (Major*0x1000) - 0x1000 for Major>=1.
func (bb *BaseBlock) VersionU32() uint32 {
	// Kernel stores: Minor + (Major*0x1000) - 0x1000 for Major>=1
	major, minor := bb.Major(), bb.Minor()
	if major == 0 {
		// observed: accepted in recovery path; keep raw interpretation simple
		return minor
	}
	return minor + (major * format.HBINAlignment) - format.HBINAlignment
}

// Type returns the hive type field.
func (bb *BaseBlock) Type() uint32 { return format.ReadU32(bb.raw, format.REGFTypeOffset) }

// Format returns the hive format field.
func (bb *BaseBlock) Format() uint32 { return format.ReadU32(bb.raw, format.REGFFormatOffset) }

// RootCellOffset returns the offset of the root cell.
func (bb *BaseBlock) RootCellOffset() uint32 {
	return format.ReadU32(bb.raw, format.REGFRootCellOffset)
}

// DataSize returns the size of the hive data area.
func (bb *BaseBlock) DataSize() uint32 { return format.ReadU32(bb.raw, format.REGFDataSizeOffset) }

// Cluster returns the cluster field.
func (bb *BaseBlock) Cluster() uint32 { return format.ReadU32(bb.raw, format.REGFClusterOffset) }

// Flags returns the hive flags field.
func (bb *BaseBlock) Flags() uint32 { return format.ReadU32(bb.raw, format.REGFFlagsOffset) }

// HasPendingTransactions returns true if the pending transactions flag is set.
func (bb *BaseBlock) HasPendingTransactions() bool {
	return (bb.Flags() & format.REGFFlagPendingTransactions) != 0
}

// IsDifferencingHive returns true if this is a differencing hive.
func (bb *BaseBlock) IsDifferencingHive() bool {
	return (bb.Flags() & format.REGFFlagDifferencingHive) != 0
}

// FileName returns the 64-byte ASCII/UTF-16-ish name field (no decoding, zero-copy).
func (bb *BaseBlock) FileName() []byte {
	return bb.raw[format.REGFFileNameOffset : format.REGFFileNameOffset+format.REGFFileNameSize]
}

// RmID returns the Resource Manager GUID (zero-copy slice).
func (bb *BaseBlock) RmID() []byte {
	return bb.raw[format.REGFRmIDOffset : format.REGFRmIDOffset+format.GUIDSize]
}

// LogID returns the Log GUID (zero-copy slice).
func (bb *BaseBlock) LogID() []byte {
	return bb.raw[format.REGFLogIDOffset : format.REGFLogIDOffset+format.GUIDSize]
}

// TmID returns the Transaction Manager GUID (zero-copy slice).
func (bb *BaseBlock) TmID() []byte {
	return bb.raw[format.REGFTmIDOffset : format.REGFTmIDOffset+format.GUIDSize]
}

// ThawTmID returns the Thaw Transaction Manager GUID (zero-copy slice).
func (bb *BaseBlock) ThawTmID() []byte {
	return bb.raw[format.REGFThawTmIdOffset : format.REGFThawTmIdOffset+format.GUIDSize]
}

// ThawRmID returns the Thaw Resource Manager GUID (zero-copy slice).
func (bb *BaseBlock) ThawRmID() []byte {
	return bb.raw[format.REGFThawRmIdOffset : format.REGFThawRmIdOffset+format.GUIDSize]
}

// ThawLogID returns the Thaw Log GUID (zero-copy slice).
func (bb *BaseBlock) ThawLogID() []byte {
	return bb.raw[format.REGFThawLogIdOffset : format.REGFThawLogIdOffset+format.GUIDSize]
}

// GUIDSignature returns the GUID signature field.
func (bb *BaseBlock) GUIDSignature() uint32 { return format.ReadU32(bb.raw, format.REGFGuidSigOffset) }

// LastReorganizeTime returns the last reorganize FILETIME or special markers 0x1/0x2.
func (bb *BaseBlock) LastReorganizeTime() uint64 {
	return format.ReadU64(bb.raw, format.REGFLastReorgTimeOffset)
}

// BootType returns the boot type field.
func (bb *BaseBlock) BootType() uint32 { return format.ReadU32(bb.raw, format.REGFBootTypeOffset) }

// BootRecover returns the boot recover field.
func (bb *BaseBlock) BootRecover() uint32 { return format.ReadU32(bb.raw, format.REGFBootRecovOffset) }

// HiveLength reports hive length = 4K header + DataSize.
func (bb *BaseBlock) HiveLength() int { return format.HeaderSize + int(bb.DataSize()) }

// ValidateSanity checks against actual file size and root-cell reachability.
func (bb *BaseBlock) ValidateSanity(fileSize int) error {
	reported := bb.HiveLength()
	if reported > fileSize {
		return fmt.Errorf("hive: reported hive length (%d) > file size (%d)", reported, fileSize)
	}
	rootAbs := format.HeaderSize + int(bb.RootCellOffset())
	if rootAbs > fileSize {
		return fmt.Errorf("hive: root cell (%d) beyond file size (%d)", rootAbs, fileSize)
	}
	return nil
}

// Validate performs a thorough header validation with descriptive errors.
// It does not read HBINs; it checks only the 4 KiB base block against the
// spec and a provided fileSize (the entire hive file length).
//
// Policy choices (conservative but practical):
//   - Signature must be "regf"
//   - Checksum must be correct (XOR of first 508 bytes w/ remapping)
//   - DataSize must be 4 KiB-aligned (multiple of 0x1000)
//   - Reported HiveLength (4 KiB + DataSize) must be <= fileSize
//   - RootCellOffset must be < DataSize (root lies within HBIN area)
//   - Version: Major == 1, Minor in [3..6]. We allow Major==0 only if you
//     explicitly want to accept “recovery” cases—here we reject it to surface issues.
//   - Sequence1/2 may differ (dirty hive) -> not an error; available via IsClean().
func (bb *BaseBlock) Validate(fileSize int) error {
	// Basic size & signature already checked by ParseBaseBlock, but keep messages local to Validate too.
	if len(bb.raw) < format.HeaderSize {
		return fmt.Errorf("regf: header truncated: have=%d need=%d", len(bb.raw), format.HeaderSize)
	}
	if !isREGF(bb.raw) {
		return errors.New("regf: bad signature")
	}

	// Checksum
	if !bb.ChecksumOK() {
		return fmt.Errorf("regf: checksum mismatch: stored=0x%08X computed=0x%08X",
			bb.StoredChecksum(), regfChecksum(bb.raw[:format.REGFChecksumRegionLen]))
	}

	// DataSize constraints
	ds := bb.DataSize()
	if ds%0x1000 != 0 {
		return fmt.Errorf("regf: data size not 4KiB-aligned: 0x%X", ds)
	}

	// Reported length vs actual file
	reported := bb.HiveLength()
	if reported > fileSize {
		return fmt.Errorf(
			"regf: reported hive length (%d) exceeds file size (%d)",
			reported,
			fileSize,
		)
	}

	// Root must be within data area
	root := bb.RootCellOffset()
	if root == 0 {
		return errors.New("regf: root cell offset is zero")
	}
	if root >= ds {
		return fmt.Errorf("regf: root cell offset (0x%X) beyond data area (size=0x%X)", root, ds)
	}

	// Version policy
	majV, minV := bb.Major(), bb.Minor()
	if majV != 1 {
		return fmt.Errorf("regf: unsupported major version %d (expected 1)", majV)
	}
	if minV < 3 || minV > 6 {
		return fmt.Errorf("regf: unsupported minor version %d (expected 3..6)", minV)
	}

	// Format/type fields are largely advisory; keep them for future checks if desired.

	return nil
}

// ChecksumOK computes the XOR checksum over the first 508 bytes and
// compares it to the stored value at 0x1FC, including the 0/-1 remapping.
func (bb *BaseBlock) ChecksumOK() bool {
	sum := regfChecksum(bb.raw[:format.REGFChecksumRegionLen]) // 508 bytes
	stored := format.ReadU32(bb.raw, format.REGFCheckSumOffset)
	return sum == stored
}

// StoredChecksum returns the checksum value stored in the header.
func (bb *BaseBlock) StoredChecksum() uint32 {
	return format.ReadU32(bb.raw, format.REGFCheckSumOffset)
}

// ---- internals ----

// regfChecksum computes the XOR checksum over 127 DWORDs (508 bytes). Then:
//
//	if xor==0xFFFFFFFF -> 0xFFFFFFFE
//	if xor==0x00000000 -> 0x00000001
func regfChecksum(head508 []byte) uint32 {
	var xor uint32
	// 127 * 4 = 508 bytes
	for i := range format.REGFChecksumDwords {
		off := i << dwordBitShift
		xor ^= format.ReadU32(head508, off)
	}
	switch xor {
	case regfChecksumAllOnes:
		return regfChecksumAllOnesReplacement
	case regfChecksumAllZeros:
		return regfChecksumAllZerosReplacement
	default:
		return xor
	}
}
