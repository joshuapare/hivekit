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

// DecodeVK decodes a VK record payload with comprehensive bounds checking.
func DecodeVK(b []byte) (VKRecord, error) {
	if len(b) < VKMinSize {
		return VKRecord{}, fmt.Errorf("vk: %w (have %d, need %d)", ErrTruncated, len(b), VKMinSize)
	}
	if !bytes.Equal(b[:SignatureSize], VKSignature) {
		return VKRecord{}, fmt.Errorf("vk: %w", ErrSignatureMismatch)
	}

	// Read all fixed fields with checked reads
	nameLen, err := CheckedReadU16(b, VKNameLenOffset)
	if err != nil {
		return VKRecord{}, fmt.Errorf("vk name len: %w", err)
	}
	// Sanity check: name length
	if int(nameLen) > MaxNameLen {
		return VKRecord{}, fmt.Errorf("vk name len %d exceeds limit %d: %w",
			nameLen, MaxNameLen, ErrSanityLimit)
	}

	dataLen, err := CheckedReadU32(b, VKDataLenOffset)
	if err != nil {
		return VKRecord{}, fmt.Errorf("vk data len: %w", err)
	}
	// Sanity check: data length (mask out inline bit for check)
	actualDataLen := dataLen & VKDataLengthMask
	if actualDataLen > MaxValueDataLen {
		return VKRecord{}, fmt.Errorf("vk data len %d exceeds limit %d: %w",
			actualDataLen, MaxValueDataLen, ErrSanityLimit)
	}

	dataOff, err := CheckedReadU32(b, VKDataOffOffset)
	if err != nil {
		return VKRecord{}, fmt.Errorf("vk data off: %w", err)
	}

	valType, err := CheckedReadU32(b, VKTypeOffset)
	if err != nil {
		return VKRecord{}, fmt.Errorf("vk type: %w", err)
	}

	flags, err := CheckedReadU16(b, VKFlagsOffset)
	if err != nil {
		return VKRecord{}, fmt.Errorf("vk flags: %w", err)
	}

	// Bounds check: name slice
	base := VKNameOffset
	nameEnd, ok := buf.AddOverflowSafe(base, int(nameLen))
	if !ok || nameEnd > len(b) {
		return VKRecord{}, fmt.Errorf("vk name: %w (need %d bytes from %d, have %d)",
			ErrTruncated, nameLen, base, len(b))
	}
	name := b[base:nameEnd]

	return VKRecord{
		NameLength: nameLen,
		DataLength: dataLen,
		DataOffset: dataOff,
		Type:       valType,
		Flags:      flags,
		NameRaw:    name,
	}, nil
}
