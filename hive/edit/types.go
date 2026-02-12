package edit

// NKRef is a type alias for NK cell references (HCELL_INDEX).
type NKRef = uint32

// VKRef is a type alias for VK cell references (HCELL_INDEX).
type VKRef = uint32

// ValueType is a type alias for registry value types.
// Use constants from github.com/joshuapare/hivekit/internal/format (REGSZ, REGDWORD, etc.)
type ValueType = uint32

// MaxInlineValueBytes is the maximum size for inline value data (stored in VK.DataOff field)
// Windows stores values ≤ 4 bytes inline.
const MaxInlineValueBytes = 4

// MaxExternalValueBytes is the maximum size for a single external data cell
// Above this size, we need to use bigdata (DB) format
// This is the hivex convention for max single block size.
const MaxExternalValueBytes = 16344

// KeyEditor provides operations for managing registry keys.
type KeyEditor interface {
	// EnsureKeyPath creates intermediate keys as needed (case-insensitive).
	// Returns the final NK reference and the count of keys created.
	// segments should not include the root - pass path segments only.
	EnsureKeyPath(root NKRef, segments []string) (NKRef, int, error)

	// DeleteKey removes a key and optionally its subkeys (recursive).
	// For phase 1, returns ErrNotImplemented.
	DeleteKey(nk NKRef, recursive bool) error

	// EnableDeferredMode enables deferred subkey list building for bulk operations.
	// In deferred mode, subkey list updates are accumulated in memory and written
	// all at once during FlushDeferredSubkeys(). This eliminates expensive
	// read-modify-write cycles and dramatically improves bulk building performance.
	EnableDeferredMode()

	// DisableDeferredMode disables deferred subkey list building.
	// Returns an error if there are pending deferred updates (call FlushDeferredSubkeys first).
	DisableDeferredMode() error

	// FlushDeferredSubkeys writes all accumulated deferred children to disk.
	// This must be called before disabling deferred mode.
	// Returns the number of parents flushed and any error encountered.
	FlushDeferredSubkeys() (int, error)
}

// ValueSpec describes a value to upsert (for batch operations).
type ValueSpec struct {
	Name string
	Type ValueType
	Data []byte
}

// ValueEditor provides operations for managing registry values.
type ValueEditor interface {
	// UpsertValue creates or updates a value under the given NK (case-insensitive name).
	// Automatically chooses inline/external/DB storage based on data size.
	// Empty name ("") is valid for the (Default) value.
	UpsertValue(nk NKRef, name string, typ ValueType, data []byte) error

	// UpsertValues creates or updates multiple values under the given NK in a single
	// operation. This is O(N) vs O(N²) for calling UpsertValue N times, because it
	// reads and writes the value list only once.
	UpsertValues(nk NKRef, values []ValueSpec) (int, error)

	// DeleteValue removes a value by name; idempotent if missing.
	// Empty name ("") targets the (Default) value.
	DeleteValue(nk NKRef, name string) error
}
