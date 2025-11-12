// Package alloc provides cell allocation and free-list management for Windows Registry hives.
//
// # Overview
//
// This package implements memory allocation for registry hive cells using a segregated
// free-list design inspired by Windows kernel's registry allocator. It provides O(1)
// allocation and deallocation while maintaining spec-compliant HBIN alignment.
//
// # Allocator Interface
//
// The core abstraction is the Allocator interface, which supports:
//
//   - Alloc(size, class): Allocate a cell of specified size and type
//   - Free(ref): Mark a cell as free for reuse
//   - GrowByPages(n): Add n×4KB HBINs to the hive
//   - TruncatePages(n): Remove n×4KB HBINs from the end
//
// # Implementations
//
// FastAllocator: Production allocator with segregated free-lists
//
//   - 10 size classes (32B to 16KB)
//   - O(1) allocation and deallocation
//   - Automatic coalescing of adjacent free cells
//   - HBIN-aware allocation (cells never cross HBIN boundaries)
//
// NoFreeAllocator: Append-only wrapper for merge operations
//
//   - Skips Free() calls (no-op)
//   - Used during hive merging where source cells shouldn't be freed
//
// # Usage Example
//
//	dt := dirty.NewTracker(hive)
//	fa, err := alloc.NewFast(hive, dt)
//	if err != nil {
//	    return err
//	}
//
//	// Allocate a cell for a value key (VK)
//	ref, buf, err := fa.Alloc(256, alloc.ClassVK)
//	if err != nil {
//	    return err
//	}
//
//	// Write VK structure to buf...
//	copy(buf[0:2], format.VKSignature)
//
//	// Later, free the cell
//	err = fa.Free(ref)
//
// # Size Classes
//
// The allocator maintains 10 segregated free-lists:
//
//	Class 0:  32 -   64 bytes
//	Class 1:  64 -  128 bytes
//	Class 2: 128 -  256 bytes
//	Class 3: 256 -  512 bytes
//	Class 4: 512 - 1024 bytes
//	Class 5:   1 -    2 KB
//	Class 6:   2 -    4 KB
//	Class 7:   4 -    8 KB
//	Class 8:   8 -   16 KB
//	Class 9:  16+      KB (large allocations)
//
// # HBIN Growth
//
// When allocating cells, if no suitable free cell exists, the allocator grows
// the hive by adding new HBINs:
//
//	// Add 8KB HBIN (2 pages)
//	err := fa.GrowByPages(2)
//
// HBINs are always 4KB-aligned and contain a 32-byte header, so a 4KB HBIN
// provides 4064 bytes of usable cell space.
//
// # Cell References
//
// Cell references (CellRef) are uint32 offsets relative to the HBIN start (0x1000).
// This matches the Windows HCELL_INDEX format:
//
//	Absolute offset = 0x1000 + CellRef
//
// # Alignment Requirements
//
// All cells must be 8-byte aligned. The allocator automatically rounds up
// allocation sizes to meet this requirement.
//
// Cells cannot cross HBIN boundaries - if a cell would span two HBINs,
// the allocator either uses a different free cell or grows the hive.
//
// # Thread Safety
//
// Allocator instances are not thread-safe. Callers must synchronize access
// externally or use the tx package for transactional modifications.
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/dirty: Tracks modified pages for commit
//   - github.com/joshuapare/hivekit/hive/edit: High-level editing with allocation
//   - github.com/joshuapare/hivekit/internal/format: Binary format constants
package alloc
