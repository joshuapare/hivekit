// Package strategy provides write strategy implementations for registry hive merge operations.
//
// # Overview
//
// This package defines the Strategy interface and provides three implementations
// that control how merge operations modify hive cells:
//   - InPlace: Mutate cells in-place when possible (may fragment over time)
//   - Append: Always allocate new cells, never free (safe, higher space usage)
//   - Hybrid: Heuristic-based selection between InPlace and Append (default)
//
// All strategies properly track dirty pages via DirtyTracker to ensure
// transaction commits flush the correct ranges.
//
// # Strategy Interface
//
// All strategies must implement four operations:
//
//	type Strategy interface {
//	    EnsureKey(path []string) (nkRef uint32, keysCreated int, err error)
//	    SetValue(path []string, name string, typ uint32, data []byte) error
//	    DeleteValue(path []string, name string) error
//	    DeleteKey(path []string, recursive bool) error
//	}
//
// # InPlace Strategy
//
// Characteristics:
//   - Mutates cells in-place when new data fits
//   - Frees cells that are replaced or deleted
//   - May reuse freed cells for new allocations
//   - May fragment over time as values grow
//
// Best for:
//   - Small updates with stable value sizes
//   - Workloads where space efficiency matters
//   - Configuration updates (values don't grow much)
//
// Trade-offs:
//   - Space efficient: Frees and reuses cells
//   - Good for stable sizes: No wasted space
//   - May fragment: Large updates cause fragmentation
//
// Example:
//
//	strategy := strategy.NewInPlace(hive, allocator, dirtyTracker, index)
//	nkRef, keysCreated, err := strategy.EnsureKey([]string{"Software", "Test"})
//	err = strategy.SetValue([]string{"Software", "Test"}, "Version", format.REGSZ, []byte("1.0\x00"))
//
// # Append Strategy
//
// Characteristics:
//   - Always allocates new cells for updates
//   - Never frees old cells (wraps allocator to make Free() a no-op)
//   - Orphaned cells remain in hive
//   - Hive grows monotonically
//
// Best for:
//   - Append-only workloads (logs, audit trails)
//   - Crash recovery scenarios (orphaned cells are safe)
//   - Avoiding Free() overhead
//   - Workloads where disk space is abundant
//
// Trade-offs:
//   - Fastest: No Free() overhead
//   - Safest: No risk of use-after-free bugs
//   - Simple: No fragmentation tracking needed
//   - Wastes space: Orphaned cells never reclaimed
//   - Grows forever: No compaction
//
// Example:
//
//	strategy := strategy.NewAppend(hive, allocator, dirtyTracker, index)
//	err = strategy.SetValue([]string{"Logs", "Event"}, "Entry", format.REGBinary, eventData)
//	// Old "Entry" value cell is orphaned, new cell allocated
//
// Implementation detail:
//
// Append wraps the allocator with noFreeAllocator which makes Free() a no-op:
//
//	type noFreeAllocator struct {
//	    *alloc.FastAllocator
//	}
//
//	func (nfa *noFreeAllocator) Free(ref uint32) error {
//	    return nil  // No-op: never free cells
//	}
//
// # Hybrid Strategy
//
// Characteristics:
//   - Chooses between InPlace and Append based on heuristics
//   - Uses slack percentage to determine if in-place update fits
//   - Small values (<1KB) always use InPlace
//   - Large values (≥1KB) use Append
//   - Deletes always use InPlace (to free cells)
//   - Keys always use InPlace (small, rarely updated)
//
// Best for:
//   - General-purpose merges (DEFAULT strategy)
//   - Balances space efficiency and performance
//   - Reduces fragmentation while allowing controlled growth
//
// Trade-offs:
//   - Balanced: Best of both worlds
//   - Adaptive: Chooses strategy per operation
//   - Tunable: Slack percentage configurable
//   - Complexity: More logic than single-strategy approaches
//
// Slack percentage:
//
// The slack percentage allows some growth before switching to append mode.
// With 12% slack (default):
//   - 100-byte value updating to 110 bytes: InPlace (110 ≤ 112)
//   - 100-byte value updating to 115 bytes: Append (115 > 112)
//
// Example:
//
//	slackPct := 12  // Allow 12% slack for in-place updates
//	strategy := strategy.NewHybrid(hive, allocator, dirtyTracker, index, slackPct)
//
//	// Small value: uses InPlace
//	err = strategy.SetValue([]string{"Config"}, "Name", format.REGSZ, []byte("Value\x00"))
//
//	// Large value: uses Append
//	largeData := make([]byte, 2048)
//	err = strategy.SetValue([]string{"Data"}, "Blob", format.REGBinary, largeData)
//
// Decision matrix:
//
//	Operation       | Decision
//	----------------|------------------------------------------
//	EnsureKey       | Always InPlace (keys are small)
//	SetValue <1KB   | InPlace (small values are efficient)
//	SetValue ≥1KB   | Append (large values may grow)
//	DeleteValue     | InPlace (free cells for reuse)
//	DeleteKey       | InPlace (free cells for reuse)
//
// # Base Struct
//
// All strategies embed the Base struct for common functionality:
//
//	type Base struct {
//	    h         *hive.Hive          // Hive being modified
//	    alloc     alloc.Allocator      // Allocator (may be wrapped for Append)
//	    dt        *dirty.Tracker       // Dirty page tracker
//	    idx       index.Index          // Index for fast lookups
//	    keyEditor edit.KeyEditor       // Key editing operations
//	    valEditor edit.ValueEditor     // Value editing operations
//	    rootRef   uint32               // Root NK cell offset
//	}
//
// Base provides:
//   - Hive and allocator references
//   - Dirty tracker for page-level change tracking
//   - Index for fast key/value lookups
//   - KeyEditor and ValueEditor for low-level operations
//   - Root cell reference
//
// Example:
//
//	base := strategy.NewBase(hive, allocator, dirtyTracker, index)
//	// Base is embedded in all strategy implementations
//
// # Dirty Tracking
//
// All strategies must properly track dirty ranges for transaction commits:
//
// InPlace strategy:
//   - Editors (KeyEditor, ValueEditor) track exactly which cells are modified
//   - Allocator.Grow() automatically tracks newly appended HBINs
//   - Precise dirty tracking (only modified pages are flushed)
//
// Append strategy:
//   - Uses conservative heuristics (marks first HBIN dirty)
//   - Editors still track modifications
//   - Slightly more pages flushed than necessary
//
// Hybrid strategy:
//   - Delegates to InPlace or Append
//   - Inherits dirty tracking from chosen delegate
//
// Example dirty tracking flow:
//
//	// 1. Modify a cell (e.g., update NK subkey list)
//	strategy.EnsureKey([]string{"Software", "NewKey"})
//
//	// 2. Editor marks dirty pages:
//	//    - Parent NK cell page
//	//    - New NK cell page
//	//    - Subkey list structure pages
//
//	// 3. Transaction commit flushes dirty pages:
//	session.Commit()  // Flushes only modified 4KB pages
//
// # Integration with Editors
//
// Strategies delegate actual hive modifications to edit package editors:
//
// KeyEditor operations:
//   - EnsureKeyPath: Create key path, ensure all ancestors exist
//   - DeleteKey: Remove key and optionally its subkeys
//   - Handles: NK cells, subkey lists (LF/LH/LI/RI), index updates
//
// ValueEditor operations:
//   - UpsertValue: Create or update a registry value
//   - DeleteValue: Remove a value from a key
//   - Handles: VK cells, value lists, data cells, big-data (DB) structures
//
// Example delegation:
//
//	func (ip *InPlace) SetValue(path []string, name string, typ uint32, data []byte) error {
//	    // 1. Ensure parent key exists
//	    keyRef, _, err := ip.keyEditor.EnsureKeyPath(ip.rootRef, path)
//
//	    // 2. Delegate to ValueEditor
//	    err = ip.valEditor.UpsertValue(keyRef, name, typ, data)
//
//	    return nil
//	}
//
// # Error Handling
//
// Strategies return errors for:
//   - Empty key paths (invalid input)
//   - Allocation failures (out of space)
//   - Editor operation failures (corrupted cells, invalid references)
//   - Index lookup failures (malformed index)
//
// Example error handling:
//
//	nkRef, keysCreated, err := strategy.EnsureKey(path)
//	if err != nil {
//	    if errors.Is(err, alloc.ErrNoSpace) {
//	        return fmt.Errorf("hive full: %w", err)
//	    }
//	    return fmt.Errorf("EnsureKey failed: %w", err)
//	}
//
// # Performance Characteristics
//
// InPlace:
//   - EnsureKey: ~500ns (if exists), ~5μs (if creating)
//   - SetValue: ~1-2μs (small update), ~5-10μs (large update with new cell)
//   - DeleteValue: ~1-2μs (free VK cell)
//   - DeleteKey: ~5-50μs (depends on recursion depth)
//
// Append:
//   - EnsureKey: ~500ns (if exists), ~5μs (if creating)
//   - SetValue: ~2-3μs (always allocates new cell)
//   - DeleteValue: ~500ns (no Free() overhead)
//   - DeleteKey: ~2-20μs (no Free() overhead)
//
// Hybrid:
//   - EnsureKey: Same as InPlace (~500ns or ~5μs)
//   - SetValue: ~1-3μs (depends on size and heuristics)
//   - DeleteValue: Same as InPlace (~1-2μs)
//   - DeleteKey: Same as InPlace (~5-50μs)
//
// # Thread Safety
//
// Strategy instances are not thread-safe. Strategies are typically owned by
// a merge.Session, which itself is not thread-safe.
//
// For concurrent processing:
//   - Use separate Session (and thus separate Strategy) per goroutine
//   - Process different hive files in parallel
//   - Do NOT share strategies across goroutines
//
// # Choosing a Strategy
//
// Use InPlace if:
//   - Space efficiency is critical
//   - Values have stable sizes (no growth)
//   - Small, frequent updates
//   - Acceptable fragmentation over time
//
// Use Append if:
//   - Append-only workload (logs, audits)
//   - Crash safety is paramount (no use-after-free risk)
//   - Disk space is abundant
//   - Avoiding Free() overhead matters
//
// Use Hybrid if:
//   - General-purpose merge workload (RECOMMENDED)
//   - Mix of small and large updates
//   - Want balance between space and performance
//   - Acceptable slight complexity
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/merge: High-level merge API using strategies
//   - github.com/joshuapare/hivekit/hive/edit: Low-level editing operations
//   - github.com/joshuapare/hivekit/hive/alloc: Cell allocation and deallocation
//   - github.com/joshuapare/hivekit/hive/dirty: Page-level dirty tracking
//   - github.com/joshuapare/hivekit/hive/index: Fast key/value lookups
package strategy
