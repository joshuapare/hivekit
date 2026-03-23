package trie

import (
	"cmp"
	"slices"
	"strings"

	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// Build constructs a PatchTrie from a slice of merge operations and returns
// the virtual root node. Each call to Build produces an independent trie;
// the root's Name and Hash are empty/zero (it has no corresponding registry key).
//
// For every op, Build walks (or creates) trie nodes from root using the op's
// KeyPath components, then attaches the operation at the leaf:
//   - OpEnsureKey → sets Node.EnsureKey
//   - OpDeleteKey → sets Node.DeleteKey
//   - OpSetValue  → appends a ValueOp{Delete: false} to Node.Values
//   - OpDeleteValue → appends a ValueOp{Delete: true} to Node.Values
//
// Children at each level are kept sorted by NameLower to allow binary search
// during the walk phase. Hash is pre-computed once via subkeys.Hash.
// CellIdx is initialised to format.InvalidOffset (0xFFFFFFFF) on every node.
func Build(ops []merge.Op) *Node {
	root := newNode("", format.InvalidOffset)

	for i := range ops {
		op := &ops[i]
		node := root
		for _, component := range op.KeyPath {
			node = getOrCreateChild(node, component)
		}
		attachOp(node, op)
	}

	return root
}

// newNode allocates a Node for the given name component and initialises its
// CellIdx to the supplied sentinel value.
func newNode(name string, cellIdx uint32) *Node {
	nameLower := strings.ToLower(name)
	var hash uint32
	if name != "" {
		hash = subkeys.Hash(nameLower)
	}
	return &Node{
		Name:      name,
		NameLower: nameLower,
		Hash:      hash,
		CellIdx:   cellIdx,
	}
}

// getOrCreateChild returns the existing child of parent whose NameLower matches
// strings.ToLower(name), or creates and inserts a new one maintaining sorted
// order by NameLower.
func getOrCreateChild(parent *Node, name string) *Node {
	nameLower := strings.ToLower(name)

	idx, found := slices.BinarySearchFunc(parent.Children, nameLower, func(n *Node, key string) int {
		return cmp.Compare(n.NameLower, key)
	})
	if found {
		return parent.Children[idx]
	}

	child := newNode(name, format.InvalidOffset)
	// Insert at idx to maintain sorted order.
	parent.Children = slices.Insert(parent.Children, idx, child)
	return child
}

// attachOp mutates node to record the effect of op.
func attachOp(node *Node, op *merge.Op) {
	switch op.Type {
	case merge.OpEnsureKey:
		node.EnsureKey = true
	case merge.OpDeleteKey:
		node.DeleteKey = true
	case merge.OpSetValue:
		node.Values = append(node.Values, ValueOp{
			Name:   op.ValueName,
			Type:   op.ValueType,
			Data:   op.Data,
			Delete: false,
		})
	case merge.OpDeleteValue:
		node.Values = append(node.Values, ValueOp{
			Name:   op.ValueName,
			Delete: true,
		})
	}
}
