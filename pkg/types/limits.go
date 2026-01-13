package types

// ============================================================================
// Windows Registry Limits Constants
// ============================================================================
// These constants define the official limits imposed by Windows Registry.
// Different Windows versions may have slightly different limits, but these
// represent the most commonly documented values.

const (
	// WindowsMaxSubkeysDefault is the standard maximum number of subkeys per key
	// for most registry key types in Windows.
	WindowsMaxSubkeysDefault = 512

	// WindowsMaxSubkeysAbsolute is the absolute maximum number of subkeys
	// that can exist under a single key. Some system keys can have more
	// than the default limit.
	WindowsMaxSubkeysAbsolute = 65535

	// WindowsMaxValues is the hard limit for the number of values per key
	// in Windows Registry.
	WindowsMaxValues = 16384

	// WindowsMaxValueSize1MB is the standard maximum size for a single
	// registry value's data (1 MB).
	WindowsMaxValueSize1MB = 1 << 20 // 1,048,576 bytes

	// WindowsMaxValueSize10MB is a relaxed maximum for large binary data.
	// Some applications may need to store larger values.
	WindowsMaxValueSize10MB = 10 << 20 // 10,485,760 bytes

	// WindowsMaxValueSize64KB is a conservative maximum for safety-critical
	// or resource-constrained environments.
	WindowsMaxValueSize64KB = 64 << 10 // 65,536 bytes

	// WindowsMaxKeyNameLen is the hard limit for registry key names
	// in Windows (measured in characters, not bytes).
	WindowsMaxKeyNameLen = 255

	// WindowsMaxKeyNameLenHalf is half the Windows limit, useful for
	// strict validation scenarios.
	WindowsMaxKeyNameLenHalf = 128

	// WindowsMaxValueNameLen is the hard limit for registry value names
	// in Windows (measured in characters, not bytes).
	WindowsMaxValueNameLen = 16383

	// WindowsMaxValueNameLenSmall is a much smaller limit for strict
	// validation scenarios.
	WindowsMaxValueNameLenSmall = 255

	// WindowsMaxTreeDepthPractical is the practical limit for registry
	// tree depth. While Windows has no hard limit, depths beyond this
	// are extremely rare and may cause performance issues.
	WindowsMaxTreeDepthPractical = 512

	// WindowsMaxTreeDepthDeep allows very deep trees for special cases.
	WindowsMaxTreeDepthDeep = 1024

	// WindowsMaxTreeDepthShallow is a conservative limit for safety-critical
	// applications.
	WindowsMaxTreeDepthShallow = 128

	// WindowsMaxHiveSize2GB is the typical maximum size for a Windows
	// registry hive file (2 GB).
	WindowsMaxHiveSize2GB = 2 << 30 // 2,147,483,648 bytes

	// WindowsMaxHiveSize4GB is a relaxed maximum for very large hives
	// (4 GB).
	WindowsMaxHiveSize4GB = 4 << 30 // 4,294,967,296 bytes

	// WindowsMaxHiveSize100MB is a conservative maximum for constrained
	// environments (100 MB).
	WindowsMaxHiveSize100MB = 100 << 20 // 104,857,600 bytes

	// StrictSubkeysDivisor is used to calculate strict subkey limits
	// (half of Windows default).
	StrictSubkeysDivisor = 2

	// StrictValuesDivisor is used to calculate strict value limits
	// (much smaller than Windows limit).
	StrictValuesDivisor = 16
)

// Limits defines constraints for registry operations to prevent
// corruption, resource exhaustion, and malformed hives.
type Limits struct {
	// MaxSubkeys is the maximum number of subkeys a node can have.
	// Windows registry limit is typically 512 for most key types,
	// but can be higher (up to ~65535) for some system keys.
	MaxSubkeys int

	// MaxValues is the maximum number of values a node can have.
	// Windows registry limit is 16384 values per key.
	MaxValues int

	// MaxValueSize is the maximum size of a single value's data in bytes.
	// Windows registry limit is 1 MB (1,048,576 bytes) for most value types.
	MaxValueSize int

	// MaxKeyNameLen is the maximum length of a key name in characters.
	// Windows registry limit is 255 characters (not bytes).
	MaxKeyNameLen int

	// MaxValueNameLen is the maximum length of a value name in characters.
	// Windows registry limit is 16,383 characters.
	MaxValueNameLen int

	// MaxTreeDepth is the maximum depth of the registry tree.
	// Windows registry has no hard limit, but practical limit is ~512 levels.
	MaxTreeDepth int

	// MaxTotalSize is the maximum total size of a hive in bytes.
	// Windows registry hives are typically limited to 2 GB.
	MaxTotalSize int64
}

// DefaultLimits returns the standard Windows registry limits.
// These are conservative defaults that should work for all real-world scenarios.
func DefaultLimits() Limits {
	return Limits{
		MaxSubkeys:      WindowsMaxSubkeysDefault,
		MaxValues:       WindowsMaxValues,
		MaxValueSize:    WindowsMaxValueSize1MB,
		MaxKeyNameLen:   WindowsMaxKeyNameLen,
		MaxValueNameLen: WindowsMaxValueNameLen,
		MaxTreeDepth:    WindowsMaxTreeDepthPractical,
		MaxTotalSize:    WindowsMaxHiveSize2GB,
	}
}

// RelaxedLimits returns more permissive limits for system keys or special cases.
// Use with caution - these allow operations that may not work on real Windows systems.
func RelaxedLimits() Limits {
	return Limits{
		MaxSubkeys:      WindowsMaxSubkeysAbsolute,
		MaxValues:       WindowsMaxValues,
		MaxValueSize:    WindowsMaxValueSize10MB,
		MaxKeyNameLen:   WindowsMaxKeyNameLen,
		MaxValueNameLen: WindowsMaxValueNameLen,
		MaxTreeDepth:    WindowsMaxTreeDepthDeep,
		MaxTotalSize:    WindowsMaxHiveSize4GB,
	}
}

// StrictLimits returns conservative limits for safety-critical applications.
// These prevent resource exhaustion in constrained environments.
func StrictLimits() Limits {
	return Limits{
		MaxSubkeys:      WindowsMaxSubkeysDefault / StrictSubkeysDivisor,
		MaxValues:       WindowsMaxValues / StrictValuesDivisor,
		MaxValueSize:    WindowsMaxValueSize64KB,
		MaxKeyNameLen:   WindowsMaxKeyNameLenHalf,
		MaxValueNameLen: WindowsMaxValueNameLenSmall,
		MaxTreeDepth:    WindowsMaxTreeDepthShallow,
		MaxTotalSize:    WindowsMaxHiveSize100MB,
	}
}
