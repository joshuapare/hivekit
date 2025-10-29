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
				ops = append(ops, types.OpDeleteKey{Path: path, Recursive: true})
				current = ""
				continue
			}
			current = section
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

	// Check for typed hex values like hex(2), hex(7)
	if typeNum, found := parseHexValueType(payload); found {
		switch typeNum {
		case "2":
			typ = types.REG_EXPAND_SZ
		case "7":
			typ = types.REG_MULTI_SZ
		default:
			typ = types.REG_BINARY
		}
	}

	// Parse the hex bytes
	data, err := parseHexBytes(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("regtext: %w", err)
	}

	return typ, data, nil
}
