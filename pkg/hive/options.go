package hive

import (
	"github.com/joshuapare/hivekit/pkg/types"
)

// MergeOptions controls merge behavior.
type MergeOptions struct {
	// Limits defines registry constraints for validation.
	// Used by ValidateHive() to check registry constraints.
	// Note: Not yet enforced during merge operations (future enhancement).
	// If nil, DefaultLimits() is used by validation functions.
	Limits *Limits

	// Defragment compacts the hive after merge, improving locality
	// and reducing file size. Adds ~10-20% overhead but recommended
	// for hives that undergo many modifications.
	Defragment bool

	// CreateBackup creates a .bak file before modifying the hive.
	// The backup is created at <hivePath>.bak.
	CreateBackup bool
}

// OperationOptions controls individual high-level operation behavior.
type OperationOptions struct {
	// Limits defines registry constraints for validation.
	// Used by ValidateHive() to check registry constraints.
	// Note: Not yet enforced during write operations (future enhancement).
	// If nil, DefaultLimits() is used by validation functions.
	Limits *Limits

	// Defragment compacts the hive after the operation.
	// Adds overhead but reduces file size.
	Defragment bool

	// CreateBackup creates a .bak file before modifying the hive.
	// The backup is created at <hivePath>.bak.
	CreateBackup bool

	// CreateKey creates the key if it doesn't exist (for SetValue).
	// If false and the key doesn't exist, SetValue returns an error.
	CreateKey bool
}

// ExportOptions controls .reg export behavior.
type ExportOptions struct {
	// SubtreePath exports only this subtree (e.g., "Software\\MyApp").
	// If empty, exports the entire
	SubtreePath string

	// Encoding specifies output encoding.
	// Supported values: "UTF-16LE" (Windows default), "UTF-8"
	// Default: "UTF-16LE"
	Encoding string

	// WithBOM includes byte-order mark in output.
	// Default: true for UTF-16LE, false for UTF-8
	WithBOM bool
}

// OpenOptions controls hive opening behavior.
// This is an alias to types.OpenOptions for convenience.
type OpenOptions = types.OpenOptions

// Limits defines registry constraints to prevent corruption.
// These match Windows registry specifications.
type Limits = types.Limits

// EditOp represents a registry edit operation (re-exported for convenience).
type EditOp = types.EditOp

// Operation types (re-exported for convenience).
type (
	OpSetValue    = types.OpSetValue
	OpDeleteValue = types.OpDeleteValue
	OpCreateKey   = types.OpCreateKey
	OpDeleteKey   = types.OpDeleteKey
)

// DefaultLimits returns standard Windows registry limits.
// These are safe for all production use cases.
//
// Limits:
//   - MaxSubkeys: 512 (Windows default)
//   - MaxValues: 16,384 (Windows hard limit)
//   - MaxValueSize: 1 MB
//   - MaxKeyNameLen: 255 characters
//   - MaxValueNameLen: 16,383 characters
//   - MaxTreeDepth: 512 levels
//   - MaxTotalSize: 2 GB
func DefaultLimits() Limits {
	return types.DefaultLimits()
}

// RelaxedLimits returns more permissive limits for system keys.
// Use with caution - may create hives that don't work on all Windows versions.
//
// Limits:
//   - MaxSubkeys: 65,535 (absolute Windows maximum)
//   - MaxValues: 16,384 (same as default)
//   - MaxValueSize: 10 MB
//   - MaxTreeDepth: 1,024 levels
//   - MaxTotalSize: 4 GB
func RelaxedLimits() Limits {
	return types.RelaxedLimits()
}

// StrictLimits returns conservative limits for safety-critical applications.
// Prevents resource exhaustion in constrained environments.
//
// Limits:
//   - MaxSubkeys: 256
//   - MaxValues: 1,024
//   - MaxValueSize: 64 KB
//   - MaxTreeDepth: 128 levels
//   - MaxTotalSize: 100 MB
func StrictLimits() Limits {
	return types.StrictLimits()
}
