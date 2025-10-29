package reader

import (
	"strings"
	"unicode/utf8"

	"github.com/joshuapare/hivekit/internal/format"
)

// decodeUTF16LE decodes UTF-16LE bytes to a UTF-8 string without intermediate allocations.
// This is an optimized version that uses strings.Builder to minimize allocations.
func decodeUTF16LE(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// Fast path: check if it's all ASCII (most common case in registry)
	// In UTF-16LE, ASCII chars are: [byte, 0x00]
	allASCII := true
	if len(data)%2 == 0 {
		for i := 0; i < len(data); i += 2 {
			if data[i+1] != 0 || data[i] >= format.UTF16ASCIIThreshold {
				allASCII = false
				break
			}
		}
	} else {
		allASCII = false
	}

	if allASCII {
		// Fast path: extract ASCII directly using strings.Builder
		var b strings.Builder
		b.Grow(len(data) / 2)
		for i := 0; i < len(data); i += 2 {
			b.WriteByte(data[i])
		}
		return b.String()
	}

	// Slow path: decode UTF-16 properly (handles surrogates, non-ASCII, etc.)
	// Decode directly to strings.Builder to avoid intermediate allocations
	var b strings.Builder
	b.Grow(estimateUTF8Size(data))

	for i := 0; i+1 < len(data); i += 2 {
		// Read UTF-16LE code unit
		r := rune(data[i]) | rune(data[i+1])<<8

		// Check for high surrogate (U+D800 to U+DBFF)
		if r >= 0xD800 && r <= 0xDBFF && i+3 < len(data) {
			// Read potential low surrogate
			r2 := rune(data[i+2]) | rune(data[i+3])<<8
			// Check if it's a valid low surrogate (U+DC00 to U+DFFF)
			if r2 >= 0xDC00 && r2 <= 0xDFFF {
				// Valid surrogate pair - combine them
				r = 0x10000 + ((r-0xD800)<<10 | (r2 - 0xDC00))
				i += 2 // Skip the low surrogate
			}
		}

		b.WriteRune(r)
	}
	return b.String()
}

// estimateUTF8Size estimates the UTF-8 byte size for a UTF-16LE encoded byte slice.
// This helps pre-allocate the right size buffer for decoding.
func estimateUTF8Size(data []byte) int {
	// Rough estimate: UTF-16LE is 2 bytes per char (or 4 for surrogates)
	// UTF-8 is 1-4 bytes per char
	// For typical Windows registry names (mostly ASCII/Latin): ~1.5x compression
	// For worst case (Chinese/Japanese): similar size
	// Safe estimate: same size as UTF-16 input
	return len(data)
}

// decodeUTF16LEToBuilder is an optimized version that writes directly to a strings.Builder
// to avoid intermediate allocations. However, strings.Builder requires more complex code
// and may not be worth it for short strings. This is here for future optimization if needed.
func decodeUTF16LEToUTF8Bytes(data []byte, out []byte) int {
	if len(data) == 0 {
		return 0
	}

	outIdx := 0
	for i := 0; i+1 < len(data); i += 2 {
		// Read UTF-16LE code unit
		r := rune(data[i]) | rune(data[i+1])<<8

		// Check if it's a surrogate pair (U+D800 to U+DFFF)
		if r >= format.UTF16HighSurrogateStart && r <= format.UTF16HighSurrogateEnd && i+3 < len(data) {
			// High surrogate, need to read low surrogate
			r2 := rune(data[i+2]) | rune(data[i+3])<<8
			if r2 >= format.UTF16LowSurrogateStart && r2 <= format.UTF16LowSurrogateEnd {
				// Valid surrogate pair
				r = format.UTF16SurrogateBase + ((r-format.UTF16HighSurrogateStart)<<10 | (r2 - format.UTF16LowSurrogateStart))
				i += 2 // Skip the low surrogate
			}
		}

		// Encode to UTF-8
		if outIdx+utf8.RuneLen(r) > len(out) {
			// Buffer too small
			break
		}
		outIdx += utf8.EncodeRune(out[outIdx:], r)
	}

	return outIdx
}
