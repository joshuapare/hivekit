package ast

import (
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
)

func TestDefaultLimits(t *testing.T) {
	limits := DefaultLimits()

	if limits.MaxSubkeys != WindowsMaxSubkeysDefault {
		t.Errorf("Expected MaxSubkeys=%d, got %d", WindowsMaxSubkeysDefault, limits.MaxSubkeys)
	}
	if limits.MaxValues != WindowsMaxValues {
		t.Errorf("Expected MaxValues=%d, got %d", WindowsMaxValues, limits.MaxValues)
	}
	if limits.MaxValueSize != WindowsMaxValueSize1MB {
		t.Errorf("Expected MaxValueSize=%d, got %d", WindowsMaxValueSize1MB, limits.MaxValueSize)
	}
}

func TestValidateNode_KeyNameLength(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxKeyNameLen = 10

	// Valid key name
	node := &Node{Name: "ShortName", Children: []*Node{}, Values: []*Value{}}
	if err := node.ValidateNode(limits); err != nil {
		t.Errorf("Valid key name failed validation: %v", err)
	}

	// Too long key name
	node.Name = strings.Repeat("a", 11)
	err := node.ValidateNode(limits)
	if err == nil {
		t.Error("Expected error for key name too long")
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxKeyNameLen" {
			t.Errorf("Expected MaxKeyNameLen error, got %s", ve.Limit)
		}
		if ve.Current != 11 {
			t.Errorf("Expected current=11, got %d", ve.Current)
		}
	} else {
		t.Error("Expected ValidationError type")
	}
}

func TestValidateNode_MaxSubkeys(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxSubkeys = 3

	node := &Node{Name: "Test", Children: []*Node{}, Values: []*Value{}}

	// Add 3 children (at limit)
	for range 3 {
		node.Children = append(
			node.Children,
			&Node{Name: "Child", Children: []*Node{}, Values: []*Value{}},
		)
	}
	if err := node.ValidateNode(limits); err != nil {
		t.Errorf("Valid subkey count failed validation: %v", err)
	}

	// Add 4th child (exceeds limit)
	node.Children = append(
		node.Children,
		&Node{Name: "Child4", Children: []*Node{}, Values: []*Value{}},
	)
	err := node.ValidateNode(limits)
	if err == nil {
		t.Error("Expected error for too many subkeys")
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxSubkeys" {
			t.Errorf("Expected MaxSubkeys error, got %s", ve.Limit)
		}
		if ve.Current != 4 {
			t.Errorf("Expected current=4, got %d", ve.Current)
		}
	}
}

func TestValidateNode_MaxValues(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxValues = 2

	node := &Node{Name: "Test", Children: []*Node{}, Values: []*Value{}}

	// Add 2 values (at limit)
	node.Values = append(node.Values, &Value{Name: "Val1", Type: types.REG_SZ, Data: []byte("data")})
	node.Values = append(node.Values, &Value{Name: "Val2", Type: types.REG_SZ, Data: []byte("data")})
	if err := node.ValidateNode(limits); err != nil {
		t.Errorf("Valid value count failed validation: %v", err)
	}

	// Add 3rd value (exceeds limit)
	node.Values = append(node.Values, &Value{Name: "Val3", Type: types.REG_SZ, Data: []byte("data")})
	err := node.ValidateNode(limits)
	if err == nil {
		t.Error("Expected error for too many values")
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxValues" {
			t.Errorf("Expected MaxValues error, got %s", ve.Limit)
		}
	}
}

func TestValidateValue_NameLength(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxValueNameLen = 10

	// Valid value name
	val := &Value{Name: "ShortName", Type: types.REG_SZ, Data: []byte("data")}
	if err := val.ValidateValue(limits); err != nil {
		t.Errorf("Valid value name failed validation: %v", err)
	}

	// Too long value name
	val.Name = strings.Repeat("a", 11)
	err := val.ValidateValue(limits)
	if err == nil {
		t.Error("Expected error for value name too long")
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxValueNameLen" {
			t.Errorf("Expected MaxValueNameLen error, got %s", ve.Limit)
		}
	}
}

func TestValidateValue_DataSize(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxValueSize = 100

	// Valid value size
	val := &Value{Name: "Test", Type: types.REG_BINARY, Data: make([]byte, 100)}
	if err := val.ValidateValue(limits); err != nil {
		t.Errorf("Valid value size failed validation: %v", err)
	}

	// Too large value
	val.Data = make([]byte, 101)
	err := val.ValidateValue(limits)
	if err == nil {
		t.Error("Expected error for value data too large")
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxValueSize" {
			t.Errorf("Expected MaxValueSize error, got %s", ve.Limit)
		}
		if ve.Current != 101 {
			t.Errorf("Expected current=101, got %d", ve.Current)
		}
	}
}

func TestValidateTreeDepth(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxTreeDepth = 3

	tree := NewTree()
	// Build a tree: Root -> Child1 -> Child2 -> Child3 (depth 4)
	child1 := tree.Root.AddChild("Child1")
	child2 := child1.AddChild("Child2")
	child2.AddChild("Child3")

	depth, err := tree.ValidateTreeDepth(limits)
	if err == nil {
		t.Error("Expected error for tree too deep")
	}
	if depth != 4 {
		t.Errorf("Expected depth=4, got %d", depth)
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxTreeDepth" {
			t.Errorf("Expected MaxTreeDepth error, got %s", ve.Limit)
		}
		if ve.Current != 4 {
			t.Errorf("Expected current=4, got %d", ve.Current)
		}
	}
}

func TestValidateTreeDepth_Valid(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxTreeDepth = 5

	tree := NewTree()
	// Build a tree: Root -> Child1 -> Child2 -> Child3 (depth 4)
	child1 := tree.Root.AddChild("Child1")
	child2 := child1.AddChild("Child2")
	child2.AddChild("Child3")

	depth, err := tree.ValidateTreeDepth(limits)
	if err != nil {
		t.Errorf("Valid tree depth failed validation: %v", err)
	}
	if depth != 4 {
		t.Errorf("Expected depth=4, got %d", depth)
	}
}

func TestValidateTreeSize(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxTotalSize = 1000 // Very small limit for testing

	tree := NewTree()
	// Add many children to exceed size limit
	for range 100 {
		child := tree.Root.AddChild("Child")
		child.AddValue("Value", types.REG_BINARY, make([]byte, 100))
	}

	size, err := tree.ValidateTreeSize(limits)
	if err == nil {
		t.Error("Expected error for tree too large")
	}
	if size <= limits.MaxTotalSize {
		t.Errorf("Expected size > %d, got %d", limits.MaxTotalSize, size)
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxTotalSize" {
			t.Errorf("Expected MaxTotalSize error, got %s", ve.Limit)
		}
	}
}

func TestValidateTree_Comprehensive(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxSubkeys = 2
	limits.MaxValues = 2
	limits.MaxTreeDepth = 3

	tree := NewTree()
	// Build valid tree at limits
	child1 := tree.Root.AddChild("Child1")
	tree.Root.AddChild("Child2")

	child1.AddValue("Val1", types.REG_SZ, []byte("data"))
	child1.AddValue("Val2", types.REG_SZ, []byte("data"))

	// Should pass
	if err := tree.ValidateTree(limits); err != nil {
		t.Errorf("Valid tree failed comprehensive validation: %v", err)
	}

	// Add one more subkey (exceeds limit)
	tree.Root.AddChild("Child3")
	err := tree.ValidateTree(limits)
	if err == nil {
		t.Error("Expected error for tree with too many subkeys")
	}
	if ve, ok := err.(*ValidationError); ok {
		if ve.Limit != "MaxSubkeys" {
			t.Errorf("Expected MaxSubkeys error, got %s", ve.Limit)
		}
		// Root node has empty path, which is correct
		if ve.Current != 3 {
			t.Errorf("Expected current=3, got %d", ve.Current)
		}
	} else {
		t.Error("Expected ValidationError type")
	}
}

func TestValidationError_Error(t *testing.T) {
	// Without path
	err := &ValidationError{
		Limit:   "MaxSubkeys",
		Current: 100,
		Maximum: 50,
	}
	msg := err.Error()
	if !strings.Contains(msg, "MaxSubkeys") {
		t.Errorf("Error message should contain limit name: %s", msg)
	}
	if !strings.Contains(msg, "100") {
		t.Errorf("Error message should contain current value: %s", msg)
	}

	// With path
	testPath := "Software" + RegistryPathSeparator + "Test"
	err.NodePath = testPath
	msg = err.Error()
	if !strings.Contains(msg, testPath) {
		t.Errorf("Error message should contain path: %s", msg)
	}
}

func TestLimitPresets(t *testing.T) {
	defaults := DefaultLimits()
	relaxed := RelaxedLimits()
	strict := StrictLimits()

	// Relaxed should be >= Default
	if relaxed.MaxSubkeys < defaults.MaxSubkeys {
		t.Error("Relaxed MaxSubkeys should be >= Default")
	}
	if relaxed.MaxValueSize < defaults.MaxValueSize {
		t.Error("Relaxed MaxValueSize should be >= Default")
	}

	// Strict should be <= Default
	if strict.MaxSubkeys > defaults.MaxSubkeys {
		t.Error("Strict MaxSubkeys should be <= Default")
	}
	if strict.MaxValueSize > defaults.MaxValueSize {
		t.Error("Strict MaxValueSize should be <= Default")
	}
}

func TestMeasureDepth(t *testing.T) {
	// Single node (root)
	root := &Node{Name: "Root", Children: []*Node{}, Values: []*Value{}}
	if depth := root.measureDepth(); depth != 1 {
		t.Errorf("Expected depth=1 for single node, got %d", depth)
	}

	// Root with children
	child1 := &Node{Name: "Child1", Children: []*Node{}, Values: []*Value{}}
	child2 := &Node{Name: "Child2", Children: []*Node{}, Values: []*Value{}}
	root.Children = []*Node{child1, child2}
	if depth := root.measureDepth(); depth != 2 {
		t.Errorf("Expected depth=2, got %d", depth)
	}

	// Add grandchild
	grandchild := &Node{Name: "Grandchild", Children: []*Node{}, Values: []*Value{}}
	child1.Children = []*Node{grandchild}
	if depth := root.measureDepth(); depth != 3 {
		t.Errorf("Expected depth=3, got %d", depth)
	}
}

func TestEstimateSize(t *testing.T) {
	// Empty node
	node := &Node{Name: "", Children: []*Node{}, Values: []*Value{}}
	size := node.estimateSize()
	if size <= 0 {
		t.Error("Empty node should have positive size estimate")
	}

	// Node with name
	node.Name = "TestKey"
	sizeWithName := node.estimateSize()
	if sizeWithName <= size {
		t.Error("Node with name should be larger")
	}

	// Node with value
	node.Values = append(node.Values, &Value{
		Name: "TestValue",
		Type: types.REG_SZ,
		Data: []byte("test data"),
	})
	sizeWithValue := node.estimateSize()
	if sizeWithValue <= sizeWithName {
		t.Error("Node with value should be larger")
	}

	// Node with child
	child := &Node{Name: "Child", Children: []*Node{}, Values: []*Value{}}
	node.Children = append(node.Children, child)
	sizeWithChild := node.estimateSize()
	if sizeWithChild <= sizeWithValue {
		t.Error("Node with child should be larger")
	}
}

func TestLimitViolation(t *testing.T) {
	ve := &ValidationError{
		Limit:   "MaxSubkeys",
		Current: 100,
		Maximum: 50,
	}

	err := LimitViolation(ve)
	if _, ok := err.(*types.Error); !ok {
		t.Error("LimitViolation should wrap in types.Error")
	}

	// Non-ValidationError should pass through
	otherErr := &types.Error{Kind: types.ErrKindFormat, Msg: "test"}
	wrapped := LimitViolation(otherErr)
	if wrapped != otherErr {
		t.Error("Non-ValidationError should pass through unchanged")
	}
}
