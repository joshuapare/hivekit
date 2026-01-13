package regtext

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strings"
)

// Profile defines characteristics for generated .reg file data.
type Profile struct {
	// TargetSize is approximate target file size in bytes
	TargetSize int

	// KeyDepth controls how many levels deep the key hierarchy goes
	// 0 = use defaults, 1-2 = shallow, 3-5 = medium, 8-12 = deep
	KeyDepth int

	// KeysPerLevel controls how many child keys each parent has
	// 0 = use defaults, 1-5 = narrow, 10-50 = medium, 100-500 = wide
	KeysPerLevel int

	// ValueTypeDistribution controls mix of value types (percentage 0.0-1.0)
	// If all zero, uses even distribution
	StringValuePct float64 // REG_SZ
	BinaryValuePct float64 // REG_BINARY (hex:)
	DWORDValuePct  float64 // REG_DWORD
	ExpandSZPct    float64 // REG_EXPAND_SZ (hex(2):)
	MultiSZPct     float64 // REG_MULTI_SZ (hex(7):)

	// ValueSize controls size of value data
	// 0 = use defaults, 1-100 = small, 100-1024 = medium, 1024-10240 = large
	MinValueSize int
	MaxValueSize int

	// ValuesPerKey controls how many values each key has
	// 0 = use defaults (1-10)
	MinValuesPerKey int
	MaxValuesPerKey int

	// SpecialCharacteristics
	EscapeFrequency float64 // 0.0-1.0: how often to include escape sequences
	DeleteKeyPct    float64 // 0.0-1.0: percentage of delete operations vs creates
	DeleteValuePct  float64 // 0.0-1.0: percentage of values that are deletions
	MultilineHex    bool    // generate multi-line hex values (for large binary)

	// Seed for reproducibility (0 = random)
	Seed int64
}

// GenerateRegFile creates a .reg file with the specified profile.
func GenerateRegFile(profile Profile) []byte {
	// Set defaults
	if profile.KeyDepth == 0 {
		profile.KeyDepth = 3
	}
	if profile.KeysPerLevel == 0 {
		profile.KeysPerLevel = 5
	}
	if profile.MinValueSize == 0 {
		profile.MinValueSize = 10
	}
	if profile.MaxValueSize == 0 {
		profile.MaxValueSize = 100
	}
	if profile.MinValuesPerKey == 0 {
		profile.MinValuesPerKey = 1
	}
	if profile.MaxValuesPerKey == 0 {
		profile.MaxValuesPerKey = 10
	}

	// Normalize value type distribution
	totalPct := profile.StringValuePct + profile.BinaryValuePct + profile.DWORDValuePct +
		profile.ExpandSZPct + profile.MultiSZPct
	if totalPct == 0 {
		// Even distribution
		profile.StringValuePct = 0.4
		profile.DWORDValuePct = 0.2
		profile.BinaryValuePct = 0.2
		profile.ExpandSZPct = 0.1
		profile.MultiSZPct = 0.1
	}

	rng := rand.New(rand.NewPCG(uint64(profile.Seed), uint64(profile.Seed)))
	var buf bytes.Buffer

	// Write header
	buf.WriteString(RegFileHeader)
	buf.WriteString(CRLF)
	buf.WriteString(CRLF)

	// Generate keys and values until we reach target size
	keyID := 0
	for buf.Len() < profile.TargetSize {
		keyPath := generateKeyPath(&keyID, profile.KeyDepth, profile.KeysPerLevel, rng)

		// Decide if this is a delete operation
		if rng.Float64() < profile.DeleteKeyPct {
			buf.WriteString(fmt.Sprintf("[-%s]%s", keyPath, CRLF))
			buf.WriteString(CRLF)
			continue
		}

		// Write key
		buf.WriteString(fmt.Sprintf("[%s]%s", keyPath, CRLF))

		// Generate values for this key
		numValues := profile.MinValuesPerKey
		if profile.MaxValuesPerKey > profile.MinValuesPerKey {
			numValues += rng.IntN(profile.MaxValuesPerKey - profile.MinValuesPerKey)
		}

		for i := range numValues {
			valueLine := generateValue(i, profile, rng)
			buf.WriteString(valueLine)
			buf.WriteString(CRLF)

			// Check if we've exceeded target size
			if buf.Len() >= profile.TargetSize {
				break
			}
		}

		buf.WriteString(CRLF)
	}

	return buf.Bytes()
}

// generateKeyPath creates a registry key path.
func generateKeyPath(keyID *int, maxDepth, _ int, rng *rand.Rand) string {
	// Use backslash as root
	depth := 1 + rng.IntN(maxDepth)
	parts := []string{"\\"}

	for range depth {
		*keyID++
		parts = append(parts, fmt.Sprintf("Key%d", *keyID))
	}

	return strings.Join(parts, "\\")
}

// generateValue creates a single value line.
func generateValue(index int, profile Profile, rng *rand.Rand) string {
	// Check if this should be a deletion
	if rng.Float64() < profile.DeleteValuePct {
		name := generateValueName(index, profile, rng)
		return fmt.Sprintf("\"%s\"=-", escapeValueName(name))
	}

	// Select value type based on distribution
	r := rng.Float64()
	var valueType string
	cumulative := 0.0

	cumulative += profile.StringValuePct
	if r < cumulative {
		valueType = "string"
	} else {
		cumulative += profile.DWORDValuePct
		if r < cumulative {
			valueType = "dword"
		} else {
			cumulative += profile.BinaryValuePct
			if r < cumulative {
				valueType = "binary"
			} else {
				cumulative += profile.ExpandSZPct
				if r < cumulative {
					valueType = "expand_sz"
				} else {
					valueType = "multi_sz"
				}
			}
		}
	}

	name := generateValueName(index, profile, rng)

	switch valueType {
	case "string":
		return generateStringValue(name, profile, rng)
	case "dword":
		return generateDWORDValue(name, rng)
	case "binary":
		return generateBinaryValue(name, profile, rng)
	case "expand_sz":
		return generateExpandSZValue(name, profile, rng)
	case "multi_sz":
		return generateMultiSZValue(name, profile, rng)
	default:
		return generateStringValue(name, profile, rng)
	}
}

// generateValueName creates a value name with optional escaping.
func generateValueName(index int, profile Profile, rng *rand.Rand) string {
	name := fmt.Sprintf("Value%d", index)

	// Add escape sequences if needed
	if rng.Float64() < profile.EscapeFrequency {
		switch rng.IntN(2) { // Only 2 cases now, removed quotes case
		case 0:
			name = "Path\\" + name // Add backslash
		case 1:
			name += "\\Data" // Add backslash at end
		}
	}

	return name
}

// escapeValueName escapes a value name for .reg format.
func escapeValueName(name string) string {
	// Escape backslashes first (must be before quotes)
	escaped := strings.ReplaceAll(name, "\\", "\\\\")
	// Then escape quotes
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return escaped
}

// generateStringValue creates a REG_SZ value.
func generateStringValue(name string, profile Profile, rng *rand.Rand) string {
	valueSize := profile.MinValueSize + rng.IntN(profile.MaxValueSize-profile.MinValueSize+1)
	value := generateRandomString(valueSize, profile.EscapeFrequency, rng)
	return fmt.Sprintf("\"%s\"=\"%s\"", escapeValueName(name), value)
}

// generateDWORDValue creates a REG_DWORD value.
func generateDWORDValue(name string, rng *rand.Rand) string {
	dwordValue := rng.Uint32()
	return fmt.Sprintf("\"%s\"=dword:%08x", escapeValueName(name), dwordValue)
}

// generateBinaryValue creates a REG_BINARY (hex:) value.
func generateBinaryValue(name string, profile Profile, rng *rand.Rand) string {
	size := profile.MinValueSize + rng.IntN(profile.MaxValueSize-profile.MinValueSize+1)
	hexData := generateHexData(size, rng)
	return fmt.Sprintf("\"%s\"=hex:%s", escapeValueName(name), hexData)
}

// generateExpandSZValue creates a REG_EXPAND_SZ (hex(2):) value.
func generateExpandSZValue(name string, profile Profile, rng *rand.Rand) string {
	valueSize := profile.MinValueSize + rng.IntN(profile.MaxValueSize-profile.MinValueSize+1)
	value := generateRandomString(valueSize, 0, rng) // No escaping for hex-encoded
	// Convert to UTF-16LE hex
	hexData := stringToUTF16LEHex(value)
	return fmt.Sprintf("\"%s\"=hex(2):%s", escapeValueName(name), hexData)
}

// generateMultiSZValue creates a REG_MULTI_SZ (hex(7):) value.
func generateMultiSZValue(name string, profile Profile, rng *rand.Rand) string {
	numStrings := 1 + rng.IntN(5) // 1-5 strings
	strings := make([]string, 0, numStrings)
	for range numStrings {
		strSize := profile.MinValueSize/numStrings + rng.IntN(20)
		strings = append(strings, generateRandomString(strSize, 0, rng))
	}
	// Convert to UTF-16LE hex with null separators
	hexData := multiStringToUTF16LEHex(strings)
	return fmt.Sprintf("\"%s\"=hex(7):%s", escapeValueName(name), hexData)
}

// generateRandomString creates a random string with optional escape sequences.
func generateRandomString(length int, escapeFreq float64, rng *rand.Rand) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-. "
	var result strings.Builder

	for range length {
		// Occasionally add escape sequences
		if escapeFreq > 0 && rng.Float64() < escapeFreq {
			switch rng.IntN(2) {
			case 0:
				result.WriteString("\\\\") // Escaped backslash
				continue
			case 1:
				result.WriteString("\\\"") // Escaped quote
				continue
			}
		}
		result.WriteByte(charset[rng.IntN(len(charset))])
	}

	return result.String()
}

// generateHexData creates comma-separated hex bytes.
func generateHexData(numBytes int, rng *rand.Rand) string {
	parts := make([]string, 0, numBytes)
	for range numBytes {
		parts = append(parts, fmt.Sprintf("%02x", rng.IntN(256)))
	}
	return strings.Join(parts, ",")
}

// stringToUTF16LEHex converts a string to UTF-16LE hex format with null terminator.
func stringToUTF16LEHex(s string) string {
	// Simple ASCII to UTF-16LE conversion (each char becomes 2 bytes)
	parts := make([]string, 0, len(s)*2+2)
	for _, ch := range s {
		// Low byte
		parts = append(parts, fmt.Sprintf("%02x", byte(ch)))
		// High byte (0 for ASCII)
		parts = append(parts, "00")
	}
	// Add null terminator
	parts = append(parts, "00", "00")
	return strings.Join(parts, ",")
}

// multiStringToUTF16LEHex converts multiple strings to UTF-16LE hex format.
func multiStringToUTF16LEHex(strs []string) string {
	// Calculate approximate capacity: each string char becomes 2 bytes, plus null terminators
	capacity := 0
	for _, s := range strs {
		capacity += len(s)*2 + 2 // each char is 2 bytes plus null terminator
	}
	capacity += 2 // final double null terminator
	parts := make([]string, 0, capacity)
	for _, s := range strs {
		for _, ch := range s {
			parts = append(parts, fmt.Sprintf("%02x", byte(ch)))
			parts = append(parts, "00")
		}
		// Add null terminator between strings
		parts = append(parts, "00", "00")
	}
	// Add double null terminator at end
	parts = append(parts, "00", "00")
	return strings.Join(parts, ",")
}

// Predefined profiles for common test scenarios

// ProfileSmallShallow creates a small file with shallow key hierarchy.
func ProfileSmallShallow() Profile {
	return Profile{
		TargetSize:      10 * 1024, // 10KB
		KeyDepth:        2,
		KeysPerLevel:    3,
		MinValuesPerKey: 1,
		MaxValuesPerKey: 5,
		MinValueSize:    10,
		MaxValueSize:    50,
	}
}

// ProfileMediumDeep creates a medium file with deep key hierarchy.
func ProfileMediumDeep() Profile {
	return Profile{
		TargetSize:      1024 * 1024, // 1MB
		KeyDepth:        8,
		KeysPerLevel:    5,
		MinValuesPerKey: 3,
		MaxValuesPerKey: 10,
		MinValueSize:    50,
		MaxValueSize:    200,
	}
}

// ProfileLargeWide creates a large file with wide key hierarchy.
func ProfileLargeWide() Profile {
	return Profile{
		TargetSize:      10 * 1024 * 1024, // 10MB
		KeyDepth:        3,
		KeysPerLevel:    100,
		MinValuesPerKey: 5,
		MaxValuesPerKey: 20,
		MinValueSize:    100,
		MaxValueSize:    500,
	}
}

// ProfileStringHeavy creates a file with mostly string values.
func ProfileStringHeavy() Profile {
	p := ProfileMediumDeep()
	p.StringValuePct = 0.8
	p.DWORDValuePct = 0.1
	p.BinaryValuePct = 0.1
	return p
}

// ProfileBinaryHeavy creates a file with mostly binary/hex values.
func ProfileBinaryHeavy() Profile {
	p := ProfileMediumDeep()
	p.BinaryValuePct = 0.5
	p.ExpandSZPct = 0.2
	p.MultiSZPct = 0.2
	p.StringValuePct = 0.1
	return p
}

// ProfileWithEscaping creates a file with heavy escape sequences.
func ProfileWithEscaping() Profile {
	p := ProfileMediumDeep()
	p.EscapeFrequency = 0.3
	return p
}

// ProfileWithDeletions creates a file with many delete operations.
func ProfileWithDeletions() Profile {
	p := ProfileMediumDeep()
	p.DeleteKeyPct = 0.2
	p.DeleteValuePct = 0.1
	return p
}
