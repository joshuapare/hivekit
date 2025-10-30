package ast

import (
	"errors"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/pkg/types"
)

// ErrNodeDeleted is returned when a node has been marked for deletion
var ErrNodeDeleted = errors.New("node is deleted")

// TransactionChanges represents the set of changes from a transaction.
// This interface allows the builder to work with transaction data without
// depending on the edit package.
type TransactionChanges interface {
	// GetCreatedKeys returns all paths that were created
	GetCreatedKeys() map[string]bool

	// GetDeletedKeys returns all paths that were deleted
	GetDeletedKeys() map[string]bool

	// GetSetValues returns all values that were set
	GetSetValues() map[ValueKey]ValueData

	// GetDeletedValues returns all values that were deleted
	GetDeletedValues() map[ValueKey]bool
}

// cachedTransactionChanges wraps TransactionChanges with caching to avoid
// repeatedly creating new maps for GetSetValues(), GetCreatedKeys(), etc.
type cachedTransactionChanges struct {
	inner         TransactionChanges
	setValues     map[ValueKey]ValueData
	createdKeys   map[string]bool
	deletedKeys   map[string]bool
	deletedValues map[ValueKey]bool
}

func newCachedTransactionChanges(tc TransactionChanges) *cachedTransactionChanges {
	return &cachedTransactionChanges{
		inner:         tc,
		setValues:     tc.GetSetValues(),     // Cache once on creation
		createdKeys:   tc.GetCreatedKeys(),   // Cache once on creation
		deletedKeys:   tc.GetDeletedKeys(),   // Cache once on creation
		deletedValues: tc.GetDeletedValues(), // Cache once on creation
	}
}

func (c *cachedTransactionChanges) GetSetValues() map[ValueKey]ValueData {
	return c.setValues // Return cached map
}

func (c *cachedTransactionChanges) GetCreatedKeys() map[string]bool {
	return c.createdKeys // Return cached map
}

func (c *cachedTransactionChanges) GetDeletedKeys() map[string]bool {
	return c.deletedKeys // Return cached map
}

func (c *cachedTransactionChanges) GetDeletedValues() map[ValueKey]bool {
	return c.deletedValues // Return cached map
}

// ValueKey identifies a value by path and name.
type ValueKey struct {
	Path string
	Name string
}

// ValueData holds value type and data.
type ValueData struct {
	Type types.RegType
	Data []byte
}

// BuildIncremental builds an AST incrementally from a base reader and transaction changes.
// Only changed subtrees are materialized in memory; unchanged subtrees use lazy loading
// and point to the base hive buffer for zero-copy efficiency.
func BuildIncremental(r types.Reader, changes TransactionChanges, baseHive []byte) (*Tree, error) {
	// Wrap changes with caching to avoid repeated map copies
	cachedChanges := newCachedTransactionChanges(changes)

	tree := NewTreeWithBase(baseHive)

	// Get root from base hive
	rootID, err := r.Root()
	if err != nil {
		return nil, fmt.Errorf("failed to get root: %w", err)
	}

	// Build root node
	tree.Root, err = buildNodeFromBase(r, rootID, "", nil, cachedChanges)
	if err != nil {
		return nil, fmt.Errorf("failed to build root: %w", err)
	}

	// Apply created keys
	for path := range cachedChanges.GetCreatedKeys() {
		if err := ensurePathExists(tree, path); err != nil {
			return nil, fmt.Errorf("failed to create path %s: %w", path, err)
		}
	}

	// Apply set values
	for vk, vd := range cachedChanges.GetSetValues() {
		node := tree.FindNode(vk.Path)
		if node == nil {
			// Ensure path exists first
			if err := ensurePathExists(tree, vk.Path); err != nil {
				return nil, fmt.Errorf("failed to create path %s for value: %w", vk.Path, err)
			}
			node = tree.FindNode(vk.Path)
		}
		if node != nil {
			node.AddValue(vk.Name, vd.Type, vd.Data)
		}
	}

	// Apply deleted values
	for vk := range changes.GetDeletedValues() {
		node := tree.FindNode(vk.Path)
		if node != nil {
			node.RemoveValue(vk.Name)
		}
	}

	// Apply deleted keys (must be done after value operations)
	for path := range changes.GetDeletedKeys() {
		if err := deletePath(tree, path); err != nil {
			return nil, fmt.Errorf("failed to delete path %s: %w", path, err)
		}
	}

	return tree, nil
}

// buildNodeFromBase builds a node from the base types.
// For unchanged subtrees, nodes are marked for lazy loading.
// For changed subtrees, nodes are fully materialized.
func buildNodeFromBase(
	r types.Reader,
	nodeID types.NodeID,
	path string,
	parent *Node,
	changes TransactionChanges,
) (*Node, error) {
	// Check if this path is deleted
	if changes.GetDeletedKeys()[path] {
		return nil, ErrNodeDeleted // deleted, don't include
	}

	// Get node metadata
	meta, err := r.StatKey(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to stat key: %w", err)
	}

	node := &Node{
		Name:       meta.Name,
		Offset:     int32(nodeID),
		Parent:     parent,
		Children:   make([]*Node, 0),
		Values:     make([]*Value, 0),
		Dirty:      false,
		BaseReader: r,
		BaseNodeID: nodeID,
	}

	// Check if this subtree has any changes
	hasChanges := pathHasChanges(path, changes)

	if !hasChanges {
		// Unchanged subtree - use lazy loading
		node.LazyChildren = true
		// Values will also be loaded lazily when accessed
		return node, nil
	}

	// This subtree has changes - materialize children
	childIDs, err := r.Subkeys(nodeID)
	if err == nil {
		for _, childID := range childIDs {
			childMeta, err := r.StatKey(childID)
			if err != nil {
				continue
			}

			childPath := path
			if childPath != "" {
				childPath += RegistryPathSeparator
			}
			childPath += childMeta.Name

			child, err := buildNodeFromBase(r, childID, childPath, node, changes)
			if err != nil {
				// Skip deleted nodes (they return ErrNodeDeleted)
				if errors.Is(err, ErrNodeDeleted) {
					continue
				}
				// For other errors, continue to skip the problematic child
				continue
			}
			node.Children = append(node.Children, child)
		}
	}

	// Load values (check for changes)
	valueIDs, err := r.Values(nodeID)
	if err == nil {
		for _, valueID := range valueIDs {
			valueMeta, err := r.StatValue(valueID)
			if err != nil {
				continue
			}

			vk := ValueKey{Path: path, Name: valueMeta.Name}

			// Check if value is deleted
			if changes.GetDeletedValues()[vk] {
				continue
			}

			// Check if value is modified
			if vd, ok := changes.GetSetValues()[vk]; ok {
				// Use modified value
				node.Values = append(node.Values, &Value{
					Name:  valueMeta.Name,
					Type:  vd.Type,
					Data:  vd.Data,
					Dirty: true,
				})
				node.Dirty = true
			} else {
				// Use original value (zero-copy)
				data, err := r.ValueBytes(valueID, types.ReadOptions{CopyData: false})
				if err != nil {
					continue
				}
				node.Values = append(node.Values, &Value{
					Name:  valueMeta.Name,
					Type:  valueMeta.Type,
					Data:  data,
					Dirty: false,
				})
			}
		}
	}

	return node, nil
}

// pathHasChanges checks if a path or any of its descendants have changes.
func pathHasChanges(path string, changes TransactionChanges) bool {
	// Check if path itself is created or deleted
	if changes.GetCreatedKeys()[path] {
		return true
	}
	if changes.GetDeletedKeys()[path] {
		return true
	}

	// Check if any descendant path has changes
	pathPrefix := path
	if pathPrefix != "" {
		pathPrefix += RegistryPathSeparator
	}

	for createdPath := range changes.GetCreatedKeys() {
		if strings.HasPrefix(createdPath, pathPrefix) {
			return true
		}
	}

	for deletedPath := range changes.GetDeletedKeys() {
		if strings.HasPrefix(deletedPath, pathPrefix) {
			return true
		}
	}

	// Check if any values in this path or descendants have changes
	for vk := range changes.GetSetValues() {
		if vk.Path == path || strings.HasPrefix(vk.Path, pathPrefix) {
			return true
		}
	}

	for vk := range changes.GetDeletedValues() {
		if vk.Path == path || strings.HasPrefix(vk.Path, pathPrefix) {
			return true
		}
	}

	return false
}

// ensurePathExists ensures all nodes along a path exist, creating them if needed.
func ensurePathExists(tree *Tree, path string) error {
	if path == "" {
		return nil
	}

	segments := splitPath(path)
	current := tree.Root

	for _, segment := range segments {
		// Ensure children are loaded
		if current.LazyChildren {
			if err := current.LoadChildren(); err != nil {
				return err
			}
		}

		// Look for existing child
		found := false
		for _, child := range current.Children {
			if child.Name == segment {
				current = child
				found = true
				break
			}
		}

		if !found {
			// Create new child
			current = current.AddChild(segment)
		}
	}

	return nil
}

// deletePath deletes a node at the given path.
func deletePath(tree *Tree, path string) error {
	if path == "" {
		return errors.New("cannot delete root")
	}

	segments := splitPath(path)
	if len(segments) == 0 {
		return nil
	}

	// Find parent
	parentPath := ""
	if len(segments) > 1 {
		parentPath = strings.Join(segments[:len(segments)-1], RegistryPathSeparator)
	}

	parent := tree.FindNode(parentPath)
	if parent == nil {
		return nil // parent doesn't exist, nothing to delete
	}

	// Remove child
	childName := segments[len(segments)-1]
	parent.RemoveChild(childName)

	return nil
}
