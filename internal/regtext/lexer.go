package regtext

import (
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"
)

var (
	errUnsupportedEncoding = errors.New("regtext: unsupported encoding")
)

// decodeInputToBytes converts input data to UTF-8 bytes if needed
// Returns the data ready for scanning (no string allocation).
func decodeInputToBytes(data []byte, enc string) ([]byte, error) {
	// Check for UTF-16LE BOM
	if len(data) >= len(UTF16LEBOM) && data[0] == UTF16LEBOM[0] && data[1] == UTF16LEBOM[1] {
		return utf16LEToBytes(data[len(UTF16LEBOM):]), nil
	}
	// Check for UTF-8 BOM - just skip it, return rest as-is
	if len(data) >= len(UTF8BOM) && data[0] == UTF8BOM[0] && data[1] == UTF8BOM[1] && data[2] == UTF8BOM[2] {
		return data[len(UTF8BOM):], nil
	}
	switch strings.ToUpper(enc) {
	case "", EncodingUTF8:
		return data, nil // No copy!
	case EncodingUTF16LE:
		return utf16LEToBytes(data), nil
	default:
		return nil, errUnsupportedEncoding
	}
}

// utf16LEToBytes converts UTF-16LE data to UTF-8 bytes.
func utf16LEToBytes(data []byte) []byte {
	if len(data)%UTF16CodeUnitSize == 1 {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return nil
	}
	words := make([]uint16, len(data)/UTF16CodeUnitSize)
	for i := 0; i < len(words); i++ {
		words[i] = binary.LittleEndian.Uint16(data[i*UTF16CodeUnitSize:])
	}
	return []byte(string(utf16.Decode(words)))
}

// encodeUTF16LE encodes a string to UTF-16LE (Phase 2: optimized single-pass).
func encodeUTF16LE(s string, withBOM bool) []byte {
	// Single conversion to runes, then encode all at once
	runes := []rune(s)
	words := utf16.Encode(runes)

	// Pre-allocate exact size needed
	bufSize := len(words) * UTF16CodeUnitSize
	if withBOM {
		bufSize += len(UTF16LEBOM)
	}
	buf := make([]byte, bufSize)

	offset := 0
	if withBOM {
		copy(buf, UTF16LEBOM)
		offset = len(UTF16LEBOM)
	}

	// Write all uint16 words directly to buffer
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[offset+i*UTF16CodeUnitSize:], w)
	}

	return buf
}

// encodeUTF16LEZeroTerminated encodes a string to UTF-16LE with null terminator
// (Phase 2: optimized - pre-allocate exact size, single encoding pass).
func encodeUTF16LEZeroTerminated(s string) []byte {
	// Single conversion and encoding
	runes := []rune(s)
	words := utf16.Encode(runes)

	// Pre-allocate exact size: encoded words + null terminator
	buf := make([]byte, (len(words)+1)*UTF16CodeUnitSize)

	// Write all words
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[i*UTF16CodeUnitSize:], w)
	}

	// Null terminator is already zero from make()
	return buf
}
