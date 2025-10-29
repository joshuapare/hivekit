package regtext

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// unescapeRegString unescapes a string from .reg format.
// .reg files escape backslashes as \\ and quotes as \"
func unescapeRegString(s string) string {
	s = strings.ReplaceAll(s, EscapedBackslash, Backslash)
	s = strings.ReplaceAll(s, EscapedQuote, Quote)
	return s
}

// findClosingQuote finds the position of the closing quote in a line,
// accounting for escaped quotes (preceded by an odd number of backslashes).
// Returns -1 if no valid closing quote is found.
// The search starts at position 1 (assuming the opening quote is at position 0).
func findClosingQuote(line string) int {
	for i := 1; i < len(line); i++ {
		if string(line[i]) == Quote {
			// Count consecutive backslashes before this quote
			numBackslashes := 0
			for j := i - 1; j >= 0 && string(line[j]) == Backslash; j-- {
				numBackslashes++
			}
			// If odd number of backslashes, the quote is escaped
			if numBackslashes%2 == 1 {
				continue // Escaped quote, keep looking
			}
			return i
		}
	}
	return -1
}

// parseHexBytes parses hex data from .reg format (hex:01,02,03,...).
// It handles:
// - Removing the prefix (hex:, hex(7):, etc.) via the colon position
// - Line continuation characters and whitespace
// - Comma-separated hex bytes
// - Single-digit bytes (auto-pads with 0)
func parseHexBytes(hexStr string) ([]byte, error) {
	// Find and remove prefix (everything up to and including the colon)
	colonPos := strings.Index(hexStr, ":")
	if colonPos == -1 {
		return nil, errors.New("invalid hex data format: missing colon")
	}
	hexStr = hexStr[colonPos+1:]

	// Remove whitespace and line continuation characters
	hexStr = removeWhitespace(hexStr)

	// Split by comma and parse each byte
	parts := strings.Split(hexStr, HexByteSeparator)
	buf := make([]byte, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Pad single-digit hex to two digits
		if len(p) == 1 {
			p = "0" + p
		}
		// Decode the hex byte
		b, err := hex.DecodeString(p)
		if err != nil {
			return nil, fmt.Errorf("invalid hex byte %q: %w", p, err)
		}
		buf = append(buf, b...)
	}

	return buf, nil
}

// removeWhitespace removes whitespace and line continuation characters
// from a string. This is used when parsing hex data that may span multiple lines.
func removeWhitespace(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for _, ch := range s {
		if ch != ' ' && ch != '\t' && ch != '\n' && ch != '\r' && string(ch) != Backslash {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

// detectValueType determines the registry value type from the value data string.
// Returns a string identifier like "string", "dword", "binary", "hex(2)", etc.
func detectValueType(data string) string {
	if strings.HasPrefix(data, Quote) {
		return ValueTypeString
	}
	if strings.HasPrefix(data, DWORDPrefix) {
		return ValueTypeDWORD
	}
	// Check for typed hex values: hex(1), hex(2), hex(7), etc.
	if strings.HasPrefix(data, "hex(") {
		endPos := strings.Index(data, ")")
		if endPos > 4 {
			// Return the full type string like "hex(1)", "hex(2)", "hex(7)"
			return data[:endPos+1]
		}
	}
	// Check for plain hex: binary data
	if strings.HasPrefix(data, HexPrefix) {
		return ValueTypeBinary
	}
	return ValueTypeUnknown
}

// parseHexValueType extracts the registry type from a hex() prefix.
// For example, "hex(2):" returns REG_EXPAND_SZ, "hex(7):" returns REG_MULTI_SZ.
// Returns the type number as a string ("2", "7", etc.) and whether it was found.
func parseHexValueType(payload string) (typeNum string, found bool) {
	openParen := strings.Index(payload, "(")
	closeParen := strings.Index(payload, ")")

	if openParen >= 0 && closeParen > openParen {
		typeNum = payload[openParen+1 : closeParen]
		return typeNum, true
	}

	return "", false
}
