package trie

// Node is a single node in the PatchTrie, corresponding to one key component
// in the registry hierarchy. The root node has an empty Name and serves as
// a virtual root above all top-level keys.
type Node struct {
	Name      string  // original case component name
	NameLower string  // pre-lowercased for comparison
	Hash      uint32  // pre-computed LH hash (computed once during Build)
	Children  []*Node // sorted by NameLower

	// Operations at this node.
	Values    []ValueOp
	DeleteKey bool
	EnsureKey bool

	// Filled during walk phase (Phase 2) — zero values initially.
	CellIdx       uint32 // NK cell offset (0xFFFFFFFF if doesn't exist)
	Exists        bool
	SKCellIdx     uint32
	SubKeyListRef uint32
	SubKeyCount   uint32
	ValueListRef  uint32
	ValueCount    uint32
}

// ValueOp represents a single value-level operation to perform at a node.
type ValueOp struct {
	Name   string
	Type   uint32
	Data   []byte
	Delete bool
}
