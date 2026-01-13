// Package index provides fast in-memory indexes for registry key and value lookups.
//
// # Overview
//
// This package implements two index strategies for rapid NK (Name Key) and VK (Value Key)
// lookups in Windows Registry hives. Indexes map (parent offset, name) tuples to cell
// offsets, eliminating the need for linear scans through subkey/value lists.
//
// # Index Implementations
//
// StringIndex: Map-based index using string keys (RECOMMENDED DEFAULT)
//   - Best for: Build-heavy workloads (scan-merge, batch processing)
//   - Build time: 3.1ms per hive (18K keys, 45K values)
//   - Lookup time: 71ns per operation
//   - Memory: 4.5MB per hive
//   - Throughput: 9.8M hives/hour
//
// UniqueIndex: Index using Go 1.23's unique package for string interning
//   - Best for: Read-heavy workloads (build once, millions of lookups)
//   - Build time: 17.5ms per hive (5.6x slower than StringIndex)
//   - Lookup time: 86ns per operation
//   - Memory: 4.0MB per hive (11% less than StringIndex)
//   - Throughput: 1.6M hives/hour
//
// # Interfaces
//
// ReadOnlyIndex: Query-only interface for lookups
//   - GetNK(parentOff, name): Find NK by parent and name
//   - GetVK(parentOff, valueName): Find VK by parent and value name
//   - Stats(): Get index statistics
//
// Index: Full mutable interface (embeds ReadOnlyIndex)
//   - AddNK/AddVK: Register new entries
//   - RemoveNK/RemoveVK: Delete entries
//
// # Usage Example
//
// Building an index while scanning a hive:
//
//	idx := index.NewStringIndex(1024, 4096)
//	walker.Walk(hive, func(nk NK, path string) error {
//	    parentOff := getParentOffset(path)
//	    idx.AddNK(parentOff, nk.Name(), nk.Offset())
//
//	    // Index all values under this key
//	    for _, vk := range nk.Values() {
//	        idx.AddVK(nk.Offset(), vk.Name(), vk.Offset())
//	    }
//	    return nil
//	})
//
// Looking up a key by path:
//
//	// Get root NK offset from hive
//	rootOff := hive.RootCellOffset()
//
//	// Walk absolute path
//	offset, ok := index.WalkPath(idx, rootOff, "System", "CurrentControlSet")
//	if !ok {
//	    return errors.New("path not found")
//	}
//
// Direct lookup (faster, no path parsing):
//
//	// If you already know the parent offset
//	parentOff := uint32(0x1000)
//	childOff, ok := idx.GetNK(parentOff, "Services")
//
//	// Lookup a value
//	vkOff, ok := idx.GetVK(childOff, "Start")
//
// # Case Sensitivity
//
// Both implementations use case-insensitive lookups (Windows Registry semantics):
//   - Names are automatically lowercased before storage/lookup
//   - "System", "SYSTEM", and "system" all map to the same entry
//   - Empty string ("") is valid for the (Default) value
//
// # Index Key Format
//
// Indexes use (parent offset, name) tuples instead of full paths:
//   - Avoids []string allocations for path splitting
//   - Mirrors the actual hive hierarchical structure
//   - Enables O(1) direct lookups when parent is known
//
// For absolute path lookups, use the WalkPath helper which steps through
// the tree one component at a time.
//
// # Performance Characteristics
//
// StringIndex (recommended default):
//   - AddNK/AddVK: ~15ns (string allocation + map insert)
//   - GetNK/GetVK: ~71ns (string key generation + map lookup)
//   - RemoveNK/RemoveVK: ~15ns (string key generation + map delete)
//
// UniqueIndex (specialized):
//   - AddNK/AddVK: ~90ns (unique.Make overhead + map insert)
//   - GetNK/GetVK: ~86ns (unique.Make overhead + map lookup)
//   - RemoveNK/RemoveVK: ~90ns (unique.Make overhead + map delete)
//   - unique.Make() adds ~50-80ns per call (function call + sync.Map lookup)
//
// # Choosing an Implementation
//
// Use StringIndex if:
//   - Processing multiple hives (scan-merge workflows)
//   - Build time matters more than memory
//   - You want simple, predictable performance
//
// Use UniqueIndex if:
//   - Building index once, querying millions of times
//   - Memory constrained (saves 11% memory)
//   - Need lower GC pressure (24% fewer allocations)
//
// # Memory Overhead
//
// StringIndex:
//   - ~36 bytes per map entry (map overhead + uint32 value)
//   - String key storage (e.g., "1234:Services" = 13 bytes)
//   - Total: ~4.5MB for typical hive (18K keys, 45K values)
//
// UniqueIndex:
//   - ~48 bytes per map entry (map overhead + PathKey struct + uint32)
//   - Shared interned strings (global string cache)
//   - Total: ~4.0MB for typical hive (18K keys, 45K values)
//
// # WalkPath Helper
//
// The WalkPath function provides convenient absolute path lookups:
//
//	offset, ok := index.WalkPath(idx, rootOff, "path", "to", "key")
//
// Internally, it chains GetNK() calls. For performance-critical code,
// manually step through the tree to avoid variadic argument allocations.
//
// # Thread Safety
//
// Index instances are not thread-safe. Callers must synchronize access
// externally or use separate indexes per goroutine.
//
// # Integration with Hive Operations
//
// Indexes are typically used with:
//   - walker package: Build index during hive scan
//   - edit package: Update index when adding/removing keys/values
//   - merge package: Maintain index during hive merging
//
// Example with edit operations:
//
//	// Create a new key
//	newOff, err := editor.CreateSubkey(parentRef, "NewKey")
//	if err == nil {
//	    idx.AddNK(parentRef, "NewKey", newOff)
//	}
//
//	// Delete a key
//	err = editor.DeleteSubkey(parentRef, "OldKey")
//	if err == nil {
//	    idx.RemoveNK(parentRef, "OldKey")
//	}
//
// # Statistics
//
// The Stats() method provides index metrics:
//
//	stats := idx.Stats()
//	fmt.Printf("NK entries: %d\n", stats.NKCount)
//	fmt.Printf("VK entries: %d\n", stats.VKCount)
//	fmt.Printf("Memory (approx): %d bytes\n", stats.BytesApprox)
//	fmt.Printf("Implementation: %s\n", stats.Impl)
//
// Note: BytesApprox is a best-effort estimate, not exact measurement.
//
// # Deprecated Aliases
//
// The Builder and ReadWriteIndex type aliases are maintained for backward
// compatibility. New code should use Index directly.
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/walker: Tree traversal for index building
//   - github.com/joshuapare/hivekit/hive/edit: Key/value editing operations
//   - github.com/joshuapare/hivekit/hive/merge: Hive merging with index updates
package index
