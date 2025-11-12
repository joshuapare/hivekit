// Package dirty provides page-level dirty tracking for registry hive modifications.
//
// # Overview
//
// This package tracks which 4KB pages of a hive have been modified, enabling efficient
// commits that only write changed data back to disk. It uses a bitmap to represent
// the dirty state of each page in the hive.
//
// # DirtyTracker Interface
//
// The main interface provides:
//
//   - Add(offset, length): Mark a range as dirty
//   - Ranges(): Get all dirty ranges (sorted, coalesced)
//   - Reset(): Clear all dirty marks
//   - Clone(): Create a copy of the tracker
//
// # Usage
//
// Creating a tracker:
//
//	tracker := dirty.NewTracker(hive)
//
// Marking modifications:
//
//	// After modifying cell at offset 0x5000
//	tracker.Add(0x5000, 128)
//
// Getting dirty ranges for commit:
//
//	ranges := tracker.Ranges()
//	for _, r := range ranges {
//	    // Write hive.Bytes()[r.Start:r.End] to disk
//	}
//
// # Page-Level Granularity
//
// The tracker operates at 4KB (0x1000 byte) page boundaries:
//   - Modifications are rounded to page boundaries
//   - A 1-byte change marks the entire 4KB page dirty
//   - This matches OS page size for efficient I/O
//
// # Range Coalescing
//
// When retrieving dirty ranges via Ranges(), consecutive dirty pages are
// automatically merged into single ranges:
//
//	Dirty pages: [0, 1, 2, 5, 6] → Ranges: [0x0-0x3000, 0x5000-0x7000]
//
// This reduces the number of write operations during commit.
//
// # Thread Safety
//
// DirtyTracker instances are not thread-safe. Callers must synchronize
// access externally or use the tx package for transactional safety.
//
// # Integration with Transactions
//
// The tx package uses dirty tracking to implement efficient commits:
//
//	tx, _ := tx.Begin(hive)
//	// ... modifications tracked automatically ...
//	tx.Commit() // Only writes dirty ranges
//
// # Memory Overhead
//
// The bitmap uses 1 bit per 4KB page:
//   - 10MB hive → 320 bytes of tracking overhead
//   - 1GB hive → 32KB of tracking overhead
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/tx: Transaction management with auto-tracking
//   - github.com/joshuapare/hivekit/hive/edit: High-level edits that mark dirty pages
//   - github.com/joshuapare/hivekit/hive/alloc: Allocator that marks pages dirty
package dirty
