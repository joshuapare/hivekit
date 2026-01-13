package regtext

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// ParseReg converts .reg text into edit operations.
func ParseReg(data []byte, opts types.RegParseOptions) ([]types.EditOp, error) {
	// Decode to UTF-8 bytes if needed (avoids string allocation for UTF-8 files)
	textBytes, err := decodeInputToBytes(data, opts.InputEncoding)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(textBytes))
	// Increase buffer size for long lines (some .reg files have huge binary values)
	buf := make([]byte, 0, ScannerInitialBufferSize)
	scanner.Buffer(buf, ScannerMaxLineSize)

	seenHeader := false
	var ops []types.EditOp
	seenKeys := make(map[string]bool)
	var current string

	for scanner.Scan() {
		line := scanner.Bytes() // Use Bytes() instead of Text() - no allocation
		line = bytes.TrimRight(line, "\r")
		trim := bytes.TrimSpace(line)
		if len(trim) == 0 || bytes.HasPrefix(trim, []byte(CommentPrefix)) {
			continue
		}
		if !seenHeader {
			if string(trim) == RegFileHeader {
				seenHeader = true
				continue
			}
			// If header not found, check if we allow missing header
			if !opts.AllowMissingHeader {
				return nil, errors.New("regtext: missing header")
			}
			// Allow missing header - mark as seen and fall through to process this line
			seenHeader = true
		}
		if bytes.HasPrefix(trim, []byte(KeyOpenBracket)) {
			if !bytes.HasSuffix(trim, []byte(KeyCloseBracket)) {
				return nil, fmt.Errorf("regtext: malformed section %q", trim)
			}
			section := bytes.TrimSuffix(bytes.TrimPrefix(trim, []byte(KeyOpenBracket)), []byte(KeyCloseBracket))
			if bytes.HasPrefix(section, []byte(DeleteKeyPrefix)) {
				path := string(bytes.TrimSpace(section[1:]))
				ops = append(ops, types.OpDeleteKey{Path: path, Recursive: true})
				current = ""
				continue
			}
			current = string(section)
			if _, ok := seenKeys[current]; !ok {
				ops = append(ops, types.OpCreateKey{Path: current})
				seenKeys[current] = true
			}
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("regtext: value without section: %q", trim)
		}
		op, parseErr := parseValueLineBytes(current, trim)
		if parseErr != nil {
			return nil, parseErr
		}
		if op != nil {
			ops = append(ops, op)
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, scanErr
	}
	return ops, nil
}

// parseValueLineBytes parses a value line from bytes (Phase 1 optimization).
func parseValueLineBytes(path string, line []byte) (types.EditOp, error) {
	if bytes.HasPrefix(line, []byte(DefaultValuePrefix)) {
		return parseValueBytes(path, "", line[len(DefaultValuePrefix):])
	}
	if !bytes.HasPrefix(line, []byte(Quote)) {
		return nil, fmt.Errorf("regtext: malformed value line %q", line)
	}
	end := findClosingQuoteBytes(line)
	if end < 0 {
		return nil, fmt.Errorf("regtext: unterminated value name in %q", line)
	}
	name := unescapeRegStringBytes(line[1:end])
	rest := line[end+1:]
	if !bytes.HasPrefix(rest, []byte(ValueAssignment)) {
		return nil, fmt.Errorf("regtext: missing '=' in %q", line)
	}
	return parseValueBytes(path, name, rest[1:])
}

// parseValueBytes parses a value from bytes (Phase 1 optimization).
func parseValueBytes(path, name string, payload []byte) (types.EditOp, error) {
	payload = bytes.TrimSpace(payload)
	if string(payload) == DeleteValueToken {
		return types.OpDeleteValue{Path: path, Name: name}, nil
	}
	if bytes.HasPrefix(payload, []byte(Quote)) {
		if !bytes.HasSuffix(payload, []byte(Quote)) {
			return nil, fmt.Errorf("regtext: unterminated string %q", payload)
		}
		value := unescapeRegStringBytes(payload[1 : len(payload)-1])
		return types.OpSetValue{
			Path: path,
			Name: name,
			Type: types.REG_SZ,
			Data: encodeUTF16LEZeroTerminated(value),
		}, nil
	}
	if bytes.HasPrefix(payload, []byte(DWORDPrefix)) {
		hexPart := payload[len(DWORDPrefix):]
		if len(hexPart) != DWORDHexLength {
			return nil, fmt.Errorf("regtext: invalid dword %q", payload)
		}
		n, err := strconv.ParseUint(string(hexPart), 16, 32)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, format.DWORDSize)
		binary.LittleEndian.PutUint32(buf, uint32(n))
		return types.OpSetValue{Path: path, Name: name, Type: types.REG_DWORD, Data: buf}, nil
	}
	if bytes.HasPrefix(payload, []byte(ValueTypeHex)) {
		typ, data, err := parseHexPayloadBytes(payload)
		if err != nil {
			return nil, err
		}
		return types.OpSetValue{Path: path, Name: name, Type: typ, Data: data}, nil
	}
	return nil, fmt.Errorf("regtext: unsupported value %q", payload)
}

// parseHexPayloadBytes parses hex payload from bytes (Phase 1 optimization).
func parseHexPayloadBytes(payload []byte) (types.RegType, []byte, error) {
	var typ = types.REG_BINARY

	// Check for typed hex values like hex(1), hex(2), hex(7), etc.
	if typeNum, found := parseHexValueTypeBytes(payload); found {
		switch typeNum {
		case "0":
			typ = types.REG_NONE
		case "1":
			typ = types.REG_SZ
		case "2":
			typ = types.REG_EXPAND_SZ
		case "3":
			typ = types.REG_BINARY
		case "4":
			typ = types.REG_DWORD
		case "5":
			typ = types.REG_DWORD_BE
		case "6":
			typ = types.REG_LINK
		case "7":
			typ = types.REG_MULTI_SZ
		case "11":
			typ = types.REG_QWORD
		default:
			typ = types.REG_BINARY
		}
	}

	// Parse the hex bytes
	data, err := parseHexBytesFromBytes(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("regtext: %w", err)
	}

	return typ, data, nil
}
