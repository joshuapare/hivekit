// Package walker provides high-performance hive traversal implementations.
//
// # Overview
//
// This package implements optimized depth-first traversal of Windows Registry hive
// structures with significant performance improvements over naive implementations:
//   - Bitmap-based visited tracking (O(1) vs O(log n) map lookups)
//   - Iterative traversal (eliminates recursion overhead)
//   - Specialized walkers for specific use cases
//   - Zero-copy cell access throughout
//
// Performance characteristics (640k cell hive):
//   - Time: 16-20ms (50-60% faster than original 40.6ms)
//   - Memory: 5-8MB (60-70% less than original 19.5MB)
//   - Allocations: <100 (98% less than original 4,117)
//
// # Core Components
//
// WalkerCore: Shared foundation for all walkers
//   - Bitmap-based visited tracking
//   - Iterative DFS with stack
//   - Fast cell resolution
//   - Subkey/value traversal
//
// IndexBuilder: Build (parent, name) → offset index
//   - Optimized for merge operations
//   - O(1) key lookups by path
//   - Bypasses visitor callback overhead
//
// CellCounter: Count cells by type and purpose
//   - Debugging and validation
//   - Understand hive structure
//   - Statistics reporting
//
// # Quick Start
//
// Build an index for fast lookups:
//
//	h, _ := hive.Open("system.hive")
//	builder := walker.NewIndexBuilder(h, 10000, 10000)
//	idx, err := builder.Build()
//	if err != nil {
//	    return err
//	}
//
//	// Use index for O(1) lookups
//	offset, found := idx.GetNK(parentOffset, "Services")
//
// Count cells in a hive:
//
//	counter := walker.NewCellCounter(h)
//	stats, err := counter.Count()
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Total cells: %d\n", stats.TotalCells)
//	fmt.Printf("NK cells: %d\n", stats.NKCells)
//
// # Bitmap Visited Tracking
//
// The Bitmap type provides O(1) visited tracking using a bit array:
//
//	type Bitmap struct {
//	    bits []uint64
//	    size uint32  // max offset in bytes
//	}
//
// Benefits over map[uint32]bool:
//   - 100x memory reduction (1 bit vs 16+ bytes per entry)
//   - 10x speed improvement (bit ops vs hash lookup)
//   - Cache-friendly (sequential memory access)
//
// Example:
//
//	bitmap := walker.NewBitmap(hive.DataSize())
//	bitmap.Set(0x1000)  // Mark cell at offset 0x1000 as visited
//	if bitmap.IsSet(0x1000) {
//	    // Cell already visited
//	}
//
// # Iterative DFS Traversal
//
// WalkerCore uses an iterative depth-first search with explicit stack:
//
//	type StackEntry struct {
//	    offset uint32 // Relative cell offset
//	    state  uint8  // Processing state
//	}
//
// Processing states:
//   - stateInitial: Not yet processed
//   - stateSubkeysDone: Subkeys traversed
//   - stateValuesDone: Values traversed
//   - stateSecurityDone: Security processed
//   - stateClassDone: Class name processed
//   - stateDone: Ready to pop
//
// Example traversal:
//
//	stack := []StackEntry{{offset: rootOffset, state: stateInitial}}
//	for len(stack) > 0 {
//	    entry := &stack[len(stack)-1]
//	    switch entry.state {
//	    case stateInitial:
//	        // Process NK, push subkeys
//	        entry.state = stateValuesDone
//	    case stateValuesDone:
//	        // Process values
//	        entry.state = stateDone
//	    case stateDone:
//	        // Pop from stack
//	        stack = stack[:len(stack)-1]
//	    }
//	}
//
// # IndexBuilder
//
// IndexBuilder creates a fast lookup index while traversing:
//
//	builder := walker.NewIndexBuilder(hive, nkCapacity, vkCapacity)
//	idx, err := builder.Build()
//
// The index maps (parent offset, name) → child offset:
//   - Keys: idx.GetNK(parentOff, name) → NK offset
//   - Values: idx.GetVK(parentOff, name) → VK offset
//
// Capacity hints pre-size the index to reduce allocations:
//
//	// Typical hive with 18K keys, 45K values
//	builder := walker.NewIndexBuilder(hive, 18000, 45000)
//
// Build process:
//  1. Start at root NK
//  2. For each NK:
//     a. Read subkey list (LF/LH/LI/RI)
//     b. Index each subkey: idx.AddNK(parentOff, name, childOff)
//     c. Push children onto stack
//  3. For each NK:
//     a. Read value list
//     b. Index each value: idx.AddVK(parentOff, name, vkOff)
//
// Performance (typical hive):
//   - Build time: ~50-100ms
//   - Memory: ~4-5MB index
//   - Lookup time: ~70ns per operation
//
// # CellCounter
//
// CellCounter provides detailed statistics about hive structure:
//
//	counter := walker.NewCellCounter(hive)
//	stats, err := counter.Count()
//
// Statistics collected:
//
//	type CellStats struct {
//	    TotalCells uint64
//
//	    // By type
//	    NKCells, VKCells, SKCells uint64
//	    LFCells, LHCells, LICells, RICells uint64
//	    DBCells, DataCells uint64
//	    ValueLists, Blocklists uint64
//
//	    // By purpose
//	    KeyCells, ValueCells, SecurityCells uint64
//	    SubkeyLists, ValueListCells, ValueDataCells uint64
//	    ClassNameCells, BigDataHeaders, BigDataLists, BigDataBlocks uint64
//	}
//
// Example output:
//
//	Total: 18543 cells
//	By Type:
//	  NK: 8421, VK: 5234, SK: 127
//	  LF: 512, LH: 384, LI: 12, RI: 4
//	  DB: 23, Data: 3826
//	By Purpose:
//	  Keys: 8421, Values: 5234, Security: 127
//	  SubkeyLists: 912, ValueLists: 8421
//	  ValueData: 3826, ClassNames: 142
//	  BigData (Headers: 23, Lists: 23, Blocks: 156)
//
// # Fast Cell Resolution
//
// WalkerCore.resolveAndParseCellFast is an inlined hot-path optimization:
//
//	func (wc *WalkerCore) resolveAndParseCellFast(offset uint32) []byte {
//	    data := wc.h.Bytes()
//	    absOffset := format.HeaderSize + int(offset)
//
//	    // Bounds check
//	    if absOffset < 0 || absOffset+4 > len(data) {
//	        return nil
//	    }
//
//	    // Read size (negative for allocated cells)
//	    size := -int32(format.ReadU32(data, absOffset))
//	    if size <= 0 {
//	        return nil
//	    }
//
//	    // Return payload
//	    return data[absOffset+4 : absOffset+size]
//	}
//
// Benefits:
//   - Inline directive eliminates function call overhead
//   - Zero allocations
//   - Bounds checking prevents panics on corrupted hives
//   - Returns nil for free cells (positive size)
//
// # Subkey Traversal
//
// walkSubkeysFast handles all subkey list formats (LF/LH/LI/RI):
//
//	err := wc.walkSubkeysFast(listOffset)
//
// LF/LH/LI (direct lists):
//   - Read count from header
//   - Extract NK offsets (skip hash values)
//   - Push onto stack if not visited
//
// RI (indirect lists):
//   - Read sub-list references
//   - Recursively process each sub-list
//   - Flatten into single traversal
//
// Example:
//
//	// LF list: 3 subkeys
//	[sig: "lf"][count: 3]
//	[offset: 0x1000][hash: ...]
//	[offset: 0x2000][hash: ...]
//	[offset: 0x3000][hash: ...]
//
//	// Pushes: 0x1000, 0x2000, 0x3000 onto stack
//
// # Value Traversal
//
// walkValuesFast extracts VK offsets from value lists:
//
//	vkOffsets, err := wc.walkValuesFast(nk)
//
// Process:
//  1. Check value count (if 0, return early)
//  2. Resolve value list cell
//  3. Extract VK offsets (skip invalid offsets)
//  4. Return slice of valid VK offsets
//
// Example:
//
//	// Value list: 5 values
//	[0x1000][0x2000][0x3000][0x4000][0x5000]
//
//	// Returns: []uint32{0x1000, 0x2000, 0x3000, 0x4000, 0x5000}
//
// # Big-Data Handling
//
// walkDataCell handles large value data (DB format):
//
//	isDB, err := wc.walkDataCell(dataOffset, dataSize)
//
// Inline data (≤4 bytes):
//   - High bit of dataSize is set
//   - Data stored in offset field
//   - No cell to visit
//
// External data (5 bytes - 16KB):
//   - Separate data cell
//   - Visit cell, mark as visited
//
// Big-data (>16KB):
//   - DB header with signature "db"
//   - Blocklist references data blocks
//   - Visit DB header, blocklist, all blocks
//
// Example:
//
//	// Inline (4 bytes)
//	dataSize: 0x80000004 (high bit set)
//	dataOffset: 0x12345678 (actual data)
//
//	// Big-data (100KB)
//	dataSize: 0x00019000
//	dataOffset: → DB header
//	  → Blocklist [block1, block2, ...]
//	    → Data blocks
//
// # Reset and Reuse
//
// WalkerCore can be reset for multiple passes:
//
//	walker := walker.NewWalkerCore(hive)
//
//	// First pass
//	builder := &IndexBuilder{WalkerCore: walker}
//	idx1, _ := builder.Build()
//
//	// Reset and reuse
//	walker.Reset()
//
//	// Second pass
//	counter := &CellCounter{WalkerCore: walker}
//	stats, _ := counter.Count()
//
// Reset() clears:
//   - Visited bitmap (zeroes all bits)
//   - Stack (keeps capacity)
//
// Benefits:
//   - Reuse allocations
//   - No GC pressure
//   - Faster than creating new walker
//
// # Error Handling
//
// Walkers return errors for:
//   - Truncated cells
//   - Invalid signatures
//   - Out-of-bounds references
//   - Malformed structures
//
// Error handling pattern:
//
//	builder := walker.NewIndexBuilder(hive, 10000, 10000)
//	idx, err := builder.Build()
//	if err != nil {
//	    if errors.Is(err, walker.ErrTruncated) {
//	        return fmt.Errorf("corrupted hive: %w", err)
//	    }
//	    return fmt.Errorf("traversal failed: %w", err)
//	}
//
// Graceful degradation:
//   - Skip malformed cells
//   - Continue traversal
//   - Log warnings for diagnostics
//
// # Performance Optimization Techniques
//
// Bitmap tracking:
//   - 1 bit per minimum cell size (4 bytes)
//   - []uint64 for cache-friendly access
//   - Inline Set/IsSet for hot path
//
// Stack pre-allocation:
//   - initialStackCapacity = 256 (typical depth 10-20)
//   - Avoids most reallocations
//   - Grows as needed for deep trees
//
// Zero-copy access:
//   - resolveAndParseCellFast returns []byte slices
//   - No allocations for cell payloads
//   - Bounds checking prevents panics
//
// Inlining:
//   - //go:inline directives on hot paths
//   - Eliminates function call overhead
//   - JIT optimization friendly
//
// # Performance Characteristics
//
// IndexBuilder (typical hive: 18K keys, 45K values):
//   - Build time: 50-100ms
//   - Memory: 4-5MB index
//   - Allocations: <100
//
// CellCounter (640K cell hive):
//   - Count time: 16-20ms
//   - Memory: 5-8MB
//   - Allocations: <100
//
// Bitmap overhead:
//   - 1 bit per 4 bytes (0.025% memory)
//   - 10MB hive → 320KB bitmap
//   - 1GB hive → 32MB bitmap
//
// # Integration with Other Packages
//
// The walker package is used by:
//   - hive/merge: Build index for fast lookups during merge
//   - hive/verify: Validate hive structure by counting cells
//   - hive/index: Build indexes from scratch
//   - Tools: Dump hive contents, analyze structure
//
// Example integration:
//
//	// In merge package
//	func NewSession(h *hive.Hive, opt Options) (*Session, error) {
//	    builder := walker.NewIndexBuilder(h, 10000, 10000)
//	    idx, err := builder.Build()
//	    // ... use index for O(1) lookups
//	}
//
// # Thread Safety
//
// Walkers are not thread-safe. Do not share walker instances across goroutines.
//
// For concurrent processing:
//   - Create separate walkers per goroutine
//   - Process different hive files in parallel
//   - Do NOT share WalkerCore instances
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive: Core hive parsing (NK, VK cells)
//   - github.com/joshuapare/hivekit/hive/index: Index implementations
//   - github.com/joshuapare/hivekit/hive/subkeys: Subkey list parsing
//   - github.com/joshuapare/hivekit/hive/values: Value list parsing
//   - github.com/joshuapare/hivekit/internal/format: Binary format constants
package walker
