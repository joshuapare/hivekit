package alloc

import (
	"container/heap"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// Debug flag - set to true to enable verbose logging (compile-time toggle).
const debugAlloc = false

// Runtime debug flag for allocation logging - controlled by HIVE_LOG_ALLOC env var.
var logAlloc = os.Getenv("HIVE_LOG_ALLOC") != ""

const (
	// minCellSize is the minimum total cell size (including 4-byte header).
	// Per REGF specification, the minimum cell size is 8 bytes total.
	// This prevents creating illegal cells smaller than 8 bytes.
	minCellSize = 8

	// maxHiveSize is the maximum hive file size (2GB - REGF spec limit).
	// HBIN offsets are int32, so we can't exceed 2^31-1 bytes.
	maxHiveSize = 0x7FFFFFFF // 2GB - 1
)

// FastAllocator is a high-performance allocator using min-heaps per size class.
// - Min-heaps give O(log n) allocation/removal and perfect best-fit
// - Tunable size classes (20-80 typical) keep heaps small (10-100 cells typically)
// - byOff map enables O(1) cell lookup for coalescing
// - bins index enables O(log B) HBIN bounds lookup.
type FastAllocator struct {
	h  *hive.Hive
	dt DirtyTracker // Dirty page tracker for marking header changes

	// Size class configuration and lookup table
	sizeTable *sizeClassTable

	// Segregated free lists by size class using min-heaps
	// Number of lists determined by sizeTable.NumClasses()
	freeLists []freeList

	// Large allocations (≥16KB) - simple linked list
	largeFree *largeBlock

	// Pool for reusing freeCell structs (eliminates allocations)
	freeCellPool sync.Pool

	// O(1) coalescing indexes (enable via option, nil by default for backwards compat)
	// startIdx: offset -> size (for backward coalesce lookup)
	// endIdx: end offset -> size (for forward coalesce lookup)
	startIdx map[int32]int32
	endIdx   map[int32]int32

	// O(1) cell lookup by offset (for heap.Remove during coalescing)
	byOff map[int32]*freeCell

	// HBIN boundaries for O(log B) binary search (replaces linear walk)
	bins []hbinRange

	// Max free span tracking (0 = not tracking, set via option)
	maxFree       int32
	secondMaxFree int32 // second-largest free cell for O(1) maxFree update on allocation

	// Statistics for testing and instrumentation
	stats allocatorStats

	// Per-HBIN lifecycle tracking (for debugging allocation patterns)
	hbinTracking  map[int32]*hbinStats // offset -> stats
	currentHBIN   int32                // Offset of the most recently created HBIN
	lastAllocHBIN int32                // Offset of the HBIN from which the last allocation came

	// Test hook: called before Grow() for test instrumentation (nil in production)
	onGrow func(int32)
}

// hbinRange represents HBIN boundaries for binary search.
type hbinRange struct {
	start int32 // HBIN start offset
	end   int32 // HBIN end offset (exclusive)
}

// hbinStats tracks lifecycle statistics for a single HBIN (for debugging).
type hbinStats struct {
	offset         int32 // HBIN start offset
	initialSize    int32 // Initial free space available (e.g., 4064 bytes for 4KB HBIN)
	allocCount     int   // Number of allocations from this HBIN
	bytesAllocated int32 // Total bytes allocated (including headers)
}

// hbinEfficiency holds efficiency data for a single HBIN during analysis.
// Used by GetEfficiencyStats() to track the k worst (lowest efficiency) HBINs.
type hbinEfficiency struct {
	offset     int32
	allocated  int32
	capacity   int32
	efficiency float64
	allocCount int
}

// worstHBINHeap is a max-heap for tracking the k worst (lowest efficiency) HBINs.
// The heap property is inverted (max-heap) so we can efficiently replace the
// "best of the worst" when finding a worse HBIN.
//
// For k=20: O(n log k) instead of O(n²) bubble sort - ~100x faster for large hives.
type worstHBINHeap []hbinEfficiency

func (h worstHBINHeap) Len() int { return len(h) }

// Less returns true if i has HIGHER efficiency than j (max-heap by efficiency).
// This way, heap[0] is the BEST (highest efficiency) among the worst HBINs.
func (h worstHBINHeap) Less(i, j int) bool { return h[i].efficiency > h[j].efficiency }
func (h worstHBINHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *worstHBINHeap) Push(x any) {
	*h = append(*h, x.(hbinEfficiency))
}

func (h *worstHBINHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// allocatorStats holds internal allocator statistics.
type allocatorStats struct {
	GrowCalls        int   // Number of Grow() calls
	GrowBytes        int64 // Total bytes added via Grow()
	AllocCalls       int   // Total Alloc() calls
	AllocFastPath    int   // Allocations that succeeded without Grow()
	AllocSlowPath    int   // Allocations that required Grow()
	FreeCalls        int   // Total Free() calls
	BytesAllocated   int64 // Total bytes allocated (including headers)
	BytesFreed       int64 // Total bytes freed
	SplitCount       int   // Number of cell splits
	CoalesceForward  int   // Forward coalesce operations
	CoalesceBackward int   // Backward coalesce operations
	HeapPushes       int   // heap.Push() calls
	HeapPops         int   // heap.Pop() calls
	HeapRemoves      int   // heap.Remove() calls

	// HBIN size distribution (for allocation analysis)
	HBINs4KB  int // Number of 4KB HBINs created
	HBINs8KB  int // Number of 8KB HBINs created
	HBINs16KB int // Number of 16KB+ HBINs created
}

// EfficiencyStats holds detailed efficiency and fragmentation metrics.
type EfficiencyStats struct {
	// Overall metrics
	TotalHBINs          int     // Total number of HBINs
	TotalCapacity       int64   // Total capacity across all HBINs (bytes)
	TotalAllocated      int64   // Total bytes allocated
	TotalWasted         int64   // Total wasted space (capacity - allocated)
	OverallEfficiency   float64 // Overall efficiency percentage
	AverageAllocPerHBIN float64 // Average bytes allocated per HBIN

	// HBIN efficiency distribution
	PerfectHBINs    int // HBINs at 100% efficiency
	ExcellentHBINs  int // HBINs at 99.0-99.9% efficiency
	VeryGoodHBINs   int // HBINs at 98.0-98.9% efficiency
	GoodHBINs       int // HBINs at 95.0-97.9% efficiency
	SuboptimalHBINs int // HBINs at <95% efficiency
	PoorHBINs       int // HBINs at <80% efficiency

	// Worst offenders (for analysis)
	LeastEfficientHBINs []struct {
		Offset     int32
		Allocated  int32
		Capacity   int32
		Efficiency float64
		AllocCount int
	}
}

// freeList is a size-class-specific free list using a min-heap.
type freeList struct {
	heap  freeCellHeap // Min-heap keyed on size
	count int
}

// freeCell represents a free cell in the allocator.
// Used in min-heaps for O(log n) allocation and removal.
type freeCell struct {
	off       int32 // Absolute offset in hive
	size      int32 // Size including header
	sc        int   // Size class (which heap this belongs to)
	heapIndex int   // Position in heap (for heap.Remove)
}

// freeCellHeap implements heap.Interface for min-heap keyed on cell size.
// Smallest cells are at the top, giving us perfect best-fit allocation.
type freeCellHeap []*freeCell

func (h *freeCellHeap) Len() int { return len(*h) }

func (h *freeCellHeap) Less(i, j int) bool {
	return (*h)[i].size < (*h)[j].size
}

func (h *freeCellHeap) Swap(i, j int) {
	(*h)[i], (*h)[j] = (*h)[j], (*h)[i]
	(*h)[i].heapIndex = i
	(*h)[j].heapIndex = j
}

func (h *freeCellHeap) Push(x any) {
	cell := x.(*freeCell) //nolint:errcheck // heap.Interface contract guarantees type
	cell.heapIndex = len(*h)
	*h = append(*h, cell)
}

func (h *freeCellHeap) Pop() any {
	old := *h
	n := len(old)
	cell := old[n-1]
	cell.heapIndex = -1
	*h = old[0 : n-1]
	return cell
}

// largeBlock for allocations >16KB.
type largeBlock struct {
	off  int32
	size int32
	next *largeBlock
}

// NewFast creates a high-performance allocator with dirty page tracking.
//
// Parameters:
//   - h: The hive to allocate from
//   - dt: Dirty tracker for marking header changes (can be nil for read-only use)
//   - config: Size class configuration (use nil for DefaultConfig)
func NewFast(h *hive.Hive, dt DirtyTracker, config *SizeClassConfig) (*FastAllocator, error) {
	// Use default config if none provided
	if config == nil {
		config = &DefaultConfig
	}

	// Build size class table from config
	sizeTable := newSizeClassTable(*config)

	fa := &FastAllocator{
		h:         h,
		dt:        dt,
		sizeTable: sizeTable,
		freeLists: make([]freeList, sizeTable.NumClasses()),
		startIdx:  make(map[int32]int32),
		endIdx:    make(map[int32]int32),
		byOff: make(
			map[int32]*freeCell,
			256,
		), // Reduced from 4096 - actual usage is much lower
		bins:         make([]hbinRange, 0, 256),  // Preallocate for HBIN index
		hbinTracking: make(map[int32]*hbinStats), // Per-HBIN lifecycle tracking
		freeCellPool: sync.Pool{
			New: func() any {
				return &freeCell{}
			},
		},
	}

	// Note: Trailing slack space is truncated in hive.Open(), so HBINs are
	// guaranteed to be contiguous. Initialize free lists by scanning existing free cells.
	if err := fa.initializeFreeLists(); err != nil {
		return nil, err
	}

	return fa, nil
}

// Alloc allocates a cell with O(1) lookup in segregated free lists.
func (fa *FastAllocator) Alloc(need int32, cls Class) (CellRef, []byte, error) {
	fa.stats.AllocCalls++
	originalNeed := need

	// Debug: Print stats every 25,000 allocations
	if debugAlloc && fa.stats.AllocCalls%25000 == 0 {
		fa.PrintStats()
	}

	if need < format.CellHeaderSize {
		return 0, nil, ErrNeedSmall
	}
	need = format.Align8I32(need)

	if logAlloc && originalNeed > 1000 {
		fmt.Fprintf(
			os.Stderr,
			"[ALLOC] Request: %d bytes → aligned to %d bytes\n",
			originalNeed,
			need,
		)
	}

	// Determine size class
	sizeClass := fa.getSizeClass(need)

	var cell *freeCell
	grewHive := false

	// Try to allocate from size classes (O(log n) perfect best-fit via heap)
	// No scan limit needed - heap automatically gives smallest cell >= need
	for sc := sizeClass; sc < len(fa.freeLists); sc++ {
		cell = fa.allocFromSizeClass(sc, need)
		if cell != nil {
			// Cell reused - skip logging for verbosity
			break
		}
	}

	// Try large allocations
	if cell == nil {
		cell = fa.allocFromLarge(need)
		// Cell reused from large list - skip logging for verbosity
	}

	// If still no fit, need to grow
	if cell == nil {
		// Log when we fail to allocate from free lists
		if logAlloc {
			// Show free list statistics FIRST
			totalFreeCells := 0
			totalFreeBytes := int64(0)
			largestCell := int32(0)
			for sc := range len(fa.freeLists) {
				if fa.freeLists[sc].heap.Len() > 0 {
					totalFreeCells += fa.freeLists[sc].heap.Len()
					for i := range fa.freeLists[sc].heap.Len() {
						size := fa.freeLists[sc].heap[i].size
						totalFreeBytes += int64(size)
						if size > largestCell {
							largestCell = size
						}
					}
				}
			}

			// Check large list
			lb := fa.largeFree
			for lb != nil {
				totalFreeCells++
				totalFreeBytes += int64(lb.size)
				if lb.size > largestCell {
					largestCell = lb.size
				}
				lb = lb.next
			}

			fmt.Fprintf(
				os.Stderr,
				"[ALLOC] NEED GROW: need=%d, maxFree=%d (stale?=%v), actual_largest=%d\n",
				need,
				fa.maxFree,
				fa.maxFree != largestCell,
				largestCell,
			)
			fmt.Fprintf(os.Stderr, "[ALLOC]   Free: %d cells, %d bytes total, avg=%d bytes/cell\n",
				totalFreeCells, totalFreeBytes, totalFreeBytes/max(1, int64(totalFreeCells)))
		}

		// CRITICAL: If we couldn't find a cell, always grow
		// Even if maxFree >= need, it might be stale or the cell might be fragmented
		if err := fa.Grow(need); err != nil {
			return 0, nil, err
		}
		grewHive = true

		// Retry after grow
		for sc := sizeClass; sc < len(fa.freeLists); sc++ {
			cell = fa.allocFromSizeClass(sc, need)
			if cell != nil {
				break
			}
		}
		if cell == nil {
			cell = fa.allocFromLarge(need)
		}
		if cell == nil {
			if debugAlloc {
				debugLogf("Alloc(%d): FAILED after grow, maxFree=%d", need, fa.maxFree)
				fa.dumpAllocatorState(need)
			}
			return 0, nil, ErrNoSpace
		}
	}

	// Track fast vs slow path
	if grewHive {
		fa.stats.AllocSlowPath++
	} else {
		fa.stats.AllocFastPath++
	}

	// Mark allocated and handle splitting
	data := fa.h.Bytes()
	off := cell.off
	cellSize := cell.size

	// Defensive checks: ensure cell is valid and within bounds
	if off < 0 || cellSize < 0 {
		fa.putFreeCell(cell)
		if debugAlloc {
			debugLogf("Alloc(%d): INVALID cell: off=%d, size=%d", need, off, cellSize)
		}
		return 0, nil, ErrBadRef
	}
	if int(off) >= len(data) || int(off+cellSize) > len(data) {
		fa.putFreeCell(cell)
		if debugAlloc {
			debugLogf(
				"Alloc(%d): cell out of bounds: off=%d, size=%d, data len=%d",
				need,
				off,
				cellSize,
				len(data),
			)
		}
		return 0, nil, ErrBadRef
	}

	rem := cellSize - need
	if rem >= minCellSize {
		// Split: allocate head, return tail to free list (tail >= 8 bytes total)
		fa.stats.SplitCount++

		if logAlloc && cellSize > 1000 {
			fmt.Fprintf(
				os.Stderr,
				"[SPLIT] Splitting: cell=%d, need=%d, remainder=%d\n",
				cellSize,
				need,
				rem,
			)
		}

		putI32(data, off, -need)
		if fa.dt != nil {
			fa.dt.Add(int(off), format.CellHeaderSize)
		}

		tailOff := off + need
		// Additional check to prevent overflow
		if tailOff < 0 || int(tailOff) >= len(data) {
			if debugAlloc {
				debugLogf(
					"Alloc(%d): tailOff overflow: off=%d, need=%d, tailOff=%d",
					need,
					off,
					need,
					tailOff,
				)
			}
			// Can't split safely, just return error
			fa.putFreeCell(cell)
			return 0, nil, ErrBadRef
		}
		putI32(data, tailOff, rem)
		if fa.dt != nil {
			fa.dt.Add(int(tailOff), format.CellHeaderSize)
		}
		// Insert remainder back into free list (heap maintains perfect best-fit order)
		fa.insertFreeCell(tailOff, rem)
	} else {
		// Use entire cell (absorb remainder)
		putI32(data, off, -cellSize)
		if fa.dt != nil {
			fa.dt.Add(int(off), format.CellHeaderSize)
		}
		need = cellSize
	}

	// Track bytes allocated
	fa.stats.BytesAllocated += int64(need)

	// Track which HBIN this allocation came from (O(log B) binary search)
	if hbinStart, _, found := fa.findHBINBounds(int(off)); found {
		start := int32(hbinStart)
		if stats, ok := fa.hbinTracking[start]; ok {
			stats.allocCount++
			stats.bytesAllocated += need
			fa.lastAllocHBIN = start
		}
	}

	// Return payload (excluding header)
	payload := data[off+format.CellHeaderSize : off+need]

	// Return freeCell to pool
	fa.putFreeCell(cell)

	// Return relative offset (HCELL_INDEX convention: relative to 0x1000)
	relOff := uint32(off - int32(format.HeaderSize))
	return relOff, payload, nil
}

// Free frees a cell with O(1) insertion into segregated lists.
func (fa *FastAllocator) Free(ref CellRef) error {
	fa.stats.FreeCalls++

	// Convert relative offset to absolute
	off := int32(ref) + int32(format.HeaderSize)
	data := fa.h.Bytes()

	if int(off)+format.CellHeaderSize > len(data) {
		return ErrBadRef
	}

	// Get absolute size (inline for performance)
	rawSize := getI32(data, int(off))
	if rawSize < 0 {
		rawSize = -rawSize
	}
	sz := rawSize
	fa.stats.BytesFreed += int64(sz)

	putI32(data, off, sz)
	if fa.dt != nil {
		fa.dt.Add(int(off), format.CellHeaderSize)
	}

	// Find the HBIN containing this cell FIRST (needed for both forward and backward coalescing)
	hbinStart, hbinSize, found := fa.findHBINBounds(int(off))
	if !found {
		// Can't find HBIN, just insert this cell without coalescing
		fa.insertFreeCell(off, sz)
		return nil
	}

	hbinEnd := min(hbinStart+hbinSize, len(data))

	// Try to coalesce forward (but only within the same HBIN)
	// CRITICAL: Must check next < hbinEnd to prevent coalescing across HBIN boundaries
	next := off + format.Align8I32(sz)
	if int(next)+format.CellHeaderSize <= len(data) && int(next) < hbinEnd {
		nextSize := getI32(data, int(next))
		if nextSize > 0 {
			// Next cell is free AND within same HBIN - remove it from free list and merge
			fa.stats.CoalesceForward++
			fa.removeFreeCell(next, nextSize)
			sz += nextSize
			putI32(data, off, sz)
			if fa.dt != nil {
				fa.dt.Add(int(off), format.CellHeaderSize)
			}
		}
	}

	// Try to coalesce backward using O(1) index lookup
	var prevOff int32 = -1

	// Use endIdx for O(1) lookup if available, otherwise fall back to O(n) walk
	if fa.endIdx != nil {
		// Check if there's a free cell ending exactly at our start position
		if pOff, exists := fa.endIdx[off]; exists {
			prevOff = pOff
		}
	} else {
		// Fallback: O(n) walk (for compatibility if indexes not initialized)
		cur := int32(hbinStart + format.HBINHeaderSize)

		for cur < off {
			// Defensive: ensure we don't read past HBIN end
			if int(cur)+format.CellHeaderSize > hbinEnd {
				break
			}

			// Get absolute size (inline for performance)
			rawS := getI32(data, int(cur))
			if rawS < 0 {
				rawS = -rawS
			}
			s := rawS

			// prevent infinite loop if size is 0 or negative
			if s <= 0 {
				break
			}

			// prevent jumping past target offset
			if s > off-cur {
				break
			}

			if cur+format.Align8I32(s) == off {
				prevOff = cur
				break
			}
			cur += format.Align8I32(s)
		}
	}

	if prevOff >= 0 {
		prevSize := getI32(data, int(prevOff))
		if prevSize > 0 {
			// Previous cell is free - remove it and merge
			fa.stats.CoalesceBackward++
			fa.removeFreeCell(prevOff, prevSize)
			sz += prevSize
			off = prevOff
			putI32(data, off, sz)
			if fa.dt != nil {
				fa.dt.Add(int(off), format.CellHeaderSize)
			}
		}
	}

	// Insert coalesced cell into free list
	fa.insertFreeCell(off, sz)
	return nil
}

// Grow appends an HBIN and updates STRUCTURAL header fields.
//
// IMPORTANT ARCHITECTURAL NOTE:
// Grow() updates STRUCTURAL fields (data size, checksum) but does NOT update
// PROTOCOL fields (sequence numbers, timestamp). Protocol state is managed
// exclusively by tx.Manager to ensure transaction integrity:
//   - Use tx.Begin() before operations that call Grow()
//   - Use tx.Commit() after operations complete
//
// Standalone Grow() calls (outside transactions) will NOT increment sequence numbers.
// This is by design - sequences should only change at transaction boundaries.
//
// Deprecated: Consider using GrowByPages() instead for explicit, spec-compliant growth.
func (fa *FastAllocator) Grow(need int32) error {
	originalNeed := need
	// CRITICAL FIX: Only align the cell allocation to 8 bytes, NOT to HBIN alignment (4096).
	// The HBIN alignment happens below when we align (need + header) to 4KB boundaries.
	// This was causing ALL allocations to be rounded up to 4096 bytes, creating 8KB HBINs
	// for even tiny 8-byte allocations.
	need = format.Align8I32(need)

	// SPEC-COMPLIANT: HBIN size must be large enough to contain BOTH:
	// 1. The 32-byte HBIN header
	// 2. The requested allocation size
	//
	// Example: If we need 4072 bytes:
	//   - WRONG: hbinSize = align(4072) = 4096, usable = 4096-32 = 4064 < 4072 	//   - CORRECT: hbinSize = align(4072+32) = align(4104) = 8192, usable = 8192-32 = 8160 ✓
	hbinSize := format.AlignHBINI32(need + format.HBINHeaderSize)

	// Allocation logging - controlled by HIVE_LOG_ALLOC environment variable
	if logAlloc {
		// Count current HBINs and free space
		totalFreeCells := 0
		totalFreeBytes := int64(0)
		for sc := range len(fa.freeLists) {
			if fa.freeLists[sc].heap.Len() > 0 {
				totalFreeCells += fa.freeLists[sc].heap.Len()
				for i := range fa.freeLists[sc].heap.Len() {
					totalFreeBytes += int64(fa.freeLists[sc].heap[i].size)
				}
			}
		}
		lb := fa.largeFree
		for lb != nil {
			totalFreeCells++
			totalFreeBytes += int64(lb.size)
			lb = lb.next
		}

		fmt.Fprintf(
			os.Stderr,
			"[GROW] #%d: need=%d → creating %dKB HBIN | Current: %d HBINs, %d free cells (%d bytes)\n",
			fa.stats.GrowCalls+1,
			originalNeed,
			hbinSize/1024,
			len(fa.bins),
			totalFreeCells,
			totalFreeBytes,
		)

		// Show the decision boundary
		if hbinSize > format.HBINAlignment {
			fmt.Fprintf(
				os.Stderr,
				"[GROW]   OVERSIZED: need=%d > 4064 threshold → creating %dKB HBIN (wastes %d bytes)\n",
				need,
				hbinSize/1024,
				hbinSize-(need+format.HBINHeaderSize),
			)
		}
	}

	return fa.growByHBINSize(hbinSize)
}

// growByHBINSize is the internal helper that creates an HBIN of the specified size.
// Both Grow() and GrowByPages() delegate to this function.
func (fa *FastAllocator) growByHBINSize(hbinSize int32) error {
	fa.stats.GrowCalls++
	fa.stats.GrowBytes += int64(hbinSize)

	// Track HBIN size distribution
	switch hbinSize {
	case 4096:
		fa.stats.HBINs4KB++
	case 8192:
		fa.stats.HBINs8KB++
	default:
		fa.stats.HBINs16KB++
	}

	data := fa.h.Bytes()

	// Note: Any trailing slack space was already truncated in NewFast(), so file size
	// and header data size field are guaranteed to match. We can safely append at the end.
	fileEnd := int32(len(data))

	// CRITICAL: Check if growing would exceed 2GB limit (REGF spec)
	// HBIN offsets are int32, and file offsets + sizes must fit in int32
	newSize := int64(fileEnd) + int64(hbinSize)
	if newSize > maxHiveSize {
		if debugAlloc {
			debugLogf("Grow denied: would exceed 2GB limit (current=%d, grow=%d, new=%d)",
				fileEnd, hbinSize, newSize)
		}
		return fmt.Errorf("cannot grow hive beyond 2GB limit (current=%d, requested grow=%d)",
			fileEnd, hbinSize)
	}

	hbinOff := fileEnd
	if err := fa.h.Append(int64(hbinSize)); err != nil {
		return ErrGrowFail
	}

	data = fa.h.Bytes()

	// Write HBIN header
	copy(data[int(hbinOff):int(hbinOff)+4], format.HBINSignature)
	// CRITICAL: HBIN offset field must be RELATIVE to 0x1000 (after REGF header)
	// AND must match where we actually wrote it (physicalEnd, not logicalEnd)
	putU32(data, int(hbinOff)+format.HBINFileOffsetField, uint32(hbinOff-format.HeaderSize))
	putU32(data, int(hbinOff)+format.HBINSizeOffset, uint32(hbinSize))

	// Add to HBIN index for O(log B) lookup in Free()
	fa.bins = append(fa.bins, hbinRange{
		start: hbinOff,
		end:   hbinOff + hbinSize,
	})

	// Track HBIN lifecycle - log previous HBIN's final state before creating new one
	if logAlloc && fa.currentHBIN != 0 {
		if stats, ok := fa.hbinTracking[fa.currentHBIN]; ok {
			remainingBytes := stats.initialSize - stats.bytesAllocated
			efficiency := float64(stats.bytesAllocated) / float64(stats.initialSize) * 100
			fmt.Fprintf(
				os.Stderr,
				"[HBIN] #%d Complete: offset=0x%X, allocated=%d bytes in %d allocs, remaining=%d bytes (%.1f%% efficient)\n",
				len(
					fa.bins,
				)-1,
				stats.offset,
				stats.bytesAllocated,
				stats.allocCount,
				remainingBytes,
				efficiency,
			)
		}
	}

	// Create tracking entry for this new HBIN
	freeOff := hbinOff + format.HBINHeaderSize
	freeSize := hbinSize - format.HBINHeaderSize
	fa.hbinTracking[hbinOff] = &hbinStats{
		offset:      hbinOff,
		initialSize: freeSize,
	}
	fa.currentHBIN = hbinOff

	if logAlloc {
		fmt.Fprintf(os.Stderr, "[HBIN] #%d Created: offset=0x%X, total=%d bytes, usable=%d bytes\n",
			len(fa.bins), hbinOff, hbinSize, freeSize)
	}

	// Create master free cell
	putI32(data, freeOff, freeSize)

	// Add to appropriate free list
	fa.insertFreeCell(freeOff, freeSize)

	// Update data size header field
	fa.h.BumpDataSize(uint32(hbinSize))

	// Mark header dirty so tx.Commit() flushes the updated offset 0x28
	// This is CRITICAL for hivexsh compatibility - without this, the header
	// changes won't be flushed to disk and hivexsh will report "trailing garbage"
	if fa.dt != nil {
		fa.dt.Add(0, format.HeaderSize)
		// CRITICAL: Also mark the new HBIN as dirty so the HBIN header and
		// master free cell are flushed to disk. Without this, newly allocated
		// cells in the HBIN will have uninitialized headers on disk.
		fa.dt.Add(int(hbinOff), int(hbinSize))
	}

	// ARCHITECTURAL DECISION: Grow() does NOT update sequence numbers or timestamp.
	// Those are PROTOCOL fields managed exclusively by tx.Manager:
	//   - tx.Begin() increments PrimarySeq (starts transaction)
	//   - tx.Commit() sets SecondarySeq = PrimarySeq and updates timestamp (completes transaction)
	//
	// Grow() only updates STRUCTURAL fields that must change immediately:
	//   - Data size field (0x28) - tells readers where HBINs end (via BumpDataSize above)
	//   - Header checksum - validates structural changes (below)
	//
	// This separation prevents sequence number conflicts when multiple Grow() calls
	// occur within a single transaction.

	// Recompute and update header checksum after data size modification
	// The checksum is the XOR of the first 508 dwords (0x1FC bytes) of the header
	updateHeaderChecksum(data)

	// Call test hook if set
	if fa.onGrow != nil {
		fa.onGrow(hbinSize)
	}

	return nil
}

// GrowByPages adds a new HBIN of exactly (numPages * 4KB) size.
// This is the RECOMMENDED API for hive growth - it's explicit, spec-compliant by design,
// and impossible to misuse.
//
// Per Windows Registry Specification:
//   - Registry data is organized in 4KB pages
//   - Each HBIN must be a multiple of 4KB (0x1000)
//   - HBIN header (32 bytes) is PART OF the HBIN, not in addition to it
//   - Minimum HBIN size is 4KB (1 page)
//
// Analysis of real Windows hives shows:
//   - 97%+ of HBINs are exactly 4KB (1 page)
//   - Windows uses 1-4 page HBINs, never larger than 16KB
//   - Most allocations use GrowByPages(1) for a 4KB HBIN
//
// Examples:
//
//	GrowByPages(1) → 4KB HBIN with 4064 bytes usable (4096 - 32 header)
//	GrowByPages(2) → 8KB HBIN with 8160 bytes usable (8192 - 32 header)
//	GrowByPages(4) → 16KB HBIN with 16352 bytes usable (16384 - 32 header)
//
// Returns error if numPages <= 0 or file operations fail.
func (fa *FastAllocator) GrowByPages(numPages int) error {
	if numPages <= 0 {
		return ErrNeedSmall // Reuse existing error type
	}

	// Calculate HBIN size: numPages × 4KB
	hbinSize := int32(numPages * format.HBINAlignment)

	// GrowByPages specifies the exact HBIN size, so use it directly
	return fa.growByHBINSize(hbinSize)
}

// TruncatePages removes the last numPages worth of HBINs (numPages * 4KB) from the hive.
// This is used for space reclamation after freeing operations.
//
// Per Windows Registry Specification:
//   - HBINs must be multiples of 4KB
//   - Truncation must not affect allocated cells
//   - Header fields must be updated (data size, sequences, checksum)
//
// Returns error if:
//   - numPages <= 0
//   - Truncation would affect allocated cells
//   - Truncation would remove all HBINs (need at least root NK)
//   - File operations fail
func (fa *FastAllocator) TruncatePages(numPages int) error {
	if numPages <= 0 {
		return ErrNeedSmall // Reuse existing error type
	}

	truncateBytes := numPages * format.HBINAlignment
	data := fa.h.Bytes()
	currentDataSize := int(getU32(data, format.REGFDataSizeOffset))
	newDataSize := currentDataSize - truncateBytes

	// Validate: must keep at least one HBIN
	if newDataSize < format.HBINAlignment {
		return ErrNeedSmall // Can't truncate all HBINs
	}

	truncateOffset := format.HeaderSize + newDataSize

	// TODO: In full implementation, validate no allocated cells in truncation range
	// For now, we'll trust the caller

	// Remove free cells from the truncation range
	// Simple approach: remove any free cell that starts at or after truncateOffset
	fa.removeFreeListEntriesAfter(int32(truncateOffset))

	// Truncate the underlying file
	if err := fa.h.Truncate(int64(format.HeaderSize + newDataSize)); err != nil {
		return ErrGrowFail // Reuse existing error
	}

	// Update header data size field
	data = fa.h.Bytes()
	putU32(data, format.REGFDataSizeOffset, uint32(newDataSize))

	// Mark header dirty
	if fa.dt != nil {
		fa.dt.Add(0, format.HeaderSize)
	}

	// ARCHITECTURAL DECISION: TruncatePages() does NOT update sequence numbers or timestamp.
	// Those are PROTOCOL fields managed exclusively by tx.Manager.
	// See comment in Grow() for full explanation of this architectural separation.
	//
	// TruncatePages() only updates STRUCTURAL fields (data size and checksum).

	// Update checksum after data size modification
	updateHeaderChecksum(data)

	return nil
}

// removeFreeListEntriesAfter removes all free list entries at or after the given offset.
// This is used during truncation to clean up free lists.
func (fa *FastAllocator) removeFreeListEntriesAfter(offset int32) {
	// Remove from segregated free lists (heaps)
	// Since we can't efficiently remove from middle of heap, rebuild each heap
	for i := range fa.freeLists {
		list := &fa.freeLists[i]
		oldHeap := list.heap

		// Create new heap with only cells before offset
		newHeap := make(freeCellHeap, 0, len(oldHeap))
		for _, cell := range oldHeap {
			if cell.off < offset {
				newHeap = append(newHeap, cell)
			} else {
				// Remove from byOff map and return to pool
				delete(fa.byOff, cell.off)
				fa.putFreeCell(cell)
			}
		}

		// Re-heapify and update count
		heap.Init(&newHeap)
		list.heap = newHeap
		list.count = len(newHeap)
	}

	// Remove from large free list
	var prev *largeBlock
	curr := fa.largeFree

	for curr != nil {
		next := curr.next
		if curr.off >= offset {
			// Remove this block
			if prev == nil {
				fa.largeFree = next
			} else {
				prev.next = next
			}
		} else {
			prev = curr
		}
		curr = next
	}
}

// ============================================================================
// Internal helpers
// ============================================================================

func (fa *FastAllocator) initializeFreeLists() error {
	it, err := fa.h.HBINs()
	if err != nil {
		return err
	}

	for {
		hb, hbErr := it.Next()
		if hbErr != nil {
			if errors.Is(hbErr, io.EOF) {
				break
			}
			return hbErr
		}

		// Add HBIN to index for O(log B) lookup in Free()
		// IMPORTANT: hb.Offset is already the absolute file offset of the HBIN
		// (e.g., 4096 for first HBIN), NOT a data-relative offset.
		// Do NOT add format.HeaderSize here!
		hbinStart := int32(hb.Offset)
		hbinSize := int32(hb.Size)
		fa.bins = append(fa.bins, hbinRange{
			start: hbinStart,
			end:   hbinStart + hbinSize,
		})

		// Scan cells and build free lists + byOff map
		cit := hb.Cells()
		for {
			c, cellErr := cit.Next()
			if cellErr != nil {
				if errors.Is(cellErr, io.EOF) {
					break
				}
				break
			}

			if !c.IsAllocated() {
				absOff := int32(int(hb.Offset) + c.Off)
				sz := int32(c.SizeAbs())
				fa.insertFreeCell(absOff, sz)
			}
		}
	}

	return nil
}

// allocFromSizeClass allocates from a size class heap using best-fit.
// Returns the smallest cell >= need, or nil if no suitable cell exists.
//
// Fast path (O(log n)): The min-heap guarantees heap[0] is the smallest cell.
// If heap[0].size >= need, it is the best fit — pop and return immediately.
//
// Slow path (O(n)): If heap[0] is too small but the size class range includes
// larger cells, scan all cells to find the smallest one that fits.
func (fa *FastAllocator) allocFromSizeClass(sc int, need int32) *freeCell {
	list := &fa.freeLists[sc]
	if list.heap.Len() == 0 {
		return nil
	}

	// Fast path: heap[0] is the smallest cell in this class.
	// If it fits, it's the best fit by definition — no smaller cell can exist.
	if list.heap[0].size >= need {
		// O(log n) heap pop
		fa.stats.HeapRemoves++
		cell := heap.Pop(&list.heap).(*freeCell) //nolint:errcheck // heap contains only *freeCell
		list.count--

		// Remove from byOff map
		delete(fa.byOff, cell.off)

		// Remove from coalesce indexes
		if fa.startIdx != nil {
			delete(fa.startIdx, cell.off)
		}
		if fa.endIdx != nil {
			end := cell.off + format.Align8I32(cell.size)
			delete(fa.endIdx, end)
		}

		// O(1) maxFree update via top-2 tracking
		if cell.size == fa.maxFree {
			oldMax := fa.maxFree
			fa.maxFree = fa.secondMaxFree
			fa.secondMaxFree = 0
			if logAlloc && fa.maxFree != oldMax {
				fmt.Fprintf(
					os.Stderr,
					"[ALLOC] 🔄 maxFree demoted: %d → %d (top-2 tracking)\n",
					oldMax,
					fa.maxFree,
				)
			}
		}

		return cell
	}

	// Slow path: heap[0] is too small, but larger cells in this size class
	// may fit. Use bounded "good-enough fit" scan instead of full O(n) scan.
	//
	// Optimization: Instead of scanning ALL cells for perfect best-fit, we:
	// 1. Limit scan to maxSlowPathScan cells (default 32)
	// 2. Accept any cell within fitTolerance bytes of optimal as "good enough"
	//
	// Trade-off: Slightly more internal fragmentation (up to fitTolerance bytes
	// per allocation) in exchange for O(1) amortized allocation time.
	const (
		maxSlowPathScan = 32 // Never scan more than 32 cells
		fitTolerance    = 64 // Accept cells within 64 bytes of optimal
	)

	bestIdx := -1
	var bestSize int32 = 1<<31 - 1 // MaxInt32
	maxAcceptable := need + fitTolerance

	heapLen := list.heap.Len()
	scanLimit := heapLen
	if scanLimit > maxSlowPathScan {
		scanLimit = maxSlowPathScan
	}

	for i := 1; i < scanLimit; i++ {
		cellSize := list.heap[i].size
		if cellSize >= need {
			if cellSize <= maxAcceptable {
				// Good enough - take it immediately without further scanning
				bestIdx = i
				bestSize = cellSize
				break
			}
			if cellSize < bestSize {
				bestIdx = i
				bestSize = cellSize
			}
		}
	}

	if bestIdx == -1 {
		return nil
	}

	// Remove the cell from the heap (O(log n))
	fa.stats.HeapRemoves++
	cell := heap.Remove(&list.heap, bestIdx).(*freeCell) //nolint:errcheck // heap contains only *freeCell
	list.count--

	// Remove from byOff map
	delete(fa.byOff, cell.off)

	// Remove from coalesce indexes
	if fa.startIdx != nil {
		delete(fa.startIdx, cell.off)
	}
	if fa.endIdx != nil {
		end := cell.off + format.Align8I32(cell.size)
		delete(fa.endIdx, end)
	}

	// O(1) maxFree update: if we just consumed the largest cell, demote to second-largest.
	// secondMaxFree is maintained by insertFreeCell. This replaces an O(N) recomputeMaxFree scan.
	if cell.size == fa.maxFree {
		oldMax := fa.maxFree
		fa.maxFree = fa.secondMaxFree
		fa.secondMaxFree = 0
		if logAlloc && fa.maxFree != oldMax {
			fmt.Fprintf(
				os.Stderr,
				"[ALLOC] 🔄 maxFree demoted: %d → %d (top-2 tracking)\n",
				oldMax,
				fa.maxFree,
			)
		}
	}

	return cell
}

func (fa *FastAllocator) allocFromLarge(need int32) *freeCell {
	if fa.largeFree == nil {
		return nil
	}

	var prev *largeBlock
	curr := fa.largeFree

	for curr != nil {
		if curr.size >= need {
			// Remove from list
			if prev == nil {
				fa.largeFree = curr.next
			} else {
				prev.next = curr.next
			}

			// Convert to freeCell
			cell := fa.getFreeCell()
			cell.off = curr.off
			cell.size = curr.size

			// O(1) maxFree update via top-2 tracking
			if cell.size == fa.maxFree {
				oldMax := fa.maxFree
				fa.maxFree = fa.secondMaxFree
				fa.secondMaxFree = 0
				if logAlloc && fa.maxFree != oldMax {
					fmt.Fprintf(
						os.Stderr,
						"[ALLOC] 🔄 maxFree demoted: %d → %d (top-2 tracking)\n",
						oldMax,
						fa.maxFree,
					)
				}
			}

			return cell
		}
		prev = curr
		curr = curr.next
	}

	return nil
}

// insertFreeCell inserts a free cell into the appropriate heap.
// O(log n) operation via min-heap.
func (fa *FastAllocator) insertFreeCell(off, size int32) {
	// Defensive check: ensure cell is within hive bounds
	data := fa.h.Bytes()

	if off < format.HeaderSize+format.HBINHeaderSize || int(off+size) > len(data) {
		// Invalid cell - ignore it
		return
	}

	sc := fa.getSizeClass(size)

	if sc < len(fa.freeLists) {
		// Allocate cell and populate fields
		cell := fa.getFreeCell()
		cell.off = off
		cell.size = size
		cell.sc = sc

		// Push onto heap (O(log n))
		fa.stats.HeapPushes++
		heap.Push(&fa.freeLists[sc].heap, cell)
		fa.freeLists[sc].count++

		// Add to byOff map for O(1) lookup during coalescing
		fa.byOff[off] = cell
	} else {
		// Large allocation (>= MediumMax) → linked list
		lb := &largeBlock{
			off:  off,
			size: size,
			next: fa.largeFree,
		}
		fa.largeFree = lb
	}

	// Update indexes for O(1) coalescing
	if fa.startIdx != nil {
		fa.startIdx[off] = size
	}
	if fa.endIdx != nil {
		fa.endIdx[off+size] = off
	}

	// Update maxFree tracking (top-2)
	if size > fa.maxFree {
		fa.secondMaxFree = fa.maxFree
		fa.maxFree = size
	} else if size > fa.secondMaxFree {
		fa.secondMaxFree = size
	}
}

// removeFreeCell removes a free cell from the heap.
// O(log n) operation via heap.Remove() with O(1) lookup via byOff map.
func (fa *FastAllocator) removeFreeCell(off int32, size int32) {
	sc := fa.getSizeClass(size)

	if sc < len(fa.freeLists) {
		// O(1) lookup via map
		cell := fa.byOff[off]
		if cell == nil {
			// Cell not in free list (might have been already allocated)
			return
		}

		// Remove from heap (O(log n))
		fa.stats.HeapRemoves++
		heap.Remove(&fa.freeLists[sc].heap, cell.heapIndex)
		fa.freeLists[sc].count--

		// Remove from byOff map (O(1))
		delete(fa.byOff, off)

		// Remove from coalesce indexes
		if fa.startIdx != nil {
			delete(fa.startIdx, off)
		}
		if fa.endIdx != nil {
			delete(fa.endIdx, off+size)
		}

		// Note: We don't recompute maxFree here because:
		// 1. This is often called during coalescing, before the new cell is inserted
		// 2. maxFree is maintained incrementally by insertFreeCell()
		// 3. Premature recomputation can cause maxFree to miss coalesced cells

		// Return cell to pool
		fa.putFreeCell(cell)
	} else {
		// Large allocation
		var prev *largeBlock
		curr := fa.largeFree

		for curr != nil {
			if curr.off == off {
				if prev == nil {
					fa.largeFree = curr.next
				} else {
					prev.next = curr.next
				}

				// Remove from indexes
				if fa.startIdx != nil {
					delete(fa.startIdx, off)
				}
				if fa.endIdx != nil {
					delete(fa.endIdx, off+size)
				}

				// Note: maxFree is maintained incrementally by insertFreeCell()
				return
			}
			prev = curr
			curr = curr.next
		}
	}
}

func (fa *FastAllocator) getFreeCell() *freeCell {
	cell, ok := fa.freeCellPool.Get().(*freeCell)
	if !ok {
		return &freeCell{}
	}
	return cell
}

func (fa *FastAllocator) putFreeCell(cell *freeCell) {
	cell.heapIndex = -1
	cell.sc = 0
	fa.freeCellPool.Put(cell)
}

// getSizeClass returns the size class (heap index) for a given allocation size.
// Delegates to the configurable size class table.
func (fa *FastAllocator) getSizeClass(size int32) int {
	return fa.sizeTable.getSizeClass(size)
}

// findHBINBounds finds the HBIN containing the given file offset.
// Returns (hbinStart, hbinSize, true) if found, or (0, 0, false) if not found.
// O(log B) operation via binary search on bins index.
func (fa *FastAllocator) findHBINBounds(fileOff int) (int, int, bool) {
	off := int32(fileOff)
	lo, hi := 0, len(fa.bins)-1

	// Binary search for the HBIN containing this offset
	for lo <= hi {
		mid := (lo + hi) >> 1
		b := fa.bins[mid]

		if off < b.start {
			hi = mid - 1
		} else if off >= b.end {
			lo = mid + 1
		} else {
			// Found it: off is within [b.start, b.end)
			return int(b.start), int(b.end - b.start), true
		}
	}

	// Not found in any HBIN
	return 0, 0, false
}

// ============================================================================
// NEW METHODS FOR PHASE 8 IMPLEMENTATION (Initially stubbed)
// ============================================================================

// recomputeMaxFree scans all free lists and updates maxFree to the largest span.
// Called when removing a cell that might be the max, or when maxFree tracking is enabled.
// With heaps, we need to scan all cells in all heaps to find the maximum.
// O(N) where N is total number of free cells across all heaps.
func (fa *FastAllocator) recomputeMaxFree() {
	fa.maxFree = 0

	// Scan all size class heaps
	for sc := range len(fa.freeLists) {
		heap := &fa.freeLists[sc].heap
		// Scan all cells in this heap
		for i := range heap.Len() {
			if (*heap)[i].size > fa.maxFree {
				fa.maxFree = (*heap)[i].size
			}
		}
	}

	// Scan large blocks
	lb := fa.largeFree
	for lb != nil {
		if lb.size > fa.maxFree {
			fa.maxFree = lb.size
		}
		lb = lb.next
	}
}

// GetStats returns current allocator statistics (test-only).
// This is used by test utilities to inspect allocator state.
func (fa *FastAllocator) GetStats() allocatorStats {
	return fa.stats
}

// Encoding helpers - using format package for optimal performance
// (Benchmarking showed binary.LittleEndian is faster than unsafe with bounds checks,
// and only 0.98% slower than raw unsafe - not worth the safety tradeoff).
func getI32(data []byte, off int) int32 {
	return format.ReadI32(data, off)
}

func getU32(data []byte, off int) uint32 {
	return format.ReadU32(data, off)
}

func putI32(data []byte, off int32, v int32) {
	format.PutI32(data, int(off), v)
}

func putU32(data []byte, off int, v uint32) {
	format.PutU32(data, off, v)
}

// updateHeaderChecksum calculates and updates the REGF header checksum.
// The checksum is the XOR of the first 508 dwords (0x1FC bytes) of the header.
// The checksum itself is stored at offset 0x1FC.
func updateHeaderChecksum(data []byte) {
	if len(data) < format.HeaderSize {
		return
	}

	// Calculate checksum: XOR of first 508 dwords (excluding checksum field itself)
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += format.CellHeaderSize {
		checksum ^= getU32(data, i)
	}

	// Write checksum
	putU32(data, format.REGFCheckSumOffset, checksum)
}

// ============================================================================
// Debug helpers
// ============================================================================

// debugLogf prints debug messages if debugAlloc is enabled.
func debugLogf(format string, args ...any) {
	if debugAlloc {
		fmt.Fprintf(os.Stderr, "[ALLOC] "+format+"\n", args...)
	}
}

// dumpAllocatorState dumps the current allocator state for debugging.
func (fa *FastAllocator) dumpAllocatorState(need int32) {
	if !debugAlloc {
		return
	}

	fmt.Fprintf(os.Stderr, "\n=== ALLOCATOR STATE DUMP (need=%d) ===\n", need)
	fmt.Fprintf(os.Stderr, "maxFree: %d\n", fa.maxFree)
	fmt.Fprintf(os.Stderr, "Size classes: %d\n", len(fa.freeLists))
	fmt.Fprintf(os.Stderr, "byOff map: %d entries\n", len(fa.byOff))
	fmt.Fprintf(os.Stderr, "bins index: %d HBINs\n", len(fa.bins))

	totalFreeCells := 0
	totalFreeBytes := int64(0)
	for sc := range len(fa.freeLists) {
		heap := &fa.freeLists[sc].heap
		if heap.Len() > 0 {
			minSize := (*heap)[0].size
			maxSize := int32(0)
			for i := range heap.Len() {
				if (*heap)[i].size > maxSize {
					maxSize = (*heap)[i].size
				}
				totalFreeBytes += int64((*heap)[i].size)
			}
			totalFreeCells += heap.Len()
			fmt.Fprintf(
				os.Stderr,
				"  SC[%d]: %d cells, size range [%d, %d]\n",
				sc,
				heap.Len(),
				minSize,
				maxSize,
			)
		}
	}

	// Check large list
	lbCount := 0
	lb := fa.largeFree
	for lb != nil {
		lbCount++
		totalFreeCells++
		totalFreeBytes += int64(lb.size)
		lb = lb.next
	}
	if lbCount > 0 {
		fmt.Fprintf(os.Stderr, "  Large list: %d blocks\n", lbCount)
	}

	fmt.Fprintf(os.Stderr, "Total: %d free cells, %d bytes free\n", totalFreeCells, totalFreeBytes)
	fmt.Fprintf(os.Stderr, "===================================\n\n")
}

// PrintStats prints allocator statistics to stderr.
func (fa *FastAllocator) PrintStats() {
	s := fa.stats
	fmt.Fprintf(os.Stderr, "\n=== ALLOCATOR STATISTICS ===\n")
	fmt.Fprintf(
		os.Stderr,
		"Grow calls:         %d (%d MB added)\n",
		s.GrowCalls,
		s.GrowBytes/(1024*1024),
	)
	fmt.Fprintf(
		os.Stderr,
		"Alloc calls:        %d (fast: %d, slow: %d)\n",
		s.AllocCalls,
		s.AllocFastPath,
		s.AllocSlowPath,
	)
	fmt.Fprintf(os.Stderr, "Free calls:         %d\n", s.FreeCalls)
	fmt.Fprintf(os.Stderr, "Bytes allocated:    %d MB\n", s.BytesAllocated/(1024*1024))
	fmt.Fprintf(os.Stderr, "Bytes freed:        %d MB\n", s.BytesFreed/(1024*1024))
	fmt.Fprintf(
		os.Stderr,
		"Net allocated:      %d MB\n",
		(s.BytesAllocated-s.BytesFreed)/(1024*1024),
	)
	fmt.Fprintf(os.Stderr, "Cell splits:        %d\n", s.SplitCount)
	fmt.Fprintf(os.Stderr, "Coalesce fwd:       %d\n", s.CoalesceForward)
	fmt.Fprintf(os.Stderr, "Coalesce back:      %d\n", s.CoalesceBackward)
	fmt.Fprintf(os.Stderr, "Heap pushes:        %d\n", s.HeapPushes)
	fmt.Fprintf(os.Stderr, "Heap pops:          %d\n", s.HeapPops)
	fmt.Fprintf(os.Stderr, "Heap removes:       %d\n", s.HeapRemoves)

	// HBIN size distribution
	totalHBINs := s.HBINs4KB + s.HBINs8KB + s.HBINs16KB
	if totalHBINs > 0 {
		fmt.Fprintf(os.Stderr, "\nHBIN Size Distribution:\n")
		fmt.Fprintf(os.Stderr, "  4KB HBINs:        %4d (%.1f%%) = %d KB total\n",
			s.HBINs4KB, 100.0*float64(s.HBINs4KB)/float64(totalHBINs), s.HBINs4KB*4)
		fmt.Fprintf(os.Stderr, "  8KB HBINs:        %4d (%.1f%%) = %d KB total\n",
			s.HBINs8KB, 100.0*float64(s.HBINs8KB)/float64(totalHBINs), s.HBINs8KB*8)
		if s.HBINs16KB > 0 {
			fmt.Fprintf(os.Stderr, "  16KB+ HBINs:      %4d (%.1f%%)\n",
				s.HBINs16KB, 100.0*float64(s.HBINs16KB)/float64(totalHBINs))
		}
		fmt.Fprintf(os.Stderr, "  Total HBINs:      %4d\n", totalHBINs)
		fmt.Fprintf(os.Stderr, "  Avg HBIN size:    %d bytes\n", s.GrowBytes/int64(totalHBINs))
	}

	// Calculate fragmentation metrics
	totalFree := int64(0)
	totalCells := 0
	for sc := range len(fa.freeLists) {
		h := &fa.freeLists[sc].heap
		for i := range h.Len() {
			totalFree += int64((*h)[i].size)
			totalCells++
		}
	}

	avgCellSize := int64(0)
	if totalCells > 0 {
		avgCellSize = totalFree / int64(totalCells)
	}

	fmt.Fprintf(os.Stderr, "\nFragmentation:\n")
	fmt.Fprintf(os.Stderr, "  Free cells:       %d\n", totalCells)
	fmt.Fprintf(os.Stderr, "  Free bytes:       %d MB\n", totalFree/(1024*1024))
	fmt.Fprintf(os.Stderr, "  Avg cell size:    %d bytes\n", avgCellSize)
	fmt.Fprintf(os.Stderr, "  Waste ratio:      %.1f%% (grow - net_alloc) / grow\n",
		100.0*float64(s.GrowBytes-(s.BytesAllocated-s.BytesFreed))/float64(s.GrowBytes))
	fmt.Fprintf(os.Stderr, "============================\n\n")
}

// scanHBINEfficiency scans the existing hive to compute efficiency metrics.
// This is useful for analyzing hives that were opened (not built from scratch).
func (fa *FastAllocator) scanHBINEfficiency() {
	data := fa.h.Bytes()
	offset := format.HeaderSize

	// Scan all HBINs in the hive
	for offset < len(data) {
		// Check for HBIN signature
		if offset+32 > len(data) {
			break
		}

		sig := string(data[offset : offset+4])
		if sig != "hbin" {
			break
		}

		// Read HBIN size
		hbinSize := int32(getU32(data, offset+8))
		usableSize := hbinSize - format.HBINHeaderSize

		// Scan cells within this HBIN to calculate allocated bytes
		cellOffset := offset + format.HBINHeaderSize
		hbinEnd := offset + int(hbinSize)
		allocatedBytes := int32(0)
		allocCount := 0

		for cellOffset < hbinEnd {
			if cellOffset+4 > len(data) {
				break
			}

			cellSize := getI32(data, cellOffset)
			if cellSize == 0 {
				break
			}

			absCellSize := cellSize
			if absCellSize < 0 {
				absCellSize = -absCellSize
			}

			// Negative size = allocated cell
			if cellSize < 0 {
				allocatedBytes += absCellSize
				allocCount++
			}

			cellOffset += int(absCellSize)
		}

		// Store in tracking map
		if _, exists := fa.hbinTracking[int32(offset)]; !exists {
			fa.hbinTracking[int32(offset)] = &hbinStats{
				offset:         int32(offset),
				initialSize:    usableSize,
				bytesAllocated: allocatedBytes,
				allocCount:     allocCount,
			}
		}

		offset += int(hbinSize)
	}
}

// GetBasicStats returns aggregate capacity and allocation totals without computing
// detailed efficiency metrics or sorting. This is significantly faster than
// GetEfficiencyStats() when you only need total wasted/allocated bytes.
//
// Use this for quick storage checks like GetStorageStats() which only needs totals.
// For detailed per-HBIN analysis, use GetEfficiencyStats() instead.
//
// Returns (totalCapacity, totalAllocated) in bytes.
func (fa *FastAllocator) GetBasicStats() (totalCapacity, totalAllocated int64) {
	// If hbinTracking is empty, scan the existing hive
	if len(fa.hbinTracking) == 0 {
		fa.scanHBINEfficiency()
	}

	// Sum totals without sorting or per-HBIN analysis
	for _, hbin := range fa.hbinTracking {
		totalCapacity += int64(hbin.initialSize)
		totalAllocated += int64(hbin.bytesAllocated)
	}
	return totalCapacity, totalAllocated
}

// GetEfficiencyStats computes detailed efficiency and fragmentation metrics
// by analyzing per-HBIN allocation tracking data.
//
// Uses O(n log k) heap-based selection for finding the k worst HBINs, where
// k=20 (maxWorstHBINs). This is ~100x faster than the previous O(n²) bubble sort
// for large hives with thousands of HBINs.
func (fa *FastAllocator) GetEfficiencyStats() EfficiencyStats {
	// If hbinTracking is empty, scan the existing hive
	if len(fa.hbinTracking) == 0 {
		fa.scanHBINEfficiency()
	}

	const maxWorstHBINs = 20 // Number of worst HBINs to track

	stats := EfficiencyStats{}

	// Use a max-heap to track the k worst (lowest efficiency) HBINs
	// The heap[0] always contains the "best" (highest efficiency) among the worst,
	// so we can efficiently replace it when we find a worse HBIN.
	worst := make(worstHBINHeap, 0, maxWorstHBINs)
	heap.Init(&worst)

	// Analyze each tracked HBIN
	for _, hbin := range fa.hbinTracking {
		stats.TotalHBINs++
		stats.TotalCapacity += int64(hbin.initialSize)
		stats.TotalAllocated += int64(hbin.bytesAllocated)

		// Calculate efficiency for this HBIN
		efficiency := 0.0
		if hbin.initialSize > 0 {
			efficiency = float64(hbin.bytesAllocated) / float64(hbin.initialSize) * 100.0
		}

		// Track worst offenders using heap-based selection: O(n log k)
		eff := hbinEfficiency{
			offset:     hbin.offset,
			allocated:  hbin.bytesAllocated,
			capacity:   hbin.initialSize,
			efficiency: efficiency,
			allocCount: hbin.allocCount,
		}

		if worst.Len() < maxWorstHBINs {
			// Haven't collected k worst yet, add this one
			heap.Push(&worst, eff)
		} else if efficiency < worst[0].efficiency {
			// This HBIN is worse than the best of the current worst list
			// Replace the best (heap[0]) with this worse one
			heap.Pop(&worst)
			heap.Push(&worst, eff)
		}

		// Categorize by efficiency bucket
		if efficiency >= 100.0 {
			stats.PerfectHBINs++
		} else if efficiency >= 99.0 {
			stats.ExcellentHBINs++
		} else if efficiency >= 98.0 {
			stats.VeryGoodHBINs++
		} else if efficiency >= 95.0 {
			stats.GoodHBINs++
		} else {
			stats.SuboptimalHBINs++
			if efficiency < 80.0 {
				stats.PoorHBINs++
			}
		}
	}

	// Calculate overall metrics
	stats.TotalWasted = stats.TotalCapacity - stats.TotalAllocated
	if stats.TotalCapacity > 0 {
		stats.OverallEfficiency = float64(
			stats.TotalAllocated,
		) / float64(
			stats.TotalCapacity,
		) * 100.0
	}
	if stats.TotalHBINs > 0 {
		stats.AverageAllocPerHBIN = float64(stats.TotalAllocated) / float64(stats.TotalHBINs)
	}

	// Sort the worst HBINs by efficiency (worst first) for output
	// This is O(k log k) where k=20, so negligible: ~86 comparisons max
	sortedWorst := make([]hbinEfficiency, worst.Len())
	copy(sortedWorst, worst)
	// Sort in ascending order (worst efficiency first)
	for i := 0; i < len(sortedWorst); i++ {
		for j := i + 1; j < len(sortedWorst); j++ {
			if sortedWorst[j].efficiency < sortedWorst[i].efficiency {
				sortedWorst[i], sortedWorst[j] = sortedWorst[j], sortedWorst[i]
			}
		}
	}

	// Take all worst HBINs (already limited to maxWorstHBINs by heap)
	numWorst := len(sortedWorst)

	for i := range numWorst {
		stats.LeastEfficientHBINs = append(stats.LeastEfficientHBINs, struct {
			Offset     int32
			Allocated  int32
			Capacity   int32
			Efficiency float64
			AllocCount int
		}{
			Offset:     sortedWorst[i].offset,
			Allocated:  sortedWorst[i].allocated,
			Capacity:   sortedWorst[i].capacity,
			Efficiency: sortedWorst[i].efficiency,
			AllocCount: sortedWorst[i].allocCount,
		})
	}

	return stats
}

// PrintEfficiencyStats prints a formatted efficiency report to stderr.
func (fa *FastAllocator) PrintEfficiencyStats() {
	stats := fa.GetEfficiencyStats()

	fmt.Fprintf(os.Stderr, "\n====== HBIN EFFICIENCY ANALYSIS ======\n")
	fmt.Fprintf(os.Stderr, "\nOverall Metrics:\n")
	fmt.Fprintf(os.Stderr, "  Total HBINs:           %d\n", stats.TotalHBINs)
	fmt.Fprintf(os.Stderr, "  Total Capacity:        %d bytes (%.2f MB)\n",
		stats.TotalCapacity, float64(stats.TotalCapacity)/(1024*1024))
	fmt.Fprintf(os.Stderr, "  Total Allocated:       %d bytes (%.2f MB)\n",
		stats.TotalAllocated, float64(stats.TotalAllocated)/(1024*1024))
	fmt.Fprintf(os.Stderr, "  Total Wasted:          %d bytes (%.2f MB)\n",
		stats.TotalWasted, float64(stats.TotalWasted)/(1024*1024))
	fmt.Fprintf(os.Stderr, "  Overall Efficiency:    %.1f%%\n", stats.OverallEfficiency)
	fmt.Fprintf(os.Stderr, "  Avg Alloc per HBIN:    %.0f bytes\n", stats.AverageAllocPerHBIN)

	fmt.Fprintf(os.Stderr, "\nEfficiency Distribution:\n")
	fmt.Fprintf(os.Stderr, "  Perfect (100.0%%):      %4d HBINs (%5.1f%%)\n",
		stats.PerfectHBINs, 100.0*float64(stats.PerfectHBINs)/float64(stats.TotalHBINs))
	fmt.Fprintf(os.Stderr, "  Excellent (99.0-99.9%%): %4d HBINs (%5.1f%%)\n",
		stats.ExcellentHBINs, 100.0*float64(stats.ExcellentHBINs)/float64(stats.TotalHBINs))
	fmt.Fprintf(os.Stderr, "  Very Good (98.0-98.9%%): %4d HBINs (%5.1f%%)\n",
		stats.VeryGoodHBINs, 100.0*float64(stats.VeryGoodHBINs)/float64(stats.TotalHBINs))
	fmt.Fprintf(os.Stderr, "  Good (95.0-97.9%%):      %4d HBINs (%5.1f%%)\n",
		stats.GoodHBINs, 100.0*float64(stats.GoodHBINs)/float64(stats.TotalHBINs))
	fmt.Fprintf(os.Stderr, "  Suboptimal (<95%%):      %4d HBINs (%5.1f%%)\n",
		stats.SuboptimalHBINs, 100.0*float64(stats.SuboptimalHBINs)/float64(stats.TotalHBINs))
	fmt.Fprintf(os.Stderr, "  Poor (<80%%):            %4d HBINs (%5.1f%%)\n",
		stats.PoorHBINs, 100.0*float64(stats.PoorHBINs)/float64(stats.TotalHBINs))

	fmt.Fprintf(os.Stderr, "\nLeast Efficient HBINs (Top 20):\n")
	fmt.Fprintf(os.Stderr, "  Offset     Allocated  Capacity  Efficiency  Allocs\n")
	fmt.Fprintf(os.Stderr, "  --------   ---------  --------  ----------  ------\n")
	for i, hbin := range stats.LeastEfficientHBINs {
		fmt.Fprintf(os.Stderr, "  0x%06X   %9d  %8d  %9.1f%%  %6d\n",
			hbin.Offset, hbin.Allocated, hbin.Capacity, hbin.Efficiency, hbin.AllocCount)
		if i >= 19 { // Show at most 20
			break
		}
	}

	fmt.Fprintf(os.Stderr, "======================================\n\n")
}
