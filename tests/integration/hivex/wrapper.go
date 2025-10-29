// +build hivex

package integration

import (
	"fmt"

	"github.com/joshuapare/hivekit/bindings"
)

// HivexHandle wraps our own hivex bindings with error-returning interface
// compatible with the test suite
type HivexHandle struct {
	handle *bindings.Hive
	path   string
}

// OpenHivex opens a hive file using our hivex CGO bindings
func OpenHivex(path string) (*HivexHandle, error) {
	h, err := bindings.Open(path, 0)
	if err != nil {
		return nil, fmt.Errorf("bindings.Open failed for %s: %w", path, err)
	}

	return &HivexHandle{
		handle: h,
		path:   path,
	}, nil
}

// Close closes the hivex handle
func (h *HivexHandle) Close() error {
	if h.handle != nil {
		return h.handle.Close()
	}
	return nil
}

// Root returns the root node ID
func (h *HivexHandle) Root() (int64, error) {
	root := h.handle.Root()
	if root == 0 {
		return 0, fmt.Errorf("hivex Root() returned 0")
	}
	return int64(root), nil
}

// NodeName returns the name of a node
func (h *HivexHandle) NodeName(node int64) (string, error) {
	name := h.handle.NodeName(bindings.NodeHandle(node))
	// Empty name is valid for root node, so don't error on it
	return name, nil
}

// NodeChildren returns the child nodes of a node
func (h *HivexHandle) NodeChildren(node int64) ([]int64, error) {
	children := h.handle.NodeChildren(bindings.NodeHandle(node))
	result := make([]int64, len(children))
	for i, child := range children {
		result[i] = int64(child)
	}
	return result, nil
}

// NodeNrChildren returns the number of child nodes
func (h *HivexHandle) NodeNrChildren(node int64) (int64, error) {
	count := h.handle.NodeNrChildren(bindings.NodeHandle(node))
	return int64(count), nil
}

// NodeValues returns the values of a node
func (h *HivexHandle) NodeValues(node int64) ([]int64, error) {
	values := h.handle.NodeValues(bindings.NodeHandle(node))
	result := make([]int64, len(values))
	for i, val := range values {
		result[i] = int64(val)
	}
	return result, nil
}

// NodeNrValues returns the number of values
func (h *HivexHandle) NodeNrValues(node int64) (int64, error) {
	count := h.handle.NodeNrValues(bindings.NodeHandle(node))
	return int64(count), nil
}

// ValueKey returns the name of a value
func (h *HivexHandle) ValueKey(value int64) (string, error) {
	key := h.handle.ValueKey(bindings.ValueHandle(value))
	return key, nil
}

// ValueType returns the type of a value
func (h *HivexHandle) ValueType(value int64) (int64, error) {
	vtype, _, err := h.handle.ValueType(bindings.ValueHandle(value))
	if err != nil {
		return 0, err
	}
	return int64(vtype), nil
}

// ValueValue returns the type and data bytes of a value
func (h *HivexHandle) ValueValue(value int64) (valType int64, data []byte, err error) {
	data, vtype, err := h.handle.ValueValue(bindings.ValueHandle(value))
	if err != nil {
		return 0, nil, err
	}
	return int64(vtype), data, nil
}

// ValueString returns a value as a string (for REG_SZ types)
func (h *HivexHandle) ValueString(value int64) (string, error) {
	return h.handle.ValueString(bindings.ValueHandle(value))
}

// ValueDword returns a value as a DWORD (for REG_DWORD types)
func (h *HivexHandle) ValueDword(value int64) (int32, error) {
	return h.handle.ValueDword(bindings.ValueHandle(value))
}

// ValueQword returns a value as a QWORD (for REG_QWORD types)
func (h *HivexHandle) ValueQword(value int64) (int64, error) {
	return h.handle.ValueQword(bindings.ValueHandle(value))
}

// HivexValueType constants matching Windows Registry types
const (
	HivexTypeNone                      = 0
	HivexTypeSZ                        = 1
	HivexTypeExpandSZ                  = 2
	HivexTypeBinary                    = 3
	HivexTypeDword                     = 4
	HivexTypeDwordBigEndian           = 5
	HivexTypeLink                      = 6
	HivexTypeMultiSZ                   = 7
	HivexTypeResourceList             = 8
	HivexTypeFullResourceDescriptor   = 9
	HivexTypeResourceRequirementsList = 10
	HivexTypeQword                     = 11
)

// HivexValueTypeName returns a human-readable name for a hivex value type
func HivexValueTypeName(typeID int64) string {
	switch typeID {
	case HivexTypeNone:
		return "REG_NONE"
	case HivexTypeSZ:
		return "REG_SZ"
	case HivexTypeExpandSZ:
		return "REG_EXPAND_SZ"
	case HivexTypeBinary:
		return "REG_BINARY"
	case HivexTypeDword:
		return "REG_DWORD"
	case HivexTypeDwordBigEndian:
		return "REG_DWORD_BIG_ENDIAN"
	case HivexTypeLink:
		return "REG_LINK"
	case HivexTypeMultiSZ:
		return "REG_MULTI_SZ"
	case HivexTypeResourceList:
		return "REG_RESOURCE_LIST"
	case HivexTypeFullResourceDescriptor:
		return "REG_FULL_RESOURCE_DESCRIPTOR"
	case HivexTypeResourceRequirementsList:
		return "REG_RESOURCE_REQUIREMENTS_LIST"
	case HivexTypeQword:
		return "REG_QWORD"
	default:
		return fmt.Sprintf("UNKNOWN_TYPE_%d", typeID)
	}
}
