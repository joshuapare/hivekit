package ast

import (
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
)

func TestNewTree(t *testing.T) {
	tree := NewTree()
	if tree.Root == nil {
		t.Fatal("Root should not be nil")
	}
	if tree.Root.Name != "" {
		t.Errorf("Root name should be empty, got %q", tree.Root.Name)
	}
	if tree.Root.Dirty {
		t.Error("Root should not be dirty initially")
	}
}

func TestNewTreeWithBase(t *testing.T) {
	baseHive := []byte{1, 2, 3, 4}
	tree := NewTreeWithBase(baseHive)
	if tree.BaseHive == nil {
		t.Fatal("BaseHive should not be nil")
	}
	if len(tree.BaseHive) != 4 {
		t.Errorf("BaseHive length should be 4, got %d", len(tree.BaseHive))
	}
}

func TestNodeAddChild(t *testing.T) {
	tree := NewTree()
	child := tree.Root.AddChild("TestKey")

	if child == nil {
		t.Fatal("Child should not be nil")
	}
	if child.Name != "TestKey" {
		t.Errorf("Child name should be 'TestKey', got %q", child.Name)
	}
	if child.Parent != tree.Root {
		t.Error("Child parent should be root")
	}
	if !child.Dirty {
		t.Error("New child should be dirty")
	}
	if !tree.Root.Dirty {
		t.Error("Root should be dirty after adding child")
	}
}

func TestNodeAddValue(t *testing.T) {
	tree := NewTree()
	data := []byte("test data")

	tree.Root.AddValue("TestValue", types.REG_SZ, data)

	if len(tree.Root.Values) != 1 {
		t.Fatalf("Expected 1 value, got %d", len(tree.Root.Values))
	}

	val := tree.Root.Values[0]
	if val.Name != "TestValue" {
		t.Errorf("Value name should be 'TestValue', got %q", val.Name)
	}
	if val.Type != types.REG_SZ {
		t.Errorf("Value type should be REG_SZ, got %d", val.Type)
	}
	if string(val.Data) != "test data" {
		t.Errorf("Value data should be 'test data', got %q", string(val.Data))
	}
	if !val.Dirty {
		t.Error("New value should be dirty")
	}
	if !tree.Root.Dirty {
		t.Error("Root should be dirty after adding value")
	}
}

func TestNodeUpdateValue(t *testing.T) {
	tree := NewTree()
	tree.Root.AddValue("TestValue", types.REG_SZ, []byte("original"))
	tree.Root.Dirty = false // reset for test

	// Update existing value
	tree.Root.AddValue("TestValue", types.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})

	if len(tree.Root.Values) != 1 {
		t.Fatalf("Expected 1 value, got %d", len(tree.Root.Values))
	}

	val := tree.Root.Values[0]
	if val.Type != types.REG_DWORD {
		t.Errorf("Value type should be updated to REG_DWORD, got %d", val.Type)
	}
	if !val.Dirty {
		t.Error("Updated value should be dirty")
	}
	if !tree.Root.Dirty {
		t.Error("Root should be dirty after updating value")
	}
}

func TestNodeRemoveValue(t *testing.T) {
	tree := NewTree()
	tree.Root.AddValue("Value1", types.REG_SZ, []byte("data1"))
	tree.Root.AddValue("Value2", types.REG_SZ, []byte("data2"))
	tree.Root.Dirty = false // reset for test

	removed := tree.Root.RemoveValue("Value1")
	if !removed {
		t.Error("RemoveValue should return true")
	}
	if len(tree.Root.Values) != 1 {
		t.Fatalf("Expected 1 value after removal, got %d", len(tree.Root.Values))
	}
	if tree.Root.Values[0].Name != "Value2" {
		t.Errorf("Remaining value should be 'Value2', got %q", tree.Root.Values[0].Name)
	}
	if !tree.Root.Dirty {
		t.Error("Root should be dirty after removing value")
	}
}

func TestNodeRemoveChild(t *testing.T) {
	tree := NewTree()
	tree.Root.AddChild("Child1")
	tree.Root.AddChild("Child2")
	tree.Root.Dirty = false // reset for test

	removed := tree.Root.RemoveChild("Child1")
	if !removed {
		t.Error("RemoveChild should return true")
	}
	if len(tree.Root.Children) != 1 {
		t.Fatalf("Expected 1 child after removal, got %d", len(tree.Root.Children))
	}
	if tree.Root.Children[0].Name != "Child2" {
		t.Errorf("Remaining child should be 'Child2', got %q", tree.Root.Children[0].Name)
	}
	if !tree.Root.Dirty {
		t.Error("Root should be dirty after removing child")
	}
}

func TestMarkDirty(t *testing.T) {
	tree := NewTree()
	child1 := tree.Root.AddChild("Child1")
	child2 := child1.AddChild("Child2")

	// Reset dirty flags
	tree.Root.Dirty = false
	child1.Dirty = false
	child2.Dirty = false

	// Mark child2 dirty
	child2.MarkDirty()

	// All ancestors should be dirty
	if !child2.Dirty {
		t.Error("child2 should be dirty")
	}
	if !child1.Dirty {
		t.Error("child1 should be dirty (ancestor)")
	}
	if !tree.Root.Dirty {
		t.Error("root should be dirty (ancestor)")
	}
}

func TestFindNode(t *testing.T) {
	tree := NewTree()
	child1 := tree.Root.AddChild("Software")
	child2 := child1.AddChild("Microsoft")
	child2.AddChild("Windows")

	tests := []struct {
		path     string
		expected string
		found    bool
	}{
		{"", "", true},                 // root
		{"Software", "Software", true}, // direct child
		{
			"Software" + RegistryPathSeparator + "Microsoft",
			"Microsoft",
			true,
		}, // nested
		{
			"Software" + RegistryPathSeparator + "Microsoft" + RegistryPathSeparator + "Windows",
			"Windows",
			true,
		}, // deeply nested
		{
			"Software" + RegistryPathSeparator + "DoesNotExist",
			"",
			false,
		}, // not found
		{"DoesNotExist", "", false}, // not found
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			node := tree.FindNode(tt.path)
			if tt.found {
				if node == nil {
					t.Errorf("Expected to find node at path %q", tt.path)
				} else if node.Name != tt.expected {
					t.Errorf("Expected node name %q, got %q", tt.expected, node.Name)
				}
			} else {
				if node != nil {
					t.Errorf("Expected not to find node at path %q, but got %q", tt.path, node.Name)
				}
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path     string
		expected []string
	}{
		{"", nil},
		{"Software", []string{"Software"}},
		{"Software" + RegistryPathSeparator + "Microsoft", []string{"Software", "Microsoft"}},
		{
			"Software" + RegistryPathSeparator + "Microsoft" + RegistryPathSeparator + "Windows",
			[]string{"Software", "Microsoft", "Windows"},
		},
		{
			RegistryPathSeparator + "Software",
			[]string{"Software"},
		}, // leading slash
		{
			"Software" + RegistryPathSeparator,
			[]string{"Software"},
		}, // trailing slash
		{
			RegistryPathSeparator + "Software" + RegistryPathSeparator + "Microsoft" + RegistryPathSeparator,
			[]string{"Software", "Microsoft"},
		}, // both
		{
			"Software" + RegistryPathSeparator + RegistryPathSeparator + "Microsoft",
			[]string{"Software", "Microsoft"},
		}, // double slash
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := splitPath(tt.path)
			if len(result) != len(tt.expected) {
				t.Fatalf("Expected %d segments, got %d", len(tt.expected), len(result))
			}
			for i, seg := range result {
				if seg != tt.expected[i] {
					t.Errorf("Segment %d: expected %q, got %q", i, tt.expected[i], seg)
				}
			}
		})
	}
}

func TestMarkDirtyIdempotent(t *testing.T) {
	tree := NewTree()
	child1 := tree.Root.AddChild("Child1")
	child2 := child1.AddChild("Child2")

	// Mark dirty multiple times
	child2.MarkDirty()
	child2.MarkDirty()
	child2.MarkDirty()

	// Should still be marked dirty exactly once
	if !child2.Dirty {
		t.Error("child2 should be dirty")
	}
	if !child1.Dirty {
		t.Error("child1 should be dirty")
	}
	if !tree.Root.Dirty {
		t.Error("root should be dirty")
	}
}
