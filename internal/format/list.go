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
	count := buf.U16LE(b[SignatureSize:ListHeaderSize])
	entryCount := uint32(count)
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
	if len(b) < int(count)*OffsetFieldSize {
		return nil, fmt.Errorf("li list: %w", ErrTruncated)
	}
	out := make([]uint32, count)
	for i := range count {
		out[i] = buf.U32LE(b[i*OffsetFieldSize:])
	}
	return out, nil
}

func decodeLF(b []byte, count uint32) ([]uint32, error) {
	if len(b) < int(count)*LFEntrySize {
		return nil, fmt.Errorf("lf list: %w", ErrTruncated)
	}
	out := make([]uint32, count)
	for i := range count {
		start := int(i) * LFEntrySize
		out[i] = buf.U32LE(b[start:])
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
	count := buf.U16LE(b[SignatureSize:ListHeaderSize])
	if len(b) < ListHeaderSize+int(count)*OffsetFieldSize {
		return nil, fmt.Errorf("ri list: %w", ErrTruncated)
	}
	// Each entry is an OffsetFieldSize-byte offset to an LF/LH list
	offsets := make([]uint32, count)
	for i := range count {
		offsets[i] = buf.U32LE(b[ListHeaderSize+i*OffsetFieldSize:])
	}
	return offsets, nil
}

// DecodeValueList decodes a value list containing offsets to VK records.
func DecodeValueList(b []byte, count uint32) ([]uint32, error) {
	need := int(count) * OffsetFieldSize
	if need == 0 {
		return nil, nil
	}
	if len(b) < need {
		return nil, fmt.Errorf("value list: %w", ErrTruncated)
	}
	out := make([]uint32, count)
	for i := range count {
		out[i] = buf.U32LE(b[i*OffsetFieldSize:])
	}
	return out, nil
}
