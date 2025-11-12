package ast

import (
	"github.com/joshuapare/hivekit/pkg/types"
)

// RegistryPathSeparator is the backslash character used to separate
// components in Windows Registry paths.
const RegistryPathSeparator = "\\"

// Tree represents the complete registry hive tree structure.
// It maintains references to the base hive buffer for zero-copy efficiency.
type Tree struct {
	Root     *Node
	BaseHive []byte // reference to original mmap buffer (for zero-copy)
}

// Node represents a registry key (NK record) in the AST.
// Nodes can be:
// - Unchanged: point to base hive data (zero-copy)
// - Modified: contain new in-memory data
// - New: created in this transaction.
type Node struct {
	// Identity
	Name   string // key name
	Offset int32  // offset in base hive (0 if new node)

	// Tree structure
	Parent   *Node
	Children []*Node
	Values   []*Value

	// State tracking
	Dirty bool // true if node or any descendant was modified

	// Zero-copy optimization: lazy loading for unchanged subtrees
	LazyChildren bool         // true if children not yet loaded from base
	BaseReader   types.Reader // reader for lazy loading
	BaseNodeID   types.NodeID // ID in base hive for lazy loading
}

// Value represents a registry value (VK record) in the AST.
type Value struct {
	Name  string        // value name ("" for default/unnamed)
	Type  types.RegType // registry type (REG_SZ, REG_DWORD, etc.)
	Data  []byte        // value data (may point to BaseHive for zero-copy)
	Dirty bool          // true if modified in this transaction
}

// NewTree creates a new empty tree.
func NewTree() *Tree {
	return &Tree{
		Root: &Node{
			Name:     "",
			Children: make([]*Node, 0),
			Values:   make([]*Value, 0),
		},
	}
}

// NewTreeWithBase creates a tree that references a base hive buffer.
func NewTreeWithBase(baseHive []byte) *Tree {
	return &Tree{
		BaseHive: baseHive,
		Root: &Node{
			Name:     "",
			Children: make([]*Node, 0),
			Values:   make([]*Value, 0),
		},
	}
}

// FindNode finds a node by path in the tree.
// Returns nil if not found.
func (t *Tree) FindNode(path string) *Node {
	if path == "" {
		return t.Root
	}
	return findNodeRecursive(t.Root, splitPath(path))
}

// findNodeRecursive finds a node by path segments.
func findNodeRecursive(node *Node, segments []string) *Node {
	if len(segments) == 0 {
		return node
	}

	// Ensure children are loaded (lazy loading)
	if node.LazyChildren {
		if err := node.LoadChildren(); err != nil {
			return nil
		}
	}

	// Search for matching child
	for _, child := range node.Children {
		if child.Name == segments[0] {
			return findNodeRecursive(child, segments[1:])
		}
	}

	return nil
}

// SplitPath splits a registry path into segments.
// Exported to allow internal packages to use it.
func SplitPath(path string) []string {
	return splitPath(path)
}

// MarkDirty marks a node and all its ancestors as dirty.
// This is used for incremental serialization to identify which subtrees need to be written.
func (n *Node) MarkDirty() {
	current := n
	for current != nil {
		if current.Dirty {
			// Already dirty, ancestors are also dirty
			break
		}
		current.Dirty = true
		current = current.Parent
	}
}

// LoadChildren loads children from base hive (lazy loading).
// This is exported to allow internal packages to trigger lazy loading.
func (n *Node) LoadChildren() error {
	if !n.LazyChildren || n.BaseReader == nil {
		return nil
	}

	// Load children from base hive
	childIDs, err := n.BaseReader.Subkeys(n.BaseNodeID)
	if err != nil {
		return err
	}

	n.Children = make([]*Node, 0, len(childIDs))
	for _, childID := range childIDs {
		meta, statErr := n.BaseReader.StatKey(childID)
		if statErr != nil {
			continue
		}

		child := &Node{
			Name:         meta.Name,
			Offset:       int32(childID),
			Parent:       n,
			Children:     make([]*Node, 0),
			Values:       make([]*Value, 0),
			Dirty:        false,
			LazyChildren: true, // children's children are also lazy
			BaseReader:   n.BaseReader,
			BaseNodeID:   childID,
		}
		n.Children = append(n.Children, child)
	}

	n.LazyChildren = false
	return nil
}

// AddChild adds a new child node.
func (n *Node) AddChild(name string) *Node {
	child := &Node{
		Name:     name,
		Parent:   n,
		Children: make([]*Node, 0),
		Values:   make([]*Value, 0),
		Dirty:    true,
	}
	n.Children = append(n.Children, child)
	n.MarkDirty()
	return child
}

// AddValue adds or updates a value on this node.
func (n *Node) AddValue(name string, typ types.RegType, data []byte) {
	// Check if value exists and update
	for i := range n.Values {
		if n.Values[i].Name == name {
			n.Values[i].Type = typ
			n.Values[i].Data = data
			n.Values[i].Dirty = true
			n.MarkDirty()
			return
		}
	}

	// Add new value
	n.Values = append(n.Values, &Value{
		Name:  name,
		Type:  typ,
		Data:  data,
		Dirty: true,
	})
	n.MarkDirty()
}

// RemoveValue removes a value by name.
func (n *Node) RemoveValue(name string) bool {
	for i, v := range n.Values {
		if v.Name == name {
			n.Values = append(n.Values[:i], n.Values[i+1:]...)
			n.MarkDirty()
			return true
		}
	}
	return false
}

// RemoveChild removes a child node by name.
func (n *Node) RemoveChild(name string) bool {
	for i, child := range n.Children {
		if child.Name == name {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			n.MarkDirty()
			return true
		}
	}
	return false
}

// splitPath splits a registry path into segments.
func splitPath(path string) []string {
	if path == "" {
		return nil
	}

	segments := make([]string, 0)
	start := 0
	for i := range len(path) {
		if path[i] == RegistryPathSeparator[0] {
			if i > start {
				segments = append(segments, path[start:i])
			}
			start = i + 1
		}
	}
	if start < len(path) {
		segments = append(segments, path[start:])
	}

	return segments
}
