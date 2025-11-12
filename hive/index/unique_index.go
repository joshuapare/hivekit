package index

import (
	"strings"
	"unique"
)

const (
	// estimatedBytesPerUniqueEntry is the rough estimate of memory overhead per unique index map entry.
	// This includes ~32 bytes for Go's map overhead, 16 bytes for PathKey struct, and 4 bytes for uint32 value.
	estimatedBytesPerUniqueEntry = 48
)

// UniqueIndex is an index using Go 1.23's unique package for string interning.
// It interns only the name strings (not the composite keys) for optimal performance.
//
// PERFORMANCE NOTE: StringIndex is 5.6x FASTER for build-heavy workloads!
//
// Benchmark results (per hive with 18K keys, 45K values):
//   - Build time:  17.5ms (vs StringIndex: 3.1ms)
//   - Lookup time: 86ns   (vs StringIndex: 71ns)
//   - Memory:      4.0MB  (vs StringIndex: 4.5MB)
//
// For processing millions of hives/hour:
//   - StringIndex: 9.8M hives/hour (RECOMMENDED)
//   - UniqueIndex: 1.6M hives/hour
//
// When to use UniqueIndex:
//
//	Read-heavy workloads (build once, lookup millions of times)
//	Memory-constrained environments (11% less memory)
//	Low GC pressure requirements (24% fewer allocations)
//
// When to use StringIndex:
//
//	Build-heavy workloads (scan-merge, processing many hives)
//	Maximum throughput (6x faster)
//	Simpler code, easier debugging
//
// Why is StringIndex faster?
//   - unique.Make() has ~50-80ns overhead per call (function call + sync.Map lookup)
//   - With 63K operations per hive, that's 3-5ms of pure overhead
//   - Go's native string maps are hyper-optimized
//
// For most use cases, prefer StringIndex. UniqueIndex is a specialized option.
type UniqueIndex struct {
	nodes  map[PathKey]uint32 // Direct PathKey as map key (single interning)
	values map[PathKey]uint32
}

// PathKey is the composite lookup key: (parent NK offset, child name).
// The name is interned using unique.Handle[string] for pointer-compare equality.
//
// Design choice: We use PathKey directly as the map key (not unique.Handle[PathKey])
// because the second interning layer adds ~10ms overhead per hive with no benefit.
type PathKey struct {
	ParentOff uint32                // Parent NK's cell offset
	Name      unique.Handle[string] // Interned child/value name (pointer-compare)
}

// NewUniqueIndex creates a UniqueIndex with optional capacity hints.
func NewUniqueIndex(nkCap, vkCap int) *UniqueIndex {
	if nkCap <= 0 {
		nkCap = 1024
	}
	if vkCap <= 0 {
		vkCap = 4096
	}
	return &UniqueIndex{
		nodes:  make(map[PathKey]uint32, nkCap),
		values: make(map[PathKey]uint32, vkCap),
	}
}

// AddNK implements Index.
// Names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (u *UniqueIndex) AddNK(parentOff uint32, name string, offset uint32) {
	key := PathKey{
		ParentOff: parentOff,
		Name:      unique.Make(strings.ToLower(name)), // Lowercase before interning
	}
	u.nodes[key] = offset
}

// AddVK implements Index.
// Value names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (u *UniqueIndex) AddVK(parentOff uint32, valueName string, offset uint32) {
	key := PathKey{
		ParentOff: parentOff,
		Name:      unique.Make(strings.ToLower(valueName)),
	}
	u.values[key] = offset
}

// RemoveNK implements Index.
// Removes the NK entry from the index. Safe to call even if the entry doesn't exist.
// Names are automatically lowercased for case-insensitive lookups.
func (u *UniqueIndex) RemoveNK(parentOff uint32, name string) {
	key := PathKey{
		ParentOff: parentOff,
		Name:      unique.Make(strings.ToLower(name)),
	}
	delete(u.nodes, key)
}

// RemoveVK implements Index.
// Removes the VK entry from the index. Safe to call even if the entry doesn't exist.
// Value names are automatically lowercased for case-insensitive lookups.
func (u *UniqueIndex) RemoveVK(parentOff uint32, valueName string) {
	key := PathKey{
		ParentOff: parentOff,
		Name:      unique.Make(strings.ToLower(valueName)),
	}
	delete(u.values, key)
}

// GetNK implements ReadOnlyIndex.
// Names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (u *UniqueIndex) GetNK(parentOff uint32, name string) (uint32, bool) {
	key := PathKey{
		ParentOff: parentOff,
		Name:      unique.Make(strings.ToLower(name)),
	}
	offset, ok := u.nodes[key]
	return offset, ok
}

// GetVK implements ReadOnlyIndex.
// Value names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (u *UniqueIndex) GetVK(parentOff uint32, valueName string) (uint32, bool) {
	key := PathKey{
		ParentOff: parentOff,
		Name:      unique.Make(strings.ToLower(valueName)),
	}
	offset, ok := u.values[key]
	return offset, ok
}

// Stats implements ReadOnlyIndex.
func (u *UniqueIndex) Stats() Stats {
	// Rough estimate: each map entry = 32 bytes overhead + 16 bytes (PathKey + uint32)
	// Interned strings are shared globally, so we don't count them here
	nkBytes := len(u.nodes) * estimatedBytesPerUniqueEntry
	vkBytes := len(u.values) * estimatedBytesPerUniqueEntry

	return Stats{
		NKCount:     len(u.nodes),
		VKCount:     len(u.values),
		BytesApprox: nkBytes + vkBytes,
		Impl:        "UniqueIndex(single-intern)",
	}
}
