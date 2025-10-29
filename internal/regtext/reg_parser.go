package regtext

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// RegValue represents a value in a .reg file
type RegValue struct {
	Name string // Value name ("" for default value @)
	Type string // Type: "string", "dword", "binary", "hex", etc.
	Data string // Raw data string from .reg file
}

// RegKey represents a key in a .reg file with its values
type RegKey struct {
	Path   string      // Full key path
	Values []*RegValue // Values under this key
}

// RegStats contains statistics parsed from a .reg file
type RegStats struct {
	KeyCount   int       // Number of registry keys
	ValueCount int       // Number of registry values
	Keys       []string  // All key paths (for validation)
	Structure  []*RegKey // Full structure with values (for detailed validation)
}

// ParseRegFile parses a Windows .reg file and returns statistics
// .reg format:
//   - Lines starting with [ are keys: [\Path\To\Key]
//   - Lines with = are values: "ValueName"=...
//   - Lines with @= are default values: @=...
//
// Note: Hivex exports .reg files in the local encoding (usually Windows-1252/Latin-1 on Linux).
// We need to convert to UTF-8 to match gohivex's correct UTF-16LE decoding from hive files.
func ParseRegFile(r io.Reader) (*RegStats, error) {
	stats := &RegStats{
		Keys:      make([]string, 0, InitialKeyCapacity),
		Structure: make([]*RegKey, 0, InitialKeyCapacity),
	}

	// Wrap reader with Windows-1252 (Latin-1) to UTF-8 decoder
	// Hivex exports in local encoding, which is typically Windows-1252 on Linux
	decoder := charmap.Windows1252.NewDecoder()
	utf8Reader := transform.NewReader(r, decoder)

	scanner := bufio.NewScanner(utf8Reader)
	// Increase buffer size for long lines (some .reg files have huge binary values)
	buf := make([]byte, 0, ScannerInitialBufferSize)
	scanner.Buffer(buf, ScannerMaxLineSize)

	var currentKey *RegKey

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, CommentPrefix) {
			continue
		}

		// Skip header
		if strings.HasPrefix(line, RegFileHeader[:len("Windows Registry Editor")]) {
			continue
		}

		// Key: [Path]
		if strings.HasPrefix(line, KeyOpenBracket) && strings.HasSuffix(line, KeyCloseBracket) {
			keyPath := strings.TrimSuffix(strings.TrimPrefix(line, KeyOpenBracket), KeyCloseBracket)
			stats.Keys = append(stats.Keys, keyPath)
			stats.KeyCount++

			// Create new key structure
			currentKey = &RegKey{
				Path:   keyPath,
				Values: make([]*RegValue, 0),
			}
			stats.Structure = append(stats.Structure, currentKey)
			continue
		}

		// Value: "Name"=... or @=...
		if currentKey != nil && strings.Contains(line, ValueAssignment) {
			// Check if it's a value line (not a continuation of previous line)
			if strings.HasPrefix(line, Quote) || strings.HasPrefix(line, DefaultValuePrefix) {
				stats.ValueCount++

				// Parse value
				value := parseRegValue(line)
				if value != nil {
					currentKey.Values = append(currentKey.Values, value)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning .reg file: %w", err)
	}

	return stats, nil
}

// parseRegValue parses a single value line from a .reg file
func parseRegValue(line string) *RegValue {
	// Handle default value: @="value" or @=hex:...
	if strings.HasPrefix(line, DefaultValuePrefix) {
		valueData := strings.TrimPrefix(line, DefaultValuePrefix)
		valueType := detectValueType(valueData)
		return &RegValue{
			Name: "", // Default value has empty name
			Type: valueType,
			Data: valueData,
		}
	}

	// Handle named value: "Name"=value
	// Find the closing quote of the value name
	if !strings.HasPrefix(line, Quote) {
		return nil
	}

	// Find the closing quote (accounting for escaped quotes)
	closingQuotePos := findClosingQuote(line)
	if closingQuotePos == -1 || closingQuotePos+1 >= len(line) || string(line[closingQuotePos+1]) != ValueAssignment {
		return nil
	}

	namePart := line[1:closingQuotePos] // Skip opening quote
	valuePart := line[closingQuotePos+2:] // Skip closing quote and =

	// Unescape the name
	name := unescapeRegString(namePart)

	valueType := detectValueType(valuePart)

	return &RegValue{
		Name: name,
		Type: valueType,
		Data: valuePart,
	}
}
