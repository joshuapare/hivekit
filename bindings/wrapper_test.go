package bindings

import (
	"os"
	"path/filepath"
	"testing"
)

// Test hives relative to bindings directory
var testHivesDir = "../testdata"

// TestOpen verifies we can open and close hives
func TestOpen(t *testing.T) {
	testHives := []struct {
		name string
		path string
	}{
		{"minimal", filepath.Join(testHivesDir, "minimal")},
		{"special", filepath.Join(testHivesDir, "special")},
		{"large", filepath.Join(testHivesDir, "large")},
	}

	for _, tc := range testHives {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(tc.path); os.IsNotExist(err) {
				t.Skipf("Test hive not found: %s", tc.path)
			}

			hive, err := Open(tc.path, 0)
			if err != nil {
				t.Fatalf("Open failed: %v", err)
			}
			defer hive.Close()

			if hive.handle == nil {
				t.Error("Expected non-nil handle")
			}

			t.Logf("✓ Successfully opened %s", tc.name)
		})
	}
}

// TestRoot verifies we can get the root node
func TestRoot(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()
	if root == 0 {
		t.Fatal("Expected non-zero root node")
	}

	t.Logf("✓ Root node: %d", root)
}

// TestNodeName verifies we can read node names
func TestNodeName(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()
	rootName := hive.NodeName(root)

	if rootName == "" {
		t.Error("Expected non-empty root name")
	}

	t.Logf("✓ Root name: %q", rootName)
}

// TestNodeChildren verifies we can enumerate children
func TestNodeChildren(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()
	children := hive.NodeChildren(root)

	t.Logf("Root has %d children", len(children))

	for i, child := range children {
		childName := hive.NodeName(child)
		t.Logf("  Child %d: %q (handle: %d)", i, childName, child)

		if childName == "" {
			t.Errorf("Child %d has empty name", i)
		}
	}

	// Test NodeNrChildren matches
	nrChildren := hive.NodeNrChildren(root)
	if nrChildren != len(children) {
		t.Errorf("NodeNrChildren() = %d, but NodeChildren() returned %d", nrChildren, len(children))
	}

	t.Logf("✓ Enumerated %d children", len(children))
}

// TestNodeValues verifies we can read values
func TestNodeValues(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()

	// Try to find a node with values by walking the tree
	var testNode NodeHandle
	testNodeName := ""

	children := hive.NodeChildren(root)
	for _, child := range children {
		nrValues := hive.NodeNrValues(child)
		if nrValues > 0 {
			testNode = child
			testNodeName = hive.NodeName(child)
			break
		}
	}

	if testNode == 0 {
		// Check root itself
		nrValues := hive.NodeNrValues(root)
		if nrValues > 0 {
			testNode = root
			testNodeName = hive.NodeName(root)
		} else {
			t.Skip("No nodes with values found in minimal hive")
		}
	}

	t.Logf("Testing values on node: %q", testNodeName)

	values := hive.NodeValues(testNode)
	t.Logf("Node has %d values", len(values))

	for i, val := range values {
		valName := hive.ValueKey(val)
		data, valType, err := hive.ValueValue(val)
		if err != nil {
			t.Errorf("Failed to read value %d: %v", i, err)
			continue
		}

		t.Logf("  Value %d: %q (type: %s, size: %d bytes)", i, valName, valType, len(data))
	}

	// Test NodeNrValues matches
	nrValues := hive.NodeNrValues(testNode)
	if nrValues != len(values) {
		t.Errorf("NodeNrValues() = %d, but NodeValues() returned %d", nrValues, len(values))
	}

	t.Logf("✓ Read %d values", len(values))
}

// TestNodeNavigation verifies parent/child navigation
func TestNodeNavigation(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()
	children := hive.NodeChildren(root)

	if len(children) == 0 {
		t.Skip("No children to test navigation")
	}

	// Test parent relationship
	firstChild := children[0]
	parent := hive.NodeParent(firstChild)

	if parent != root {
		t.Errorf("Parent of child should be root: got %d, expected %d", parent, root)
	}

	// Test NodeGetChild
	childName := hive.NodeName(firstChild)
	foundChild := hive.NodeGetChild(root, childName)

	if foundChild != firstChild {
		t.Errorf("NodeGetChild(%q) returned different handle: got %d, expected %d", childName, foundChild, firstChild)
	}

	t.Logf("✓ Navigation works: parent/child relationships correct")
}

// TestValueTypes verifies we can read different value types
func TestValueTypes(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "large")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("large hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	// Walk tree to find different value types
	seenTypes := make(map[ValueType]int)

	var walkNode func(NodeHandle, int)
	walkNode = func(node NodeHandle, depth int) {
		if depth > 3 {
			return // Limit depth to keep test fast
		}

		values := hive.NodeValues(node)
		for _, val := range values {
			_, valType, err := hive.ValueValue(val)
			if err == nil {
				seenTypes[valType]++
			}
		}

		children := hive.NodeChildren(node)
		for _, child := range children {
			walkNode(child, depth+1)
		}
	}

	root := hive.Root()
	walkNode(root, 0)

	t.Logf("Value types found:")
	for vtype, count := range seenTypes {
		t.Logf("  %s: %d values", vtype, count)
	}

	if len(seenTypes) == 0 {
		t.Error("Expected to find some values")
	}

	t.Logf("✓ Found %d different value types", len(seenTypes))
}

// TestTreeWalk verifies we can walk entire tree
func TestTreeWalk(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	nodeCount := 0
	valueCount := 0

	var walkNode func(NodeHandle, int)
	walkNode = func(node NodeHandle, depth int) {
		if depth > 10 {
			return // Safety limit
		}

		nodeCount++

		values := hive.NodeValues(node)
		valueCount += len(values)

		children := hive.NodeChildren(node)
		for _, child := range children {
			walkNode(child, depth+1)
		}
	}

	root := hive.Root()
	walkNode(root, 0)

	t.Logf("✓ Walked tree: %d nodes, %d values", nodeCount, valueCount)

	if nodeCount == 0 {
		t.Error("Expected to find at least one node")
	}
}

// TestLastModified verifies we can read hive timestamp
func TestLastModified(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	timestamp := hive.LastModified()
	t.Logf("Hive last modified: %d", timestamp)

	// Timestamp should be non-zero for real hives
	if timestamp == 0 {
		t.Log("Warning: LastModified returned 0 (may be valid for some hives)")
	}

	t.Logf("✓ LastModified: %d", timestamp)
}

// TestNodeTimestamp verifies we can read node timestamps
func TestNodeTimestamp(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()
	timestamp := hive.NodeTimestamp(root)

	t.Logf("Root node timestamp: %d", timestamp)

	// Some nodes may have zero timestamp
	t.Logf("✓ NodeTimestamp: %d", timestamp)
}

// TestMultipleHives verifies we can open multiple hives simultaneously
func TestMultipleHives(t *testing.T) {
	hive1Path := filepath.Join(testHivesDir, "minimal")
	hive2Path := filepath.Join(testHivesDir, "special")

	if _, err := os.Stat(hive1Path); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}
	if _, err := os.Stat(hive2Path); os.IsNotExist(err) {
		t.Skip("special hive not found")
	}

	hive1, err := Open(hive1Path, 0)
	if err != nil {
		t.Fatalf("Open hive1 failed: %v", err)
	}
	defer hive1.Close()

	hive2, err := Open(hive2Path, 0)
	if err != nil {
		t.Fatalf("Open hive2 failed: %v", err)
	}
	defer hive2.Close()

	// Both should work independently
	root1 := hive1.Root()
	root2 := hive2.Root()

	name1 := hive1.NodeName(root1)
	name2 := hive2.NodeName(root2)

	t.Logf("Hive1 root: %q", name1)
	t.Logf("Hive2 root: %q", name2)

	t.Logf("✓ Multiple hives work simultaneously")
}

// TestOpenInvalidFile verifies error handling for invalid files
func TestOpenInvalidFile(t *testing.T) {
	_, err := Open("/nonexistent/path/to/hive", 0)
	if err == nil {
		t.Error("Expected error opening nonexistent file")
	}

	t.Logf("✓ Error handling works: %v", err)
}

// TestCloseIdempotent verifies Close can be called multiple times
func TestCloseIdempotent(t *testing.T) {
	hivePath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	hive, err := Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Close once
	err1 := hive.Close()
	if err1 != nil {
		t.Errorf("First Close failed: %v", err1)
	}

	// Close again
	err2 := hive.Close()
	if err2 != nil {
		t.Errorf("Second Close failed: %v", err2)
	}

	t.Logf("✓ Close is idempotent")
}

// copyHiveToTemp creates a temporary copy of a hive file for write testing
func copyHiveToTemp(t *testing.T, srcPath string) (string, func()) {
	t.Helper()

	// Read source hive
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("Failed to read source hive: %v", err)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "test-hive-*.tmp")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		t.Fatalf("Failed to write temp file: %v", err)
	}
	tmpFile.Close()

	// Return path and cleanup function
	cleanup := func() {
		os.Remove(tmpPath)
	}

	return tmpPath, cleanup
}

// TestNodeAddChild verifies we can create new child nodes
func TestNodeAddChild(t *testing.T) {
	srcPath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	tmpPath, cleanup := copyHiveToTemp(t, srcPath)
	defer cleanup()

	// Open with write flag
	hive, err := Open(tmpPath, OPEN_WRITE)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()
	initialChildren := hive.NodeChildren(root)
	initialCount := len(initialChildren)

	// Add a new child
	childName := "TestChild"
	newChild, err := hive.NodeAddChild(root, childName)
	if err != nil {
		t.Fatalf("NodeAddChild failed: %v", err)
	}

	if newChild == 0 {
		t.Fatal("Expected non-zero handle for new child")
	}

	// Verify child exists
	foundChild := hive.NodeGetChild(root, childName)
	if foundChild != newChild {
		t.Errorf("NodeGetChild returned different handle: got %d, expected %d", foundChild, newChild)
	}

	// Verify name
	actualName := hive.NodeName(newChild)
	if actualName != childName {
		t.Errorf("Child name = %q, want %q", actualName, childName)
	}

	// Verify child count increased
	afterChildren := hive.NodeChildren(root)
	if len(afterChildren) != initialCount+1 {
		t.Errorf("Child count = %d, want %d", len(afterChildren), initialCount+1)
	}

	t.Logf("✓ Created child node %q (handle: %d)", childName, newChild)
}

// TestNodeSetValue verifies we can set values on nodes
func TestNodeSetValue(t *testing.T) {
	srcPath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	tmpPath, cleanup := copyHiveToTemp(t, srcPath)
	defer cleanup()

	// Open with write flag
	hive, err := Open(tmpPath, OPEN_WRITE)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()

	// Set a string value
	valueName := "TestStringValue"
	valueData := []byte("T\x00e\x00s\x00t\x00\x00\x00") // UTF-16LE "Test" with null terminator
	err = hive.NodeSetValue(root, valueName, REG_SZ, valueData)
	if err != nil {
		t.Fatalf("NodeSetValue failed: %v", err)
	}

	// Verify value exists
	val := hive.NodeGetValue(root, valueName)
	if val == 0 {
		t.Fatal("Value not found after setting")
	}

	// Verify value name
	actualName := hive.ValueKey(val)
	if actualName != valueName {
		t.Errorf("Value name = %q, want %q", actualName, valueName)
	}

	// Verify value type
	actualType, size, err := hive.ValueType(val)
	if err != nil {
		t.Fatalf("ValueType failed: %v", err)
	}
	if actualType != REG_SZ {
		t.Errorf("Value type = %v, want %v", actualType, REG_SZ)
	}
	if size != len(valueData) {
		t.Errorf("Value size = %d, want %d", size, len(valueData))
	}

	// Verify value data
	actualData, actualDataType, err := hive.ValueValue(val)
	if err != nil {
		t.Fatalf("ValueValue failed: %v", err)
	}
	if actualDataType != REG_SZ {
		t.Errorf("ValueValue type = %v, want %v", actualDataType, REG_SZ)
	}
	if len(actualData) != len(valueData) {
		t.Errorf("Value data length = %d, want %d", len(actualData), len(valueData))
	}

	t.Logf("✓ Set value %q (type: %s, size: %d bytes)", valueName, actualType, size)
}

// TestNodeSetValueDword verifies we can set DWORD values
func TestNodeSetValueDword(t *testing.T) {
	srcPath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	tmpPath, cleanup := copyHiveToTemp(t, srcPath)
	defer cleanup()

	// Open with write flag
	hive, err := Open(tmpPath, OPEN_WRITE)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()

	// Set a DWORD value
	valueName := "TestDwordValue"
	dwordValue := uint32(42)
	valueData := []byte{byte(dwordValue), byte(dwordValue >> 8), byte(dwordValue >> 16), byte(dwordValue >> 24)}

	err = hive.NodeSetValue(root, valueName, REG_DWORD, valueData)
	if err != nil {
		t.Fatalf("NodeSetValue failed: %v", err)
	}

	// Verify value
	val := hive.NodeGetValue(root, valueName)
	if val == 0 {
		t.Fatal("Value not found after setting")
	}

	// Verify type
	actualType, _, err := hive.ValueType(val)
	if err != nil {
		t.Fatalf("ValueType failed: %v", err)
	}
	if actualType != REG_DWORD {
		t.Errorf("Value type = %v, want %v", actualType, REG_DWORD)
	}

	// Read back as DWORD
	actualValue, err := hive.ValueDword(val)
	if err != nil {
		t.Fatalf("ValueDword failed: %v", err)
	}
	if uint32(actualValue) != dwordValue {
		t.Errorf("DWORD value = %d, want %d", actualValue, dwordValue)
	}

	t.Logf("✓ Set DWORD value %q = %d", valueName, dwordValue)
}

// TestNodeSetValues verifies we can set multiple values at once
func TestNodeSetValues(t *testing.T) {
	srcPath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	tmpPath, cleanup := copyHiveToTemp(t, srcPath)
	defer cleanup()

	// Open with write flag
	hive, err := Open(tmpPath, OPEN_WRITE)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()

	// Prepare multiple values
	values := []SetValue{
		{
			Key:   "MultiValue1",
			Type:  REG_SZ,
			Value: []byte("V\x00a\x00l\x001\x00\x00\x00"), // UTF-16LE "Val1"
		},
		{
			Key:   "MultiValue2",
			Type:  REG_DWORD,
			Value: []byte{10, 0, 0, 0}, // DWORD 10
		},
	}

	// Set multiple values
	err = hive.NodeSetValues(root, values)
	if err != nil {
		t.Fatalf("NodeSetValues failed: %v", err)
	}

	// Verify first value
	val1 := hive.NodeGetValue(root, "MultiValue1")
	if val1 == 0 {
		t.Error("MultiValue1 not found")
	} else {
		name1 := hive.ValueKey(val1)
		if name1 != "MultiValue1" {
			t.Errorf("Value1 name = %q, want %q", name1, "MultiValue1")
		}
	}

	// Verify second value
	val2 := hive.NodeGetValue(root, "MultiValue2")
	if val2 == 0 {
		t.Error("MultiValue2 not found")
	} else {
		dwordVal, err := hive.ValueDword(val2)
		if err != nil {
			t.Errorf("ValueDword failed: %v", err)
		} else if dwordVal != 10 {
			t.Errorf("Value2 = %d, want 10", dwordVal)
		}
	}

	t.Logf("✓ Set %d values in one operation", len(values))
}

// TestNodeDeleteChild verifies we can delete child nodes
func TestNodeDeleteChild(t *testing.T) {
	srcPath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	tmpPath, cleanup := copyHiveToTemp(t, srcPath)
	defer cleanup()

	// Open with write flag
	hive, err := Open(tmpPath, OPEN_WRITE)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer hive.Close()

	root := hive.Root()

	// First create a child to delete
	childName := "ChildToDelete"
	newChild, err := hive.NodeAddChild(root, childName)
	if err != nil {
		t.Fatalf("NodeAddChild failed: %v", err)
	}

	// Verify child exists
	foundChild := hive.NodeGetChild(root, childName)
	if foundChild != newChild {
		t.Fatal("Child not found before deletion")
	}

	initialChildren := hive.NodeChildren(root)
	initialCount := len(initialChildren)

	// Delete the child
	err = hive.NodeDeleteChild(newChild)
	if err != nil {
		t.Fatalf("NodeDeleteChild failed: %v", err)
	}

	// Verify child is gone
	afterChild := hive.NodeGetChild(root, childName)
	if afterChild != 0 {
		t.Error("Child still exists after deletion")
	}

	// Verify child count decreased
	afterChildren := hive.NodeChildren(root)
	if len(afterChildren) != initialCount-1 {
		t.Errorf("Child count = %d, want %d", len(afterChildren), initialCount-1)
	}

	t.Logf("✓ Deleted child node %q", childName)
}

// TestCommit verifies we can commit changes to disk
func TestCommit(t *testing.T) {
	srcPath := filepath.Join(testHivesDir, "minimal")
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		t.Skip("minimal hive not found")
	}

	tmpPath, cleanup := copyHiveToTemp(t, srcPath)
	defer cleanup()

	childName := "CommitTestChild"
	valueName := "CommitTestValue"

	// First session: make changes and commit
	{
		hive, err := Open(tmpPath, OPEN_WRITE)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}

		root := hive.Root()

		// Add a child
		newChild, err := hive.NodeAddChild(root, childName)
		if err != nil {
			hive.Close()
			t.Fatalf("NodeAddChild failed: %v", err)
		}

		// Set a value on the child
		valueData := []byte("T\x00e\x00s\x00t\x00\x00\x00") // UTF-16LE "Test"
		err = hive.NodeSetValue(newChild, valueName, REG_SZ, valueData)
		if err != nil {
			hive.Close()
			t.Fatalf("NodeSetValue failed: %v", err)
		}

		// Commit changes
		err = hive.Commit(tmpPath)
		if err != nil {
			hive.Close()
			t.Fatalf("Commit failed: %v", err)
		}

		hive.Close()
		t.Logf("✓ Committed changes to disk")
	}

	// Second session: reopen and verify changes persisted
	{
		hive, err := Open(tmpPath, 0)
		if err != nil {
			t.Fatalf("Reopen failed: %v", err)
		}
		defer hive.Close()

		root := hive.Root()

		// Verify child exists
		child := hive.NodeGetChild(root, childName)
		if child == 0 {
			t.Fatal("Child not found after commit/reopen")
		}

		actualChildName := hive.NodeName(child)
		if actualChildName != childName {
			t.Errorf("Child name = %q, want %q", actualChildName, childName)
		}

		// Verify value exists on child
		val := hive.NodeGetValue(child, valueName)
		if val == 0 {
			t.Fatal("Value not found after commit/reopen")
		}

		actualValueName := hive.ValueKey(val)
		if actualValueName != valueName {
			t.Errorf("Value name = %q, want %q", actualValueName, valueName)
		}

		t.Logf("✓ Changes persisted after commit and reopen")
	}
}
