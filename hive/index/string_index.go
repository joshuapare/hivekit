package index

import (
	"strconv"
	"strings"
	"unsafe"
)

const (
	// estimatedBytesPerMapEntry is the rough estimate of memory overhead per map entry.
	// This includes ~32 bytes for Go's map overhead plus 4 bytes for the value (uint32).
	estimatedBytesPerMapEntry = 36

	// maxUint32Digits is the maximum number of decimal digits in a uint32 (4,294,967,295 = 10 digits).
	maxUint32Digits = 10

	// decimalBase is the base for decimal number formatting.
	decimalBase = 10

	// keyPrefixSize is the size needed for "offset:" prefix in makeKey (max digits + colon).
	keyPrefixSize = maxUint32Digits + 1
)

// StringIndex is a simple map-based index using string keys.
// Keys are formatted as "parentOffset:name".
//
// # RECOMMENDED DEFAULT for scan-merge workloads
//
// Benchmark results (per hive with 18K keys, 45K values):
//   - Build time:  3.1ms  (vs UniqueIndex: 17.5ms)
//   - Lookup time: 71ns   (vs UniqueIndex: 86ns)
//   - Memory:      4.5MB  (vs UniqueIndex: 4.0MB)
//
// For processing millions of hives/hour:
//   - StringIndex: 9.8M hives/hour (RECOMMENDED)
//   - UniqueIndex: 1.6M hives/hour
//
// When to use StringIndex:
//
//	Build-heavy workloads (scan-merge, processing many hives)
//	Maximum throughput (5.6x faster build)
//
// When to use UniqueIndex instead:
//   - Read-heavy workloads (build once, lookup millions of times)
//   - Memory-constrained environments (saves 11% memory)
//   - Low GC pressure requirements (24% fewer allocations)
//
// Why is StringIndex faster?
//   - Go's native string maps are hyper-optimized
//   - No interning overhead (~50-80ns per unique.Make() call)
//   - Simple, predictable performance
type StringIndex struct {
	nodes  map[string]uint32 // "parentOff:name" → NK offset
	values map[string]uint32 // "parentOff:name" → VK offset
}

// NewStringIndex creates a StringIndex with optional capacity hints.
func NewStringIndex(nkCap, vkCap int) *StringIndex {
	if nkCap <= 0 {
		nkCap = 1024
	}
	if vkCap <= 0 {
		vkCap = 4096
	}
	return &StringIndex{
		nodes:  make(map[string]uint32, nkCap),
		values: make(map[string]uint32, vkCap),
	}
}

// AddNK implements Index.
// Names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (s *StringIndex) AddNK(parentOff uint32, name string, offset uint32) {
	key := makeKey(parentOff, strings.ToLower(name))
	s.nodes[key] = offset
}

// AddVK implements Index.
// Value names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (s *StringIndex) AddVK(parentOff uint32, valueName string, offset uint32) {
	key := makeKey(parentOff, strings.ToLower(valueName))
	s.values[key] = offset
}

// RemoveNK implements Index.
// Removes the NK entry from the index. Safe to call even if the entry doesn't exist.
// Names are automatically lowercased for case-insensitive lookups.
func (s *StringIndex) RemoveNK(parentOff uint32, name string) {
	key := makeKey(parentOff, strings.ToLower(name))
	delete(s.nodes, key)
}

// RemoveVK implements Index.
// Removes the VK entry from the index. Safe to call even if the entry doesn't exist.
// Value names are automatically lowercased for case-insensitive lookups.
func (s *StringIndex) RemoveVK(parentOff uint32, valueName string) {
	key := makeKey(parentOff, strings.ToLower(valueName))
	delete(s.values, key)
}

// GetNK implements ReadOnlyIndex.
// Names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (s *StringIndex) GetNK(parentOff uint32, name string) (uint32, bool) {
	offset, ok := s.nodes[makeKey(parentOff, strings.ToLower(name))]
	return offset, ok
}

// GetVK implements ReadOnlyIndex.
// Value names are automatically lowercased for case-insensitive lookups (Windows Registry semantics).
func (s *StringIndex) GetVK(parentOff uint32, valueName string) (uint32, bool) {
	offset, ok := s.values[makeKey(parentOff, strings.ToLower(valueName))]
	return offset, ok
}

// Stats implements ReadOnlyIndex.
func (s *StringIndex) Stats() Stats {
	// Rough estimate: each map entry = 32 bytes overhead + key string + 4 bytes value
	nkBytes := len(s.nodes) * estimatedBytesPerMapEntry
	vkBytes := len(s.values) * estimatedBytesPerMapEntry

	// Add estimated string storage (very rough)
	for k := range s.nodes {
		nkBytes += len(k)
	}
	for k := range s.values {
		vkBytes += len(k)
	}

	return Stats{
		NKCount:     len(s.nodes),
		VKCount:     len(s.values),
		BytesApprox: nkBytes + vkBytes,
		Impl:        "StringIndex",
	}
}

// makeKey creates a map key from parent offset and name.
// Format: "offset:name" (e.g., "1234:ControlSet001")
//
// Why this format:
//   - Colon separator is fast and unambiguous
//   - strconv.AppendUint is faster than fmt.Sprintf
//   - Most names are ASCII, so no escaping needed
//
// Benchmark: ~15ns per call (dominated by string allocation).
func makeKey(parentOff uint32, name string) string {
	// Pre-allocate buffer: uint32 max = 10 digits + ':' + name
	buf := make([]byte, 0, keyPrefixSize+len(name))
	buf = strconv.AppendUint(buf, uint64(parentOff), decimalBase)
	buf = append(buf, ':')
	buf = append(buf, name...)
	return unsafe.String(&buf[0], len(buf))
}
