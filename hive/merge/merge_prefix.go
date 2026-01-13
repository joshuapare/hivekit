package merge

import (
	"context"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// MergeRegTextWithPrefix applies regtext operations scoped under a prefix path.
//
// This function parses Windows .reg file format and applies all operations under
// a specified prefix path. This is useful for merging registry deltas that should
// be scoped to a specific subtree (e.g., merging SOFTWARE subtrees).
//
// Behavior:
//   - Parses the regtext into individual operations
//   - Prepends the prefix to each operation's path
//   - Applies all operations in a single transaction
//
// The context can be used to cancel the operation. If cancelled during apply,
// partial operations may have been applied.
//
// Example:
//
//	regText := `Windows Registry Editor Version 5.00
//
//	[Microsoft\Windows]
//	"Version"="10.0"
//	`
//
//	// Apply under SOFTWARE prefix
//	applied, err := merge.MergeRegTextWithPrefix(
//	    ctx,
//	    "my.hive",
//	    regText,
//	    "SOFTWARE",
//	    nil,
//	)
//	if err != nil {
//	    return err
//	}
//	// Result: SOFTWARE\Microsoft\Windows\Version="10.0"
//
// Parameters:
//   - ctx: Context for cancellation support
//   - hivePath: Path to existing hive file to modify
//   - regText: Windows .reg file format text
//   - prefix: Path prefix to prepend to all operations (e.g., "SOFTWARE" or "SOFTWARE\Microsoft")
//   - opts: Merge options (can be nil for defaults)
//
// Returns:
//   - Applied: Statistics about operations applied
//   - error: If parsing fails, hive cannot be opened, or operations fail
func MergeRegTextWithPrefix(ctx context.Context, hivePath string, regText string, prefix string, opts *Options) (Applied, error) {
	// Get parse options from opts
	var parseOpts types.RegParseOptions
	if opts != nil {
		parseOpts = opts.ParseOptions
	}

	// Parse regtext with prefix transformation (validates input before opening hive)
	plan, err := PlanFromRegTextWithPrefixOpts(regText, prefix, parseOpts)
	if err != nil {
		return Applied{}, err
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		return Applied{}, fmt.Errorf("open hive: %w", err)
	}
	defer h.Close()

	// Create session with provided options
	if opts == nil {
		opts = &Options{}
	}
	sess, err := NewSession(ctx, h, *opts)
	if err != nil {
		return Applied{}, fmt.Errorf("create session: %w", err)
	}
	defer sess.Close(ctx)

	// Apply plan
	result, err := sess.ApplyWithTx(ctx, plan)
	if err != nil {
		return result, fmt.Errorf("apply operations: %w", err)
	}

	return result, nil
}

// transformOp prepends prefix to an operation's path.
func transformOp(op types.EditOp, prefix []string) (types.EditOp, error) {
	switch op := op.(type) {
	case types.OpCreateKey:
		// Strip hive root and split path
		pathParts := splitPath(op.Path)
		// Prepend prefix
		newPath := append(prefix, pathParts...)
		// Join back to string
		return types.OpCreateKey{
			Path: joinPath(newPath),
		}, nil

	case types.OpSetValue:
		// Strip hive root and split path
		pathParts := splitPath(op.Path)
		// Prepend prefix
		newPath := append(prefix, pathParts...)
		// Join back to string
		return types.OpSetValue{
			Path: joinPath(newPath),
			Name: op.Name,
			Type: op.Type,
			Data: op.Data,
		}, nil

	case types.OpDeleteValue:
		// Strip hive root and split path
		pathParts := splitPath(op.Path)
		// Prepend prefix
		newPath := append(prefix, pathParts...)
		// Join back to string
		return types.OpDeleteValue{
			Path: joinPath(newPath),
			Name: op.Name,
		}, nil

	case types.OpDeleteKey:
		// Strip hive root and split path
		pathParts := splitPath(op.Path)
		// Prepend prefix
		newPath := append(prefix, pathParts...)
		// Join back to string
		return types.OpDeleteKey{
			Path:      joinPath(newPath),
			Recursive: op.Recursive,
		}, nil

	default:
		return nil, fmt.Errorf("unknown operation type: %T", op)
	}
}

// convertEditOpToMergeOp converts a types.EditOp to a merge.Op.
func convertEditOpToMergeOp(editOp types.EditOp) (*Op, error) {
	switch op := editOp.(type) {
	case types.OpCreateKey:
		pathParts := splitPath(op.Path)
		return &Op{
			Type:    OpEnsureKey,
			KeyPath: pathParts,
		}, nil

	case types.OpSetValue:
		pathParts := splitPath(op.Path)
		return &Op{
			Type:      OpSetValue,
			KeyPath:   pathParts,
			ValueName: op.Name,
			ValueType: uint32(op.Type),
			Data:      op.Data,
		}, nil

	case types.OpDeleteValue:
		pathParts := splitPath(op.Path)
		return &Op{
			Type:      OpDeleteValue,
			KeyPath:   pathParts,
			ValueName: op.Name,
		}, nil

	case types.OpDeleteKey:
		pathParts := splitPath(op.Path)
		return &Op{
			Type:    OpDeleteKey,
			KeyPath: pathParts,
		}, nil

	default:
		return nil, fmt.Errorf("unknown operation type: %T", editOp)
	}
}

// stripHiveRootPrefix removes common hive root prefixes from a path string.
func stripHiveRootPrefix(path string) string {
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
	return path
}

// splitPath splits a path string into components.
func splitPath(path string) []string {
	if path == "" {
		return []string{}
	}

	// Strip hive root prefix
	path = stripHiveRootPrefix(path)

	if path == "" {
		return []string{}
	}

	// Split by backslash and filter empty segments
	segments := strings.Split(path, "\\")
	result := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			result = append(result, seg)
		}
	}

	return result
}

// joinPath joins path components into a string.
func joinPath(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\\")
}

// RegType converts to uint32 for internal use.
func regTypeToUint32(rt types.RegType) uint32 {
	switch rt {
	case types.REG_NONE:
		return format.REGNone
	case types.REG_SZ:
		return format.REGSZ
	case types.REG_EXPAND_SZ:
		return format.REGExpandSZ
	case types.REG_BINARY:
		return format.REGBinary
	case types.REG_DWORD_LE:
		return format.REGDWORD
	case types.REG_DWORD_BE:
		return format.REGDWORDBigEndian
	case types.REG_LINK:
		return format.REGLink
	case types.REG_MULTI_SZ:
		return format.REGMultiSZ
	case types.REG_QWORD:
		return format.REGQWORD
	default:
		return uint32(rt) // For unknown types, use the raw value
	}
}
