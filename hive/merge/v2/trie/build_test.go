package trie_test

import (
	"testing"

	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/internal/format"
)

// TestBuild_SingleKey verifies that a single EnsureKey op produces the correct
// trie structure: a root with one child whose fields are populated.
func TestBuild_SingleKey(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"Software"}},
	}
	root := trie.Build(ops)

	if root == nil {
		t.Fatal("Build returned nil root")
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}

	child := root.Children[0]
	if child.Name != "Software" {
		t.Errorf("expected Name %q, got %q", "Software", child.Name)
	}
	if child.NameLower != "software" {
		t.Errorf("expected NameLower %q, got %q", "software", child.NameLower)
	}
	if !child.EnsureKey {
		t.Error("expected EnsureKey to be true")
	}
	if child.DeleteKey {
		t.Error("expected DeleteKey to be false")
	}
	if child.CellIdx != format.InvalidOffset {
		t.Errorf("expected CellIdx %#x, got %#x", format.InvalidOffset, child.CellIdx)
	}
}

// TestBuild_PrefixSharing verifies that 3 ops under the same parent key share
// a single parent node and produce exactly 3 sorted children.
func TestBuild_PrefixSharing(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"Software", "Zebra"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"Software", "Alpha"}},
		{Type: merge.OpEnsureKey, KeyPath: []string{"Software", "Middle"}},
	}
	root := trie.Build(ops)

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 top-level child (Software), got %d", len(root.Children))
	}

	sw := root.Children[0]
	if sw.Name != "Software" {
		t.Errorf("expected Name %q, got %q", "Software", sw.Name)
	}
	if len(sw.Children) != 3 {
		t.Fatalf("expected 3 children under Software, got %d", len(sw.Children))
	}

	// Children must be sorted by NameLower.
	expected := []string{"alpha", "middle", "zebra"}
	for i, child := range sw.Children {
		if child.NameLower != expected[i] {
			t.Errorf("child[%d]: expected NameLower %q, got %q", i, expected[i], child.NameLower)
		}
	}
}

// TestBuild_ValueOps verifies that SetValue and DeleteValue ops attach the
// correct ValueOp entries to the leaf node.
func TestBuild_ValueOps(t *testing.T) {
	ops := []merge.Op{
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"Control"},
			ValueName: "Version",
			ValueType: 1, // REG_SZ
			Data:      []byte("v2"),
		},
		{
			Type:      merge.OpDeleteValue,
			KeyPath:   []string{"Control"},
			ValueName: "OldValue",
		},
	}
	root := trie.Build(ops)

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(root.Children))
	}
	leaf := root.Children[0]

	if len(leaf.Values) != 2 {
		t.Fatalf("expected 2 ValueOps, got %d", len(leaf.Values))
	}

	// First op: SetValue
	v0 := leaf.Values[0]
	if v0.Name != "Version" {
		t.Errorf("Values[0].Name: expected %q, got %q", "Version", v0.Name)
	}
	if v0.Type != 1 {
		t.Errorf("Values[0].Type: expected 1, got %d", v0.Type)
	}
	if string(v0.Data) != "v2" {
		t.Errorf("Values[0].Data: expected %q, got %q", "v2", string(v0.Data))
	}
	if v0.Delete {
		t.Error("Values[0].Delete: expected false for SetValue")
	}

	// Second op: DeleteValue
	v1 := leaf.Values[1]
	if v1.Name != "OldValue" {
		t.Errorf("Values[1].Name: expected %q, got %q", "OldValue", v1.Name)
	}
	if !v1.Delete {
		t.Error("Values[1].Delete: expected true for DeleteValue")
	}
}

// TestBuild_DeleteKey verifies that a DeleteKey op sets DeleteKey on the leaf.
func TestBuild_DeleteKey(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpDeleteKey, KeyPath: []string{"Obsolete", "Sub"}},
	}
	root := trie.Build(ops)

	if len(root.Children) != 1 {
		t.Fatalf("expected 1 top-level child, got %d", len(root.Children))
	}
	obsolete := root.Children[0]

	if len(obsolete.Children) != 1 {
		t.Fatalf("expected 1 child under Obsolete, got %d", len(obsolete.Children))
	}
	sub := obsolete.Children[0]

	if !sub.DeleteKey {
		t.Error("expected DeleteKey to be true on leaf")
	}
	if sub.EnsureKey {
		t.Error("expected EnsureKey to be false on leaf")
	}
}

// TestBuild_HashesPreComputed verifies that every node has a non-zero Hash
// (since the Windows LH hash of any non-empty name is non-zero in practice).
func TestBuild_HashesPreComputed(t *testing.T) {
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"HKLM", "System", "CurrentControlSet"}},
	}
	root := trie.Build(ops)

	var checkHashes func(n *trie.Node, depth int)
	checkHashes = func(n *trie.Node, depth int) {
		// Root has empty Name and Hash == 0, skip it.
		if depth > 0 && n.Hash == 0 {
			t.Errorf("node %q has Hash == 0", n.Name)
		}
		for _, c := range n.Children {
			checkHashes(c, depth+1)
		}
	}
	checkHashes(root, 0)
}

// TestBuild_EmptyOps verifies that nil (or empty) ops return an empty root.
func TestBuild_EmptyOps(t *testing.T) {
	root := trie.Build(nil)
	if root == nil {
		t.Fatal("Build(nil) returned nil root")
	}
	if len(root.Children) != 0 {
		t.Errorf("expected empty root children, got %d", len(root.Children))
	}

	root2 := trie.Build([]merge.Op{})
	if root2 == nil {
		t.Fatal("Build([]merge.Op{}) returned nil root")
	}
	if len(root2.Children) != 0 {
		t.Errorf("expected empty root children, got %d", len(root2.Children))
	}
}
