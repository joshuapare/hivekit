package hive

import (
	"fmt"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Reader returns a types.Reader for this Hive.
//
// The Reader provides high-level ergonomic access to keys and values using
// path-based lookups, value decoders, and tree traversal.
//
// The Reader uses zero-copy access to the Hive's memory-mapped data, so it's
// very efficient. The Reader remains valid until the Hive is closed.
//
// Example:
//
//	h, _ := hive.Open("SOFTWARE")
//	defer h.Close()
//	r, _ := h.Reader()
//	node, _ := r.Find("Microsoft\\Windows\\CurrentVersion")
//	meta, _ := r.StatKey(node)
//	fmt.Println("Key:", meta.Name)
func (h *Hive) Reader() (types.Reader, error) {
	if h == nil {
		return nil, fmt.Errorf("hive: nil hive")
	}
	if h.data == nil {
		return nil, fmt.Errorf("hive: uninitialized data")
	}

	// Use zero-copy since the Reader will reference the same mmap
	return reader.OpenBytes(h.data, types.OpenOptions{
		ZeroCopy: true,
	})
}

// Find returns the NodeID for a registry key path.
//
// Path Format:
//   - Separator: backslash '\' or forward slash '/' (both work)
//   - Case-insensitive: "Software" matches "SOFTWARE" or "software"
//   - Root prefixes automatically stripped: HKLM\, HKEY_LOCAL_MACHINE\, etc.
//   - Empty string or "\" or "/" returns the root key
//   - Leading/trailing slashes are ignored
//
// Example:
//
//	node, err := h.Find("Software\\Microsoft\\Windows")
//	node, err := h.Find("Software/Microsoft/Windows")     // forward slash works too
//	node, err := h.Find("HKLM\\Software\\Microsoft")      // HKLM prefix stripped
//	node, err := h.Find("")                               // returns root
//
// For more control or when you already have path parts, use FindParts().
func (h *Hive) Find(path string) (types.NodeID, error) {
	r, err := h.Reader()
	if err != nil {
		return 0, err
	}
	return r.Find(path)
}

// FindParts returns the NodeID for a registry key specified as path parts.
//
// This is useful when you already have the key path split into components,
// avoiding the need to join and re-parse them. The lookup is case-insensitive.
//
// Pass nil or an empty slice to get the root key.
//
// Example:
//
//	// Navigate to Software\Microsoft\Windows
//	node, err := h.FindParts([]string{"Software", "Microsoft", "Windows"})
//
//	// Get root
//	node, err := h.FindParts(nil)
//	node, err := h.FindParts([]string{})
func (h *Hive) FindParts(parts []string) (types.NodeID, error) {
	r, err := h.Reader()
	if err != nil {
		return 0, err
	}
	return r.FindParts(parts)
}

// GetKey returns metadata for a key at the given path.
//
// The returned KeyMeta includes the key name, subkey count, value count,
// and last write timestamp.
//
// Example:
//
//	meta, err := h.GetKey("Software\\Microsoft\\Windows\\CurrentVersion")
//	fmt.Printf("%s: %d subkeys, %d values\n", meta.Name, meta.SubkeyCount, meta.ValueCount)
func (h *Hive) GetKey(path string) (types.KeyMeta, error) {
	r, err := h.Reader()
	if err != nil {
		return types.KeyMeta{}, err
	}
	node, err := r.Find(path)
	if err != nil {
		return types.KeyMeta{}, err
	}
	return r.StatKey(node)
}

// ListSubkeys returns metadata for all direct children of a key.
//
// This is not recursive - it only returns immediate children.
// Use Walk() for recursive traversal.
//
// Example:
//
//	keys, err := h.ListSubkeys("Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall")
//	for _, key := range keys {
//	    fmt.Printf("- %s (last write: %s)\n", key.Name, key.LastWrite)
//	}
func (h *Hive) ListSubkeys(path string) ([]types.KeyMeta, error) {
	r, err := h.Reader()
	if err != nil {
		return nil, err
	}
	node, err := r.Find(path)
	if err != nil {
		return nil, err
	}
	children, err := r.Subkeys(node)
	if err != nil {
		return nil, err
	}

	result := make([]types.KeyMeta, 0, len(children))
	for _, child := range children {
		meta, err := r.StatKey(child)
		if err != nil {
			continue // Skip corrupted keys
		}
		result = append(result, meta)
	}
	return result, nil
}

// GetValue returns metadata and raw bytes for a value.
//
// For type-specific decoding, use GetString(), GetDWORD(), GetQWORD(), etc.
//
// Example:
//
//	meta, data, err := h.GetValue("Software\\MyApp", "Version")
//	fmt.Printf("%s (%s): %d bytes\n", meta.Name, meta.Type, len(data))
func (h *Hive) GetValue(keyPath, valueName string) (*types.ValueMeta, []byte, error) {
	r, err := h.Reader()
	if err != nil {
		return nil, nil, err
	}
	node, err := r.Find(keyPath)
	if err != nil {
		return nil, nil, err
	}
	valID, err := r.GetValue(node, valueName)
	if err != nil {
		return nil, nil, err
	}
	meta, err := r.StatValue(valID)
	if err != nil {
		return nil, nil, err
	}
	data, err := r.ValueBytes(valID, types.ReadOptions{CopyData: true})
	if err != nil {
		return nil, nil, err
	}
	return &meta, data, nil
}

// GetString reads a REG_SZ or REG_EXPAND_SZ value and decodes it to a Go string.
//
// The value is automatically decoded from UTF-16LE to UTF-8.
// For REG_EXPAND_SZ, environment variables are NOT expanded.
//
// Example:
//
//	version, err := h.GetString("Microsoft\\Windows NT\\CurrentVersion", "ProductName")
//	fmt.Println("Windows:", version)
func (h *Hive) GetString(keyPath, valueName string) (string, error) {
	r, err := h.Reader()
	if err != nil {
		return "", err
	}
	node, err := r.Find(keyPath)
	if err != nil {
		return "", err
	}
	valID, err := r.GetValue(node, valueName)
	if err != nil {
		return "", err
	}
	return r.ValueString(valID, types.ReadOptions{})
}

// GetDWORD reads a REG_DWORD or REG_DWORD_BIG_ENDIAN value.
//
// Endianness is handled automatically based on the value type.
//
// Example:
//
//	timeout, err := h.GetDWORD("Software\\MyApp", "TimeoutSeconds")
//	fmt.Printf("Timeout: %d seconds\n", timeout)
func (h *Hive) GetDWORD(keyPath, valueName string) (uint32, error) {
	r, err := h.Reader()
	if err != nil {
		return 0, err
	}
	node, err := r.Find(keyPath)
	if err != nil {
		return 0, err
	}
	valID, err := r.GetValue(node, valueName)
	if err != nil {
		return 0, err
	}
	return r.ValueDWORD(valID)
}

// GetQWORD reads a REG_QWORD (64-bit little-endian integer) value.
//
// Example:
//
//	size, err := h.GetQWORD("Software\\MyApp", "MaxFileSize")
//	fmt.Printf("Max size: %d bytes\n", size)
func (h *Hive) GetQWORD(keyPath, valueName string) (uint64, error) {
	r, err := h.Reader()
	if err != nil {
		return 0, err
	}
	node, err := r.Find(keyPath)
	if err != nil {
		return 0, err
	}
	valID, err := r.GetValue(node, valueName)
	if err != nil {
		return 0, err
	}
	return r.ValueQWORD(valID)
}

// GetMultiString reads a REG_MULTI_SZ value and returns the array of strings.
//
// The value is automatically decoded from UTF-16LE to UTF-8.
// Double-null termination is handled automatically.
//
// Example:
//
//	paths, err := h.GetMultiString("System\\CurrentControlSet\\Control\\Session Manager\\Environment", "Path")
//	for _, p := range paths {
//	    fmt.Println("Path entry:", p)
//	}
func (h *Hive) GetMultiString(keyPath, valueName string) ([]string, error) {
	r, err := h.Reader()
	if err != nil {
		return nil, err
	}
	node, err := r.Find(keyPath)
	if err != nil {
		return nil, err
	}
	valID, err := r.GetValue(node, valueName)
	if err != nil {
		return nil, err
	}
	return r.ValueStrings(valID, types.ReadOptions{})
}

// Walk performs a pre-order tree traversal starting at the given path.
//
// The callback function is called for each key in the tree. Return an error
// from the callback to stop traversal.
//
// Example:
//
//	err := h.Walk("Software\\Microsoft", func(id types.NodeID, meta types.KeyMeta) error {
//	    if strings.Contains(meta.Name, "Windows") {
//	        fmt.Println("Found Windows key:", meta.Name)
//	    }
//	    return nil
//	})
func (h *Hive) Walk(path string, fn func(types.NodeID, types.KeyMeta) error) error {
	r, err := h.Reader()
	if err != nil {
		return err
	}
	node, err := r.Find(path)
	if err != nil {
		return err
	}
	return r.Walk(node, func(id types.NodeID) error {
		meta, err := r.StatKey(id)
		if err != nil {
			return err
		}
		return fn(id, meta)
	})
}

// ListValues returns metadata for all values in a key.
//
// Example:
//
//	values, err := h.ListValues("Software\\MyApp")
//	for _, val := range values {
//	    fmt.Printf("- %s (%s): %d bytes\n", val.Name, val.Type, val.Size)
//	}
func (h *Hive) ListValues(keyPath string) ([]types.ValueMeta, error) {
	r, err := h.Reader()
	if err != nil {
		return nil, err
	}
	node, err := r.Find(keyPath)
	if err != nil {
		return nil, err
	}
	valueIDs, err := r.Values(node)
	if err != nil {
		return nil, err
	}

	result := make([]types.ValueMeta, 0, len(valueIDs))
	for _, valID := range valueIDs {
		meta, err := r.StatValue(valID)
		if err != nil {
			continue // Skip corrupted values
		}
		result = append(result, meta)
	}
	return result, nil
}
