// Package subkeys handles reading and writing of Windows Registry subkey lists.
//
// # Overview
//
// This package implements parsing and creation of the four subkey list formats
// used by Windows Registry hives:
//   - LI (Indexed): Simple offset list, no hashes
//   - LF (Fast Leaf): Offset + basic hash, for ≤12 entries
//   - LH (Hash Leaf): Offset + improved hash, for >12 entries
//   - RI (Indirect): List of list references, for >1024 entries
//
// All lists maintain entries in sorted order by lowercased key names for
// efficient binary search and consistent Windows Registry semantics.
//
// # Key Types
//
// Entry: Single subkey entry
//
//	type Entry struct {
//	    NameLower string // Lowercased key name
//	    NKRef     uint32 // NK cell reference
//	}
//
// List: Collection of entries in sorted order
//
//	type List struct {
//	    Entries []Entry
//	}
//
// ListKind: Format identifier
//
//	const (
//	    KindLI ListKind = 1 // Indexed list (no hash)
//	    KindLF ListKind = 2 // Fast leaf (basic hash)
//	    KindLH ListKind = 3 // Hash leaf (improved hash)
//	    KindRI ListKind = 4 // Indirect list (list of lists)
//	)
//
// # Reading Subkey Lists
//
// The Read function automatically handles all four list formats:
//
//	list, err := subkeys.Read(hive, listRef)
//	if err != nil {
//	    return err
//	}
//	for _, entry := range list.Entries {
//	    fmt.Printf("Key: %s (NK ref: 0x%X)\n", entry.NameLower, entry.NKRef)
//	}
//
// For RI (indirect) lists, Read automatically follows references and
// flattens all sub-lists into a single List.
//
// # Writing Subkey Lists
//
// The Write function automatically selects the optimal format based on entry count:
//
//	entries := []subkeys.Entry{
//	    {NameLower: "controlset001", NKRef: 0x1000},
//	    {NameLower: "controlset002", NKRef: 0x2000},
//	}
//	listRef, err := subkeys.Write(hive, allocator, entries)
//	if err != nil {
//	    return err
//	}
//	// listRef points to LF list (≤12 entries)
//
// Format selection rules:
//   - 0 entries: Returns InvalidOffset (no list needed)
//   - 1-12 entries: LF list (fast leaf with hash)
//   - 13-1024 entries: LH list (hash leaf with improved hash)
//   - >1024 entries: RI list (indirect, splits into 512-entry chunks)
//
// # List Manipulation
//
// Lists provide Insert, Remove, and Find operations:
//
// Insert (maintains sorted order):
//
//	newList := list.Insert(subkeys.Entry{
//	    NameLower: "newkey",
//	    NKRef:     0x3000,
//	})
//
// Remove (by lowercased name):
//
//	newList := list.Remove("oldkey")
//
// Find (binary search):
//
//	entry, found := list.Find("controlset001")
//	if found {
//	    fmt.Printf("Found at NK ref: 0x%X\n", entry.NKRef)
//	}
//
// Note: Insert and Remove return new List instances (immutable pattern).
//
// # List Formats
//
// LI (Indexed List):
//   - Structure: Signature (2) + Count (2) + [Offset (4)] * count
//   - No hash values, just NK cell offsets
//   - Rarely used in modern hives (legacy format)
//
// LF (Fast Leaf):
//   - Structure: Signature (2) + Count (2) + [Offset (4) + Hash (4)] * count
//   - Each entry is 8 bytes (offset + basic hash)
//   - Used for ≤12 entries (threshold configurable)
//   - Hash algorithm: Basic 4-byte hash
//
// LH (Hash Leaf):
//   - Structure: Signature (2) + Count (2) + [Offset (4) + Hash (4)] * count
//   - Identical structure to LF, different hash algorithm
//   - Used for >12 entries (up to 1024)
//   - Hash algorithm: Improved hash (hash * 37 + toupper(char))
//
// RI (Indirect List):
//   - Structure: Signature (2) + Count (2) + [SubListRef (4)] * count
//   - References other LF/LH lists
//   - Used for >1024 entries
//   - Splits entries into 512-entry chunks
//
// # Hash Algorithm
//
// Windows Registry uses a specific hash for LH lists:
//
//	hash = 0
//	for each character:
//	    hash = hash * 37 + toupper(char)
//
// The Hash function implements this algorithm:
//
//	hash := subkeys.Hash("ControlSet001")
//	// Returns: Windows Registry compatible hash value
//
// Note: Names are stored lowercased in entries, but the hash algorithm
// uppercases each character during computation (Windows semantics).
//
// # Format Selection Thresholds
//
// Constants control when to switch formats:
//
//	LFThreshold = 12    // Above this, use LH instead of LF
//	RIThreshold = 1024  // Above this, use RI (split into chunks)
//
// These thresholds balance:
//   - Lookup performance (hash vs. no-hash)
//   - Cell size (more entries = larger cells)
//   - Memory overhead (RI adds indirection)
//
// # Encoding and Decoding
//
// The package handles name encoding automatically:
//
// Compressed names (ASCII/Windows-1252):
//   - Fast path for pure ASCII (most common)
//   - Slow path for Windows-1252 extended characters (0x80-0xFF)
//   - Used when NK.IsCompressedName() is true
//
// UTF-16LE names:
//   - Full Unicode support
//   - Decoded using utf16.Decode
//   - Used when NK.IsCompressedName() is false
//
// Example:
//
//	// ASCII name: "ControlSet001" (compressed)
//	// Unicode name: "日本語キー" (UTF-16LE)
//
// Both are automatically lowercased for storage in Entry.NameLower.
//
// # Sorting and Ordering
//
// All lists maintain entries in sorted order by lowercased name:
//
//	// Automatically sorted:
//	entries := []subkeys.Entry{
//	    {NameLower: "zebra", NKRef: 0x3000},
//	    {NameLower: "apple", NKRef: 0x1000},
//	    {NameLower: "banana", NKRef: 0x2000},
//	}
//	listRef, _ := subkeys.Write(hive, allocator, entries)
//	// Written as: apple, banana, zebra
//
// Sorting enables:
//   - Binary search in Find() (O(log n) instead of O(n))
//   - Consistent ordering across operations
//   - Windows Registry compatibility
//
// # RI List Chunking
//
// For large lists (>1024 entries), RI format splits into chunks:
//
//	// 2000 entries:
//	// RI list (4 chunks):
//	//   → LH list (512 entries)
//	//   → LH list (512 entries)
//	//   → LH list (512 entries)
//	//   → LH list (464 entries)
//
// Chunk size is hardcoded to 512 entries, balancing:
//   - Cell size (larger chunks = fewer indirections)
//   - Allocation efficiency (avoid huge cells)
//
// # Error Handling
//
// Operations return errors for:
//   - Truncated cells (insufficient data)
//   - Invalid signatures (not lf/lh/li/ri)
//   - Cell reference out of bounds
//   - Free cells (positive size)
//   - Allocation failures
//   - Name decoding failures
//
// Example error handling:
//
//	list, err := subkeys.Read(hive, listRef)
//	if err != nil {
//	    if errors.Is(err, subkeys.ErrTruncated) {
//	        return fmt.Errorf("corrupted list: %w", err)
//	    }
//	    return fmt.Errorf("read failed: %w", err)
//	}
//
// # Performance Characteristics
//
// Read operations:
//   - LI/LF/LH: O(n) where n = entry count
//   - RI: O(n) + overhead of following indirections
//   - Name decoding: ~100ns per entry (ASCII), ~500ns (UTF-16LE)
//
// Write operations:
//   - Sort: O(n log n)
//   - Allocate + write: O(n)
//   - Total: O(n log n)
//
// List operations:
//   - Insert: O(n) for insertion, list is rebuilt
//   - Remove: O(n) for filtering
//   - Find: O(log n) binary search
//
// Typical performance (18K keys per hive):
//   - Read all subkey lists: ~50ms
//   - Write all subkey lists: ~80ms
//   - Find in large list (500 entries): ~9 comparisons
//
// # Memory Overhead
//
// Per-entry overhead:
//   - Entry struct: 24 bytes (string + uint32 + padding)
//   - Name string: len(name) + 16 bytes (Go string header + allocation)
//   - Total: ~40-60 bytes per entry
//
// List overhead:
//   - List struct: 24 bytes (slice header)
//   - Entries slice: 24 bytes * count
//   - Total: ~24 + 40*n bytes for n entries
//
// On-disk overhead:
//   - LI: 4 + 4*n bytes
//   - LF/LH: 4 + 8*n bytes
//   - RI: 4 + 4*chunks + (4 + 8*entries_per_chunk) * chunks
//
// # Integration with Other Packages
//
// The subkeys package is used by:
//   - hive/edit: Modify subkey lists when adding/removing keys
//   - hive/walker: Traverse hive by following subkey lists
//   - hive/index: Build indexes by reading all subkey lists
//   - hive/merge/strategy: Update parent NK subkey lists during merges
//
// Example integration:
//
//	// Read NK cell
//	nk, _ := hive.ParseNK(payload)
//
//	// Read its subkey list
//	subkeyListRef := nk.SubkeyListRef()
//	list, _ := subkeys.Read(hive, subkeyListRef)
//
//	// Iterate subkeys
//	for _, entry := range list.Entries {
//	    childNK, _ := hive.ParseNK(...)
//	    // Process child key
//	}
//
// # Thread Safety
//
// Functions in this package are stateless and thread-safe for concurrent reads.
// However, List modification (Insert/Remove) returns new instances and should
// not be shared across goroutines without synchronization.
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive: Core hive parsing (NK cells)
//   - github.com/joshuapare/hivekit/hive/alloc: Cell allocation for Write
//   - github.com/joshuapare/hivekit/hive/edit: High-level key editing
//   - github.com/joshuapare/hivekit/internal/format: Binary format constants
package subkeys
