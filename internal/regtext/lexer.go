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

func decodeInput(data []byte, enc string) (string, error) {
	if len(data) >= len(UTF16LEBOM) && data[0] == UTF16LEBOM[0] && data[1] == UTF16LEBOM[1] {
		return utf16LEToString(data[len(UTF16LEBOM):]), nil
	}
	if len(data) >= len(UTF8BOM) && data[0] == UTF8BOM[0] && data[1] == UTF8BOM[1] && data[2] == UTF8BOM[2] {
		return string(data[len(UTF8BOM):]), nil
	}
	switch strings.ToUpper(enc) {
	case "", EncodingUTF8:
		return string(data), nil
	case EncodingUTF16LE:
		return utf16LEToString(data), nil
	default:
		return "", errUnsupportedEncoding
	}
}

func utf16LEToString(data []byte) string {
	if len(data)%UTF16CodeUnitSize == 1 {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return ""
	}
	words := make([]uint16, len(data)/UTF16CodeUnitSize)
	for i := 0; i < len(words); i++ {
		words[i] = binary.LittleEndian.Uint16(data[i*UTF16CodeUnitSize:])
	}
	return string(utf16.Decode(words))
}

func encodeUTF16LE(s string, withBOM bool) []byte {
	var words []uint16
	for _, r := range s {
		words = append(words, utf16.Encode([]rune{r})...)
	}
	buf := make([]byte, 0, len(words)*UTF16CodeUnitSize+UTF16CodeUnitSize)
	if withBOM {
		buf = append(buf, UTF16LEBOM...)
	}
	tmp := make([]byte, UTF16CodeUnitSize)
	for _, w := range words {
		binary.LittleEndian.PutUint16(tmp, w)
		buf = append(buf, tmp...)
	}
	return buf
}

func encodeUTF16LEZeroTerminated(s string) []byte {
	words := utf16.Encode([]rune(s))
	words = append(words, 0)
	buf := make([]byte, len(words)*UTF16CodeUnitSize)
	for i, w := range words {
		binary.LittleEndian.PutUint16(buf[i*UTF16CodeUnitSize:], w)
	}
	return buf
}
