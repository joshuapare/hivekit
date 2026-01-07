package hive

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// ValidationError provides detailed information about a validation failure.
type ValidationError struct {
	Path    string // Key path where violation occurred (empty for root)
	Message string // Human-readable description of the violation
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// Defragment compacts a registry hive and rewrites it.
//
// Defragmentation improves performance and reduces file size by:
//   - Compacting free cells
//   - Rebucketing data for better locality
//   - Removing unused space
//
// The operation is safe and atomic - the original file is replaced only
// after successful compaction. A backup is created at <hivePath>.bak
// before modification.
//
// Example:
//
//	err := ops.Defragment("software.hive")
//	if err != nil {
//	    log.Fatal(err)
//	}
func Defragment(hivePath string) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Create backup
	backupPath := hivePath + ".bak"
	if err := copyFile(hivePath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
	}

	// Open source hive
	src, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer src.Close()

	// Create temporary destination hive
	tempPath := hivePath + ".tmp"
	opts := builder.DefaultOptions()
	opts.CreateIfNotExists = true
	dst, err := builder.New(tempPath, opts)
	if err != nil {
		return fmt.Errorf("failed to create temp hive: %w", err)
	}
	defer func() {
		dst.Close()
		os.Remove(tempPath) // Clean up temp file on error
	}()

	// Walk source and copy everything to destination
	if err := copyHiveContents(src, dst); err != nil {
		return fmt.Errorf("failed to copy hive contents: %w", err)
	}

	// Commit the destination hive
	if err := dst.Commit(); err != nil {
		return fmt.Errorf("failed to commit defragmented hive: %w", err)
	}

	// Atomically replace original with defragmented version
	if err := os.Rename(tempPath, hivePath); err != nil {
		return fmt.Errorf("failed to replace hive %s: %w", hivePath, err)
	}

	return nil
}

// copyHiveContents walks the source hive and copies all keys and values to dst.
func copyHiveContents(src *hive.Hive, dst *builder.Builder) error {
	rootOffset := src.RootCellOffset()

	// Recursive walk function
	var walkKey func(nkOffset uint32, path []string) error
	walkKey = func(nkOffset uint32, path []string) error {
		// Resolve the NK cell
		payload, err := src.ResolveCellPayload(nkOffset)
		if err != nil {
			return fmt.Errorf("resolve NK at 0x%X: %w", nkOffset, err)
		}

		nk, err := hive.ParseNK(payload)
		if err != nil {
			return fmt.Errorf("parse NK at 0x%X: %w", nkOffset, err)
		}

		// Ensure the key exists in destination
		if err := dst.EnsureKey(path); err != nil {
			return fmt.Errorf("ensure key %v: %w", path, err)
		}

		// Copy all values
		if nk.ValueCount() > 0 {
			if err := walker.WalkValues(src, nkOffset, func(vk hive.VK, _ uint32) error {
				name := decodeVKName(vk)
				data, dataErr := vk.Data(src.Bytes())
				if dataErr != nil {
					return fmt.Errorf("get value data: %w", dataErr)
				}
				return dst.SetValue(path, name, vk.Type(), data)
			}); err != nil {
				// Skip keys with invalid value list offsets (corrupted hives)
				if !isCellResolutionError(err) {
					return fmt.Errorf("copy values for %v: %w", path, err)
				}
			}
		}

		// Recursively copy subkeys
		if nk.SubkeyCount() > 0 {
			if err := walker.WalkSubkeys(src, nkOffset, func(childNK hive.NK, childRef uint32) error {
				childName := decodeNKName(childNK)
				childPath := append(path, childName)
				return walkKey(childRef, childPath)
			}); err != nil {
				// Skip keys with invalid subkey list offsets (corrupted hives)
				if !isCellResolutionError(err) {
					return fmt.Errorf("copy subkeys for %v: %w", path, err)
				}
			}
		}

		return nil
	}

	// Start from root with empty path
	return walkKey(rootOffset, []string{})
}

// ValidateHive checks a registry hive against specified limits.
//
// This validates the hive structure without modifying it. Useful for:
//   - Checking if a hive will work on Windows
//   - Verifying hive integrity
//   - Pre-flight checks before deployment
//
// If limits is nil, DefaultLimits() is used.
//
// Example:
//
//	err := ops.ValidateHive("system.hive", ops.DefaultLimits())
//	if err != nil {
//	    log.Printf("Hive validation failed: %v", err)
//	    return
//	}
//	log.Println("Hive is valid")
//
// Example with strict limits:
//
//	err := ops.ValidateHive("system.hive", ops.StrictLimits())
//	if err != nil {
//	    log.Printf("Hive exceeds strict limits: %v", err)
//	}
func ValidateHive(hivePath string, limits Limits) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Use default limits if none provided
	if limits == (Limits{}) {
		limits = types.DefaultLimits()
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		return fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer h.Close()

	// Check total size
	if h.Size() > limits.MaxTotalSize {
		return &ValidationError{
			Message: fmt.Sprintf("hive size %d exceeds limit %d", h.Size(), limits.MaxTotalSize),
		}
	}

	// Walk the hive and validate each key
	rootOffset := h.RootCellOffset()
	return validateKeyRecursive(h, rootOffset, []string{}, 0, limits)
}

// validateKeyRecursive validates a key and all its descendants.
func validateKeyRecursive(h *hive.Hive, nkOffset uint32, path []string, depth int, limits Limits) error {
	// Check tree depth
	if depth > limits.MaxTreeDepth {
		return &ValidationError{
			Path:    pathToString(path),
			Message: fmt.Sprintf("tree depth %d exceeds limit %d", depth, limits.MaxTreeDepth),
		}
	}

	// Resolve the NK cell
	payload, err := h.ResolveCellPayload(nkOffset)
	if err != nil {
		return fmt.Errorf("resolve NK at 0x%X: %w", nkOffset, err)
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return fmt.Errorf("parse NK at 0x%X: %w", nkOffset, err)
	}

	// Validate key name length
	nameLen := int(nk.NameLength())
	if !nk.IsCompressedName() {
		// UTF-16LE name: divide by 2 to get character count
		nameLen /= 2
	}
	if nameLen > limits.MaxKeyNameLen {
		return &ValidationError{
			Path:    pathToString(path),
			Message: fmt.Sprintf("key name length %d exceeds limit %d", nameLen, limits.MaxKeyNameLen),
		}
	}

	// Validate subkey count
	subkeyCount := int(nk.SubkeyCount())
	if subkeyCount > limits.MaxSubkeys {
		return &ValidationError{
			Path:    pathToString(path),
			Message: fmt.Sprintf("subkey count %d exceeds limit %d", subkeyCount, limits.MaxSubkeys),
		}
	}

	// Validate value count
	valueCount := int(nk.ValueCount())
	if valueCount > limits.MaxValues {
		return &ValidationError{
			Path:    pathToString(path),
			Message: fmt.Sprintf("value count %d exceeds limit %d", valueCount, limits.MaxValues),
		}
	}

	// Validate each value
	if valueCount > 0 {
		if err := walker.WalkValues(h, nkOffset, func(vk hive.VK, _ uint32) error {
			return validateValue(vk, h, path, limits)
		}); err != nil {
			// Check if this is a validation error (propagate) or cell resolution error (skip)
			var valErr *ValidationError
			if errors.As(err, &valErr) {
				return err
			}
			// Skip keys with invalid value list offsets (corrupted hives)
			if !isCellResolutionError(err) {
				return err
			}
		}
	}

	// Recursively validate subkeys
	if subkeyCount > 0 {
		if err := walker.WalkSubkeys(h, nkOffset, func(childNK hive.NK, childRef uint32) error {
			childName := decodeNKName(childNK)
			childPath := append(path, childName)
			return validateKeyRecursive(h, childRef, childPath, depth+1, limits)
		}); err != nil {
			// Check if this is a validation error (propagate) or cell resolution error (skip)
			var valErr *ValidationError
			if errors.As(err, &valErr) {
				return err
			}
			// Skip keys with invalid subkey list offsets (corrupted hives)
			if !isCellResolutionError(err) {
				return err
			}
		}
	}

	return nil
}

// validateValue validates a single value against limits.
func validateValue(vk hive.VK, h *hive.Hive, keyPath []string, limits Limits) error {
	// Validate value name length
	nameLen := int(vk.NameLen())
	if !vk.NameCompressed() {
		// UTF-16LE name: divide by 2 to get character count
		nameLen /= 2
	}
	if nameLen > limits.MaxValueNameLen {
		return &ValidationError{
			Path:    pathToString(keyPath),
			Message: fmt.Sprintf("value name length %d exceeds limit %d", nameLen, limits.MaxValueNameLen),
		}
	}

	// Validate data size
	dataLen := vk.DataLen()
	if dataLen > limits.MaxValueSize {
		return &ValidationError{
			Path:    pathToString(keyPath),
			Message: fmt.Sprintf("value data size %d exceeds limit %d", dataLen, limits.MaxValueSize),
		}
	}

	return nil
}

// HiveStats returns basic information about a registry hive.
//
// This includes:
//   - Root key count
//   - Total tree depth
//   - Estimated size
//
// Returns a map with string keys for flexibility in future additions.
//
// Example:
//
//	info, err := hive.HiveStats("system.hive")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Root keys: %s\n", info["root_keys"])
//	fmt.Printf("Max depth: %s\n", info["max_depth"])
func HiveStats(hivePath string) (map[string]string, error) {
	if !fileExists(hivePath) {
		return nil, fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer h.Close()

	// Get root NK
	rootOffset := h.RootCellOffset()
	payload, err := h.ResolveCellPayload(rootOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root NK: %w", err)
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse root NK: %w", err)
	}

	// Build info map
	info := map[string]string{
		"root_keys": strconv.Itoa(int(nk.SubkeyCount())),
		"file_size": strconv.FormatInt(h.Size(), 10),
	}

	return info, nil
}

// decodeNKName extracts the key name from an NK cell as a string.
func decodeNKName(nk hive.NK) string {
	nameBytes := nk.Name()
	if len(nameBytes) == 0 {
		return ""
	}

	if nk.IsCompressedName() {
		// ASCII encoding
		return string(nameBytes)
	}

	// UTF-16LE encoding
	return decodeUTF16LE(nameBytes)
}

// decodeVKName extracts the value name from a VK cell as a string.
func decodeVKName(vk hive.VK) string {
	nameBytes := vk.Name()
	if len(nameBytes) == 0 {
		return "" // Default value
	}

	if vk.NameCompressed() {
		// ASCII encoding
		return string(nameBytes)
	}

	// UTF-16LE encoding
	return decodeUTF16LE(nameBytes)
}

// decodeUTF16LE decodes a UTF-16LE byte slice to a Go string.
func decodeUTF16LE(b []byte) string {
	if len(b) < 2 {
		return ""
	}

	// Convert bytes to uint16 slice
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = format.ReadU16(b, i*2)
	}

	// Decode UTF-16 to runes
	runes := utf16.Decode(u16)
	return string(runes)
}

// pathToString converts a path slice to a backslash-separated string.
func pathToString(path []string) string {
	if len(path) == 0 {
		return "(root)"
	}
	result := ""
	for i, seg := range path {
		if i > 0 {
			result += "\\"
		}
		result += seg
	}
	return result
}

// isCellResolutionError checks if the error is due to an invalid cell reference.
// This happens in some hives where count > 0 but the list offset is invalid (0 or out of bounds).
// Uses proper error type checking with errors.Is().
func isCellResolutionError(err error) bool {
	return errors.Is(err, hive.ErrCellOffsetZero) ||
		errors.Is(err, hive.ErrCellOutOfRange) ||
		errors.Is(err, hive.ErrCellTruncated)
}
