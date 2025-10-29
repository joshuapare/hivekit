// Package bindings provides a Go-idiomatic wrapper around the c-for-go generated hivex bindings.
//
// The generated bindings use awkward types ([]HiveH instead of *Hive), so this wrapper
// provides a cleaner API with methods on structs.
package bindings

import (
	"fmt"
	"unsafe"

	"github.com/joshuapare/hivekit/bindings/hivex"
)

// Hive represents an open Windows Registry hive file.
type Hive struct {
	handle []hivex.HiveH // Awkward c-for-go representation of hive_h*
	path   string
}

// sliceHeader is the runtime representation of a slice
type sliceHeader struct {
	Data uintptr
	Len  int
	Cap  int
}

// Open opens a hive file for reading.
func Open(path string, flags int) (hive *Hive, err error) {
	// Use defer/recover to catch segfault from NULL pointer in generated code
	defer func() {
		if r := recover(); r != nil {
			// NULL pointer was dereferenced in generated code
			hive = nil
			err = fmt.Errorf("hivex_open failed for %s: %v", path, r)
		}
	}()

	handle := hivex.HivexOpen(path, int32(flags))

	// Check if the underlying Data pointer is nil
	// The generated code returns a slice with length 0, but Data may be nil on failure
	if handle == nil || (*sliceHeader)(unsafe.Pointer(&handle)).Data == 0 {
		return nil, fmt.Errorf("hivex_open failed for %s", path)
	}

	return &Hive{
		handle: handle,
		path:   path,
	}, nil
}

// Close closes the hive and releases resources.
func (h *Hive) Close() error {
	if h.handle != nil {
		ret := hivex.HivexClose(h.handle)
		h.handle = nil
		if ret != 0 {
			return fmt.Errorf("hivex_close failed: %d", ret)
		}
	}
	return nil
}

// Root returns the root node of the hive.
func (h *Hive) Root() NodeHandle {
	return NodeHandle(hivex.HivexRoot(h.handle))
}

// LastModified returns the last modification timestamp of the hive.
func (h *Hive) LastModified() int64 {
	return hivex.HivexLastModified(h.handle)
}

// NodeHandle represents a handle to a registry key (node).
type NodeHandle hivex.HiveNodeH

// ValueHandle represents a handle to a registry value.
type ValueHandle hivex.HiveValueH

// NodeName returns the name of a node.
func (h *Hive) NodeName(node NodeHandle) string {
	// Get the length first
	length := hivex.HivexNodeNameLen(h.handle, hivex.HiveNodeH(node))
	if length == 0 {
		return ""
	}

	// Get the name pointer (returns slice with length 0 but valid Data pointer)
	namePtr := hivex.HivexNodeName(h.handle, hivex.HiveNodeH(node))
	if namePtr == nil {
		return ""
	}

	// Get the Data pointer from the slice header
	dataPtr := (*sliceHeader)(unsafe.Pointer(&namePtr)).Data
	if dataPtr == 0 {
		return ""
	}

	// Create slice with correct length using the Data pointer
	nameBytes := (*(*[0x7fffffff]byte)(unsafe.Pointer(dataPtr)))[:length]
	return string(nameBytes)
}

// NodeChildren returns all child nodes of a node.
func (h *Hive) NodeChildren(node NodeHandle) []NodeHandle {
	// Get the count first
	count := hivex.HivexNodeNrChildren(h.handle, hivex.HiveNodeH(node))
	if count == 0 {
		return nil
	}

	// Get the children pointer (returns slice with length 0 but valid Data pointer)
	childrenPtr := hivex.HivexNodeChildren(h.handle, hivex.HiveNodeH(node))
	if childrenPtr == nil {
		return nil
	}

	// Get the Data pointer from the slice header
	dataPtr := (*sliceHeader)(unsafe.Pointer(&childrenPtr)).Data
	if dataPtr == 0 {
		return nil
	}

	// Create slice with correct length using the Data pointer
	childrenSlice := (*(*[0x7fffffff]hivex.HiveNodeH)(unsafe.Pointer(dataPtr)))[:count]

	// Convert to NodeHandle slice
	result := make([]NodeHandle, count)
	for i := uint64(0); i < count; i++ {
		result[i] = NodeHandle(childrenSlice[i])
	}
	return result
}

// NodeGetChild finds a child node by name (case-insensitive).
func (h *Hive) NodeGetChild(node NodeHandle, name string) NodeHandle {
	return NodeHandle(hivex.HivexNodeGetChild(h.handle, hivex.HiveNodeH(node), name))
}

// NodeNrChildren returns the number of child nodes.
func (h *Hive) NodeNrChildren(node NodeHandle) int {
	return int(hivex.HivexNodeNrChildren(h.handle, hivex.HiveNodeH(node)))
}

// NodeParent returns the parent node.
func (h *Hive) NodeParent(node NodeHandle) NodeHandle {
	return NodeHandle(hivex.HivexNodeParent(h.handle, hivex.HiveNodeH(node)))
}

// NodeTimestamp returns the last write timestamp of a node.
func (h *Hive) NodeTimestamp(node NodeHandle) int64 {
	return hivex.HivexNodeTimestamp(h.handle, hivex.HiveNodeH(node))
}

// NodeValues returns all values of a node.
func (h *Hive) NodeValues(node NodeHandle) []ValueHandle {
	// Get the count first
	count := hivex.HivexNodeNrValues(h.handle, hivex.HiveNodeH(node))
	if count == 0 {
		return nil
	}

	// Get the values pointer (returns slice with length 0 but valid Data pointer)
	valuesPtr := hivex.HivexNodeValues(h.handle, hivex.HiveNodeH(node))
	if valuesPtr == nil {
		return nil
	}

	// Get the Data pointer from the slice header
	dataPtr := (*sliceHeader)(unsafe.Pointer(&valuesPtr)).Data
	if dataPtr == 0 {
		return nil
	}

	// Create slice with correct length using the Data pointer
	valuesSlice := (*(*[0x7fffffff]hivex.HiveValueH)(unsafe.Pointer(dataPtr)))[:count]

	// Convert to ValueHandle slice
	result := make([]ValueHandle, count)
	for i := uint64(0); i < count; i++ {
		result[i] = ValueHandle(valuesSlice[i])
	}
	return result
}

// NodeGetValue finds a value by name (case-insensitive).
func (h *Hive) NodeGetValue(node NodeHandle, key string) ValueHandle {
	return ValueHandle(hivex.HivexNodeGetValue(h.handle, hivex.HiveNodeH(node), key))
}

// NodeNrValues returns the number of values.
func (h *Hive) NodeNrValues(node NodeHandle) int {
	return int(hivex.HivexNodeNrValues(h.handle, hivex.HiveNodeH(node)))
}

// ValueKey returns the name of a value.
func (h *Hive) ValueKey(val ValueHandle) string {
	// Get the length first
	length := hivex.HivexValueKeyLen(h.handle, hivex.HiveValueH(val))
	if length == 0 {
		return ""
	}

	// Get the key pointer (returns slice with length 0 but valid Data pointer)
	keyPtr := hivex.HivexValueKey(h.handle, hivex.HiveValueH(val))
	if keyPtr == nil {
		return ""
	}

	// Get the Data pointer from the slice header
	dataPtr := (*sliceHeader)(unsafe.Pointer(&keyPtr)).Data
	if dataPtr == 0 {
		return ""
	}

	// Create slice with correct length using the Data pointer
	keyBytes := (*(*[0x7fffffff]byte)(unsafe.Pointer(dataPtr)))[:length]
	return string(keyBytes)
}

// ValueType returns the type and length of a value.
func (h *Hive) ValueType(val ValueHandle) (ValueType, int, error) {
	// Create slices with one element each for output parameters
	vtypeSlice := make([]hivex.HiveType, 1)
	lengthSlice := make([]uint64, 1)

	ret := hivex.HivexValueType(h.handle, hivex.HiveValueH(val), vtypeSlice, lengthSlice)
	if ret != 0 {
		return 0, 0, fmt.Errorf("hivex_value_type failed: %d", ret)
	}

	// Read back from the slices after C function writes to them
	return ValueType(vtypeSlice[0]), int(lengthSlice[0]), nil
}

// ValueValue returns the raw value data.
func (h *Hive) ValueValue(val ValueHandle) ([]byte, ValueType, error) {
	// Create slices with one element each for output parameters
	vtypeSlice := make([]hivex.HiveType, 1)
	lengthSlice := make([]uint64, 1)

	// The generated binding returns a [:0] slice, but we need the actual data
	dataSlice := hivex.HivexValueValue(h.handle, hivex.HiveValueH(val), vtypeSlice, lengthSlice)

	// Read back from the slices after C function writes to them
	vtype := vtypeSlice[0]
	length := lengthSlice[0]

	// The generated binding creates a [:0] slice, so we need to reconstruct it
	// Get the Data pointer from the slice header
	dataPtr := (*sliceHeader)(unsafe.Pointer(&dataSlice)).Data
	if dataPtr == 0 || length == 0 {
		return nil, ValueType(vtype), nil
	}

	// Create slice with correct length using the Data pointer
	data := (*(*[0x7fffffff]byte)(unsafe.Pointer(dataPtr)))[:length]

	return data, ValueType(vtype), nil
}

// ValueDword returns a DWORD value.
func (h *Hive) ValueDword(val ValueHandle) (int32, error) {
	dword := hivex.HivexValueDword(h.handle, hivex.HiveValueH(val))
	return dword, nil
}

// ValueQword returns a QWORD value.
func (h *Hive) ValueQword(val ValueHandle) (int64, error) {
	qword := hivex.HivexValueQword(h.handle, hivex.HiveValueH(val))
	return qword, nil
}

// ValueString returns a string value (REG_SZ or REG_EXPAND_SZ).
func (h *Hive) ValueString(val ValueHandle) (string, error) {
	strBytes := hivex.HivexValueString(h.handle, hivex.HiveValueH(val))
	// Find null terminator
	for i, b := range strBytes {
		if b == 0 {
			return string(strBytes[:i]), nil
		}
	}
	return string(strBytes), nil
}

// ValueMultipleStrings returns a multi-string value (REG_MULTI_SZ).
func (h *Hive) ValueMultipleStrings(val ValueHandle) ([]string, error) {
	stringsData := hivex.HivexValueMultipleStrings(h.handle, hivex.HiveValueH(val))
	result := make([]string, len(stringsData))
	for i, strBytes := range stringsData {
		// Find null terminator
		for j, b := range strBytes {
			if b == 0 {
				result[i] = string(strBytes[:j])
				break
			}
		}
		if result[i] == "" {
			result[i] = string(strBytes)
		}
	}
	return result, nil
}

// ValueType represents a Windows Registry value type.
type ValueType int32

// Registry value type constants.
const (
	REG_NONE                       ValueType = 0
	REG_SZ                         ValueType = 1
	REG_EXPAND_SZ                  ValueType = 2
	REG_BINARY                     ValueType = 3
	REG_DWORD                      ValueType = 4
	REG_DWORD_BIG_ENDIAN          ValueType = 5
	REG_LINK                       ValueType = 6
	REG_MULTI_SZ                   ValueType = 7
	REG_RESOURCE_LIST             ValueType = 8
	REG_FULL_RESOURCE_DESCRIPTOR  ValueType = 9
	REG_RESOURCE_REQUIREMENTS_LIST ValueType = 10
	REG_QWORD                      ValueType = 11
)

// String returns the name of the value type.
func (t ValueType) String() string {
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
	case REG_DWORD_BIG_ENDIAN:
		return "REG_DWORD_BIG_ENDIAN"
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
		return fmt.Sprintf("UNKNOWN_TYPE_%d", t)
	}
}

// NodeNameLen returns the byte length of a node's name without decoding.
func (h *Hive) NodeNameLen(node NodeHandle) uint64 {
	return hivex.HivexNodeNameLen(h.handle, hivex.HiveNodeH(node))
}

// ValueKeyLen returns the byte length of a value's name without decoding.
func (h *Hive) ValueKeyLen(val ValueHandle) uint64 {
	return hivex.HivexValueKeyLen(h.handle, hivex.HiveValueH(val))
}

// NodeStructLength returns the size of the NK record structure in bytes.
func (h *Hive) NodeStructLength(node NodeHandle) uint64 {
	return hivex.HivexNodeStructLength(h.handle, hivex.HiveNodeH(node))
}

// ValueStructLength returns the size of the VK record structure in bytes.
func (h *Hive) ValueStructLength(val ValueHandle) uint64 {
	return hivex.HivexValueStructLength(h.handle, hivex.HiveValueH(val))
}

// ValueDataCellOffset returns the file offset and length of the data cell for a value.
// Returns (offset, length).
func (h *Hive) ValueDataCellOffset(val ValueHandle) (uint64, uint64) {
	var length uint64
	offset := hivex.HivexValueDataCellOffset(h.handle, hivex.HiveValueH(val), []uint64{length})
	// The length is returned via the pointer, but c-for-go doesn't handle this well
	// For now, we return 0 for length and let the caller use ValueType to get the size
	// This is a limitation of the binding generation
	return uint64(offset), length
}

// SetValue represents a registry value to be written.
type SetValue struct {
	Key   string
	Type  ValueType
	Value []byte
}

// Commit writes any changes to the hive file.
// This must be called after making modifications with NodeAddChild, NodeSetValue, etc.
func (h *Hive) Commit(filename string) error {
	if h.handle == nil {
		return fmt.Errorf("hive is closed")
	}
	ret := hivex.HivexCommit(h.handle, filename, 0)
	if ret != 0 {
		return fmt.Errorf("hivex_commit failed: %d", ret)
	}
	return nil
}

// NodeAddChild creates a new child key under the given parent node.
func (h *Hive) NodeAddChild(parent NodeHandle, name string) (NodeHandle, error) {
	if h.handle == nil {
		return 0, fmt.Errorf("hive is closed")
	}
	node := hivex.HivexNodeAddChild(h.handle, hivex.HiveNodeH(parent), name)
	if node == 0 {
		return 0, fmt.Errorf("hivex_node_add_child failed")
	}
	return NodeHandle(node), nil
}

// NodeDeleteChild deletes the given node from the hive.
func (h *Hive) NodeDeleteChild(node NodeHandle) error {
	if h.handle == nil {
		return fmt.Errorf("hive is closed")
	}
	ret := hivex.HivexNodeDeleteChild(h.handle, hivex.HiveNodeH(node))
	if ret != 0 {
		return fmt.Errorf("hivex_node_delete_child failed: %d", ret)
	}
	return nil
}

// NodeSetValue sets a single value on a node.
func (h *Hive) NodeSetValue(node NodeHandle, key string, valueType ValueType, data []byte) error {
	if h.handle == nil {
		return fmt.Errorf("hive is closed")
	}

	// Create HiveSetValue struct for the C binding
	setValue := hivex.HiveSetValue{
		Key:   []byte(key),
		T:     hivex.HiveType(valueType),
		Len:   uint64(len(data)),
		Value: data,
	}

	ret := hivex.HivexNodeSetValue(h.handle, hivex.HiveNodeH(node), []hivex.HiveSetValue{setValue}, 0)
	if ret != 0 {
		return fmt.Errorf("hivex_node_set_value failed: %d", ret)
	}
	return nil
}

// NodeSetValues sets multiple values on a node in a single operation.
func (h *Hive) NodeSetValues(node NodeHandle, values []SetValue) error {
	if h.handle == nil {
		return fmt.Errorf("hive is closed")
	}

	// Convert SetValue slice to HiveSetValue slice for C binding
	hiveValues := make([]hivex.HiveSetValue, len(values))
	for i, v := range values {
		// Null-terminate the key for C API (hivex expects null-terminated strings)
		keyBytes := make([]byte, len(v.Key)+1)
		copy(keyBytes, v.Key)
		// keyBytes[len(v.Key)] is already 0 from make()

		hiveValues[i] = hivex.HiveSetValue{
			Key:   keyBytes,
			T:     hivex.HiveType(v.Type),
			Len:   uint64(len(v.Value)),
			Value: v.Value,
		}
	}

	ret := hivex.HivexNodeSetValues(h.handle, hivex.HiveNodeH(node), uint64(len(hiveValues)), hiveValues, 0)
	if ret != 0 {
		return fmt.Errorf("hivex_node_set_values failed: %d", ret)
	}
	return nil
}

// Open flags.
const (
	OPEN_VERBOSE = 1 << 0
	OPEN_DEBUG   = 1 << 1
	OPEN_WRITE   = 1 << 2
	OPEN_UNSAFE  = 1 << 3 // Disable safety checks
)

// Suppress unused import warning
var _ = unsafe.Sizeof(0)
