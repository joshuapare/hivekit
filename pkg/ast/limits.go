package ast

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Limits defines constraints for registry operations to prevent
// corruption, resource exhaustion, and malformed hives.
type Limits struct {
	// MaxSubkeys is the maximum number of subkeys a node can have.
	// Windows registry limit is typically 512 for most key types,
	// but can be higher (up to ~65535) for some system keys.
	MaxSubkeys int

	// MaxValues is the maximum number of values a node can have.
	// Windows registry limit is 16384 values per key.
	MaxValues int

	// MaxValueSize is the maximum size of a single value's data in bytes.
	// Windows registry limit is 1 MB (1,048,576 bytes) for most value types.
	MaxValueSize int

	// MaxKeyNameLen is the maximum length of a key name in characters.
	// Windows registry limit is 255 characters (not bytes).
	MaxKeyNameLen int

	// MaxValueNameLen is the maximum length of a value name in characters.
	// Windows registry limit is 16,383 characters.
	MaxValueNameLen int

	// MaxTreeDepth is the maximum depth of the registry tree.
	// Windows registry has no hard limit, but practical limit is ~512 levels.
	MaxTreeDepth int

	// MaxTotalSize is the maximum total size of a hive in bytes.
	// Windows registry hives are typically limited to 2 GB.
	MaxTotalSize int64
}

// DefaultLimits returns the standard Windows registry limits.
// These are conservative defaults that should work for all real-world scenarios.
func DefaultLimits() Limits {
	return Limits{
		MaxSubkeys:      WindowsMaxSubkeysDefault,
		MaxValues:       WindowsMaxValues,
		MaxValueSize:    WindowsMaxValueSize1MB,
		MaxKeyNameLen:   WindowsMaxKeyNameLen,
		MaxValueNameLen: WindowsMaxValueNameLen,
		MaxTreeDepth:    WindowsMaxTreeDepthPractical,
		MaxTotalSize:    WindowsMaxHiveSize2GB,
	}
}

// RelaxedLimits returns more permissive limits for system keys or special cases.
// Use with caution - these allow operations that may not work on real Windows systems.
func RelaxedLimits() Limits {
	return Limits{
		MaxSubkeys:      WindowsMaxSubkeysAbsolute,
		MaxValues:       WindowsMaxValues,
		MaxValueSize:    WindowsMaxValueSize10MB,
		MaxKeyNameLen:   WindowsMaxKeyNameLen,
		MaxValueNameLen: WindowsMaxValueNameLen,
		MaxTreeDepth:    WindowsMaxTreeDepthDeep,
		MaxTotalSize:    WindowsMaxHiveSize4GB,
	}
}

// StrictLimits returns conservative limits for safety-critical applications.
// These prevent resource exhaustion in constrained environments.
func StrictLimits() Limits {
	return Limits{
		MaxSubkeys:      WindowsMaxSubkeysDefault / StrictSubkeysDivisor,
		MaxValues:       WindowsMaxValues / StrictValuesDivisor,
		MaxValueSize:    WindowsMaxValueSize64KB,
		MaxKeyNameLen:   WindowsMaxKeyNameLenHalf,
		MaxValueNameLen: WindowsMaxValueNameLenSmall,
		MaxTreeDepth:    WindowsMaxTreeDepthShallow,
		MaxTotalSize:    WindowsMaxHiveSize100MB,
	}
}

// ValidationError represents a limit validation failure.
type ValidationError struct {
	Limit    string // Name of the limit that was exceeded
	Current  int64  // Current value
	Maximum  int64  // Maximum allowed value
	NodePath string // Path to the node (if applicable)
}

func (e *ValidationError) Error() string {
	if e.NodePath != "" {
		return fmt.Sprintf("registry limit exceeded at '%s': %s is %d (max %d)",
			e.NodePath, e.Limit, e.Current, e.Maximum)
	}
	return fmt.Sprintf("registry limit exceeded: %s is %d (max %d)",
		e.Limit, e.Current, e.Maximum)
}

// ValidateNode validates a node against the given limits.
// Returns ValidationError if any limit is exceeded.
func (n *Node) ValidateNode(limits Limits) error {
	// Validate key name length
	if len(n.Name) > limits.MaxKeyNameLen {
		return &ValidationError{
			Limit:   "MaxKeyNameLen",
			Current: int64(len(n.Name)),
			Maximum: int64(limits.MaxKeyNameLen),
		}
	}

	// Validate number of subkeys
	if len(n.Children) > limits.MaxSubkeys {
		return &ValidationError{
			Limit:   "MaxSubkeys",
			Current: int64(len(n.Children)),
			Maximum: int64(limits.MaxSubkeys),
		}
	}

	// Validate number of values
	if len(n.Values) > limits.MaxValues {
		return &ValidationError{
			Limit:   "MaxValues",
			Current: int64(len(n.Values)),
			Maximum: int64(limits.MaxValues),
		}
	}

	// Validate each value
	for _, val := range n.Values {
		if err := val.ValidateValue(limits); err != nil {
			return err
		}
	}

	return nil
}

// ValidateValue validates a value against the given limits.
func (v *Value) ValidateValue(limits Limits) error {
	// Validate value name length
	if len(v.Name) > limits.MaxValueNameLen {
		return &ValidationError{
			Limit:   "MaxValueNameLen",
			Current: int64(len(v.Name)),
			Maximum: int64(limits.MaxValueNameLen),
		}
	}

	// Validate value data size
	if len(v.Data) > limits.MaxValueSize {
		return &ValidationError{
			Limit:   "MaxValueSize",
			Current: int64(len(v.Data)),
			Maximum: int64(limits.MaxValueSize),
		}
	}

	return nil
}

// ValidateTreeDepth validates that the tree depth doesn't exceed the limit.
// Returns the depth of the tree and any validation error.
func (t *Tree) ValidateTreeDepth(limits Limits) (int, error) {
	maxDepth := t.Root.measureDepth()
	if maxDepth > limits.MaxTreeDepth {
		return maxDepth, &ValidationError{
			Limit:   "MaxTreeDepth",
			Current: int64(maxDepth),
			Maximum: int64(limits.MaxTreeDepth),
		}
	}
	return maxDepth, nil
}

// measureDepth recursively measures the maximum depth of the tree.
func (n *Node) measureDepth() int {
	if len(n.Children) == 0 {
		return 1
	}

	maxChildDepth := 0
	for _, child := range n.Children {
		depth := child.measureDepth()
		if depth > maxChildDepth {
			maxChildDepth = depth
		}
	}

	return maxChildDepth + 1
}

// ValidateTreeSize estimates the serialized size of the tree and validates
// against the MaxTotalSize limit.
func (t *Tree) ValidateTreeSize(limits Limits) (int64, error) {
	size := t.Root.estimateSize()
	if size > limits.MaxTotalSize {
		return size, &ValidationError{
			Limit:   "MaxTotalSize",
			Current: size,
			Maximum: limits.MaxTotalSize,
		}
	}
	return size, nil
}

// estimateSize recursively estimates the serialized size of a node and its descendants.
// This is an approximation used for limit validation.
func (n *Node) estimateSize() int64 {
	var size int64

	// NK cell: header + fixed fields + name
	size += int64(format.CellHeaderSize + format.NKFixedHeaderSize + len(n.Name))
	size = alignTo8(size)

	// Value list cell if there are values
	if len(n.Values) > 0 {
		size += int64(format.CellHeaderSize + len(n.Values)*format.OffsetFieldSize)
		size = alignTo8(size)

		// Each VK cell
		for _, val := range n.Values {
			vkSize := int64(format.CellHeaderSize + format.VKFixedHeaderSize + len(val.Name))
			size += alignTo8(vkSize)

			// Data cell if data > inline threshold
			if len(val.Data) > format.OffsetFieldSize {
				dataSize := int64(format.CellHeaderSize + len(val.Data))
				size += alignTo8(dataSize)
			}
		}
	}

	// Subkey list cell if there are children
	if len(n.Children) > 0 {
		size += int64(format.CellHeaderSize + format.ListHeaderSize + len(n.Children)*format.LFEntrySize)
		size = alignTo8(size)
	}

	// Recursively add children
	for _, child := range n.Children {
		size += child.estimateSize()
	}

	return size
}

// alignTo8 returns size aligned to 8-byte boundary.
func alignTo8(size int64) int64 {
	if size%format.CellAlignment == 0 {
		return size
	}
	return size + (format.CellAlignment - size%format.CellAlignment)
}

// ValidateTree performs comprehensive validation of the entire tree.
// This checks all limits and returns detailed errors for any violations.
func (t *Tree) ValidateTree(limits Limits) error {
	// Validate tree depth
	if _, err := t.ValidateTreeDepth(limits); err != nil {
		return err
	}

	// Validate total size
	if _, err := t.ValidateTreeSize(limits); err != nil {
		return err
	}

	// Validate all nodes recursively
	return t.Root.validateNodeRecursive(limits, "", 0)
}

// validateNodeRecursive validates a node and all its descendants.
func (n *Node) validateNodeRecursive(limits Limits, parentPath string, depth int) error {
	// Build current path
	path := parentPath
	if path != "" && n.Name != "" {
		path += RegistryPathSeparator
	}
	if n.Name != "" {
		path += n.Name
	}

	// Validate this node
	if err := n.ValidateNode(limits); err != nil {
		ve := &ValidationError{}
		if errors.As(err, &ve) {
			ve.NodePath = path
			return ve
		}
		return err
	}

	// Validate depth
	if depth > limits.MaxTreeDepth {
		return &ValidationError{
			Limit:    "MaxTreeDepth",
			Current:  int64(depth),
			Maximum:  int64(limits.MaxTreeDepth),
			NodePath: path,
		}
	}

	// Recursively validate children
	for _, child := range n.Children {
		if err := child.validateNodeRecursive(limits, path, depth+1); err != nil {
			return err
		}
	}

	return nil
}

// LimitViolation wraps a types.Error for limit violations.
func LimitViolation(err error) error {
	ve := &ValidationError{}
	if errors.As(err, &ve) {
		return &types.Error{
			Kind: types.ErrKindState,
			Msg:  ve.Error(),
			Err:  ve,
		}
	}
	return err
}
