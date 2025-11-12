// Package builder provides a high-performance API for building Windows Registry
// hive files programmatically. It uses a path-based API optimized for parsing
// loops and data import scenarios.
//
// The builder supports creating new hives from scratch or modifying existing
// ones, with progressive writes to disk for memory-efficient handling of large
// hives.
package builder

import (
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/merge"
)

// StrategyType defines the write strategy for the builder.
// This mirrors merge.StrategyKind for the builder API.
type StrategyType int

const (
	// StrategyInPlace reuses freed cells when possible, minimizing hive growth.
	// Best for: Frequent updates to existing values, memory-constrained environments.
	StrategyInPlace StrategyType = iota

	// StrategyAppend never reuses cells, growing the hive monotonically.
	// Best for: Logging/audit scenarios, avoiding fragmentation.
	StrategyAppend

	// StrategyHybrid uses heuristics to choose between InPlace and Append.
	// Best for: General-purpose building (default recommendation).
	StrategyHybrid
)

// toMergeStrategy converts StrategyType to merge.StrategyKind.
func (s StrategyType) toMergeStrategy() merge.StrategyKind {
	switch s {
	case StrategyInPlace:
		return merge.StrategyInPlace
	case StrategyAppend:
		return merge.StrategyAppend
	case StrategyHybrid:
		return merge.StrategyHybrid
	default:
		return merge.StrategyHybrid
	}
}

// HiveVersion specifies the Windows Registry hive format version.
type HiveVersion int

const (
	// Version1_3 is Windows NT 3.51 / Windows 95 format (most compatible).
	Version1_3 HiveVersion = iota

	// Version1_4 is Windows NT 4.0 format.
	Version1_4

	// Version1_5 is Windows 2000 / XP format.
	Version1_5

	// Version1_6 is Windows 10 / 11 format (supports transaction logs).
	Version1_6
)

// Options configures the builder's behavior.
type Options struct {
	// Strategy determines how cells are allocated and reused.
	// Default: StrategyHybrid
	Strategy StrategyType

	// PreallocPages pre-allocates this many 4KB pages to avoid repeated
	// mremap() calls during building. Set to 0 to grow dynamically.
	// Default: 0 (dynamic growth)
	PreallocPages int

	// AutoFlushThreshold triggers a progressive flush (transaction commit)
	// after this many operations. This enables constant memory usage for
	// building arbitrarily large hives. Set to 0 to disable progressive
	// writes (commit-only mode).
	// Default: 1000
	AutoFlushThreshold int

	// CreateIfNotExists controls whether the builder creates a new minimal
	// hive if the specified path doesn't exist.
	// Default: true
	CreateIfNotExists bool

	// HiveVersion specifies the hive format version for newly created hives.
	// Ignored when opening existing hives.
	// Default: Version1_3 (maximum compatibility)
	HiveVersion HiveVersion

	// FlushMode controls the durability guarantees for commits.
	// Default: FlushDataAndMeta (full fsync)
	FlushMode dirty.FlushMode

	// SlackPct is the slack percentage for the Hybrid strategy.
	// Only used when Strategy is StrategyHybrid.
	// Default: 12 (12%)
	SlackPct int
}

// DefaultOptions returns the recommended options for general-purpose hive building.
func DefaultOptions() *Options {
	return &Options{
		Strategy:           StrategyHybrid,
		PreallocPages:      0,
		AutoFlushThreshold: 1000,
		CreateIfNotExists:  true,
		HiveVersion:        Version1_3,
		FlushMode:          dirty.FlushAuto,
		SlackPct:           12, // Default slack percentage for Hybrid strategy
	}
}

// toMinorVersion converts HiveVersion to minor version number.
// Major version is always 1 for all Windows registry hive formats.
func (v HiveVersion) toMinorVersion() uint32 {
	switch v {
	case Version1_3:
		return 3
	case Version1_4:
		return 4
	case Version1_5:
		return 5
	case Version1_6:
		return 6
	default:
		return 3
	}
}
