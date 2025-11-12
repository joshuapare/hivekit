package builder

import (
	"encoding/binary"
	"unicode/utf16"
)

// encodeString encodes a Go string to UTF-16LE with null terminator (REG_SZ format).
//
// Example:
//
//	"Hello" -> []byte{0x48, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00, 0x00, 0x00}
//	        UTF-16LE: H    e    l    l    o    \0
func encodeString(s string) []byte {
	// Convert UTF-8 string to UTF-16
	runes := []rune(s)
	utf16Codes := utf16.Encode(runes)

	// Allocate buffer: 2 bytes per UTF-16 code unit + 2 bytes for null terminator
	buf := make([]byte, (len(utf16Codes)+1)*2)

	// Write UTF-16LE codes
	for i, code := range utf16Codes {
		binary.LittleEndian.PutUint16(buf[i*2:], code)
	}

	// Null terminator already zero from make()

	return buf
}

// encodeMultiString encodes a string array to REG_MULTI_SZ format.
// Each string is UTF-16LE null-terminated, with a final double-null terminator.
//
// Example:
//
//	[]string{"A", "B"} -> UTF-16LE: "A\0B\0\0"
//	[]byte{0x41, 0x00, 0x00, 0x00, 0x42, 0x00, 0x00, 0x00, 0x00, 0x00}
func encodeMultiString(values []string) []byte {
	// Special case: empty array -> just double-null terminator
	if len(values) == 0 {
		return []byte{0x00, 0x00, 0x00, 0x00}
	}

	// Calculate total size needed
	totalSize := 0
	for _, s := range values {
		runes := []rune(s)
		utf16Codes := utf16.Encode(runes)
		totalSize += (len(utf16Codes) + 1) * 2 // +1 for null separator
	}
	totalSize += 2 // Final null terminator

	// Allocate buffer
	buf := make([]byte, totalSize)
	offset := 0

	// Encode each string
	for _, s := range values {
		runes := []rune(s)
		utf16Codes := utf16.Encode(runes)

		// Write UTF-16LE codes
		for _, code := range utf16Codes {
			binary.LittleEndian.PutUint16(buf[offset:], code)
			offset += 2
		}

		// Write null separator (2 bytes)
		offset += 2 // Already zero from make()
	}

	// Final null terminator (2 bytes) already zero from make()

	return buf
}

// encodeDWORD encodes a uint32 to 4-byte little-endian representation (REG_DWORD format).
//
// Example:
//
//	0x12345678 -> []byte{0x78, 0x56, 0x34, 0x12}
func encodeDWORD(v uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	return buf
}

// encodeQWORD encodes a uint64 to 8-byte little-endian representation (REG_QWORD format).
//
// Example:
//
//	0x123456789ABCDEF0 -> []byte{0xF0, 0xDE, 0xBC, 0x9A, 0x78, 0x56, 0x34, 0x12}
func encodeQWORD(v uint64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, v)
	return buf
}

// encodeDWORDBigEndian encodes a uint32 to 4-byte big-endian representation
// (REG_DWORD_BIG_ENDIAN format).
//
// Example:
//
//	0x12345678 -> []byte{0x12, 0x34, 0x56, 0x78}
func encodeDWORDBigEndian(v uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return buf
}
