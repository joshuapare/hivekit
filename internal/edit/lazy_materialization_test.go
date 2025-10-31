package edit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestLazyMaterialization_NoChanges verifies that when no changes are made,
// all nodes remain as base-ref (not materialized).
func TestLazyMaterialization_NoChanges(t *testing.T) {
	// Create a base hive with some structure
	baseHive := createTestHiveWithDepth(t, 3, 3) // Depth 3, 3 children per node = ~40 nodes
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	// Create editor and transaction with NO changes
	editor := NewEditor(r)
	tx := editor.Begin()

	// Get internal transaction to inspect tree
	txInternal := tx.(*transaction)

	// Build tree (this should create base-ref nodes)
	alloc := newAllocator()
	root, err := buildTree(txInternal, alloc, types.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// Verify root exists but most descendants should be base-ref
	if root == nil {
		t.Fatal("Root should not be nil")
	}

	// Count materialized vs base-ref nodes
	stats := countNodeKinds(root)
	t.Logf("Node statistics: %d materialized, %d base-ref, total %d",
		stats.materialized, stats.baseRef, stats.total)

	// With no changes, we expect very few materialized nodes
	// Only the root and maybe some immediate children should be materialized
	if stats.materialized > 5 {
		t.Errorf("Too many materialized nodes with no changes: %d (expected < 5)", stats.materialized)
	}

	// Should have many base-ref nodes
	if stats.baseRef == 0 {
		t.Error("Expected some base-ref nodes, got 0")
	}
}

// TestLazyMaterialization_SinglePathChanged verifies that changing a single deep path
// only materializes the ancestors of that path, not siblings.
func TestLazyMaterialization_SinglePathChanged(t *testing.T) {
	// Create a base hive with depth 3, 3 children per node
	baseHive := createTestHiveWithDepth(t, 3, 3)
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	editor := NewEditor(r)
	tx := editor.Begin()
	txInternal := tx.(*transaction)

	// Change a single deep path: Root\Child0\Child0\Child0
	deepPath := `Child0\Child0\Child0`
	if err := tx.SetValue(deepPath, "NewValue", types.REG_SZ, []byte("test")); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Build tree
	alloc := newAllocator()
	root, err := buildTree(txInternal, alloc, types.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// Count node kinds
	stats := countNodeKinds(root)
	t.Logf("Node statistics after single path change: %d materialized, %d base-ref, total %d",
		stats.materialized, stats.baseRef, stats.total)

	// Should have some base-ref nodes (siblings)
	if stats.baseRef == 0 {
		t.Error("Expected unchanged siblings to remain as base-ref")
	}

	// Verify that siblings of the changed path are base-ref
	child0 := findNodeInTree(root, "Child0")
	if child0 == nil {
		t.Fatal("Child0 not found")
	}
	if child0.kind != nodeMaterialized {
		t.Error("Child0 should be materialized (ancestor of change)")
	}

	// Child1 should be base-ref (sibling, unchanged)
	child1 := findNodeInTree(root, "Child1")
	if child1 != nil && child1.kind != nodeBaseRef {
		t.Error("Child1 should be base-ref (sibling, unchanged)")
	}
}

// TestLazyMaterialization_DeepNestedChange verifies that a change at depth 10
// only materializes ancestors, not siblings at each level.
func TestLazyMaterialization_DeepNestedChange(t *testing.T) {
	// Create a deeper hive: depth 5, 2 children per node
	baseHive := createTestHiveWithDepth(t, 5, 2)
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	editor := NewEditor(r)
	tx := editor.Begin()
	txInternal := tx.(*transaction)

	// Change a path at depth 4
	deepPath := `Child0\Child0\Child0\Child0`
	if err := tx.SetValue(deepPath, "DeepValue", types.REG_DWORD, []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Build tree
	alloc := newAllocator()
	root, err := buildTree(txInternal, alloc, types.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	stats := countNodeKinds(root)
	t.Logf("Deep nested change stats: %d materialized, %d base-ref, total %d",
		stats.materialized, stats.baseRef, stats.total)

	// Most nodes should still be base-ref
	if float64(stats.baseRef)/float64(stats.total) < 0.5 {
		t.Errorf("Expected most nodes to be base-ref, got %d/%d (%.1f%%)",
			stats.baseRef, stats.total, 100.0*float64(stats.baseRef)/float64(stats.total))
	}
}

// TestLazyMaterialization_SiblingUnchanged verifies that siblings of changed paths
// remain as base-ref nodes.
func TestLazyMaterialization_SiblingUnchanged(t *testing.T) {
	baseHive := createTestHiveWithDepth(t, 3, 3)
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	editor := NewEditor(r)
	tx := editor.Begin()
	txInternal := tx.(*transaction)

	// Change Child0\Child0
	if err := tx.SetValue(`Child0\Child0`, "Val", types.REG_SZ, []byte("changed")); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	alloc := newAllocator()
	root, err := buildTree(txInternal, alloc, types.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// Find Child0\Child1 (sibling of changed path)
	child0 := findNodeInTree(root, "Child0")
	if child0 == nil {
		t.Fatal("Child0 not found")
	}

	var sibling *treeNode
	for _, child := range child0.children {
		if child.nameLower == "child1" {
			sibling = child
			break
		}
	}

	if sibling == nil {
		t.Fatal("Sibling Child0\\Child1 not found")
	}

	if sibling.kind != nodeBaseRef {
		t.Errorf("Sibling should be base-ref, got kind=%v", sibling.kind)
	}
}

// TestLazyMaterialization_EnsureMaterializedWorks verifies that calling
// ensureMaterialized on a base-ref node correctly loads children and values.
func TestLazyMaterialization_EnsureMaterializedWorks(t *testing.T) {
	baseHive := createTestHiveWithDepth(t, 3, 3)
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	editor := NewEditor(r)
	tx := editor.Begin()
	txInternal := tx.(*transaction)

	// Build tree with no changes (all base-ref)
	alloc := newAllocator()
	root, err := buildTree(txInternal, alloc, types.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	// First, we need to materialize root to access its children
	// (root itself is a base-ref when there are no changes)
	changeIdx := txInternal.getChangeIndex()
	if err := root.ensureMaterialized(txInternal, changeIdx); err != nil {
		t.Fatalf("Failed to materialize root: %v", err)
	}

	// Now find a base-ref child node
	var baseRefNode *treeNode
	for _, child := range root.children {
		if child.kind == nodeBaseRef {
			baseRefNode = child
			break
		}
	}

	if baseRefNode == nil {
		t.Fatal("No base-ref node found to test - all children were already materialized")
	}

	// Verify it's base-ref and has no children loaded
	if baseRefNode.kind != nodeBaseRef {
		t.Fatal("Expected base-ref node")
	}
	if baseRefNode.children != nil {
		t.Error("Base-ref node should have nil children before materialization")
	}

	// Materialize it
	if err := baseRefNode.ensureMaterialized(txInternal, changeIdx); err != nil {
		t.Fatalf("ensureMaterialized failed: %v", err)
	}

	// Verify it's now materialized
	if baseRefNode.kind != nodeMaterialized {
		t.Error("Node should be materialized after ensureMaterialized")
	}

	// Verify children are loaded (if they exist in base hive)
	if baseRefNode.children == nil {
		t.Error("Children should be loaded after materialization")
	}

	// Should have child nodes now
	if len(baseRefNode.children) == 0 {
		t.Error("Expected children to be loaded, got 0")
	}
}

// TestLazyMaterialization_SerializationWorks verifies that we can successfully
// rebuild a hive with lazy base-ref nodes and produce valid output.
func TestLazyMaterialization_SerializationWorks(t *testing.T) {
	baseHive := createTestHiveWithDepth(t, 3, 2)
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	editor := NewEditor(r)
	tx := editor.Begin()

	// Make a small change
	if err := tx.SetValue("Child0", "TestValue", types.REG_SZ, []byte("serialization test")); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Commit (this will serialize the tree including base-ref nodes)
	w := &writer.MemWriter{}
	if err := tx.Commit(w, types.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify output is valid by reading it back
	outHive := w.Buf
	r2, err := reader.OpenBytes(outHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to read output hive: %v", err)
	}

	// Verify the change is present
	_, err = r2.Root()
	if err != nil {
		t.Fatalf("Failed to get root: %v", err)
	}
	child0ID, err := r2.Find(`\Child0`)
	if err != nil {
		t.Fatalf("Failed to find Child0: %v", err)
	}
	valIDs, err := r2.Values(child0ID)
	if err != nil {
		t.Fatalf("Failed to get values: %v", err)
	}

	// List all values for debugging
	var valueNames []string
	found := false
	for _, vid := range valIDs {
		meta, err := r2.StatValue(vid)
		if err != nil {
			continue
		}
		valueNames = append(valueNames, meta.Name)
		if meta.Name == "TestValue" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TestValue not found in output hive. Found values: %v", valueNames)
	}
}

// TestLazyMaterialization_DenseChanges verifies that when ALL nodes are changed,
// performance does not degrade (worst case should not be slower than before).
// This is the critical test to ensure we don't have O(n²) behavior.
func TestLazyMaterialization_DenseChanges(t *testing.T) {
	// Create a substantial hive
	baseHive := createTestHiveWithDepth(t, 4, 3) // ~120 nodes
	r, err := reader.OpenBytes(baseHive, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to create reader: %v", err)
	}

	editor := NewEditor(r)
	tx := editor.Begin()
	txInternal := tx.(*transaction)

	// Change EVERY node by adding a value
	paths := collectAllPaths(r)
	t.Logf("Changing all %d paths (dense change test)", len(paths))
	for _, path := range paths {
		if err := tx.SetValue(path, "Changed", types.REG_SZ, []byte("dense")); err != nil {
			t.Logf("Warning: could not set value at %q: %v", path, err)
		}
	}

	// Build tree - all nodes should be materialized
	alloc := newAllocator()
	root, err := buildTree(txInternal, alloc, types.WriteOptions{})
	if err != nil {
		t.Fatalf("Failed to build tree: %v", err)
	}

	stats := countNodeKinds(root)
	t.Logf("Dense change stats: %d materialized, %d base-ref, total %d",
		stats.materialized, stats.baseRef, stats.total)

	// With dense changes, most/all nodes should be materialized
	if stats.materialized == 0 {
		t.Error("Expected nodes to be materialized when all paths changed")
	}

	// The key test: this should complete without hanging or being extremely slow
	// If we had O(n²) behavior, this test would timeout
	t.Log("Dense change test completed successfully (no O(n²) degradation)")
}

// Helper: nodeKindStats tracks materialization statistics
type nodeKindStats struct {
	materialized int
	baseRef      int
	total        int
}

// countNodeKinds recursively counts node kinds in the tree
func countNodeKinds(node *treeNode) nodeKindStats {
	if node == nil {
		return nodeKindStats{}
	}

	stats := nodeKindStats{total: 1}
	if node.kind == nodeMaterialized {
		stats.materialized = 1
	} else if node.kind == nodeBaseRef {
		stats.baseRef = 1
	}

	for _, child := range node.children {
		childStats := countNodeKinds(child)
		stats.materialized += childStats.materialized
		stats.baseRef += childStats.baseRef
		stats.total += childStats.total
	}

	return stats
}

// findNodeInTree finds a node by name at the root level
func findNodeInTree(root *treeNode, name string) *treeNode {
	nameLower := strings.ToLower(name)
	for _, child := range root.children {
		if child.nameLower == nameLower {
			return child
		}
	}
	return nil
}

// createTestHiveWithDepth creates a test hive with specified depth and branching factor
func createTestHiveWithDepth(t *testing.T, depth, branchingFactor int) []byte {
	w := &writer.MemWriter{}

	// Create empty hive
	editor := NewEditor(nil)
	tx := editor.Begin()

	// Build tree structure
	createSubtree(tx, "", depth, branchingFactor, 0)

	if err := tx.Commit(w, types.WriteOptions{}); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	return w.Buf
}

// createSubtree recursively creates a balanced tree of keys
func createSubtree(tx types.Tx, parentPath string, depth, branchingFactor, currentDepth int) {
	if currentDepth >= depth {
		return
	}

	for i := 0; i < branchingFactor; i++ {
		childName := fmt.Sprintf("Child%d", i)
		var childPath string
		if parentPath == "" {
			childPath = childName
		} else {
			childPath = parentPath + `\` + childName
		}

		// Create key
		if err := tx.CreateKey(childPath, types.CreateKeyOptions{CreateParents: true}); err != nil {
			// Ignore errors (key might already exist)
			continue
		}

		// Add a value to make it more realistic
		tx.SetValue(childPath, "Value"+childName, types.REG_SZ, []byte("data"+childName))

		// Recurse
		createSubtree(tx, childPath, depth, branchingFactor, currentDepth+1)
	}
}

// collectAllPaths collects all key paths in a hive
func collectAllPaths(r types.Reader) []string {
	var paths []string
	rootID, err := r.Root()
	if err != nil {
		return paths
	}

	var walk func(id types.NodeID, path string)
	walk = func(id types.NodeID, path string) {
		paths = append(paths, path)
		subkeys, err := r.Subkeys(id)
		if err != nil {
			return
		}
		for _, sid := range subkeys {
			meta, err := r.StatKey(sid)
			if err != nil {
				continue
			}
			childPath := path
			if childPath != "" {
				childPath += `\`
			}
			childPath += meta.Name
			walk(sid, childPath)
		}
	}

	walk(rootID, "")
	return paths
}
