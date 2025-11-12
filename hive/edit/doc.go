// Package edit provides high-level editing operations for Windows Registry hives.
//
// # Overview
//
// This package offers convenient APIs for modifying registry hives without requiring
// deep knowledge of the binary format. It handles cell allocation, pointer updates,
// and dirty tracking automatically.
//
// # Key Types
//
// The main editing interfaces are:
//
//   - NKEditor: Operations on registry keys (NK cells)
//   - VKEditor: Operations on registry values (VK cells)
//
// # NKEditor Operations
//
// Creating an editor:
//
//	nked := edit.NewNKEditor(hive, allocator, dirtyTracker)
//
// Common operations:
//
//	// Create a new subkey
//	subkeyRef, err := nked.CreateSubkey(parentRef, "NewKey")
//
//	// Delete a subkey
//	err = nked.DeleteSubkey(parentRef, "OldKey")
//
//	// Update key name
//	err = nked.SetName(keyRef, "RenamedKey")
//
//	// Set class name
//	err = nked.SetClassName(keyRef, "ClassName")
//
// # VKEditor Operations
//
// Creating an editor:
//
//	vked := edit.NewVKEditor(hive, allocator, dirtyTracker, bigdataWriter)
//
// Common operations:
//
//	// Create or update a value
//	valueRef, err := vked.SetValue(keyRef, "ValueName", format.REGSZ, []byte("data"))
//
//	// Delete a value
//	err = vked.DeleteValue(keyRef, "ValueName")
//
//	// Update value data
//	err = vked.UpdateValueData(valueRef, []byte("new data"))
//
// # Value Size Handling
//
// The package automatically handles different value sizes:
//
//   - Inline (≤4 bytes): Stored directly in VK cell
//   - External (5 bytes - 16KB): Separate data cell
//   - Big-data (>16KB): DB format with chunking
//
// # Allocation Strategy
//
// Editors use the provided allocator to:
//   - Allocate new cells for keys/values
//   - Free old cells when data is replaced
//   - Grow the hive when needed
//
// # Dirty Tracking
//
// All modifications are automatically tracked via the dirty tracker,
// enabling efficient commits that only write changed pages.
//
// # Atomic Operations
//
// Individual edit operations are not atomic by themselves. For atomicity,
// use the tx package which provides transactions with rollback support.
//
// # Index Management
//
// When adding/removing subkeys, the package automatically:
//   - Updates parent NK subkey counts
//   - Rebuilds index structures (LF/LH/RI/LI) as needed
//   - Maintains sorted order in hash lists
//
// # Error Handling
//
// Operations return errors for:
//   - Invalid cell references
//   - Allocation failures
//   - Malformed structures
//   - Name collisions
//
// # Thread Safety
//
// Editors are not thread-safe. Use the tx package for concurrent access.
//
// # Related Packages
//
//   - github.com/joshuapare/hivekit/hive/alloc: Cell allocation
//   - github.com/joshuapare/hivekit/hive/bigdata: Large value storage
//   - github.com/joshuapare/hivekit/hive/subkeys: Subkey list management
//   - github.com/joshuapare/hivekit/hive/values: Value list management
//   - github.com/joshuapare/hivekit/hive/tx: Transactional wrapper
package edit
