package walk_test

import (
	"testing"

	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/merge/v2/walk"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil"
)

const largeHivePath = "testdata/large"

// TestAnnotate_RootAnnotated verifies that after Annotate the root trie node
// is marked as existing and has a valid CellIdx.
func TestAnnotate_RootAnnotated(t *testing.T) {
	h, cleanup := testutil.SetupTestHiveFrom(t, largeHivePath, "walk-test")
	defer cleanup()

	// The large hive has subkeys: "a", "another", "the".
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"a"}},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	if !root.Exists {
		t.Error("root.Exists should be true after Annotate")
	}
	if root.CellIdx == format.InvalidOffset {
		t.Error("root.CellIdx should not be InvalidOffset after Annotate")
	}
}

// TestAnnotate_ExistingKeyAnnotated verifies that a trie child node
// corresponding to an existing hive key is annotated correctly.
func TestAnnotate_ExistingKeyAnnotated(t *testing.T) {
	h, cleanup := testutil.SetupTestHiveFrom(t, largeHivePath, "walk-test")
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"the"}},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	if len(root.Children) == 0 {
		t.Fatal("expected at least one child on root")
	}

	child := root.Children[0]
	if !child.Exists {
		t.Errorf("child %q should exist in the hive", child.Name)
	}
	if child.CellIdx == format.InvalidOffset {
		t.Errorf("child %q CellIdx should not be InvalidOffset", child.Name)
	}
	if child.SKCellIdx == format.InvalidOffset {
		t.Errorf("child %q SKCellIdx should not be InvalidOffset", child.Name)
	}
}

// TestAnnotate_NewKeyMarkedAsNotExisting verifies that a trie node for a key
// that does not exist in the hive is annotated with Exists=false.
func TestAnnotate_NewKeyMarkedAsNotExisting(t *testing.T) {
	h, cleanup := testutil.SetupTestHiveFrom(t, largeHivePath, "walk-test")
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"CompletelyNewKey12345"}},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	if len(root.Children) == 0 {
		t.Fatal("expected 1 child on root")
	}

	child := root.Children[0]
	if child.Exists {
		t.Errorf("child %q should not exist in hive", child.Name)
	}
	if child.CellIdx != format.InvalidOffset {
		t.Errorf("child %q CellIdx should be InvalidOffset, got %#x", child.Name, child.CellIdx)
	}
}

// TestAnnotate_SubtreeMarkedAsNew verifies that when a parent key does not
// exist, all of its descendants are also marked as non-existent.
func TestAnnotate_SubtreeMarkedAsNew(t *testing.T) {
	h, cleanup := testutil.SetupTestHiveFrom(t, largeHivePath, "walk-test")
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"New", "Deep", "Path"}},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	// Verify that the entire chain from "New" onwards is marked as non-existent.
	var checkNotExist func(n *trie.Node, path string)
	checkNotExist = func(n *trie.Node, path string) {
		if n.Exists {
			t.Errorf("node %q should not exist", path)
		}
		if n.CellIdx != format.InvalidOffset {
			t.Errorf("node %q CellIdx should be InvalidOffset, got %#x", path, n.CellIdx)
		}
		for _, c := range n.Children {
			checkNotExist(c, path+"\\"+c.Name)
		}
	}

	// "New" is root.Children[0]; all 3 nodes should be non-existent.
	if len(root.Children) == 0 {
		t.Fatal("expected at least 1 child on root")
	}
	newNode := root.Children[0]
	checkNotExist(newNode, "New")
}

// TestAnnotate_EmptyTrie verifies that Annotate succeeds without error when the
// trie has no children (no operations).
func TestAnnotate_EmptyTrie(t *testing.T) {
	h, cleanup := testutil.SetupTestHiveFrom(t, largeHivePath, "walk-test")
	defer cleanup()

	root := trie.Build(nil)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate on empty trie failed: %v", err)
	}
}

// TestAnnotate_MixedExistingAndNew verifies that Annotate correctly handles a
// trie where some children exist in the hive and some do not.
func TestAnnotate_MixedExistingAndNew(t *testing.T) {
	h, cleanup := testutil.SetupTestHiveFrom(t, largeHivePath, "walk-test")
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"a"}},                  // exists in large hive
		{Type: merge.OpEnsureKey, KeyPath: []string{"DoesNotExist999999"}}, // does not exist
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	if len(root.Children) != 2 {
		t.Fatalf("expected 2 children on root, got %d", len(root.Children))
	}

	var existingNode, newNode *trie.Node
	for _, c := range root.Children {
		switch c.NameLower {
		case "a":
			existingNode = c
		case "doesnotexist999999":
			newNode = c
		}
	}

	if existingNode == nil {
		t.Fatal("'a' child not found in trie")
	}
	if !existingNode.Exists {
		t.Error("'a' should be marked as existing")
	}
	if existingNode.CellIdx == format.InvalidOffset {
		t.Error("'a' CellIdx should not be InvalidOffset")
	}

	if newNode == nil {
		t.Fatal("DoesNotExist999999 child not found in trie")
	}
	if newNode.Exists {
		t.Error("DoesNotExist999999 should be marked as non-existing")
	}
	if newNode.CellIdx != format.InvalidOffset {
		t.Errorf("DoesNotExist999999 CellIdx should be InvalidOffset, got %#x", newNode.CellIdx)
	}
}
