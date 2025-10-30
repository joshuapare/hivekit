package regtext

import (
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
func (c *Codec) ParseReg(regText []byte, opts types.RegParseOptions) ([]types.EditOp, error) {
	// Use the parser.go ParseReg function which supports prefix stripping
	return ParseReg(regText, opts)
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
			return types.REG_SZ, encodeUTF16LEZeroTerminated(value.Data), nil
		}
		return types.REG_BINARY, data, nil
	}
}
