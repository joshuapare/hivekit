package regtext

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// ParseReg converts .reg text into edit operations.
func ParseReg(data []byte, opts types.RegParseOptions) ([]types.EditOp, error) {
	text, err := decodeInput(data, opts.InputEncoding)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(strings.NewReader(text))
	seenHeader := false
	var ops []types.EditOp
	seenKeys := make(map[string]bool)
	var current string

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimRight(line, CR)
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, CommentPrefix) {
			continue
		}
		if !seenHeader {
			if trim != RegFileHeader {
				return nil, errors.New("regtext: missing header")
			}
			seenHeader = true
			continue
		}
		if strings.HasPrefix(trim, KeyOpenBracket) {
			if !strings.HasSuffix(trim, KeyCloseBracket) {
				return nil, fmt.Errorf("regtext: malformed section %q", trim)
			}
			section := strings.TrimSuffix(strings.TrimPrefix(trim, KeyOpenBracket), KeyCloseBracket)
			if strings.HasPrefix(section, DeleteKeyPrefix) {
				path := strings.TrimSpace(section[1:])
				// Strip prefix from delete key path
				strippedPath, err := stripPrefix(path, opts)
				if err != nil {
					return nil, err
				}
				ops = append(ops, types.OpDeleteKey{Path: strippedPath, Recursive: true})
				current = ""
				continue
			}
			// Strip prefix from regular key path
			strippedSection, err := stripPrefix(section, opts)
			if err != nil {
				return nil, err
			}
			current = strippedSection
			if _, ok := seenKeys[current]; !ok {
				ops = append(ops, types.OpCreateKey{Path: current})
				seenKeys[current] = true
			}
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("regtext: value without section: %q", trim)
		}
		op, err := parseValueLine(current, trim)
		if err != nil {
			return nil, err
		}
		if op != nil {
			ops = append(ops, op)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ops, nil
}

func parseValueLine(path, line string) (types.EditOp, error) {
	if strings.HasPrefix(line, DefaultValuePrefix) {
		return parseValue(path, "", line[len(DefaultValuePrefix):])
	}
	if !strings.HasPrefix(line, Quote) {
		return nil, fmt.Errorf("regtext: malformed value line %q", line)
	}
	end := findClosingQuote(line)
	if end < 0 {
		return nil, fmt.Errorf("regtext: unterminated value name in %q", line)
	}
	name := unescapeRegString(line[1:end])
	rest := line[end+1:]
	if !strings.HasPrefix(rest, ValueAssignment) {
		return nil, fmt.Errorf("regtext: missing '=' in %q", line)
	}
	return parseValue(path, name, rest[1:])
}

func parseValue(path, name, payload string) (types.EditOp, error) {
	payload = strings.TrimSpace(payload)
	if payload == DeleteValueToken {
		return types.OpDeleteValue{Path: path, Name: name}, nil
	}
	if strings.HasPrefix(payload, Quote) {
		if !strings.HasSuffix(payload, Quote) {
			return nil, fmt.Errorf("regtext: unterminated string %q", payload)
		}
		value := unescapeRegString(payload[1 : len(payload)-1])
		return types.OpSetValue{Path: path, Name: name, Type: types.REG_SZ, Data: encodeUTF16LEZeroTerminated(value)}, nil
	}
	if strings.HasPrefix(payload, DWORDPrefix) {
		hexPart := payload[len(DWORDPrefix):]
		if len(hexPart) != DWORDHexLength {
			return nil, fmt.Errorf("regtext: invalid dword %q", payload)
		}
		n, err := strconv.ParseUint(hexPart, 16, 32)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, format.DWORDSize)
		binary.LittleEndian.PutUint32(buf, uint32(n))
		return types.OpSetValue{Path: path, Name: name, Type: types.REG_DWORD, Data: buf}, nil
	}
	if strings.HasPrefix(payload, ValueTypeHex) {
		typ, data, err := parseHexPayload(payload)
		if err != nil {
			return nil, err
		}
		return types.OpSetValue{Path: path, Name: name, Type: typ, Data: data}, nil
	}
	return nil, fmt.Errorf("regtext: unsupported value %q", payload)
}

func parseHexPayload(payload string) (types.RegType, []byte, error) {
	var typ = types.REG_BINARY

	// Check for typed hex values like hex(0), hex(2), hex(7), hex(b)
	if typeNum, found := parseHexValueType(payload); found {
		// Parse the type number (supports both decimal and hex notation)
		var parsed uint64
		var err error

		// Try parsing as hex first (for types like 'b' for REG_QWORD)
		parsed, err = strconv.ParseUint(typeNum, 16, 32)
		if err != nil {
			// If hex parsing fails, it might be an invalid type
			return 0, nil, fmt.Errorf("regtext: invalid hex type number %q", typeNum)
		}

		// Convert to RegType - this preserves all type values (0-11 and beyond)
		typ = types.RegType(parsed)
	}

	// Parse the hex bytes
	data, err := parseHexBytes(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("regtext: %w", err)
	}

	return typ, data, nil
}

// stripPrefix removes registry root key prefixes from a path according to the
// provided options. It handles:
// - Root key alias expansion (HKLM → HKEY_LOCAL_MACHINE)
// - Manual prefix stripping (opts.Prefix)
// - Automatic standard prefix detection (opts.AutoPrefix)
func stripPrefix(path string, opts types.RegParseOptions) (string, error) {
	if path == "" {
		return "", nil
	}

	// First, expand any root key aliases
	path = expandRootKeyAlias(path)

	// If no prefix stripping is requested, strip only the root key name
	// (for backward compatibility with old behavior)
	if opts.Prefix == "" && !opts.AutoPrefix {
		return stripRootKeyOnly(path), nil
	}

	// Manual prefix stripping
	if opts.Prefix != "" {
		stripped, err := tryStripPrefix(path, opts.Prefix)
		if err != nil {
			return "", err
		}
		return stripped, nil
	}

	// AutoPrefix: try standard Windows registry prefixes
	if opts.AutoPrefix {
		standardPrefixes := []string{
			HKEYLocalMachine + Backslash + "SOFTWARE",
			HKEYLocalMachine + Backslash + "SYSTEM",
			HKEYLocalMachine + Backslash + "SAM",
			HKEYLocalMachine + Backslash + "SECURITY",
			HKEYLocalMachine + Backslash + "HARDWARE",
			HKEYCurrentUser,
			HKEYUsers,
			HKEYClassesRoot,
			HKEYCurrentConfig,
		}

		for _, prefix := range standardPrefixes {
			if hasPrefix(path, prefix) {
				// Strip the prefix, keeping the remainder
				stripped := path[len(prefix):]
				// Remove leading backslash if present
				stripped = strings.TrimPrefix(stripped, Backslash)
				return stripped, nil
			}
		}
		// If no standard prefix matched, return as-is (could be a relative path)
	}

	return path, nil
}

// expandRootKeyAlias expands abbreviated root key names to their full forms.
// Examples: HKLM → HKEY_LOCAL_MACHINE, HKCU → HKEY_CURRENT_USER
func expandRootKeyAlias(path string) string {
	aliases := map[string]string{
		HKEYLocalMachineShort:   HKEYLocalMachine,
		HKEYCurrentUserShort:    HKEYCurrentUser,
		HKEYClassesRootShort:    HKEYClassesRoot,
		HKEYUsersShort:          HKEYUsers,
		HKEYCurrentConfigShort:  HKEYCurrentConfig,
	}

	for short, long := range aliases {
		if hasPrefix(path, short) {
			// Replace the short form with the long form
			return long + path[len(short):]
		}
	}

	return path
}

// tryStripPrefix attempts to strip the given prefix from the path (case-insensitive).
// Returns an error if the path doesn't start with the expected prefix.
func tryStripPrefix(path, prefix string) (string, error) {
	if !hasPrefix(path, prefix) {
		return "", fmt.Errorf("regtext: path %q does not start with expected prefix %q", path, prefix)
	}

	// Strip the prefix
	stripped := path[len(prefix):]
	// Remove leading backslash if present
	stripped = strings.TrimPrefix(stripped, Backslash)

	return stripped, nil
}

// hasPrefix performs a case-insensitive prefix check.
func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return strings.EqualFold(s[:len(prefix)], prefix)
}

// stripRootKeyOnly strips only the root key name (HKEY_LOCAL_MACHINE\, etc.)
// but keeps the rest of the path. This maintains backward compatibility with
// the old normalizePath behavior.
func stripRootKeyOnly(path string) string {
	rootKeys := []string{
		HKEYLocalMachine + Backslash,
		HKEYCurrentUser + Backslash,
		HKEYUsers + Backslash,
		HKEYClassesRoot + Backslash,
		HKEYCurrentConfig + Backslash,
	}

	for _, rootKey := range rootKeys {
		if hasPrefix(path, rootKey) {
			return path[len(rootKey):]
		}
	}

	return path
}
