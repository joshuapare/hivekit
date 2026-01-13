package hivexval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Root returns the root node handle.
//
// Example:
//
//	root, err := v.Root()
//	if err != nil {
//	    t.Fatal(err)
//	}
func (v *Validator) Root() (interface{}, error) {
	switch v.backend {
	case BackendBindings:
		if v.hive == nil {
			return nil, errors.New("bindings backend not initialized")
		}
		root := v.hive.Root()
		if root == 0 {
			return nil, errors.New("root node is zero (invalid)")
		}
		return root, nil

	case BackendReader:
		if v.reader == nil {
			return nil, errors.New("reader backend not initialized")
		}
		root, err := v.reader.Root()
		if err != nil {
			return nil, fmt.Errorf("get root: %w", err)
		}
		return root, nil

	case BackendNone, BackendHivexsh:
		return nil, errors.New("no supported backend for Root()")

	default:
		return nil, errors.New("no supported backend for Root()")
	}
}

// GetKey finds a key by path.
//
// Path should be a slice of key names (e.g., []string{"Software", "MyApp"}).
//
// Example:
//
//	key, err := v.GetKey([]string{"Software", "MyApp"})
//	if err != nil {
//	    t.Fatal("Key not found")
//	}
func (v *Validator) GetKey(path []string) (interface{}, error) {
	root, err := v.Root()
	if err != nil {
		return nil, err
	}

	// Empty path returns root
	if len(path) == 0 {
		return root, nil
	}

	// Navigate to target key
	current := root
	for _, name := range path {
		current, err = v.getChild(current, name)
		if err != nil {
			return nil, fmt.Errorf("navigate to '%s': %w", strings.Join(path, "\\"), err)
		}
	}

	return current, nil
}

// GetKeyName returns the name of a key.
//
// Returns empty string for the root key.
func (v *Validator) GetKeyName(key interface{}) (string, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return "", errors.New("invalid key handle type for bindings")
		}
		return v.hive.NodeName(node), nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return "", errors.New("invalid key handle type for reader")
		}
		return v.reader.KeyName(nodeID)

	case BackendNone, BackendHivexsh:
		return "", errors.New("backend does not support GetKeyName")

	default:
		return "", errors.New("backend does not support GetKeyName")
	}
}

// GetSubkeys lists all child keys.
//
// Example:
//
//	children, err := v.GetSubkeys(key)
//	for _, child := range children {
//	    name, _ := v.GetKeyName(child)
//	    t.Logf("Child: %s", name)
//	}
func (v *Validator) GetSubkeys(key interface{}) ([]interface{}, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return nil, errors.New("invalid key handle type for bindings")
		}
		children := v.hive.NodeChildren(node)
		result := make([]interface{}, len(children))
		for i, child := range children {
			result[i] = child
		}
		return result, nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return nil, errors.New("invalid key handle type for reader")
		}
		children, err := v.reader.Subkeys(nodeID)
		if err != nil {
			return nil, err
		}
		result := make([]interface{}, len(children))
		for i, child := range children {
			result[i] = child
		}
		return result, nil

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support GetSubkeys")

	default:
		return nil, errors.New("backend does not support GetSubkeys")
	}
}

// GetSubkeyCount returns number of child keys.
func (v *Validator) GetSubkeyCount(key interface{}) (int, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return 0, errors.New("invalid key handle type for bindings")
		}
		return v.hive.NodeNrChildren(node), nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return 0, errors.New("invalid key handle type for reader")
		}
		return v.reader.KeySubkeyCount(nodeID)

	case BackendNone, BackendHivexsh:
		return 0, errors.New("backend does not support GetSubkeyCount")

	default:
		return 0, errors.New("backend does not support GetSubkeyCount")
	}
}

// GetParent returns the parent key.
//
// Returns error if key is root (has no parent).
func (v *Validator) GetParent(key interface{}) (interface{}, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return nil, errors.New("invalid key handle type for bindings")
		}
		parent := v.hive.NodeParent(node)
		if parent == 0 {
			return nil, errors.New("key has no parent (is root)")
		}
		return parent, nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return nil, errors.New("invalid key handle type for reader")
		}
		return v.reader.Parent(nodeID)

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support GetParent")

	default:
		return nil, errors.New("backend does not support GetParent")
	}
}

// CountKeys recursively counts all keys in the hive.
//
// Example:
//
//	count, err := v.CountKeys()
//	require.NoError(t, err)
//	require.Equal(t, 42, count)
func (v *Validator) CountKeys() (int, error) {
	keys, _, err := v.CountTree()
	return keys, err
}

// CountValues recursively counts all values in the hive.
//
// Example:
//
//	count, err := v.CountValues()
//	require.NoError(t, err)
//	require.Equal(t, 100, count)
func (v *Validator) CountValues() (int, error) {
	_, values, err := v.CountTree()
	return values, err
}

// CountTree returns both key and value counts.
//
// Example:
//
//	keys, values, err := v.CountTree()
//	require.NoError(t, err)
//	t.Logf("Hive has %d keys, %d values", keys, values)
func (v *Validator) CountTree() (int, int, error) {
	root, err := v.Root()
	if err != nil {
		return 0, 0, err
	}

	return v.countNode(root)
}

// countNode recursively counts keys and values from a given node.
func (v *Validator) countNode(node interface{}) (int, int, error) {
	// Count this key
	keys := 1

	// Count values at this key
	values := 0
	valCount, err := v.GetValueCount(node)
	if err == nil {
		values += valCount
	}

	// Recursively count children
	children, err := v.GetSubkeys(node)
	if err != nil {
		return keys, values, nil //nolint:nilerr // No children is OK for counting
	}

	for _, child := range children {
		childKeys, childValues, err := v.countNode(child)
		if err != nil {
			return keys, values, err
		}
		keys += childKeys
		values += childValues
	}

	return keys, values, nil
}

// WalkTree performs recursive traversal with a callback.
//
// The callback is invoked for each key (with isValue=false) and
// each value (with isValue=true).
//
// Example:
//
//	err := v.WalkTree(func(path string, depth int, isValue bool) error {
//	    if isValue {
//	        t.Logf("[VALUE] %s", path)
//	    } else {
//	        t.Logf("[KEY] %s (depth: %d)", path, depth)
//	    }
//	    return nil
//	})
func (v *Validator) WalkTree(fn func(path string, depth int, isValue bool) error) error {
	root, err := v.Root()
	if err != nil {
		return err
	}

	return v.walkNode(root, "", 0, fn)
}

// walkNode recursively walks a node and its children.
func (v *Validator) walkNode(node interface{}, currentPath string, depth int, fn func(string, int, bool) error) error {
	// Get node name
	name, err := v.GetKeyName(node)
	if err != nil {
		return err
	}

	// Build path
	var nodePath string
	if currentPath == "" {
		// Root node
		nodePath = "\\"
	} else {
		nodePath = currentPath + "\\" + name
	}

	// Invoke callback for this key
	if err := fn(nodePath, depth, false); err != nil {
		return err
	}

	// Walk values
	values, err := v.GetValues(node)
	if err == nil {
		for _, val := range values {
			valName, err := v.GetValueName(val)
			if err != nil {
				continue
			}
			valuePath := nodePath + "\\" + valName
			if err := fn(valuePath, depth, true); err != nil {
				return err
			}
		}
	}

	// Walk children
	children, err := v.GetSubkeys(node)
	if err != nil {
		return nil //nolint:nilerr // No children is OK for walking
	}

	for _, child := range children {
		if err := v.walkNode(child, nodePath, depth+1, fn); err != nil {
			return err
		}
	}

	return nil
}

// getChild finds a child key by name (case-insensitive).
func (v *Validator) getChild(parent interface{}, name string) (interface{}, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := parent.(bindings.NodeHandle)
		if !ok {
			return nil, errors.New("invalid key handle type for bindings")
		}
		child := v.hive.NodeGetChild(node, name)
		if child == 0 {
			return nil, fmt.Errorf("child '%s' not found", name)
		}
		return child, nil

	case BackendReader:
		nodeID, ok := parent.(types.NodeID)
		if !ok {
			return nil, errors.New("invalid key handle type for reader")
		}
		return v.reader.GetChild(nodeID, name)

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support getChild")

	default:
		return nil, errors.New("backend does not support getChild")
	}
}
