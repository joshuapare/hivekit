// Package hive provides low-level access to Windows Registry hive files.
//
// # Overview
//
// This package implements zero-copy parsing and manipulation of Windows Registry
// hive files (REGF format). It provides direct access to the binary structures
// without unnecessary allocations, making it suitable for high-performance
// registry analysis and forensics tools.
//
// # Key Types
//
// The main types provided by this package are:
//
//   - Hive: The root structure representing an opened registry hive file
//   - BaseBlock: The 4KB REGF header containing hive metadata
//   - HBIN: A hive bin (4KB-aligned data block)
//   - NK (Name Key): Registry key structure
//   - VK (Value Key): Registry value structure
//   - SK (Security Key): Security descriptor structure
//   - LF/LH/LI/RI: Index structures for subkey lists
//   - Cell: A generic cell container with size header
//
// # File Structure
//
// A registry hive file consists of:
//
//	[REGF Header - 4KB] [HBIN 0] [HBIN 1] ... [HBIN N]
//
// Each HBIN contains cells that store registry keys, values, and index structures.
// Cells are identified by relative offsets from the HBIN start (0x1000).
//
// # Opening a Hive
//
// The primary way to open a hive is through the Open function:
//
//	h, err := hive.Open("/path/to/SYSTEM")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer h.Close()
//
// On Unix/Linux/macOS, the file is memory-mapped for efficient access.
// On other platforms, the entire file is read into memory.
//
// # Accessing Registry Data
//
// The package provides low-level accessors for registry structures:
//
//	// Get root key
//	rootOff := h.RootOffset()
//	nk, err := ParseNK(h.Bytes(), rootOff)
//
//	// Get key name
//	name := nk.Name()
//
//	// Iterate over subkeys
//	listOff := nk.SubkeyListOffset()
//	// ... parse list structure ...
//
// For higher-level operations (transactions, editing), see the edit and tx packages.
//
// # Zero-Copy Design
//
// Most types in this package are zero-copy views over the underlying byte slice.
// This means:
//
//   - No allocation overhead for parsing
//   - Direct access to the hive's memory-mapped data
//   - Changes require explicit write operations (see edit package)
//
// # Thread Safety
//
// Hive instances are not thread-safe for concurrent modifications.
// Multiple goroutines can safely read from the same Hive concurrently,
// but writes must be synchronized externally or use the tx package.
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/alloc: Cell allocation and free-list management
//   - github.com/joshuapare/hivekit/hive/edit: High-level editing operations
//   - github.com/joshuapare/hivekit/hive/tx: Transactional modifications
//   - github.com/joshuapare/hivekit/hive/walker: Recursive tree walking
//   - github.com/joshuapare/hivekit/hive/verify: Integrity verification
package hive
