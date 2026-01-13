package hive

import (
	"fmt"
	"sort"
	"time"

	"github.com/joshuapare/hivekit/internal/reader"
)

// KeyInfo contains information about a registry key.
type KeyInfo struct {
	Name      string
	SubkeyN   int
	ValueN    int
	Path      string
	LastWrite time.Time
}

// ValueInfo contains information about a registry value.
type ValueInfo struct {
	Name       string
	Type       string
	Size       int
	Data       []byte
	StringVal  string   // For REG_SZ/REG_EXPAND_SZ
	StringVals []string // For REG_MULTI_SZ
	DWordVal   uint32   // For REG_DWORD
	QWordVal   uint64   // For REG_QWORD
}

// GetHiveInfo returns metadata from the hive header.
func GetHiveInfo(hivePath string) (HiveInfo, error) {
	// Open the hive
	r, err := reader.Open(hivePath, OpenOptions{})
	if err != nil {
		return HiveInfo{}, fmt.Errorf("failed to open hive: %w", err)
	}
	defer r.Close()

	return r.Info(), nil
}

// GetKeyDetail returns detailed NK record information for a key path.
func GetKeyDetail(hivePath, keyPath string) (KeyDetail, error) {
	r, err := reader.Open(hivePath, OpenOptions{})
	if err != nil {
		return KeyDetail{}, fmt.Errorf("failed to open hive: %w", err)
	}
	defer r.Close()

	return GetKeyDetailWithReader(r, keyPath)
}

// GetKeyDetailWithReader returns detailed NK record information using an existing reader
// This is more efficient when making multiple calls to the same hive.
func GetKeyDetailWithReader(r Reader, keyPath string) (KeyDetail, error) {
	// Navigate to key
	var node NodeID
	var err error
	if keyPath == "" {
		node, err = r.Root()
	} else {
		node, err = r.Find(keyPath)
	}
	if err != nil {
		return KeyDetail{}, fmt.Errorf("failed to find key %q: %w", keyPath, err)
	}

	return r.DetailKey(node)
}

// ListKeys lists all keys at the specified path in the
// If path is empty, lists keys at the root.
// If recursive is true, lists all subkeys recursively up to maxDepth.
func ListKeys(hivePath string, keyPath string, recursive bool, maxDepth int) ([]KeyInfo, error) {
	// Open the hive
	r, err := reader.Open(hivePath, OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open hive: %w", err)
	}
	defer r.Close()

	// Get root node
	root, err := r.Root()
	if err != nil {
		return nil, fmt.Errorf("failed to get root: %w", err)
	}

	// Navigate to the specified path if provided
	currentNode := root
	if keyPath != "" {
		currentNode, err = r.Find(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to find path: %w", err)
		}
	}

	// List keys
	// Pass keyPath as parentPath so children get full paths
	keys, err := listKeysRecursive(r, currentNode, keyPath, recursive, maxDepth, 0)
	if err != nil {
		return nil, err
	}

	// Sort by path
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Path < keys[j].Path
	})

	return keys, nil
}

func listKeysRecursive(
	r Reader,
	node NodeID,
	parentPath string,
	recursive bool,
	maxDepth int,
	currentDepth int,
) ([]KeyInfo, error) {
	// Get subkeys
	children, err := r.Subkeys(node)
	if err != nil {
		return nil, err
	}

	// Pre-allocate for at least the direct children count
	keys := make([]KeyInfo, 0, len(children))

	for _, child := range children {
		meta, statErr := r.StatKey(child)
		if statErr != nil {
			continue
		}

		childPath := meta.Name
		if parentPath != "" {
			childPath = parentPath + "\\" + meta.Name
		}

		keyInfo := KeyInfo{
			Name:      meta.Name,
			SubkeyN:   meta.SubkeyN,
			ValueN:    meta.ValueN,
			Path:      childPath,
			LastWrite: meta.LastWrite,
		}
		keys = append(keys, keyInfo)

		// Recurse if requested and within depth limit
		if recursive && (maxDepth == 0 || currentDepth < maxDepth-1) {
			childKeys, recurseErr := listKeysRecursive(r, child, childPath, true, maxDepth, currentDepth+1)
			if recurseErr != nil {
				continue
			}
			keys = append(keys, childKeys...)
		}
	}

	return keys, nil
}

// ListValues lists all values at the specified key path in the.
func ListValues(hivePath string, keyPath string) ([]ValueInfo, error) {
	// Open the hive
	r, err := reader.Open(hivePath, OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open hive: %w", err)
	}
	defer r.Close()

	return ListValuesWithReader(r, keyPath)
}

// ListValuesWithReader lists all values for a specific key path using an existing reader
// This is more efficient when making multiple calls to the same hive.
func ListValuesWithReader(r Reader, keyPath string) ([]ValueInfo, error) {
	// Get root node
	root, err := r.Root()
	if err != nil {
		return nil, fmt.Errorf("failed to get root: %w", err)
	}

	// Navigate to the key
	node := root
	if keyPath != "" {
		node, err = r.Find(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to find path: %w", err)
		}
	}

	// Get all values
	valueIDs, err := r.Values(node)
	if err != nil {
		return nil, fmt.Errorf("failed to get values: %w", err)
	}

	// Pre-allocate for the known number of values
	values := make([]ValueInfo, 0, len(valueIDs))
	for _, valueID := range valueIDs {
		meta, statErr := r.StatValue(valueID)
		if statErr != nil {
			continue
		}

		valueInfo := ValueInfo{
			Name: meta.Name,
			Type: meta.Type.String(),
			Size: meta.Size,
		}

		// Get raw data
		data, readErr := r.ValueBytes(valueID, ReadOptions{})
		if readErr == nil {
			valueInfo.Data = data
		}

		// Decode based on type
		switch meta.Type {
		case REG_SZ, REG_EXPAND_SZ:
			if val, stringErr := r.ValueString(valueID, ReadOptions{}); stringErr == nil {
				valueInfo.StringVal = val
			}
		case REG_DWORD, REG_DWORD_BE:
			if val, dwordErr := r.ValueDWORD(valueID); dwordErr == nil {
				valueInfo.DWordVal = val
			}
		case REG_QWORD:
			if val, qwordErr := r.ValueQWORD(valueID); qwordErr == nil {
				valueInfo.QWordVal = val
			}
		case REG_MULTI_SZ:
			if val, stringsErr := r.ValueStrings(valueID, ReadOptions{}); stringsErr == nil {
				valueInfo.StringVals = val
			}
		case REG_NONE, REG_BINARY, REG_LINK:
			// Binary data already populated in Data field above
		}

		values = append(values, valueInfo)
	}

	return values, nil
}

// GetValue retrieves a specific value from a key in the.
func GetValue(hivePath string, keyPath string, valueName string) (*ValueInfo, error) {
	// Open the hive
	r, err := reader.Open(hivePath, OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open hive: %w", err)
	}
	defer r.Close()

	// Get root node
	root, err := r.Root()
	if err != nil {
		return nil, fmt.Errorf("failed to get root: %w", err)
	}

	// Navigate to the key
	node := root
	if keyPath != "" {
		node, err = r.Find(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to find path: %w", err)
		}
	}

	// Get the value
	valueID, err := r.GetValue(node, valueName)
	if err != nil {
		return nil, fmt.Errorf("value not found: %w", err)
	}

	// Get value metadata
	meta, err := r.StatValue(valueID)
	if err != nil {
		return nil, fmt.Errorf("failed to get value metadata: %w", err)
	}

	valueInfo := &ValueInfo{
		Name: meta.Name,
		Type: meta.Type.String(),
		Size: meta.Size,
	}

	// Get raw data
	data, readErr := r.ValueBytes(valueID, ReadOptions{})
	if readErr == nil {
		valueInfo.Data = data
	}

	// Decode based on type
	switch meta.Type {
	case REG_SZ, REG_EXPAND_SZ:
		if val, stringErr := r.ValueString(valueID, ReadOptions{}); stringErr == nil {
			valueInfo.StringVal = val
		}
	case REG_DWORD, REG_DWORD_BE:
		if val, dwordErr := r.ValueDWORD(valueID); dwordErr == nil {
			valueInfo.DWordVal = val
		}
	case REG_QWORD:
		if val, qwordErr := r.ValueQWORD(valueID); qwordErr == nil {
			valueInfo.QWordVal = val
		}
	case REG_MULTI_SZ:
		if val, stringsErr := r.ValueStrings(valueID, ReadOptions{}); stringsErr == nil {
			valueInfo.StringVals = val
		}
	case REG_NONE, REG_BINARY, REG_LINK:
		// Binary data already populated in Data field above
	}

	return valueInfo, nil
}
