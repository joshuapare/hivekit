package format

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// NKRecord captures metadata extracted from an NK record. NK cells describe
// registry keys. The structure (simplified) is shown below:
//
//	Offset  Size  Field
//	0x00    2     'n' 'k'
//	0x02    2     Flags (bit 0x20 => name stored as ASCII)
//	0x04    8     Last write time (FILETIME)
//	0x0C    4     Access bits (Windows 8+, ignored)
//	0x10    4     Parent cell offset
//	0x14    4     Number of subkeys
//	0x18    4     Number of volatile subkeys (ignored)
//	0x1C    4     Offset to subkey list
//	0x20    4     Volatile subkey list offset (ignored)
//	0x24    4     Number of values
//	0x28    4     Offset to value list
//	0x2C    4     Security offset
//	0x30    4     Class name offset
//	0x34    4     Max subkey name length
//	0x38    4     Max subkey class name length
//	0x3C    4     Max value name length
//	0x40    4     Max value data length
//	0x44    4     Work var (ignored)
//	0x48    2     Name length
//	0x4A    2     Class length
//	0x4C    n     Name bytes (ASCII or UTF-16LE)
type NKRecord struct {
	Flags              uint16
	LastWriteRaw       uint64
	ParentOffset       uint32
	SubkeyCount        uint32
	SubkeyListOffset   uint32
	ValueCount         uint32
	ValueListOffset    uint32
	SecurityOffset     uint32
	ClassNameOffset    uint32
	MaxNameLength      uint32
	MaxClassLength     uint32
	MaxValueNameLength uint32
	MaxValueDataLength uint32
	NameLength         uint16
	ClassLength        uint16
	NameRaw            []byte
}

// NameIsCompressed returns true when the name is stored in 8-bit form.
func (nk NKRecord) NameIsCompressed() bool {
	return nk.Flags&NKFlagCompressedName != 0
}

// DecodeNK decodes an NK record payload with comprehensive bounds checking.
func DecodeNK(b []byte) (NKRecord, error) {
	if len(b) < NKMinSize {
		return NKRecord{}, fmt.Errorf("nk: %w (have %d, need %d)", ErrTruncated, len(b), NKMinSize)
	}
	if !bytes.Equal(b[:SignatureSize], NKSignature) {
		return NKRecord{}, fmt.Errorf("nk: %w", ErrSignatureMismatch)
	}

	// Read all fixed fields with checked reads
	flags, err := CheckedReadU16(b, NKFlagsOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk flags: %w", err)
	}

	lastWrite, err := CheckedReadU64(b, NKLastWriteOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk lastwrite: %w", err)
	}

	// NKAccessBitsOffset = access bits (Windows 8+), skip it
	parent, err := CheckedReadU32(b, NKParentOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk parent: %w", err)
	}

	subkeyCount, err := CheckedReadU32(b, NKSubkeyCountOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk subkey count: %w", err)
	}
	// Sanity check: subkey count
	if subkeyCount > MaxSubkeyCount {
		return NKRecord{}, fmt.Errorf("nk subkey count %d exceeds limit %d: %w",
			subkeyCount, MaxSubkeyCount, ErrSanityLimit)
	}

	// NKVolSubkeyCountOffset = volatile subkey count, skip it
	subkeyListOff, err := CheckedReadU32(b, NKSubkeyListOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk subkey list: %w", err)
	}

	// NKVolSubkeyListOffset = volatile subkey list offset, skip it
	valueCount, err := CheckedReadU32(b, NKValueCountOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk value count: %w", err)
	}
	// Sanity check: value count
	if valueCount > MaxValueCount {
		return NKRecord{}, fmt.Errorf("nk value count %d exceeds limit %d: %w",
			valueCount, MaxValueCount, ErrSanityLimit)
	}

	valueListOff, err := CheckedReadU32(b, NKValueListOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk value list: %w", err)
	}

	securityOff, err := CheckedReadU32(b, NKSecurityOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk security: %w", err)
	}

	classOff, err := CheckedReadU32(b, NKClassNameOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk class name: %w", err)
	}

	maxNameLen, err := CheckedReadU32(b, NKMaxNameLenOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk max name len: %w", err)
	}

	maxClassLen, err := CheckedReadU32(b, NKMaxClassLenOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk max class len: %w", err)
	}

	maxValueNameLen, err := CheckedReadU32(b, NKMaxValueNameOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk max value name len: %w", err)
	}

	maxValueDataLen, err := CheckedReadU32(b, NKMaxValueDataOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk max value data len: %w", err)
	}

	// NKWorkVarOffset -> workvar (ignored)
	nameLen, err := CheckedReadU16(b, NKNameLenOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk name len: %w", err)
	}
	// Sanity check: name length
	if int(nameLen) > MaxNameLen {
		return NKRecord{}, fmt.Errorf("nk name len %d exceeds limit %d: %w",
			nameLen, MaxNameLen, ErrSanityLimit)
	}

	classLen, err := CheckedReadU16(b, NKClassLenOffset)
	if err != nil {
		return NKRecord{}, fmt.Errorf("nk class len: %w", err)
	}
	// Sanity check: class length
	if int(classLen) > MaxClassLen {
		return NKRecord{}, fmt.Errorf("nk class len %d exceeds limit %d: %w",
			classLen, MaxClassLen, ErrSanityLimit)
	}

	// Bounds check: name slice
	base := NKNameOffset
	nameEnd, ok := buf.AddOverflowSafe(base, int(nameLen))
	if !ok || nameEnd > len(b) {
		return NKRecord{}, fmt.Errorf("nk name: %w (need %d bytes from %d, have %d)",
			ErrTruncated, nameLen, base, len(b))
	}
	name := b[base:nameEnd]

	return NKRecord{
		Flags:              flags,
		LastWriteRaw:       lastWrite,
		ParentOffset:       parent,
		SubkeyCount:        subkeyCount,
		SubkeyListOffset:   subkeyListOff,
		ValueCount:         valueCount,
		ValueListOffset:    valueListOff,
		SecurityOffset:     securityOff,
		ClassNameOffset:    classOff,
		MaxNameLength:      maxNameLen,
		MaxClassLength:     maxClassLen,
		MaxValueNameLength: maxValueNameLen,
		MaxValueDataLength: maxValueDataLen,
		NameLength:         nameLen,
		ClassLength:        classLen,
		NameRaw:            name,
	}, nil
}
