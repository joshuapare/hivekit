package format

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// VKRecord models a value key record header. VK cells describe registry values
// and reference the actual data payload (either inline or via another cell).
type VKRecord struct {
	NameLength uint16
	DataLength uint32
	DataOffset uint32
	Type       uint32
	Flags      uint16
	NameRaw    []byte
}

// NameIsASCII reports whether the name is stored as ANSI bytes (flag 0x01).
func (vk VKRecord) NameIsASCII() bool {
	return vk.Flags&VKFlagASCIIName != 0
}

// DataInline reports whether the data is stored within the DataOffset field.
func (vk VKRecord) DataInline() bool {
	return vk.DataLength&VKDataInlineBit != 0
}

// InlineLength returns the actual data length when DataInline is true.
func (vk VKRecord) InlineLength() int {
	if !vk.DataInline() {
		return int(vk.DataLength)
	}
	return int(vk.DataLength & VKDataLengthMask)
}

// DecodeVK decodes a VK record payload.
func DecodeVK(b []byte) (VKRecord, error) {
	if len(b) < VKMinSize {
		return VKRecord{}, fmt.Errorf("vk: %w", ErrTruncated)
	}
	if !bytes.Equal(b[:SignatureSize], VKSignature) {
		return VKRecord{}, fmt.Errorf("vk: %w", ErrSignatureMismatch)
	}
	nameLen := buf.U16LE(b[VKNameLenOffset:])
	dataLen := buf.U32LE(b[VKDataLenOffset:])
	dataOff := buf.U32LE(b[VKDataOffsetField:])
	valType := buf.U32LE(b[VKTypeOffset:])
	flags := buf.U16LE(b[VKFlagsOffset:])
	base := VKNameOffset
	if len(b) < base+int(nameLen) {
		return VKRecord{}, fmt.Errorf("vk name: %w", ErrTruncated)
	}
	name := b[base : base+int(nameLen)]
	return VKRecord{
		NameLength: nameLen,
		DataLength: dataLen,
		DataOffset: dataOff,
		Type:       valType,
		Flags:      flags,
		NameRaw:    name,
	}, nil
}
