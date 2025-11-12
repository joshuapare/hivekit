package builder

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
)

// BuildFromRegText creates a new hive file from .reg file text.
//
// This function parses Windows .reg file format and builds a complete hive from scratch.
// It handles all standard .reg operations including key creation, value setting, and deletions.
//
// The .reg text must have a valid header (e.g., "Windows Registry Editor Version 5.00").
// Hive root prefixes (HKEY_LOCAL_MACHINE\, HKLM\, etc.) are automatically stripped.
//
// Process:
//  1. Parse .reg text into operations
//  2. Create new builder for output hive
//  3. Apply all operations in order
//  4. Commit the hive to disk
//
// Example:
//
//	regText := `Windows Registry Editor Version 5.00
//
//	[HKEY_LOCAL_MACHINE\Software\MyApp]
//	"Version"="1.0.0"
//	"Timeout"=dword:0000001e
//	`
//
//	err := builder.BuildFromRegText("output.hive", regText, nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Parameters:
//   - hivePath: Path where the new hive file will be created
//   - regText: Windows .reg file format text
//   - opts: Builder options (can be nil for defaults)
//
// Returns an error if:
//   - .reg text is malformed
//   - Hive file cannot be created
//   - Any operation fails to apply
func BuildFromRegText(hivePath string, regText string, opts *Options) error {
	// Parse .reg text into operations
	ops, err := regtext.ParseReg([]byte(regText), types.RegParseOptions{
		InputEncoding: "UTF-8", // Most .reg files are UTF-8
	})
	if err != nil {
		return fmt.Errorf("parse .reg text: %w", err)
	}

	return BuildFromOps(hivePath, ops, opts)
}

// BuildFromRegFile creates a new hive file from a .reg file on disk.
//
// This is a convenience wrapper around BuildFromRegText that reads the
// .reg file from disk first.
//
// Example:
//
//	err := builder.BuildFromRegFile("output.hive", "input.reg", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// Parameters:
//   - hivePath: Path where the new hive file will be created
//   - regFilePath: Path to the .reg file to read
//   - opts: Builder options (can be nil for defaults)
func BuildFromRegFile(hivePath string, regFilePath string, opts *Options) error {
	// Read .reg file
	regData, err := os.ReadFile(regFilePath)
	if err != nil {
		return fmt.Errorf("read .reg file: %w", err)
	}

	return BuildFromRegText(hivePath, string(regData), opts)
}

// BuildFromOps creates a new hive from a slice of EditOp operations.
//
// This is the lowest-level function for building hives from parsed operations.
// It's used internally by BuildFromRegText and BuildFromRegFile.
//
// Operations are applied in order. Keys are created implicitly when values
// are set on them.
//
// Parameters:
//   - hivePath: Path where the new hive file will be created
//   - ops: Slice of parsed registry operations
//   - opts: Builder options (can be nil for defaults)
func BuildFromOps(hivePath string, ops []types.EditOp, opts *Options) error {
	// Create new builder
	b, err := New(hivePath, opts)
	if err != nil {
		return fmt.Errorf("create builder: %w", err)
	}
	defer b.Close()

	// Enable deferred mode for bulk building performance
	// This eliminates expensive read-modify-write cycles by accumulating
	// subkey lists in memory and writing them all at once
	b.session.EnableDeferredMode()

	// Apply all operations
	for i, op := range ops {
		if err := applyOp(b, op); err != nil {
			return fmt.Errorf("apply operation %d: %w", i, err)
		}
	}

	// Flush all deferred subkey lists before commit
	if flushedCount, flushErr := b.session.FlushDeferredSubkeys(); flushErr != nil {
		return fmt.Errorf("flush deferred subkeys: %w", flushErr)
	} else if flushedCount > 0 {
		// Successfully flushed deferred updates
		_ = flushedCount // Could log this for debugging
	}

	// Commit the hive
	if err := b.Commit(); err != nil {
		return fmt.Errorf("commit hive: %w", err)
	}

	return nil
}

// applyOp applies a single EditOp to the builder.
func applyOp(b *Builder, op types.EditOp) error {
	switch op := op.(type) {
	case types.OpCreateKey:
		// Strip hive root prefix (if configured) and convert to path segments
		path := b.splitPath(op.Path)
		// Skip root key creation (it already exists)
		if len(path) == 0 {
			return nil
		}
		// Ensure the key exists (idempotent operation)
		// This is the proper way to create empty keys from .reg files
		return b.EnsureKey(path)

	case types.OpSetValue:
		// Strip hive root prefix (if configured) and convert to path segments
		path := b.splitPath(op.Path)

		// Apply the operation based on type
		switch op.Type {
		case types.REG_NONE:
			return b.SetNone(path, op.Name)

		case types.REG_SZ:
			return b.SetString(path, op.Name, decodeUTF16LEString(op.Data))

		case types.REG_EXPAND_SZ:
			return b.SetExpandString(path, op.Name, decodeUTF16LEString(op.Data))

		case types.REG_BINARY:
			return b.SetBinary(path, op.Name, op.Data)

		case types.REG_DWORD: // REG_DWORD_LE is an alias for the same value
			if len(op.Data) != 4 {
				return fmt.Errorf("invalid DWORD data length: %d", len(op.Data))
			}
			dword := binary.LittleEndian.Uint32(op.Data)
			return b.SetDWORD(path, op.Name, dword)

		case types.REG_DWORD_BE:
			if len(op.Data) != 4 {
				return fmt.Errorf("invalid DWORD_BE data length: %d", len(op.Data))
			}
			dword := binary.BigEndian.Uint32(op.Data)
			return b.SetDWORDBigEndian(path, op.Name, dword)

		case types.REG_LINK:
			return b.SetLink(path, op.Name, op.Data)

		case types.REG_MULTI_SZ:
			strs := decodeMultiString(op.Data)
			return b.SetMultiString(path, op.Name, strs)

		case types.REG_QWORD:
			if len(op.Data) != 8 {
				return fmt.Errorf("invalid QWORD data length: %d", len(op.Data))
			}
			qword := binary.LittleEndian.Uint64(op.Data)
			return b.SetQWORD(path, op.Name, qword)

		default:
			// For unknown types, use raw binary
			return b.SetValue(path, op.Name, uint32(op.Type), op.Data)
		}

	case types.OpDeleteValue:
		// Strip hive root prefix (if configured) and convert to path segments
		path := b.splitPath(op.Path)
		return b.DeleteValue(path, op.Name)

	case types.OpDeleteKey:
		// Strip hive root prefix (if configured) and convert to path segments
		path := b.splitPath(op.Path)
		// Note: Builder's DeleteKey always deletes recursively
		// The op.Recursive flag is ignored since that's the builder's behavior
		return b.DeleteKey(path)

	default:
		return fmt.Errorf("unknown operation type: %T", op)
	}
}

// stripHiveRootAndSplit removes the hive root prefix and splits the path into segments.
//
// Examples:
//   - "HKEY_LOCAL_MACHINE\Software\Test" → ["Software", "Test"]
//   - "HKLM\Software\Test" → ["Software", "Test"]
//   - "Software\Test" → ["Software", "Test"]
//   - "" → []
func stripHiveRootAndSplit(path string) []string {
	if path == "" {
		return nil
	}

	// Strip common hive root prefixes
	path = strings.TrimPrefix(path, "HKEY_LOCAL_MACHINE\\")
	path = strings.TrimPrefix(path, "HKEY_CURRENT_USER\\")
	path = strings.TrimPrefix(path, "HKEY_CLASSES_ROOT\\")
	path = strings.TrimPrefix(path, "HKEY_USERS\\")
	path = strings.TrimPrefix(path, "HKEY_CURRENT_CONFIG\\")
	path = strings.TrimPrefix(path, "HKLM\\")
	path = strings.TrimPrefix(path, "HKCU\\")
	path = strings.TrimPrefix(path, "HKCR\\")
	path = strings.TrimPrefix(path, "HKU\\")
	path = strings.TrimPrefix(path, "HKCC\\")

	// Handle edge case of just root with no subpath
	if path == "" {
		return nil
	}

	// Split by backslash
	segments := strings.Split(path, "\\")

	// Filter out empty segments
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}

	return result
}

// decodeUTF16LEString decodes a UTF-16LE byte slice to a Go string.
func decodeUTF16LEString(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	// Remove null terminator if present
	if len(data) >= 2 && data[len(data)-2] == 0 && data[len(data)-1] == 0 {
		data = data[:len(data)-2]
	}

	// Convert UTF-16LE to runes
	runes := make([]rune, 0, len(data)/2)
	for i := 0; i < len(data)-1; i += 2 {
		r := rune(data[i]) | rune(data[i+1])<<8
		if r != 0 { // Skip embedded nulls
			runes = append(runes, r)
		}
	}

	return string(runes)
}

// decodeMultiString decodes a REG_MULTI_SZ value into a string slice.
func decodeMultiString(data []byte) []string {
	if len(data) == 0 {
		return nil
	}

	var result []string
	var current []rune

	for i := 0; i < len(data)-1; i += 2 {
		r := rune(data[i]) | rune(data[i+1])<<8
		if r == 0 {
			// Null terminator - end of string
			if len(current) > 0 {
				result = append(result, string(current))
				current = nil
			} else if len(result) > 0 {
				// Double null - end of list
				break
			}
		} else {
			current = append(current, r)
		}
	}

	// Add final string if present
	if len(current) > 0 {
		result = append(result, string(current))
	}

	return result
}
