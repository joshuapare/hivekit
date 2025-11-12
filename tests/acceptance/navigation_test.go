package acceptance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestRoot tests hivex_root
// Gets the root node of the hive.
func TestRoot(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"minimal", TestHives.Minimal},
		{"special", TestHives.Special},
		{"rlenvalue", TestHives.RLenValue},
		{"large", TestHives.Large},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root from both
			goRoot, err := goHive.Root()
			require.NoError(t, err, "gohivex Root() failed")

			hivexRoot := hivexHive.Root()

			// Should point to same node
			assertSameNodeID(t, goRoot, hivexRoot, "Root node")

			// Root should have a name
			goMeta, err := goHive.StatKey(goRoot)
			require.NoError(t, err)

			hivexName := hivexHive.NodeName(hivexRoot)

			assertStringsEqual(t, goMeta.Name, hivexName, "Root node name")
		})
	}
}

// TestNodeChildren tests hivex_node_children
// Enumerates all child nodes of a given node.
func TestNodeChildren(t *testing.T) {
	tests := []struct {
		name        string
		hivePath    string
		expectCount int // Expected minimum children at root
	}{
		{"minimal", TestHives.Minimal, 0},
		{"special", TestHives.Special, 3}, // Has abcd_äöüß, zero\x00key, weird™
		{"large", TestHives.Large, 1},     // Has at least some structure
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Get children from both
			goChildren, err := goHive.Subkeys(goRoot)
			require.NoError(t, err, "gohivex Subkeys() failed")

			hivexChildren := hivexHive.NodeChildren(hivexRoot)

			// Should have same number of children
			assertIntEqual(t, len(goChildren), len(hivexChildren), "Number of children")

			// Should be at least the expected count
			assert.GreaterOrEqual(t, len(goChildren), tt.expectCount,
				"Should have at least %d children", tt.expectCount)

			// Children should be in same order and point to same nodes
			assertNodeListsEqual(t, goChildren, hivexChildren, "Children list")

			// Verify each child has a name
			for i, goChild := range goChildren {
				goChildMeta, statErr := goHive.StatKey(goChild)
				require.NoError(t, statErr, "Failed to stat child %d", i)

				hivexChildName := hivexHive.NodeName(hivexChildren[i])

				assertStringsEqual(t, goChildMeta.Name, hivexChildName,
					"Child %d name", i)
			}
		})
	}
}

// TestNodeChildrenRecursive tests recursive traversal
// Walks the entire tree comparing structure.
func TestNodeChildrenRecursive(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"minimal", TestHives.Minimal},
		{"special", TestHives.Special},
		{"large", TestHives.Large},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get roots
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Walk both trees and compare
			walkAndCompare(t, goHive, hivexHive, goRoot, hivexRoot, 0)
		})
	}
}

// walkAndCompare recursively walks both trees and compares structure.
func walkAndCompare(t *testing.T, goHive hive.Reader, hivexHive *bindings.Hive,
	goNode hive.NodeID, hivexNode bindings.NodeHandle, depth int) {
	t.Helper()

	// Limit depth to prevent infinite recursion on malformed hives
	if depth > 100 {
		t.Fatal("Tree depth exceeded 100 levels, possible cycle")
	}

	// Compare node IDs
	assertSameNodeID(t, goNode, hivexNode, "Node at depth %d", depth)

	// Compare metadata
	goMeta, err := goHive.StatKey(goNode)
	require.NoError(t, err)

	hivexName := hivexHive.NodeName(hivexNode)
	assertStringsEqual(t, goMeta.Name, hivexName, "Node name at depth %d", depth)

	// Compare children
	goChildren, err := goHive.Subkeys(goNode)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexNode)

	assertIntEqual(t, len(goChildren), len(hivexChildren),
		"Child count for node '%s' at depth %d", goMeta.Name, depth)

	// Recurse into children
	for i := range goChildren {
		walkAndCompare(t, goHive, hivexHive, goChildren[i], hivexChildren[i], depth+1)
	}
}

// TestNodeChildrenEmpty tests children of leaf nodes.
func TestNodeChildrenEmpty(t *testing.T) {
	// Use special hive which has known leaf nodes
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root and its children
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChildren, err := goHive.Subkeys(goRoot)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexRoot)

	// The special hive children are leaf nodes (no children of their own)
	if len(goChildren) > 0 {
		// Check first child has no children
		goLeafChildren, leafErr := goHive.Subkeys(goChildren[0])
		require.NoError(t, leafErr)

		hivexLeafChildren := hivexHive.NodeChildren(hivexChildren[0])

		assertIntEqual(t, len(goLeafChildren), len(hivexLeafChildren),
			"Leaf node should have same child count")

		assert.Empty(t, goLeafChildren, "Leaf node should have no children")
		assert.Empty(t, hivexLeafChildren, "Leaf node should have no children")
	}
}

// TestNodeParent tests hivex_node_parent
// Gets the parent node of a given node.
func TestNodeParent(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root and a child
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChildren, err := goHive.Subkeys(goRoot)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexRoot)

	if len(goChildren) > 0 {
		// Get parent of first child
		goParent, parentErr := goHive.Parent(goChildren[0])
		require.NoError(t, parentErr)

		hivexParent := hivexHive.NodeParent(hivexChildren[0])

		// Parent should be root
		assertSameNodeID(t, goParent, hivexParent, "Parent node")
		assert.Equal(t, goRoot, goParent, "Parent should be root")
	}
}

// TestNodeParentOfRoot tests parent of root node.
func TestNodeParentOfRoot(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	// Get parent of root
	goParent, err := goHive.Parent(goRoot)
	hivexParent := hivexHive.NodeParent(hivexRoot)

	// Both should return 0 (no parent) or error
	// hivex returns 0 for root's parent
	// gohivex may return error or 0
	if err != nil {
		// gohivex returns error - acceptable
		t.Logf("gohivex returns error for root's parent: %v", err)
		assert.Equal(t, bindings.NodeHandle(0), hivexParent, "hivex should return 0 for root's parent")
	} else {
		// gohivex returns 0 - should match hivex
		assertSameNodeID(t, goParent, hivexParent, "Parent of root")
		assert.Equal(t, hive.NodeID(0), goParent, "Parent of root should be 0")
	}
}

// TestNodeGetChild tests hivex_node_get_child
// Finds a child node by name.
func TestNodeGetChild(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		childName string
	}{
		{"special_with_umlaut", TestHives.Special, "abcd_äöüß"},
		{"special_with_zero", TestHives.Special, "zero\x00key"},
		{"special_with_trademark", TestHives.Special, "weird™"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Look for child by name
			goChild, err := goHive.Lookup(goRoot, tt.childName)
			require.NoError(t, err, "gohivex failed to find child '%s'", tt.childName)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.childName)
			require.NotZero(t, hivexChild, "hivex failed to find child '%s'", tt.childName)

			// Should find same child
			assertSameNodeID(t, goChild, hivexChild, "Found child '%s'", tt.childName)

			// Verify it's actually the child we wanted
			goMeta, err := goHive.StatKey(goChild)
			require.NoError(t, err)

			assertStringsEqual(t, tt.childName, goMeta.Name, "Child name")
		})
	}
}

// TestNodeGetChildNotFound tests GetChild with non-existent name.
func TestNodeGetChildNotFound(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	// Try to find non-existent child
	nonExistent := "this_key_does_not_exist_12345"

	_, goErr := goHive.Lookup(goRoot, nonExistent)
	require.Error(t, goErr, "gohivex should error for non-existent child")

	hivexChild := hivexHive.NodeGetChild(hivexRoot, nonExistent)
	assert.Zero(t, hivexChild, "hivex should return 0 for non-existent child")
}

// TestNodeGetChildCaseInsensitive tests that child lookup is case-insensitive
// Note: Per the gohivex API docs, Lookup is case-insensitive.
func TestNodeGetChildCaseInsensitive(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	// Original name
	originalName := "weird™"

	// Try different cases - note: this may not work for all Unicode chars
	// but should work for ASCII portions
	upperCase := "WEIRD™"

	// Look for original name
	goChild1, err := goHive.Lookup(goRoot, originalName)
	require.NoError(t, err)

	hivexChild1 := hivexHive.NodeGetChild(hivexRoot, originalName)
	require.NotZero(t, hivexChild1)

	// Verify they match
	assertSameNodeID(t, goChild1, hivexChild1, "Original case lookup")

	// Look for uppercase version
	goChild2, err := goHive.Lookup(goRoot, upperCase)
	// May or may not succeed depending on case sensitivity
	// Just verify both implementations behave the same
	hivexChild2 := hivexHive.NodeGetChild(hivexRoot, upperCase)

	if err == nil {
		// If gohivex found it, hivex should too
		assert.NotZero(t, hivexChild2, "Both should find or not find uppercase variant")
		assertSameNodeID(t, goChild2, hivexChild2, "Uppercase lookup")
	} else {
		// If gohivex didn't find it, hivex shouldn't either
		assert.Zero(t, hivexChild2, "Both should fail to find uppercase variant")
	}
}
