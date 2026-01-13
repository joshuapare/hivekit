package format

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/buf"
)

// DecodeSubkeyList extracts NK offsets from list records (LI, LF, LH). Each
// entry stores the relative offset of a child NK cell. LF/LH additionally store
// a hashed name which we skip here because higher layers compare names.
func DecodeSubkeyList(b []byte, expected uint32) ([]uint32, error) {
	if len(b) < ListHeaderSize {
		return nil, fmt.Errorf("subkey list: %w", ErrTruncated)
	}
	sig := b[:SignatureSize]
	count, err := CheckedReadU16(b, SignatureSize)
	if err != nil {
		return nil, fmt.Errorf("subkey list count: %w", err)
	}
	entryCount := uint32(count)

	// Sanity check: subkey count
	if entryCount > MaxSubkeyCount {
		return nil, fmt.Errorf("subkey list count %d exceeds limit %d: %w", entryCount, MaxSubkeyCount, ErrSanityLimit)
	}

	if expected != 0 && expected < entryCount {
		entryCount = expected
	}
	switch {
	case bytes.Equal(sig, LISignature):
		return decodeLI(b[ListHeaderSize:], entryCount)
	case bytes.Equal(sig, LFSignature), bytes.Equal(sig, LHSignature):
		return decodeLF(b[ListHeaderSize:], entryCount)
	default:
		return nil, fmt.Errorf("subkey list: %w", ErrUnsupported)
	}
}

func decodeLI(b []byte, count uint32) ([]uint32, error) {
	// Sanity check count
	if count > MaxSubkeyCount {
		return nil, fmt.Errorf("li count %d exceeds limit: %w", count, ErrSanityLimit)
	}

	// Safe overflow check for count * OffsetFieldSize
	_, err := buf.CheckListBounds(len(b), 0, int(count), OffsetFieldSize)
	if err != nil {
		return nil, fmt.Errorf("li list bounds: %w", err)
	}

	out := make([]uint32, count)
	for i := range count {
		off := int(i) * OffsetFieldSize
		val, err := CheckedReadU32(b, off)
		if err != nil {
			return nil, fmt.Errorf("li entry %d: %w", i, err)
		}
		out[i] = val
	}
	return out, nil
}

func decodeLF(b []byte, count uint32) ([]uint32, error) {
	// Sanity check count
	if count > MaxSubkeyCount {
		return nil, fmt.Errorf("lf count %d exceeds limit: %w", count, ErrSanityLimit)
	}

	// Safe overflow check for count * LFEntrySize
	_, err := buf.CheckListBounds(len(b), 0, int(count), LFEntrySize)
	if err != nil {
		return nil, fmt.Errorf("lf list bounds: %w", err)
	}

	out := make([]uint32, count)
	for i := range count {
		off := int(i) * LFEntrySize
		val, err := CheckedReadU32(b, off)
		if err != nil {
			return nil, fmt.Errorf("lf entry %d: %w", i, err)
		}
		out[i] = val
	}
	return out, nil
}

// IsRIList checks if a byte slice contains an RI (indirect) subkey list.
// RI lists are used when a key has many subkeys (>~100) and contain offsets
// to multiple LF/LH lists rather than direct NK offsets.
func IsRIList(b []byte) bool {
	if len(b) < SignatureSize {
		return false
	}
	return bytes.Equal(b[:SignatureSize], RISignature)
}

// DecodeRIList decodes an RI (indirect) subkey list and returns the offsets
// to the constituent LF/LH lists. The caller must fetch and decode each sub-list.
// RI structure: signature (SignatureSize bytes) + count (2 bytes) + array of offsets (OffsetFieldSize bytes each).
func DecodeRIList(b []byte) ([]uint32, error) {
	if len(b) < ListHeaderSize {
		return nil, fmt.Errorf("ri list: %w", ErrTruncated)
	}
	sig := b[:SignatureSize]
	if !bytes.Equal(sig, RISignature) {
		return nil, errors.New("ri list: invalid signature")
	}
	count, err := CheckedReadU16(b, SignatureSize)
	if err != nil {
		return nil, fmt.Errorf("ri list count: %w", err)
	}

	// Sanity check: RI list count
	if count > MaxRIListCount {
		return nil, fmt.Errorf("ri list count %d exceeds limit %d: %w", count, MaxRIListCount, ErrSanityLimit)
	}

	// Safe overflow check for count * OffsetFieldSize
	_, err = buf.CheckListBounds(len(b), ListHeaderSize, int(count), OffsetFieldSize)
	if err != nil {
		return nil, fmt.Errorf("ri list bounds: %w", err)
	}

	// Each entry is an OffsetFieldSize-byte offset to an LF/LH list
	offsets := make([]uint32, count)
	for i := range count {
		off := ListHeaderSize + int(i)*OffsetFieldSize
		val, err := CheckedReadU32(b, off)
		if err != nil {
			return nil, fmt.Errorf("ri entry %d: %w", i, err)
		}
		offsets[i] = val
	}
	return offsets, nil
}

// DecodeValueList decodes a value list containing offsets to VK records.
func DecodeValueList(b []byte, count uint32) ([]uint32, error) {
	if count == 0 {
		return nil, nil
	}

	// Sanity check: value count
	if count > MaxValueCount {
		return nil, fmt.Errorf("value list count %d exceeds limit %d: %w", count, MaxValueCount, ErrSanityLimit)
	}

	// Safe overflow check for count * OffsetFieldSize
	_, err := buf.CheckListBounds(len(b), 0, int(count), OffsetFieldSize)
	if err != nil {
		return nil, fmt.Errorf("value list bounds: %w", err)
	}

	out := make([]uint32, count)
	for i := range count {
		off := int(i) * OffsetFieldSize
		val, err := CheckedReadU32(b, off)
		if err != nil {
			return nil, fmt.Errorf("value entry %d: %w", i, err)
		}
		out[i] = val
	}
	return out, nil
}
