package regtext

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Codec provides .reg parsing and export functionality.
type Codec struct{}

// NewCodec creates a new registry codec.
func NewCodec() *Codec {
	return &Codec{}
}

// ParseReg converts .reg text into edit operations.
func (c *Codec) ParseReg(regText []byte, _ types.RegParseOptions) ([]types.EditOp, error) {
	// Parse the .reg file to get structure
	stats, err := ParseRegFile(bytes.NewReader(regText))
	if err != nil {
		return nil, fmt.Errorf("failed to parse .reg file: %w", err)
	}

	// Convert to edit operations
	// Pre-allocate: 1 CreateKey + N SetValue ops per key
	capacity := len(stats.Structure)
	for _, key := range stats.Structure {
		capacity += len(key.Values)
	}
	ops := make([]types.EditOp, 0, capacity)

	for _, key := range stats.Structure {
		// Normalize path (remove HKEY_ prefix if present)
		path := normalizePath(key.Path)

		// Create key operation
		ops = append(ops, types.OpCreateKey{
			Path: path,
		})

		// Create set value operations for each value
		for _, value := range key.Values {
			// Convert reg value to hive value
			regType, data, convertErr := convertRegValue(value)
			if convertErr != nil {
				// Skip invalid values but continue processing
				continue
			}

			ops = append(ops, types.OpSetValue{
				Path: path,
				Name: value.Name,
				Type: regType,
				Data: data,
			})
		}
	}

	return ops, nil
}

// ExportReg walks a subtree and emits .reg text.
func (c *Codec) ExportReg(r types.Reader, root types.NodeID, opts types.RegExportOptions) ([]byte, error) {
	return ExportReg(r, root, opts)
}

// normalizePath removes HKEY_ prefixes and normalizes the path.
func normalizePath(path string) string {
	// Remove common prefixes
	prefixes := []string{
		HKEYLocalMachine + Backslash,
		HKEYLocalMachineShort + Backslash,
		HKEYCurrentUser + Backslash,
		HKEYCurrentUserShort + Backslash,
		HKEYUsers + Backslash,
		HKEYUsersShort + Backslash,
		HKEYClassesRoot + Backslash,
		HKEYClassesRootShort + Backslash,
	}

	for _, prefix := range prefixes {
		if len(path) > len(prefix) && path[:len(prefix)] == prefix {
			return path[len(prefix):]
		}
	}

	return path
}

// convertRegValue converts a .reg value to a hive value type and data.
func convertRegValue(value *RegValue) (types.RegType, []byte, error) {
	switch value.Type {
	case ValueTypeString:
		// Remove surrounding quotes and unescape
		data := value.Data
		if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
			data = data[1 : len(data)-1]
		}
		data = unescapeRegString(data)
		// Convert to UTF-16LE with null terminator
		return types.REG_SZ, encodeUTF16LEZeroTerminated(data), nil

	case ValueTypeDWORD:
		// Parse dword value
		var dw uint32
		if _, err := fmt.Sscanf(value.Data, DWORDPrefix+"%x", &dw); err != nil {
			return 0, nil, fmt.Errorf("invalid dword value: %s", value.Data)
		}
		// Encode as little-endian
		data := make([]byte, format.DWORDSize)
		binary.LittleEndian.PutUint32(data, dw)
		return types.REG_DWORD, data, nil

	case ValueTypeBinary, ValueTypeHex:
		// Parse hex data
		data, err := parseHexBytes(value.Data)
		if err != nil {
			return 0, nil, err
		}
		return types.REG_BINARY, data, nil

	case ValueTypeHex7:
		// REG_MULTI_SZ
		data, err := parseHexBytes(value.Data)
		if err != nil {
			return 0, nil, err
		}
		return types.REG_MULTI_SZ, data, nil

	case ValueTypeHex2:
		// REG_EXPAND_SZ
		data, err := parseHexBytes(value.Data)
		if err != nil {
			return 0, nil, err
		}
		return types.REG_EXPAND_SZ, data, nil

	default:
		// Unknown type - try to parse as hex bytes first
		data, err := parseHexBytes(value.Data)
		if err != nil {
			// Not valid hex - treat as string fallback
			// This is intentional: if hex parsing fails, assume it's plain text
			//nolint:nilerr // Intentionally fallback to string when hex parsing fails
			return types.REG_SZ, encodeUTF16LEZeroTerminated(value.Data), nil
		}
		return types.REG_BINARY, data, nil
	}
}
