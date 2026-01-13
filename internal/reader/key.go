package reader

import (
	"errors"
	"fmt"

	"golang.org/x/text/encoding/charmap"

	"github.com/joshuapare/hivekit/internal/format"
)

// DecodeKeyName converts the NK name encoding into UTF-8.
func DecodeKeyName(nk format.NKRecord) (string, error) {
	if nk.NameLength == 0 {
		return "", nil
	}
	data := nk.NameRaw
	if nk.NameIsCompressed() {
		// Compressed names use Windows-1252 (Latin-1) encoding
		// Fast path: ASCII doesn't need decoding (same in Windows-1252 and UTF-8)
		if isASCII(data) {
			return string(data), nil
		}
		// Slow path: Use decoder for extended characters (0x80-0xFF)
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err != nil {
			return "", fmt.Errorf("failed to decode Windows-1252 name: %w", err)
		}
		return string(decoded), nil
	}
	if len(data)%2 != 0 {
		return "", errors.New("nk name has odd length")
	}
	// Use optimized UTF-16LE decoder (avoids intermediate []uint16 allocation)
	return decodeUTF16LE(data), nil
}

// EncodeKeyName converts a UTF-8 string to Windows-1252 bytes for compressed names.
// This is the reverse of DecodeKeyName for compressed names.
func EncodeKeyName(name string) ([]byte, error) {
	if name == "" {
		return nil, nil
	}
	// Encode UTF-8 string to Windows-1252 bytes
	encoded, err := charmap.Windows1252.NewEncoder().Bytes([]byte(name))
	if err != nil {
		return nil, fmt.Errorf("failed to encode name to Windows-1252: %w", err)
	}
	return encoded, nil
}

// isASCII checks if all bytes in data are ASCII (< 0x80).
// ASCII characters have the same encoding in Windows-1252 and UTF-8.
func isASCII(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 {
			return false
		}
	}
	return true
}
