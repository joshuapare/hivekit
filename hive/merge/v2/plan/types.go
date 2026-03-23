package plan

import "github.com/joshuapare/hivekit/hive/merge/v2/trie"

// CellKind identifies the registry cell type represented by an AllocEntry.
type CellKind uint8

const (
	// CellNK is a Node Key cell (registry key).
	CellNK CellKind = iota
	// CellVK is a Value Key cell (registry value header).
	CellVK
	// CellData is a data cell (value payload stored outside the VK).
	CellData
	// CellSubkeyList is a subkey list cell (LH/LF rebuild for a parent key).
	CellSubkeyList
	// CellValueList is a value list cell (flat offset array rebuild).
	CellValueList
)

// SpacePlan captures the result of the plan phase: how many bytes of new cells
// are needed and a detailed manifest of every individual allocation required.
type SpacePlan struct {
	// TotalNewBytes is the sum of all aligned cell sizes across the manifest.
	// This is the value to supply to EnableBumpMode(int32).
	TotalNewBytes int32

	// NewNKCount is the number of new NK (key) cells to allocate.
	NewNKCount int

	// NewVKCount is the number of new VK (value header) cells to allocate.
	NewVKCount int

	// NewDataCount is the number of new data cells to allocate (non-inline values).
	NewDataCount int

	// ListRebuilds is the number of subkey or value lists that must be rewritten.
	ListRebuilds int

	// InPlaceUpdates is the number of existing cells that can be patched in place
	// without allocating new storage.
	InPlaceUpdates int

	// Manifest is the ordered list of every allocation the write phase must make.
	Manifest []AllocEntry
}

// AllocEntry describes a single cell allocation within the manifest.
type AllocEntry struct {
	// Node is the trie node this allocation belongs to.
	Node *trie.Node

	// Kind identifies the cell type.
	Kind CellKind

	// Size is the aligned total cell size in bytes (including the 4-byte header).
	Size int32
}
