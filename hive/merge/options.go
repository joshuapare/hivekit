// Package merge provides high-level API for merging changes into hive files.
//
// The merge system supports three write strategies:
//   - InPlace: Mutate cells in-place when possible (best for small changes)
//   - Append: Append-only, never free cells (safe for logs)
//   - Hybrid: Heuristic-based selection (default, best for most use cases)
//
// Performance is optimized through:
//   - Page-aligned dirty tracking
//   - Transaction-safe REGF sequence management
//   - Configurable HBIN growth and RAID striping
package merge

import (
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/pkg/types"
)

// StrategyKind selects the write strategy for merge operations.
type StrategyKind int

const (
	// StrategyInPlace mutates cells in-place when they fit.
	// Best for: Small updates, minimal fragmentation tolerance
	// Trade-off: May fragment over time.
	StrategyInPlace StrategyKind = iota

	// StrategyAppend always allocates new cells, never frees old ones.
	// Best for: Append-only logs, crash recovery scenarios
	// Trade-off: Higher space usage, but no fragmentation.
	StrategyAppend

	// StrategyHybrid uses heuristics to choose between in-place and append.
	// If the new data fits in the old cell with slack%, use in-place.
	// Otherwise, allocate new cell and append.
	// Best for: General-purpose merges (default)
	// Trade-off: Balanced space/performance.
	StrategyHybrid
)

// IndexMode controls how path lookups are performed during merge operations.
type IndexMode int

const (
	// IndexModeAuto selects single-pass or full index based on plan characteristics.
	// For small plans (< IndexThreshold operations), uses single-pass walk-apply.
	// For large plans, builds a full index upfront.
	// Default mode, recommended for most use cases.
	IndexModeAuto IndexMode = iota

	// IndexModeFull always builds full index upfront (traditional behavior).
	// Best for: Large plans with many operations, or when you need the index
	// for post-merge queries (e.g., HasKey).
	IndexModeFull

	// IndexModeSinglePass uses single-pass walk-apply (no index build).
	// Best for: Small plans where index build overhead dominates.
	// Operations are sorted by path and applied during a single DFS traversal,
	// with subtree pruning to skip irrelevant branches.
	IndexModeSinglePass
)

const (
	// DefaultIndexThreshold is the default ops threshold for Auto mode.
	// Plans with fewer operations use single-pass walk-apply.
	// Plans with more operations build a full index.
	DefaultIndexThreshold = 100
)

const (
	// defaultGrowChunkShift is the bit shift for default HBIN growth chunk (1 << 20 = 1MB).
	defaultGrowChunkShift = 20

	// defaultHybridSlackPct is the default slack percentage for hybrid strategy.
	// Allows 12% extra space in cells to reduce fragmentation while allowing some growth.
	defaultHybridSlackPct = 12

	// defaultCompactThreshold is the default percentage threshold for auto-compaction.
	// When fragmentation exceeds 30%, consider compacting the hive.
	defaultCompactThreshold = 30
)

// Options configures merge behavior and performance tuning.
//
// Use DefaultOptions() for production-ready defaults.
type Options struct {
	// Strategy selects the write approach (InPlace, Append, or Hybrid)
	// Default: StrategyHybrid
	Strategy StrategyKind

	// GrowChunk is the HBIN growth size in bytes (rounded to 4096).
	// When the allocator runs out of space, a new HBIN of this size is appended.
	// Default: 1MB (1048576)
	// Recommendation: 1MB for general use, 4MB+ for large batch merges
	GrowChunk int

	// StripeUnit for RAID/EBS alignment in bytes (0 = disabled).
	// When non-zero, HBIN boundaries are aligned to this value.
	// Default: 0 (disabled)
	// Recommendation: 262144 (256KB) for AWS EBS, 65536 (64KB) for RAID0
	StripeUnit int

	// Flush mode for transaction commits (controls durability guarantees).
	// Default: dirty.FlushAuto (recommended)
	// Options:
	//  - FlushAuto: Safe defaults (msync + fdatasync)
	//  - FlushDataOnly: Data flush only (caller handles fdatasync)
	//  - FlushFull: Ultra-safe (msync + fdatasync + F_FULLFSYNC on macOS)
	Flush dirty.FlushMode

	// HugePages hint for Linux madvise (large hives only).
	// When true, advise kernel to use huge pages (2MB/1GB TLB entries).
	// Default: false
	// Recommendation: true for hives > 1GB on Linux with hugepages enabled
	HugePages bool

	// WillNeedHint: madvise MADV_WILLNEED on newly appended ranges.
	// When true, pre-fault pages for better sequential write performance.
	// Default: false
	// Recommendation: true for sequential batch merges, false for random access
	WillNeedHint bool

	// HybridSlackPct: allowed slack percentage for in-place updates (Hybrid only).
	// If (needed_size + slack%) fits in the existing cell, use in-place.
	// Otherwise, allocate new cell.
	// Default: 12 (12% slack)
	// Example: 100-byte value updating to 110 bytes → in-place (110 < 112)
	//          100-byte value updating to 115 bytes → append (115 > 112)
	HybridSlackPct int

	// CompactThreshold: fragmentation percentage to trigger compaction (future).
	// When (free_space / total_space) > threshold%, suggest compaction.
	// Default: 30 (30% fragmented = compact)
	// Note: Auto-compaction not yet implemented (Phase 6)
	CompactThreshold int

	// ParseOptions configures .reg text parsing behavior.
	// These options are used by MergeRegText, MergeRegTextWithPrefix, and session
	// ApplyRegText/ApplyRegTextWithPrefix methods.
	//
	// Key options:
	//   - AllowMissingHeader: When true, allows parsing .reg text without the
	//     "Windows Registry Editor Version 5.00" header. Default: false.
	//   - InputEncoding: Specifies input encoding (default: UTF-8).
	ParseOptions types.RegParseOptions

	// IndexKind selects the index implementation for key/value lookups.
	// Default: index.IndexNumeric (zero-allocation, faster)
	// Alternative: index.IndexString (traditional, useful for debugging)
	IndexKind index.IndexKind

	// IndexMode controls how path lookups are performed during merge operations.
	// Default: IndexModeAuto (selects based on plan size)
	// See IndexMode constants for details.
	IndexMode IndexMode

	// IndexThreshold is the ops threshold for Auto mode.
	// Plans with fewer operations use single-pass walk-apply.
	// Plans with more operations build a full index.
	// Default: 100
	// Only used when IndexMode is IndexModeAuto.
	IndexThreshold int

	// NKCapacity is the initial capacity hint for the NK (Named Key) index hash map.
	// When set to 0 (default), capacity is automatically estimated from hive size.
	// For large hives, set explicitly to reduce hash map resize operations.
	// Default: 0 (auto-estimate)
	NKCapacity int

	// VKCapacity is the initial capacity hint for the VK (Value Key) index hash map.
	// When set to 0 (default), capacity is automatically estimated from hive size.
	// Typically 2-4x NKCapacity in practice.
	// Default: 0 (auto-estimate)
	VKCapacity int
}

// DefaultOptions returns production-ready defaults optimized for general use.
//
// The defaults provide:
//   - Hybrid strategy (balanced space/performance)
//   - 1MB HBIN growth (good for most workloads)
//   - Safe flush mode (AutoFlush with fdatasync)
//   - 12% in-place slack (reduces fragmentation while allowing some growth)
//   - 30% compaction threshold (for future auto-compact)
func DefaultOptions() Options {
	return Options{
		Strategy:         StrategyHybrid,
		GrowChunk:        1 << defaultGrowChunkShift, // 1MB
		StripeUnit:       0,                          // disabled
		Flush:            dirty.FlushAuto,
		HugePages:        false,
		WillNeedHint:     false,
		HybridSlackPct:   defaultHybridSlackPct,
		CompactThreshold: defaultCompactThreshold,
		IndexKind:        index.IndexNumeric, // zero-allocation, faster
		IndexMode:        IndexModeAuto,      // auto-select based on plan size
		IndexThreshold:   DefaultIndexThreshold,
		NKCapacity:       0, // auto-estimate from hive size
		VKCapacity:       0, // auto-estimate from hive size
	}
}
