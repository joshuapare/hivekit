// Package dirty provides efficient tracking and flushing of dirty pages
// in memory-mapped hive files.
//
// The tracker maintains a list of dirty byte ranges, coalesces them into
// page-aligned ranges, and flushes them to disk using platform-specific
// system calls (msync on Unix, FlushViewOfFile on Windows).
package dirty

import (
	"context"
	"sort"

	"github.com/joshuapare/hivekit/hive"
)

const (
	// defaultRangeCapacity is the pre-allocated capacity for dirty ranges.
	// This reduces allocations during typical workloads.
	defaultRangeCapacity = 64

	// standardPageSize is the typical OS page size (4KB).
	standardPageSize = 4096
)

// FlushMode controls durability guarantees for transaction commits.
type FlushMode int

const (
	// FlushAuto provides safe defaults for most use cases:
	// - msync() dirty data pages
	// - fdatasync() after header write
	// - On macOS, uses F_FULLFSYNC for maximum durability.
	FlushAuto FlushMode = iota

	// FlushDataOnly only flushes dirty data pages via msync().
	// The caller is responsible for calling fdatasync() later.
	// Use this when batching multiple transactions together.
	FlushDataOnly

	// FlushFull provides ultra-safe durability:
	// - msync() dirty data pages
	// - msync() header page
	// - fdatasync() file descriptor
	// - On macOS, uses F_FULLFSYNC
	// Use this for power-loss sensitive workflows.
	FlushFull
)

// Range represents a dirty byte range (absolute file offsets).
type Range struct {
	Off int64 // Absolute offset in file
	Len int64 // Length in bytes
}

// Tracker accumulates dirty ranges and flushes them efficiently.
//
// NOT thread-safe. Only one goroutine should use it at a time.
type Tracker struct {
	h        *hive.Hive
	ranges   []Range // Dirty data ranges (will be coalesced at flush time)
	pageSize int64   // OS page size (typically 4096)
}

// NewTracker creates a dirty tracker for the given hive.
//
// The tracker pre-allocates capacity for 64 ranges to minimize allocations
// during typical workloads.
func NewTracker(h *hive.Hive) *Tracker {
	return &Tracker{
		h:        h,
		ranges:   make([]Range, 0, defaultRangeCapacity), // Pre-allocate to avoid allocs
		pageSize: standardPageSize,                       // Standard page size
	}
}

// Add records a dirty range.
//
// The range will be page-aligned and coalesced with other ranges at flush time.
// This method is very fast (< 50 ns) as it only appends to a slice.
//
// Performance: < 50 ns, zero allocations after initial capacity.
func (t *Tracker) Add(off, length int) {
	t.ranges = append(t.ranges, Range{
		Off: int64(off),
		Len: int64(length),
	})
}

// FlushDataOnly flushes all dirty data ranges (not header) to disk.
//
// This method:
//  1. Coalesces all ranges into page-aligned, non-overlapping ranges
//  2. Flushes each range using msync() (Unix) or FlushViewOfFile (Windows)
//  3. Clears the ranges slice
//
// The header page (offset 0, length 4096) is NOT flushed.
//
// The context can be used to cancel the flush operation. If cancelled during
// flushing, some ranges may have been flushed while others have not.
//
// Performance: Depends on number of ranges and OS page cache, typically < 5 ms for 10 ranges.
func (t *Tracker) FlushDataOnly(ctx context.Context) error {
	if len(t.ranges) == 0 {
		return nil
	}

	// Check for cancellation before starting
	if err := ctx.Err(); err != nil {
		return err
	}

	// Flush dirty ranges
	data := t.h.Bytes()
	if len(data) == 0 {
		return nil
	}

	// Platform-specific flushing
	if err := t.flushRanges(ctx, data); err != nil {
		return err
	}

	// Clear ranges
	t.ranges = t.ranges[:0]
	return nil
}

// FlushHeaderAndMeta flushes the header page and optionally syncs the file descriptor.
//
// This method:
//  1. Flushes the header page (offset 0, length 4096) using msync()
//  2. Calls fdatasync() based on the FlushMode:
//     - FlushAuto: fdatasync()
//     - FlushDataOnly: no fdatasync()
//     - FlushFull: fdatasync() + F_FULLFSYNC on macOS
//
// The context can be used to cancel the operation. Note that if cancelled after
// the header is flushed but before fdatasync completes, the header may be
// inconsistent with the data pages on disk.
//
// Performance: Typically < 5 ms (OS dependent).
func (t *Tracker) FlushHeaderAndMeta(ctx context.Context, mode FlushMode) error {
	// Check for cancellation before starting
	if err := ctx.Err(); err != nil {
		return err
	}

	// Flush header page
	data := t.h.Bytes()
	if len(data) == 0 {
		return nil
	}

	// Flush the first page (header)
	headerLen := int(t.pageSize)
	if headerLen > len(data) {
		headerLen = len(data)
	}
	if err := msync(data[:headerLen]); err != nil {
		return err
	}

	// Check for cancellation before fdatasync
	if err := ctx.Err(); err != nil {
		return err
	}

	// Optionally fdatasync based on mode
	if mode == FlushDataOnly {
		return nil
	}

	fd := t.h.FD()
	fullfsync := (mode == FlushFull)
	return fdatasync(fd, fullfsync)
}

// Reset clears all tracked ranges.
//
// This is useful for testing or when aborting a transaction.
func (t *Tracker) Reset() {
	t.ranges = t.ranges[:0]
}

// DebugRanges returns the current dirty ranges (for testing/debugging).
//
// The returned ranges are the raw, uncoalesced ranges.
func (t *Tracker) DebugRanges() []Range {
	// Return a copy to prevent external modification
	result := make([]Range, len(t.ranges))
	copy(result, t.ranges)
	return result
}

// DebugCoalescedRanges returns the coalesced dirty ranges (for testing/debugging).
//
// These are page-aligned, sorted, and merged ranges that will be flushed.
func (t *Tracker) DebugCoalescedRanges() []Range {
	return t.coalesce()
}

// coalesce page-aligns all ranges, sorts them, and merges overlapping/adjacent ranges.
//
// Returns a new slice of non-overlapping, sorted ranges.
//
// Performance: < 10 μs for 100 ranges.
func (t *Tracker) coalesce() []Range {
	if len(t.ranges) == 0 {
		return nil
	}

	// Page-align all ranges
	aligned := make([]Range, len(t.ranges))
	for i, r := range t.ranges {
		// Round down start to page boundary
		start := (r.Off / t.pageSize) * t.pageSize

		// Round up end to page boundary
		end := r.Off + r.Len
		if end%t.pageSize != 0 {
			end = ((end / t.pageSize) + 1) * t.pageSize
		}

		aligned[i] = Range{
			Off: start,
			Len: end - start,
		}
	}

	// Sort by offset
	sort.Slice(aligned, func(i, j int) bool {
		return aligned[i].Off < aligned[j].Off
	})

	// Merge overlapping/adjacent ranges
	merged := make([]Range, 0, len(aligned))
	current := aligned[0]

	for i := 1; i < len(aligned); i++ {
		next := aligned[i]

		// Check if next overlaps or is adjacent to current
		if next.Off <= current.Off+current.Len {
			// Merge: extend current to include next
			end := current.Off + current.Len
			nextEnd := next.Off + next.Len
			if nextEnd > end {
				end = nextEnd
			}
			current.Len = end - current.Off
		} else {
			// No overlap: save current and start new range
			merged = append(merged, current)
			current = next
		}
	}

	// Don't forget the last range
	merged = append(merged, current)

	return merged
}
