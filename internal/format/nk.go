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

// DecodeNK decodes an NK record payload.
func DecodeNK(b []byte) (NKRecord, error) {
	if len(b) < NKMinSize {
		return NKRecord{}, fmt.Errorf("nk: %w", ErrTruncated)
	}
	if !bytes.Equal(b[:SignatureSize], NKSignature) {
		return NKRecord{}, fmt.Errorf("nk: %w", ErrSignatureMismatch)
	}
	flags := buf.U16LE(b[NKFlagsOffset:])
	lastWrite := buf.U64LE(b[NKLastWriteOffset:])
	// NKAccessBitsOffset = access bits (Windows 8+), skip it
	parent := buf.U32LE(b[NKParentOffset:])
	subkeyCount := buf.U32LE(b[NKSubkeyCountOffset:])
	// NKVolSubkeyCountOffset = volatile subkey count, skip it
	subkeyListOff := buf.U32LE(b[NKSubkeyListOffset:])
	// NKVolSubkeyListOffset = volatile subkey list offset, skip it
	valueCount := buf.U32LE(b[NKValueCountOffset:])
	valueListOff := buf.U32LE(b[NKValueListOffset:])
	securityOff := buf.U32LE(b[NKSecurityOffset:])
	classOff := buf.U32LE(b[NKClassNameOffset:])
	maxNameLen := buf.U32LE(b[NKMaxNameLenOffset:])
	maxClassLen := buf.U32LE(b[NKMaxClassLenOffset:])
	maxValueNameLen := buf.U32LE(b[NKMaxValueNameOffset:])
	maxValueDataLen := buf.U32LE(b[NKMaxValueDataOffset:])
	// NKWorkVarOffset -> workvar (ignored)
	nameLen := buf.U16LE(b[NKNameLenOffset:])
	classLen := buf.U16LE(b[NKClassLenOffset:])
	base := NKNameOffset
	if len(b) < base+int(nameLen) {
		return NKRecord{}, fmt.Errorf("nk name: %w", ErrTruncated)
	}
	name := b[base : base+int(nameLen)]
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
