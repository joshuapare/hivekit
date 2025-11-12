// Package values handles reading and writing of Windows Registry value lists.
//
// # Overview
//
// This package implements parsing and creation of value lists stored in Windows
// Registry hives. Value lists are flat arrays of VK (Value Key) cell references
// stored in NK (Name Key) cells.
//
// Unlike subkey lists (LF/LH/LI/RI), value lists have no special structure:
//   - No hash values
//   - No sorting requirements
//   - Single format (flat uint32 array)
//   - Preserved order from hive
//
// # Key Types
//
// List: Collection of VK cell references
//
//	type List struct {
//	    VKRefs []uint32 // HCELL_INDEX references to VK cells
//	}
//
// Methods:
//   - Len(): Returns number of values
//   - Append(vkRef): Add VK reference to end
//   - Remove(vkRef): Remove first occurrence
//   - Find(vkRef): Search for VK reference
//
// # Reading Value Lists
//
// Read value list from an NK cell:
//
//	nk, err := hive.ParseNK(payload)
//	if err != nil {
//	    return err
//	}
//
//	list, err := values.Read(hive, nk)
//	if err != nil {
//	    if errors.Is(err, values.ErrNoValueList) {
//	        // Key has no values
//	        return nil
//	    }
//	    return err
//	}
//
//	// Iterate VK references
//	for _, vkRef := range list.VKRefs {
//	    vk, _ := hive.ParseVK(...)
//	    fmt.Printf("Value: %s\n", vk.Name())
//	}
//
// # Writing Value Lists
//
// Create a new value list:
//
//	list := &values.List{
//	    VKRefs: []uint32{0x1000, 0x2000, 0x3000},
//	}
//
//	listRef, err := values.Write(hive, allocator, list)
//	if err != nil {
//	    return err
//	}
//
//	// Update NK cell to point to new list
//	err = values.UpdateNK(hive, nkRef, listRef, uint32(list.Len()))
//
// # Value List Format
//
// On-disk structure:
//
//	[Cell Header: 4 bytes]
//	[VK Ref 1: 4 bytes]
//	[VK Ref 2: 4 bytes]
//	...
//	[VK Ref N: 4 bytes]
//
// Each entry is a uint32 HCELL_INDEX offset to a VK cell.
//
// Example (3 values):
//
//	Offset  Value       Meaning
//	------  ----------  -------
//	0x0000  0xFFFFFFF0  Cell size (-16 bytes allocated)
//	0x0004  0x00001000  VK ref 1 (offset 0x1000)
//	0x0008  0x00002000  VK ref 2 (offset 0x2000)
//	0x000C  0x00003000  VK ref 3 (offset 0x3000)
//
// Total size: 4 + (4 * count) bytes
//
// # List Manipulation
//
// Append a value:
//
//	newList := list.Append(0x4000)
//	// newList.VKRefs = [0x1000, 0x2000, 0x3000, 0x4000]
//
// Remove a value:
//
//	newList := list.Remove(0x2000)
//	// newList.VKRefs = [0x1000, 0x3000]
//
// Find a value:
//
//	index := list.Find(0x2000)
//	if index != -1 {
//	    fmt.Printf("Found at index %d\n", index)
//	}
//
// Note: Append and Remove return new List instances (immutable pattern).
//
// # Updating NK Cells
//
// After modifying a value list, update the NK cell:
//
//	// Original NK has 2 values
//	nk, _ := hive.ParseNK(payload)
//	list, _ := values.Read(hive, nk)
//
//	// Add a new value
//	newList := list.Append(newVKRef)
//
//	// Write new list
//	newListRef, _ := values.Write(hive, allocator, newList)
//
//	// Update NK cell to point to new list
//	err := values.UpdateNK(hive, nkRef, newListRef, uint32(newList.Len()))
//
// UpdateNK modifies two fields in the NK cell:
//   - Value count (offset 0x24, uint32)
//   - Value list offset (offset 0x28, uint32)
//
// # Empty Value Lists
//
// Keys with no values:
//
//	// NK cell has ValueCount = 0, ValueListOffset = InvalidOffset (0xFFFFFFFF)
//	list, err := values.Read(hive, nk)
//	if errors.Is(err, values.ErrNoValueList) {
//	    // Key has no values - this is valid
//	}
//
// Writing empty lists:
//
//	list := &values.List{VKRefs: []uint32{}}
//	listRef, _ := values.Write(hive, allocator, list)
//	// listRef = InvalidOffset (0xFFFFFFFF)
//
// # Value Ordering
//
// Value lists preserve insertion order:
//   - No sorting (unlike subkey lists)
//   - Order determined by creation sequence
//   - Windows Registry preserves this order
//
// Example:
//
//	// Create values in this order:
//	SetValue("Version", ...)  // VK ref 0x1000
//	SetValue("Author", ...)   // VK ref 0x2000
//	SetValue("Date", ...)     // VK ref 0x3000
//
//	// Value list:
//	VKRefs = [0x1000, 0x2000, 0x3000]
//	// Order preserved: Version, Author, Date
//
// # Performance Characteristics
//
// Read operations:
//   - Parse list: O(n) where n = value count
//   - Resolve cell: O(1)
//   - Total: O(n)
//
// Write operations:
//   - Allocate cell: O(1)
//   - Write references: O(n)
//   - Total: O(n)
//
// List operations:
//   - Append: O(n) (creates new slice)
//   - Remove: O(n) (linear search + create new slice)
//   - Find: O(n) (linear search)
//
// Typical performance (typical key with 5 values):
//   - Read: ~500ns
//   - Write: ~1μs
//   - Append: ~100ns
//   - Find: ~50ns
//
// # Memory Overhead
//
// Per-list overhead:
//   - List struct: 24 bytes (slice header)
//   - VKRefs slice: 8 bytes per reference
//   - Total: 24 + 8*n bytes for n values
//
// On-disk overhead:
//   - Cell header: 4 bytes
//   - References: 4 bytes per value
//   - Total: 4 + 4*n bytes
//
// Example (5 values):
//   - Memory: 24 + 40 = 64 bytes
//   - Disk: 4 + 20 = 24 bytes
//
// # Error Handling
//
// Operations return errors for:
//   - No value list (count = 0 or offset = InvalidOffset)
//   - Truncated cells (insufficient data)
//   - Cell reference out of bounds
//   - Free cells (positive size)
//   - Allocation failures
//   - NK cell out of bounds
//
// Example error handling:
//
//	list, err := values.Read(hive, nk)
//	if err != nil {
//	    if errors.Is(err, values.ErrNoValueList) {
//	        // No values - create empty list
//	        list = &values.List{VKRefs: []uint32{}}
//	    } else if errors.Is(err, values.ErrTruncated) {
//	        return fmt.Errorf("corrupted value list: %w", err)
//	    } else {
//	        return fmt.Errorf("read failed: %w", err)
//	    }
//	}
//
// # Integration with VK Cells
//
// Value lists reference VK cells, which contain the actual value data:
//
//	// Read value list
//	list, _ := values.Read(hive, nk)
//
//	// Read each VK cell
//	for _, vkRef := range list.VKRefs {
//	    vkPayload, _ := resolveCell(hive, vkRef)
//	    vk, _ := hive.ParseVK(vkPayload)
//
//	    // Get value metadata
//	    name := vk.Name()
//	    typ := vk.Type()
//	    dataSize := vk.DataSize()
//
//	    // Get value data
//	    data, _ := vk.Data()
//	    fmt.Printf("%s = %v (type: %d)\n", name, data, typ)
//	}
//
// # Comparison with Subkey Lists
//
// Value lists vs. subkey lists:
//
//	Feature           | Value Lists    | Subkey Lists
//	------------------|----------------|------------------
//	Format            | Flat array     | LF/LH/LI/RI
//	Hash values       | No             | Yes (LF/LH)
//	Sorting           | No             | Yes
//	Indirection       | No             | Yes (RI)
//	Order             | Preserved      | Alphabetical
//	Multiple formats  | No (1)         | Yes (4)
//
// Value lists are simpler because:
//   - Registry values are accessed by name lookup in VK cells
//   - No need for hash-based indexing
//   - Order doesn't affect correctness
//
// # Integration with Other Packages
//
// The values package is used by:
//   - hive/edit: Modify value lists when adding/removing values
//   - hive/walker: Read all values from NK cells
//   - hive/index: Build value indexes by reading VK cells
//   - hive/merge/strategy: Update NK value lists during merges
//
// Example integration:
//
//	// Read NK cell
//	nk, _ := hive.ParseNK(payload)
//
//	// Read value list
//	list, _ := values.Read(hive, nk)
//
//	// Process each value
//	for _, vkRef := range list.VKRefs {
//	    vkPayload, _ := resolveCell(hive, vkRef)
//	    vk, _ := hive.ParseVK(vkPayload)
//	    // Process VK
//	}
//
// # Thread Safety
//
// Functions in this package are stateless and thread-safe for concurrent reads.
// However, List modification (Append/Remove) returns new instances and should
// not be shared across goroutines without synchronization.
//
// # Design Notes
//
// Flat array design:
//   - Simple and efficient
//   - No overhead for small value counts
//   - Windows Registry doesn't sort values
//   - Order preservation matches Windows behavior
//
// No deduplication:
//   - Value lists can contain duplicate VK references
//   - Responsibility of caller to avoid duplicates
//   - Windows Registry allows duplicates (though unusual)
//
// Immutable operations:
//   - Append/Remove return new List instances
//   - Original list unchanged
//   - Safe for concurrent reads
//   - Caller manages memory
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive: Core hive parsing (NK, VK cells)
//   - github.com/joshuapare/hivekit/hive/alloc: Cell allocation for Write
//   - github.com/joshuapare/hivekit/hive/edit: High-level value editing
//   - github.com/joshuapare/hivekit/hive/subkeys: Subkey list management (similar structure)
//   - github.com/joshuapare/hivekit/internal/format: Binary format constants
package values
