package builder

import (
	"errors"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/internal/format"
)

// Public encoding helpers for external use

// EncodeStringHelper encodes a Go string to UTF-16LE for registry use.
// This is exported for convenience when building raw data externally.
func EncodeStringHelper(s string) []byte {
	return encodeString(s)
}

// EncodeDWORDHelper encodes a uint32 to little-endian bytes for registry use.
// This is exported for convenience when building raw data externally.
func EncodeDWORDHelper(v uint32) []byte {
	return encodeDWORD(v)
}

// EncodeQWORDHelper encodes a uint64 to little-endian bytes for registry use.
// This is exported for convenience when building raw data externally.
func EncodeQWORDHelper(v uint64) []byte {
	return encodeQWORD(v)
}

// EncodeMultiStringHelper encodes a string array to REG_MULTI_SZ format.
// This is exported for convenience when building raw data externally.
func EncodeMultiStringHelper(values []string) []byte {
	return encodeMultiString(values)
}

// EncodeDWORDBigEndianHelper encodes a uint32 to big-endian bytes for registry use.
// This is exported for convenience when building raw data externally.
func EncodeDWORDBigEndianHelper(v uint32) []byte {
	return encodeDWORDBigEndian(v)
}

// SplitPath converts a registry path string with backslash separators into a path array.
//
// It strips common hive root prefixes (HKEY_LOCAL_MACHINE, HKLM, etc.) and splits
// on backslashes.
//
// Examples:
//
//	"Software\\MyApp" -> []string{"Software", "MyApp"}
//	"HKEY_LOCAL_MACHINE\\Software\\MyApp" -> []string{"Software", "MyApp"}
//	"HKLM\\System\\CurrentControlSet" -> []string{"System", "CurrentControlSet"}
func SplitPath(path string) []string {
	// Strip hive root prefixes (always strips for backward compatibility)
	path = stripHiveRoot(path, true)

	// Handle empty path
	if path == "" {
		return []string{}
	}

	// Split on backslashes
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

// stripHiveRoot removes common hive root prefixes from a registry path.
// If shouldStrip is false, the path is returned unchanged.
func stripHiveRoot(path string, shouldStrip bool) string {
	if !shouldStrip {
		return path
	}

	// List of common hive root prefixes to strip
	prefixes := []string{
		"HKEY_LOCAL_MACHINE\\",
		"HKEY_CURRENT_USER\\",
		"HKEY_CLASSES_ROOT\\",
		"HKEY_USERS\\",
		"HKEY_CURRENT_CONFIG\\",
		"HKLM\\",
		"HKCU\\",
		"HKCR\\",
		"HKU\\",
		"HKCC\\",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix)
		}
	}

	return path
}

// ParseValueType converts a registry type string to its numeric constant.
//
// Supports both full names and short names:
//   - "REG_SZ", "SZ", "STRING" -> 1
//   - "REG_EXPAND_SZ", "EXPAND_SZ" -> 2
//   - "REG_BINARY", "BINARY" -> 3
//   - "REG_DWORD", "DWORD" -> 4
//   - "REG_DWORD_BIG_ENDIAN", "DWORD_BIG_ENDIAN", "DWORD_BE" -> 5
//   - "REG_LINK", "LINK" -> 6
//   - "REG_MULTI_SZ", "MULTI_SZ" -> 7
//   - "REG_RESOURCE_LIST", "RESOURCE_LIST" -> 8
//   - "REG_FULL_RESOURCE_DESCRIPTOR", "FULL_RESOURCE_DESCRIPTOR" -> 9
//   - "REG_RESOURCE_REQUIREMENTS_LIST", "RESOURCE_REQUIREMENTS_LIST" -> 10
//   - "REG_QWORD", "QWORD" -> 11
//   - "REG_NONE", "NONE" -> 0
//
// Returns an error if the type string is not recognized.
func ParseValueType(typeStr string) (uint32, error) {
	// Normalize to uppercase
	typeStr = strings.ToUpper(strings.TrimSpace(typeStr))

	switch typeStr {
	case "REG_NONE", "NONE":
		return format.REGNone, nil
	case "REG_SZ", "SZ", "STRING":
		return format.REGSZ, nil
	case "REG_EXPAND_SZ", "EXPAND_SZ":
		return format.REGExpandSZ, nil
	case "REG_BINARY", "BINARY":
		return format.REGBinary, nil
	case "REG_DWORD", "DWORD":
		return format.REGDWORD, nil
	case "REG_DWORD_BIG_ENDIAN", "DWORD_BIG_ENDIAN", "DWORD_BE":
		return format.REGDWORDBigEndian, nil
	case "REG_LINK", "LINK":
		return format.REGLink, nil
	case "REG_MULTI_SZ", "MULTI_SZ":
		return format.REGMultiSZ, nil
	case "REG_RESOURCE_LIST", "RESOURCE_LIST":
		return format.REGResourceList, nil
	case "REG_FULL_RESOURCE_DESCRIPTOR", "FULL_RESOURCE_DESCRIPTOR":
		return format.REGFullResourceDescriptor, nil
	case "REG_RESOURCE_REQUIREMENTS_LIST", "RESOURCE_REQUIREMENTS_LIST":
		return format.REGResourceRequirementsList, nil
	case "REG_QWORD", "QWORD":
		return format.REGQWORD, nil
	default:
		return 0, fmt.Errorf("unknown registry type: %s", typeStr)
	}
}

// SetValueFromString is a convenience method for parsing registry data from string format.
//
// This is perfect for parsing .reg files or similar text formats where you have:
//   - A full path string with backslashes
//   - A value name
//   - A type string (like "REG_SZ", "REG_DWORD")
//   - Raw data bytes
//
// Example:
//
//	b.SetValueFromString("HKLM\\Software\\MyApp", "Version", "REG_SZ", []byte("1.0\x00"))
//	b.SetValueFromString("Software\\MyApp", "Timeout", "REG_DWORD", []byte{0x1E, 0x00, 0x00, 0x00})
func (b *Builder) SetValueFromString(pathStr string, name string, typeStr string, data []byte) error {
	// Split path
	path := SplitPath(pathStr)
	if len(path) == 0 {
		return errors.New("path cannot be empty")
	}

	// Parse type
	typ, err := ParseValueType(typeStr)
	if err != nil {
		return fmt.Errorf("parse value type: %w", err)
	}

	// Use existing SetValue
	return b.SetValue(path, name, typ, data)
}

// SetNone sets a REG_NONE value (type 0, typically empty).
//
// This is a rare type used for placeholder or undefined values.
//
// Example:
//
//	b.SetNone([]string{"Software", "MyApp"}, "Placeholder")
func (b *Builder) SetNone(path []string, name string) error {
	return b.SetValue(path, name, format.REGNone, []byte{})
}

// SetLink sets a REG_LINK value (type 6, symbolic link).
//
// This type is used for registry symbolic links and is rarely used in practice.
//
// Example:
//
//	linkTarget := "\\Registry\\Machine\\Software\\Classes"
//	b.SetLink([]string{"Software", "Link"}, "Target", []byte(linkTarget))
func (b *Builder) SetLink(path []string, name string, linkData []byte) error {
	return b.SetValue(path, name, format.REGLink, linkData)
}

// SetResourceList sets a REG_RESOURCE_LIST value (type 8).
//
// This type stores hardware resource lists and is typically used by device drivers.
//
// Example:
//
//	b.SetResourceList([]string{"System", "Device"}, "Resources", resourceData)
func (b *Builder) SetResourceList(path []string, name string, data []byte) error {
	return b.SetValue(path, name, format.REGResourceList, data)
}

// SetFullResourceDescriptor sets a REG_FULL_RESOURCE_DESCRIPTOR value (type 9).
//
// This type stores complete hardware resource descriptors.
//
// Example:
//
//	b.SetFullResourceDescriptor([]string{"System", "Device"}, "Descriptor", descriptorData)
func (b *Builder) SetFullResourceDescriptor(path []string, name string, data []byte) error {
	return b.SetValue(path, name, format.REGFullResourceDescriptor, data)
}

// SetResourceRequirementsList sets a REG_RESOURCE_REQUIREMENTS_LIST value (type 10).
//
// This type stores hardware resource requirements lists.
//
// Example:
//
//	b.SetResourceRequirementsList([]string{"System", "Device"}, "Requirements", reqData)
func (b *Builder) SetResourceRequirementsList(path []string, name string, data []byte) error {
	return b.SetValue(path, name, format.REGResourceRequirementsList, data)
}
