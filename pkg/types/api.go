package types

import (
	"fmt"
	"time"
)

// -----------------------------------------------------------------------------
// Typed Errors (stable categories for programmatic handling)
// -----------------------------------------------------------------------------

// ErrKind classifies errors so callers can branch on intent rather than text.
type ErrKind int

const (
	ErrKindFormat      ErrKind = iota // malformed headers/signatures (e.g., bad "regf")
	ErrKindCorrupt                    // structural corruption (bad sizes/offsets/tags)
	ErrKindUnsupported                // valid feature we don't support (yet)
	ErrKindNotFound                   // missing key/value/path
	ErrKindType                       // requested decode doesn't match value RegType
	ErrKindState                      // invalid operation for current state (e.g., readonly)
)

// Error is a typed error with an optional underlying cause.
type Error struct {
	Kind ErrKind
	Msg  string
	Err  error // optional underlying cause
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

// Sentinels commonly returned by implementations.
var (
	// ErrNotHive indicates the file lacks a valid "regf" header.
	ErrNotHive = &Error{Kind: ErrKindFormat, Msg: "not a registry hive (bad regf header)"}
	// ErrCorrupt indicates non-recoverable structural inconsistency.
	ErrCorrupt = &Error{Kind: ErrKindCorrupt, Msg: "corrupt hive structure"}
	// ErrUnsupported indicates a recognized but unsupported feature/variant.
	ErrUnsupported = &Error{Kind: ErrKindUnsupported, Msg: "unsupported hive feature"}
	// ErrNotFound indicates a missing key/value/path.
	ErrNotFound = &Error{Kind: ErrKindNotFound, Msg: "not found"}
	// ErrTypeMismatch indicates the requested decode doesn't match the value type.
	ErrTypeMismatch = &Error{Kind: ErrKindType, Msg: "registry value has different type"}
	// ErrReadonly indicates a mutation was attempted on a read-only handle.
	ErrReadonly = &Error{Kind: ErrKindState, Msg: "reader is read-only"}
)

// -----------------------------------------------------------------------------
// Core Identifiers & Metadata
// -----------------------------------------------------------------------------

// NodeID and ValueID are small, copyable handles referring to NK/VK records.
// Implementations typically encode absolute offsets into the backing []byte.
// Using handles (instead of large structs) keeps traversals allocation-light.
type (
	NodeID  uint32
	ValueID uint32
)

// RegType enumerates Windows registry value types commonly encountered.
// (The numbers align with Windows definitions.)
type RegType uint32

const (
	REG_NONE                        RegType = 0
	REG_SZ                          RegType = 1
	REG_EXPAND_SZ                   RegType = 2
	REG_BINARY                      RegType = 3
	REG_DWORD                       RegType = 4
	REG_DWORD_LE                    RegType = 4 // alias for clarity
	REG_DWORD_BE                    RegType = 5
	REG_LINK                        RegType = 6
	REG_MULTI_SZ                    RegType = 7
	REG_RESOURCE_LIST               RegType = 8
	REG_FULL_RESOURCE_DESCRIPTOR    RegType = 9
	REG_RESOURCE_REQUIREMENTS_LIST  RegType = 10
	REG_QWORD                       RegType = 11
)

// String implements the Stringer interface for RegType
func (t RegType) String() string {
	switch t {
	case REG_NONE:
		return "REG_NONE"
	case REG_SZ:
		return "REG_SZ"
	case REG_EXPAND_SZ:
		return "REG_EXPAND_SZ"
	case REG_BINARY:
		return "REG_BINARY"
	case REG_DWORD:
		return "REG_DWORD"
	case REG_DWORD_BE:
		return "REG_DWORD_BE"
	case REG_LINK:
		return "REG_LINK"
	case REG_MULTI_SZ:
		return "REG_MULTI_SZ"
	case REG_RESOURCE_LIST:
		return "REG_RESOURCE_LIST"
	case REG_FULL_RESOURCE_DESCRIPTOR:
		return "REG_FULL_RESOURCE_DESCRIPTOR"
	case REG_RESOURCE_REQUIREMENTS_LIST:
		return "REG_RESOURCE_REQUIREMENTS_LIST"
	case REG_QWORD:
		return "REG_QWORD"
	default:
		// Format as signed int32 to match hivex (shows negative values for invalid types)
		return fmt.Sprintf("UNKNOWN_TYPE_%d", int32(t))
	}
}

// ValueMeta describes a value without forcing data decoding or allocation.
// Implementations should fill this from the VK header only.
type ValueMeta struct {
	Name           string  // value name ("" for default/unnamed)
	Type           RegType // declared registry type
	Size           int     // logical payload size (from VK)
	Inline         bool    // true if VK embeds data inline per spec heuristics
	NameCompressed bool    // true if name is stored in compressed (Windows-1252) format
	NameRaw        []byte  // original encoded name bytes (for zero-copy serialization)
}

// KeyMeta exposes cheap NK-level information useful for listings and planning.
type KeyMeta struct {
	Name           string    // key name as UTF-8 (decoded lazily)
	NameLower      string    // cached lowercase name for case-insensitive comparisons (avoids repeated ToLower calls)
	LastWrite      time.Time // NK timestamp if present
	SubkeyN        int       // number of subkeys (from list)
	ValueN         int       // number of values
	HasSecDesc     bool      // whether an SK record is associated
	NameCompressed bool      // true if name is stored in compressed (Windows-1252) format
	NameRaw        []byte    // original encoded name bytes (for zero-copy serialization)
}

// KeyDetail exposes detailed NK record metadata for inspection/forensics.
type KeyDetail struct {
	KeyMeta                      // Embedded basic metadata
	Flags              uint16    // NK flags (compressed name, root key, etc.)
	ParentOffset       uint32    // Cell offset of parent NK
	SubkeyListOffset   uint32    // Cell offset of subkey list
	ValueListOffset    uint32    // Cell offset of value list
	SecurityOffset     uint32    // Cell offset of security descriptor (SK)
	ClassNameOffset    uint32    // Cell offset of class name
	MaxNameLength      uint32    // Maximum subkey name length
	MaxClassLength     uint32    // Maximum class length
	MaxValueNameLength uint32    // Maximum value name length
	MaxValueDataLength uint32    // Maximum value data length
	ClassName          string    // Class name (if present)
}

// HiveInfo exposes registry hive header (REGF) metadata.
type HiveInfo struct {
	PrimarySequence   uint32    // Primary sequence number (for atomicity checks)
	SecondarySequence uint32    // Secondary sequence number
	LastWrite         time.Time // Last write timestamp
	MajorVersion      uint32    // Format major version
	MinorVersion      uint32    // Format minor version
	Type              uint32    // 0 = primary, 1 = alternate
	RootCellOffset    uint32    // Offset of root NK record
	HiveBinsDataSize  uint32    // Total size of HBIN data
	ClusteringFactor  uint32    // Clustering factor (rarely used)
}

// -----------------------------------------------------------------------------
// Open Options & Read Options
// -----------------------------------------------------------------------------

// OpenOptions controls safety/performance tradeoffs for constructing a Reader.
type OpenOptions struct {
	// ZeroCopy allows returned slices to alias the underlying mapped buffer
	// when safe. Callers must treat these as read-only and must not retain
	// them after Close. If unsure, set ReadOptions.CopyData per call.
	ZeroCopy bool

	// Tolerant enables best-effort traversal on mild inconsistencies where
	// recovery is possible (bounds are still enforced).
	Tolerant bool

	// MaxCellSize guards against absurd/malicious cell sizes.
	// Zero selects a conservative default (e.g., 64 MiB).
	MaxCellSize int

	// CollectDiagnostics enables passive diagnostic collection during normal
	// operations. Issues encountered during traversal are recorded and can be
	// retrieved via GetDiagnostics(). Has minimal overhead (~0-1ns per check).
	// Use this for monitoring production hives. For comprehensive scanning,
	// use Diagnose() instead which performs exhaustive validation.
	CollectDiagnostics bool
}

// ReadOptions let callers request per-call behavior (e.g., forced copying).
type ReadOptions struct {
	// CopyData forces a heap copy even if ZeroCopy is enabled globally.
	CopyData bool
}

// -----------------------------------------------------------------------------
// Read-Only API (high-performance navigation & decoding)
// -----------------------------------------------------------------------------

// Reader is a high-level, read-only view over a registry hive.
// Implementations should be safe for concurrent, independent calls.
type Reader interface {
	// Close releases resources (e.g., unmaps the file). After Close, any
	// previously returned zero-copy slices are invalid.
	Close() error

	// Info returns hive header metadata (version, timestamps, etc).
	Info() HiveInfo

	// Root returns the root key node ID (typically "\").
	Root() (NodeID, error)

	// StatKey returns cheap NK metadata (no deep decoding).
	StatKey(NodeID) (KeyMeta, error)

	// DetailKey returns full NK record metadata for inspection.
	DetailKey(NodeID) (KeyDetail, error)

	// Lightweight metadata getters (optimized for single-field access):
	// These methods skip name decoding and make zero allocations, providing
	// performance comparable to hivex's individual getters.
	// Use these when you only need one field; use StatKey() when you need multiple.

	// KeyTimestamp returns the LastWrite timestamp for a key.
	// Zero allocations, ~30-40ns. Equivalent to StatKey().LastWrite but much faster.
	KeyTimestamp(NodeID) (time.Time, error)

	// KeySubkeyCount returns the number of direct child keys.
	// Zero allocations, ~30-40ns. Equivalent to StatKey().SubkeyN but much faster.
	KeySubkeyCount(NodeID) (int, error)

	// KeyValueCount returns the number of values in a key.
	// Zero allocations, ~30-40ns. Equivalent to StatKey().ValueN but much faster.
	KeyValueCount(NodeID) (int, error)

	// KeyName returns just the key name without building the full metadata struct.
	// Lighter than StatKey() but still allocates for the string.
	// Equivalent to StatKey().Name but avoids time conversion and struct construction.
	KeyName(NodeID) (string, error)

	// Subkeys lists direct child keys; for very large fan-outs, prefer Scanner.
	Subkeys(NodeID) ([]NodeID, error)

	// Lookup finds a direct child key by name (case-insensitive).
	// Returns ErrNotFound if the child doesn't exist.
	Lookup(parent NodeID, childName string) (NodeID, error)

	// Values lists value handles for a key.
	Values(NodeID) ([]ValueID, error)

	// StatValue returns cheap VK metadata (no data decode).
	StatValue(ValueID) (ValueMeta, error)

	// Lightweight value getter (optimized for single-field access):
	// This method skips name decoding and makes zero allocations, providing
	// performance comparable to hivex's individual getters.
	// Use this when you only need the type; use StatValue() when you need multiple fields.

	// ValueType returns the registry type of a value without decoding the name.
	// Zero allocations, ~50-60ns. Equivalent to StatValue().Type but much faster.
	ValueType(ValueID) (RegType, error)

	// ValueName returns the name of a value without fetching full metadata.
	// Lighter than StatValue() as it skips struct construction, but still allocates
	// for string decoding (Windows-1252/UTF-16LE → UTF-8).
	// Equivalent to StatValue().Name but faster.
	ValueName(ValueID) (string, error)

	// ValueBytes returns raw value bytes. If ZeroCopy is enabled and safe—and
	// CopyData is false—the returned slice aliases the backing buffer.
	ValueBytes(ValueID, ReadOptions) ([]byte, error)

	// Decoders with type checks:
	ValueString(
		ValueID,
		ReadOptions,
	) (string, error) // REG_SZ / REG_EXPAND_SZ (UTF-16LE → UTF-8)
	ValueStrings(ValueID, ReadOptions) ([]string, error) // REG_MULTI_SZ
	ValueDWORD(ValueID) (uint32, error)                  // REG_DWORD (handles LE/BE)
	ValueQWORD(ValueID) (uint64, error)                  // REG_QWORD

	// Path helpers for ergonomics (not performance critical):
	// Path syntax mirrors Windows-style roots (e.g., "HKLM\\Software\\Vendor").
	Find(path string) (NodeID, error)

	// Walk performs pre-order traversal starting at n. Returning a non-nil error
	// aborts the traversal; return a sentinel (implementation-defined) to skip children.
	Walk(n NodeID, fn func(NodeID) error) error

	// Navigation helpers (hivex compatibility):

	// Parent returns the parent node of the given node.
	// Returns ErrNotFound if the node is the root node (which has no parent).
	Parent(NodeID) (NodeID, error)

	// GetChild finds a direct child node by name (case-insensitive).
	// Returns ErrNotFound if no matching child exists.
	GetChild(parent NodeID, name string) (NodeID, error)

	// GetValue finds a value by name (case-insensitive) at the given node.
	// Returns ErrNotFound if no matching value exists.
	// For the default/unnamed value, use an empty string "".
	GetValue(node NodeID, name string) (ValueID, error)

	// Introspection functions (for forensics/debugging):

	// KeyNameLen returns the byte length of the key name without decoding.
	// This is cheaper than StatKey if you only need the length.
	// NOTE: Returns raw byte count from NK structure. For hivex-compatible
	// behavior (UTF-8 decoded length), use KeyNameLenDecoded().
	KeyNameLen(NodeID) (int, error)

	// ValueNameLen returns the byte length of the value name without decoding.
	// This is cheaper than StatValue if you only need the length.
	// NOTE: Returns raw byte count from VK structure. For hivex-compatible
	// behavior (UTF-8 decoded length), use ValueNameLenDecoded().
	ValueNameLen(ValueID) (int, error)

	// NodeStructSize returns the size of the NK record structure in bytes.
	// This is the total cell size, including the cell header.
	// NOTE: Returns actual allocated cell size with alignment. For hivex-compatible
	// behavior (calculated minimum size), use NodeStructSizeCalculated().
	NodeStructSize(NodeID) (int, error)

	// ValueStructSize returns the size of the VK record structure in bytes.
	// This is the total cell size, including the cell header.
	// NOTE: Returns actual allocated cell size with alignment. For hivex-compatible
	// behavior (calculated minimum size), use ValueStructSizeCalculated().
	ValueStructSize(ValueID) (int, error)

	// ValueDataCellOffset returns the file offset and length of the data cell for a value.
	// For inline values (stored in the VK record itself), returns (0, length).
	// Otherwise returns the absolute file offset and size of the data cell.
	// Returns (offset uint32, length int, error).
	// NOTE: Returns actual data size for inline values. For hivex-compatible
	// behavior (0 length as flag), use ValueDataCellOffsetHivex().
	ValueDataCellOffset(ValueID) (uint32, int, error)

	// Hivex-compatible introspection methods:
	// These methods return values matching hivex behavior exactly, for drop-in compatibility.

	// KeyNameLenDecoded returns the UTF-8 string length of the decoded key name.
	// This matches hivex_node_name_len() behavior, which decodes the name first.
	KeyNameLenDecoded(NodeID) (int, error)

	// ValueNameLenDecoded returns the UTF-8 string length of the decoded value name.
	// This matches hivex_value_key_len() behavior, which decodes the name first.
	ValueNameLenDecoded(ValueID) (int, error)

	// NodeStructSizeCalculated returns the calculated minimum NK structure size.
	// This matches hivex_node_struct_length() behavior, which calculates based on fields.
	NodeStructSizeCalculated(NodeID) (int, error)

	// ValueStructSizeCalculated returns the calculated minimum VK structure size.
	// This matches hivex_value_struct_length() behavior, which calculates based on fields.
	ValueStructSizeCalculated(ValueID) (int, error)

	// ValueDataCellOffsetHivex returns data cell info matching hivex behavior.
	// For inline values, returns (offset, 0) as a flag instead of (offset, actualSize).
	// This matches hivex_value_data_cell_offset() behavior.
	ValueDataCellOffsetHivex(ValueID) (uint32, int, error)

	// Diagnostics & Forensics
	// ------------------------

	// Diagnose performs exhaustive validation of the entire hive structure,
	// collecting all issues found (not just the first error). This is an
	// on-demand scan that walks every HBIN, NK, VK, and data cell.
	// Use this for forensic analysis or comprehensive health checks.
	// Unlike passive collection, this scans structures even if not accessed.
	Diagnose() (*DiagnosticReport, error)

	// GetDiagnostics returns diagnostics passively collected during normal
	// operations (only if OpenOptions.CollectDiagnostics was true).
	// Returns nil if diagnostic collection was not enabled.
	// This provides lightweight monitoring with minimal performance impact.
	GetDiagnostics() *DiagnosticReport
}

// -----------------------------------------------------------------------------
// Allocation-Light Iteration (for huge fan-out trees)
// -----------------------------------------------------------------------------

// NodeIter scans subkeys without allocating large slices.
type NodeIter interface {
	Next() bool
	Err() error
	Node() NodeID
}

// ValueIter scans values on demand.
type ValueIter interface {
	Next() bool
	Err() error
	Value() ValueID
}

// Scanner constructs iterators; implementations may reuse pooled instances.
type Scanner interface {
	ScanSubkeys(NodeID) (NodeIter, error)
	ScanValues(NodeID) (ValueIter, error)
}

// -----------------------------------------------------------------------------
// Editing API (transactional, copy-on-write planner)
// -----------------------------------------------------------------------------

// Editor creates transactions that plan copy-on-write allocations and emit a
// new, consistent hive image on Commit. Implementations should not mutate the
// original backing buffer.
type Editor interface {
	Begin() Tx
}

// Tx is a unit of work that can be committed atomically to a destination.
type Tx interface {
	// CreateKey ensures a key exists at path. If CreateParents is true, any
	// missing ancestors are created.
	CreateKey(path string, opts CreateKeyOptions) error

	// DeleteKey removes a key. If Recursive is true, it removes the subtree.
	DeleteKey(path string, opts DeleteKeyOptions) error

	// SetValue sets or replaces a named value at path with the given type/data.
	SetValue(path, name string, t RegType, data []byte) error

	// DeleteValue removes a named value at path.
	DeleteValue(path, name string) error

	// Commit validates the plan, rebuilds HBINs as needed, recomputes header fields,
	// and writes a new hive to the provided Writer.
	Commit(dst Writer, opts WriteOptions) error

	// Rollback discards the plan and its resources.
	Rollback() error
}

// CreateKeyOptions controls CreateKey behavior.
type CreateKeyOptions struct {
	CreateParents bool
}

// DeleteKeyOptions controls DeleteKey behavior.
type DeleteKeyOptions struct {
	Recursive bool
}

// -----------------------------------------------------------------------------
// Emit / Writer targets
// -----------------------------------------------------------------------------

// Writer is an abstract sink for Commit results (file or memory).
type Writer interface {
	// WriteHive receives the fully materialized hive bytes. The buffer must be
	// treated as immutable after return.
	WriteHive(buf []byte) error
}

// WriteOptions control emission characteristics.
type WriteOptions struct {
	// Repack compacts free cells and may rebucket data to improve locality.
	Repack bool
	// Timestamp overrides NK last-write times for created entries (zero = leave).
	Timestamp time.Time
}

// -----------------------------------------------------------------------------
// .REG (regedit) import/export
// -----------------------------------------------------------------------------

// RegCodec parses and emits textual .reg format, mapping to high-level edit ops.
type RegCodec interface {
	// ParseReg converts .reg text into edit operations suitable for a Tx.
	ParseReg(regText []byte, opts RegParseOptions) ([]EditOp, error)

	// ExportReg walks a subtree and emits .reg text.
	ExportReg(r Reader, root NodeID, opts RegExportOptions) ([]byte, error)
}

// EditOp represents a high-level registry edit.
type EditOp interface{ isEdit() }

type OpSetValue struct {
	Path string
	Name string
	Type RegType
	Data []byte
}

func (OpSetValue) isEdit() {}

type OpDeleteValue struct {
	Path string
	Name string
}

func (OpDeleteValue) isEdit() {}

type OpCreateKey struct {
	Path string
}

func (OpCreateKey) isEdit() {}

type OpDeleteKey struct {
	Path      string
	Recursive bool
}

func (OpDeleteKey) isEdit() {}

type RegParseOptions struct {
	// InputEncoding declares the .reg text encoding (e.g., "UTF-16LE").
	// Implementations may transcode to UTF-8 internally.
	InputEncoding string

	// Prefix to strip from all key paths in the .reg file.
	// Example: "HKEY_LOCAL_MACHINE\\SOFTWARE" for SOFTWARE hive
	// If empty and AutoPrefix is false, paths are used as-is.
	Prefix string

	// AutoPrefix automatically detects and strips standard Windows registry
	// prefixes (HKEY_LOCAL_MACHINE\SOFTWARE, HKEY_LOCAL_MACHINE\SYSTEM, etc.)
	// This is useful when the target hive is unknown but standard prefixes apply.
	AutoPrefix bool
}

type RegExportOptions struct {
	// Output encoding for emitted .reg (e.g., "UTF-16LE" with BOM to match regedit.exe).
	OutputEncoding string
	WithBOM        bool
}

// -----------------------------------------------------------------------------
// Transaction-log application (optional seam)
// -----------------------------------------------------------------------------

// LogApplier applies transaction/redo logs to a base hive image to produce a
// normalized view. Not required for all use-cases but common in DFIR workflows.
type LogApplier interface {
	// Apply returns a new image (or a thin overlay) representing base + logs.
	Apply(base []byte, logs ...[]byte) ([]byte, error)
}
