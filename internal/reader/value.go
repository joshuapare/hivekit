package reader

import (
	"errors"
	"fmt"

	"golang.org/x/text/encoding/charmap"

	"github.com/joshuapare/hivekit/internal/format"
)

// DecodeValueName converts the raw name stored in a VK record into UTF-8. VK
// names follow the same compression rules as NK names: when the format.VKFlagASCIIName flag is
// set, the name is ASCII; otherwise it is UTF-16LE.
func DecodeValueName(vk format.VKRecord) (string, error) {
	if vk.NameLength == 0 {
		return "", nil
	}
	data := vk.NameRaw
	if vk.NameIsASCII() {
		// ASCII names use Windows-1252 (Latin-1) encoding
		// Fast path: ASCII doesn't need decoding (same in Windows-1252 and UTF-8)
		if isASCII(data) {
			return string(data), nil
		}
		// Slow path: Use decoder for extended characters (0x80-0xFF)
		decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err != nil {
			return "", fmt.Errorf("failed to decode Windows-1252 value name: %w", err)
		}
		return string(decoded), nil
	}
	if len(data)%2 != 0 {
		return "", errors.New("vk name has odd length")
	}
	// Use optimized UTF-16LE decoder (avoids intermediate []uint16 allocation)
	return decodeUTF16LE(data), nil
}

func DecodeUTF16(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	if len(data)%2 != 0 {
		return "", errors.New("utf16 string has odd length")
	}
	// Trim null terminator
	if len(data) >= 2 && data[len(data)-2] == 0 && data[len(data)-1] == 0 {
		data = data[:len(data)-2]
	}
	// Use optimized UTF-16LE decoder (avoids intermediate []uint16 allocation)
	return decodeUTF16LE(data), nil
}

func DecodeMultiString(data []byte) ([]string, error) {
	if len(data)%2 != 0 {
		return nil, errors.New("multisz has odd length")
	}
	if len(data) < 2 || data[len(data)-1] != 0 || data[len(data)-2] != 0 {
		return nil, errors.New("multisz missing terminator")
	}
	var result []string
	start := 0
	for i := 0; i < len(data); i += 2 {
		if data[i] == 0 && data[i+1] == 0 {
			if i == start {
				break
			}
			s, err := DecodeUTF16(data[start:i])
			if err != nil {
				return nil, err
			}
			result = append(result, s)
			start = i + 2
		}
	}
	return result, nil
}
