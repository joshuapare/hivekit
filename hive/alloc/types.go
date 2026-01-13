package alloc

// CellRef is an HCELL_INDEX - a uint32 offset relative to the hive data start (0x1000).
type CellRef = uint32

// Class represents the type of cell being allocated (NK, VK, LF, LH, etc.).
type Class uint8

const (
	ClassNK Class = 1
	ClassVK Class = 2
	ClassLF Class = 3
	ClassLH Class = 4
	ClassRI Class = 5
	ClassLI Class = 6
	ClassDB Class = 7  // big-data header
	ClassBL Class = 8  // blocklist cell
	ClassRD Class = 9  // raw data block for big-data
	ClassSK Class = 10 // security descriptor
)

// Allocator defines the interface for hive cell allocation and deallocation.
//
// Implementations:
//   - FastAllocator: Standard allocator with free-list management
//   - NoFreeAllocator: Append-only wrapper that skips Free() calls
//
// This interface enables different allocation strategies while maintaining
// compatibility with editors and other components.
type Allocator interface {
	// Alloc allocates a cell of the given size and class.
	// Returns the cell reference, a slice to the cell payload, and any error.
	Alloc(need int32, cls Class) (CellRef, []byte, error)

	// Free marks a cell as free and available for reuse.
	// In append-only mode, this may be a no-op.
	Free(ref CellRef) error

	// GrowByPages adds a new HBIN of exactly (numPages * 4KB) size.
	// This is the RECOMMENDED API for spec-compliant hive growth.
	//
	// Examples:
	//   GrowByPages(1) → 4KB HBIN with 4064 bytes usable
	//   GrowByPages(2) → 8KB HBIN with 8160 bytes usable
	//   GrowByPages(4) → 16KB HBIN with 16352 bytes usable
	//
	// Per Windows Registry Specification, HBINs must be multiples of 4KB.
	// The HBIN header (32 bytes) is PART OF the HBIN size, not in addition to it.
	GrowByPages(numPages int) error

	// TruncatePages removes the last numPages worth of HBINs (numPages * 4KB).
	// This is used for space reclamation after removing cells.
	//
	// Examples:
	//   TruncatePages(1) → Remove 1×4KB HBIN
	//   TruncatePages(2) → Remove 2×4KB = 8KB
	//
	// IMPORTANT: Caller must ensure no allocated cells exist in the truncation range.
	// This operation shrinks the hive file and updates the registry header.
	TruncatePages(numPages int) error

	// Grow expands the hive by adding a new HBIN of at least the given size.
	// The actual size will be aligned to HBIN alignment requirements.
	//
	// Deprecated: Use GrowByPages() instead for explicit, spec-compliant growth.
	// This method is maintained for backward compatibility.
	//
	// Migration examples:
	//   Grow(4096)  → GrowByPages(1)  // 4KB HBIN
	//   Grow(8192)  → GrowByPages(2)  // 8KB HBIN
	//   Grow(16384) → GrowByPages(4)  // 16KB HBIN
	Grow(need int32) error
}
