package edit

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/internal/format"
)

const testHivePath = "../../testdata/suite/windows-2003-server-system"

// Test_RealHive_EnsureKeyPath tests creating keys in a real Windows hive.
func Test_RealHive_EnsureKeyPath(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	editor := NewKeyEditor(h, allocator, idx, dt)

	// Get root key reference from the hive
	rootRef := h.RootCellOffset()

	// Create a test key path under root
	// We use a unique name to avoid conflicts with existing keys
	testPath := []string{"_HivekitTest", "TestSubkey", "DeepKey"}

	finalRef, keysCreated, err := editor.EnsureKeyPath(rootRef, testPath)
	if err != nil {
		t.Fatalf("EnsureKeyPath failed: %v", err)
	}

	if keysCreated == 0 {
		t.Log("Keys already existed (unexpected for unique test path)")
	}

	if finalRef == 0 {
		t.Error("Expected non-zero final reference")
	}

	// Verify the keys exist in the index
	testRef, ok := idx.GetNK(rootRef, "_hivekittest")
	if !ok {
		t.Error("_HivekitTest key not found in index")
	}

	subRef, ok := idx.GetNK(testRef, "testsubkey")
	if !ok {
		t.Error("TestSubkey not found in index")
	}

	deepRef, ok := idx.GetNK(subRef, "deepkey")
	if !ok {
		t.Error("DeepKey not found in index")
	}

	if deepRef != finalRef {
		t.Errorf("Final ref mismatch: got 0x%X, want 0x%X", finalRef, deepRef)
	}

	// Test idempotency - calling again should return existing
	finalRef2, keysCreated2, err := editor.EnsureKeyPath(rootRef, testPath)
	if err != nil {
		t.Fatalf("Second EnsureKeyPath failed: %v", err)
	}

	if keysCreated2 > 0 {
		t.Error("Expected no new keys on second call")
	}

	if finalRef != finalRef2 {
		t.Errorf("References differ: first=0x%X, second=0x%X", finalRef, finalRef2)
	}
}

// Test_RealHive_UpsertValue_Inline tests creating inline values (≤4 bytes).
func Test_RealHive_UpsertValue_Inline(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	// Create a test key
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_HivekitTest_Inline"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	// Create value editor
	valueEditor := NewValueEditor(h, allocator, idx, dt)

	// Create a DWORD value (4 bytes - inline)
	dwordData := []byte{0x01, 0x02, 0x03, 0x04}
	err = valueEditor.UpsertValue(keyRef, "TestDword", format.REGDWORD, dwordData)
	if err != nil {
		t.Fatalf("UpsertValue failed: %v", err)
	}

	// Verify value exists in index
	vkRef, ok := idx.GetVK(keyRef, "testdword")
	if !ok {
		t.Error("Value not found in index")
	}
	if vkRef == 0 {
		t.Error("Expected non-zero VK reference")
	}

	// Test updating the same value (should be no-op for identical data)
	err = valueEditor.UpsertValue(keyRef, "TestDword", format.REGDWORD, dwordData)
	if err != nil {
		t.Fatalf("Second UpsertValue failed: %v", err)
	}

	// Update with different data
	dwordData2 := []byte{0x05, 0x06, 0x07, 0x08}
	err = valueEditor.UpsertValue(keyRef, "TestDword", format.REGDWORD, dwordData2)
	if err != nil {
		t.Fatalf("UpsertValue update failed: %v", err)
	}
}

// Test_RealHive_UpsertValue_External tests external storage (5-16344 bytes).
func Test_RealHive_UpsertValue_External(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_HivekitTest_External"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	valueEditor := NewValueEditor(h, allocator, idx, dt)

	// Create a binary value with 100 bytes (external storage)
	binaryData := bytes.Repeat([]byte{0xAB}, 100)
	err = valueEditor.UpsertValue(keyRef, "TestBinary", format.REGBinary, binaryData)
	if err != nil {
		t.Fatalf("UpsertValue failed: %v", err)
	}

	// Verify value exists
	vkRef, ok := idx.GetVK(keyRef, "testbinary")
	if !ok {
		t.Error("Value not found in index")
	}
	if vkRef == 0 {
		t.Error("Expected non-zero VK reference")
	}

	// Create a larger value (1KB)
	largeData := bytes.Repeat([]byte{0xCD}, 1024)
	err = valueEditor.UpsertValue(keyRef, "LargeValue", format.REGBinary, largeData)
	if err != nil {
		t.Fatalf("UpsertValue for large data failed: %v", err)
	}

	vkRef2, ok := idx.GetVK(keyRef, "largevalue")
	if !ok {
		t.Error("Large value not found in index")
	}
	if vkRef2 == 0 {
		t.Error("Expected non-zero VK reference for large value")
	}
}

// Test_RealHive_UpsertValue_BigData tests big-data storage (>16344 bytes).
func Test_RealHive_UpsertValue_BigData(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_HivekitTest_BigData"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	valueEditor := NewValueEditor(h, allocator, idx, dt)

	// Create a big-data value (>16344 bytes)
	// Use 20KB to ensure we hit the big-data path
	bigData := bytes.Repeat([]byte{0xEF}, 20*1024)
	err = valueEditor.UpsertValue(keyRef, "BigValue", format.REGBinary, bigData)
	if err != nil {
		t.Fatalf("UpsertValue for big-data failed: %v", err)
	}

	// Verify value exists
	vkRef, ok := idx.GetVK(keyRef, "bigvalue")
	if !ok {
		t.Error("Big-data value not found in index")
	}
	if vkRef == 0 {
		t.Error("Expected non-zero VK reference for big-data value")
	}
}

// Test_RealHive_DeleteValue tests value deletion.
func Test_RealHive_DeleteValue(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_HivekitTest_Delete"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	valueEditor := NewValueEditor(h, allocator, idx, dt)

	// Create a value
	data := []byte{0x01, 0x02, 0x03, 0x04}
	err = valueEditor.UpsertValue(keyRef, "TestValue", format.REGDWORD, data)
	if err != nil {
		t.Fatalf("UpsertValue failed: %v", err)
	}

	// Verify it exists
	_, ok := idx.GetVK(keyRef, "testvalue")
	if !ok {
		t.Error("Value not found after creation")
	}

	// Delete the value
	err = valueEditor.DeleteValue(keyRef, "TestValue")
	if err != nil {
		t.Fatalf("DeleteValue failed: %v", err)
	}

	// Test idempotency - deleting non-existent value should succeed
	err = valueEditor.DeleteValue(keyRef, "NonExistent")
	if err != nil {
		t.Errorf("DeleteValue on non-existent value should succeed, got: %v", err)
	}
}

// Test_RealHive_MultipleValues tests creating multiple values in a key.
func Test_RealHive_MultipleValues(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_HivekitTest_Multi"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	valueEditor := NewValueEditor(h, allocator, idx, dt)

	// Create multiple values of different types
	values := []struct {
		name string
		typ  ValueType
		data []byte
	}{
		{"DwordValue", format.REGDWORD, []byte{0x01, 0x02, 0x03, 0x04}},
		{"StringValue", format.REGSZ, []byte("Test String\x00")},
		{"BinaryValue", format.REGBinary, bytes.Repeat([]byte{0xAB}, 50)},
		{"QwordValue", format.REGQWORD, []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}},
	}

	for _, v := range values {
		upsertErr := valueEditor.UpsertValue(keyRef, v.name, v.typ, v.data)
		if upsertErr != nil {
			t.Errorf("UpsertValue for %s failed: %v", v.name, upsertErr)
		}
	}

	// Verify all values exist
	for _, v := range values {
		_, ok := idx.GetVK(keyRef, normalizeName(v.name))
		if !ok {
			t.Errorf("Value %s not found in index", v.name)
		}
	}
}

// setupRealHive opens a real Windows hive for testing
// Returns hive, allocator, index, temp hive path, and cleanup function.
func setupRealHive(t testing.TB) (*hive.Hive, *alloc.FastAllocator, index.Index, string, func()) {
	t.Helper()

	// Check if test hive exists
	if _, err := os.Stat(testHivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", testHivePath)
	}

	// Create a copy in temp dir (to avoid modifying the original)
	tempDir := t.TempDir()
	tempHivePath := filepath.Join(tempDir, "test-hive")

	// Copy the hive
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Fatalf("Failed to open source hive: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}

	_, err = io.Copy(dst, src)
	dst.Close()
	if err != nil {
		t.Fatalf("Failed to copy hive: %v", err)
	}

	// Open the hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	// Create allocator
	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Create index and build it from the hive
	// Use larger capacity hints for real hives
	idx := index.NewStringIndex(10000, 10000)

	// Build index from the hive
	// This requires walking the hive structure (we'll do a simple scan)
	// For now, just create an empty index - the editor will populate it as we create keys/values
	// TODO: In a real implementation, you'd want to pre-populate the index by scanning the hive

	cleanup := func() {
		h.Close()
	}

	return h, allocator, idx, tempHivePath, cleanup
}

// Test_RealHive_DeleteKey_EmptyLeaf tests deleting a key with no subkeys or values.
func Test_RealHive_DeleteKey_EmptyLeaf(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create an empty test key
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_DeleteTest_EmptyLeaf"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	// Verify key exists in index
	_, ok := idx.GetNK(rootRef, "_deletetest_emptyleaf")
	if !ok {
		t.Fatal("Test key not found in index after creation")
	}

	// Delete the key
	err = keyEditor.DeleteKey(keyRef, false)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Successful deletion is the main verification
	// Index may still have stale entry (acceptable per design)
	t.Log("Successfully deleted empty leaf key")
}

// Test_RealHive_DeleteKey_WithValues tests deleting a key with multiple values.
func Test_RealHive_DeleteKey_WithValues(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create test key
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_DeleteTest_WithValues"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	// Add multiple values
	values := []struct {
		name string
		typ  ValueType
		data []byte
	}{
		{"DwordValue", format.REGDWORD, []byte{0x01, 0x02, 0x03, 0x04}},
		{"StringValue", format.REGSZ, []byte("Test\x00")},
		{"BinaryValue", format.REGBinary, bytes.Repeat([]byte{0xAB}, 50)},
	}

	for _, v := range values {
		upsertErr := valueEditor.UpsertValue(keyRef, v.name, v.typ, v.data)
		if upsertErr != nil {
			t.Fatalf("UpsertValue for %s failed: %v", v.name, upsertErr)
		}
	}

	// Verify values exist
	for _, v := range values {
		_, ok := idx.GetVK(keyRef, normalizeName(v.name))
		if !ok {
			t.Errorf("Value %s not found in index", v.name)
		}
	}

	// Delete the key (should delete all values too)
	err = keyEditor.DeleteKey(keyRef, false)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	t.Log("Successfully deleted key with multiple values")
}

// Test_RealHive_DeleteKey_Recursive tests deleting a tree with subkeys.
func Test_RealHive_DeleteKey_Recursive(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create a tree structure
	parentRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_DeleteTest_Tree"})
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	// Create multiple subkeys
	subkeys := []string{"Child1", "Child2", "Child3"}
	for _, subkey := range subkeys {
		_, _, ensureErr := keyEditor.EnsureKeyPath(parentRef, []string{subkey})
		if ensureErr != nil {
			t.Fatalf("Failed to create subkey %s: %v", subkey, ensureErr)
		}
	}

	// Verify subkeys exist
	for _, subkey := range subkeys {
		_, ok := idx.GetNK(parentRef, normalizeName(subkey))
		if !ok {
			t.Errorf("Subkey %s not found in index", subkey)
		}
	}

	// Delete recursively
	err = keyEditor.DeleteKey(parentRef, true)
	if err != nil {
		t.Fatalf("Recursive delete failed: %v", err)
	}

	t.Log("Successfully deleted key tree recursively")
}

// Test_RealHive_DeleteKey_NonRecursiveFails tests that non-recursive delete fails for keys with subkeys.
func Test_RealHive_DeleteKey_NonRecursiveFails(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create a parent with a child
	parentRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_DeleteTest_NonRecursive"})
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	_, _, err = keyEditor.EnsureKeyPath(parentRef, []string{"Child"})
	if err != nil {
		t.Fatalf("Failed to create child: %v", err)
	}

	// Try to delete parent without recursive flag (should fail)
	err = keyEditor.DeleteKey(parentRef, false)
	if err == nil {
		t.Fatal("Expected error when deleting key with subkeys non-recursively, got nil")
	}

	if !errors.Is(err, ErrKeyHasSubkeys) {
		t.Errorf("Expected ErrKeyHasSubkeys, got: %v", err)
	}

	t.Logf("Correctly rejected non-recursive delete: %v", err)
}

// Test_RealHive_DeleteKey_DeepHierarchy tests deleting a deep tree (3+ levels).
func Test_RealHive_DeleteKey_DeepHierarchy(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create a deep hierarchy
	deepPath := []string{"_DeleteTest_Deep", "Level1", "Level2", "Level3"}
	deepRef, _, err := keyEditor.EnsureKeyPath(rootRef, deepPath)
	if err != nil {
		t.Fatalf("Failed to create deep hierarchy: %v", err)
	}

	// Verify deepest key exists
	if deepRef == 0 {
		t.Fatal("Deep key ref is zero")
	}

	// Delete from the root of the tree
	rootOfTreeRef, ok := idx.GetNK(rootRef, "_deletetest_deep")
	if !ok {
		t.Fatal("Root of tree not found in index")
	}

	// Delete recursively
	err = keyEditor.DeleteKey(rootOfTreeRef, true)
	if err != nil {
		t.Fatalf("Deep hierarchy delete failed: %v", err)
	}

	t.Log("Successfully deleted deep hierarchy")
}

// Test_RealHive_DeleteKey_InvalidRef tests error handling for invalid references.
func Test_RealHive_DeleteKey_InvalidRef(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)

	// Test with zero ref
	err := keyEditor.DeleteKey(0, false)
	if !errors.Is(err, ErrInvalidRef) {
		t.Errorf("Expected ErrInvalidRef for zero ref, got: %v", err)
	}

	// Test with 0xFFFFFFFF ref
	err = keyEditor.DeleteKey(0xFFFFFFFF, false)
	if !errors.Is(err, ErrInvalidRef) {
		t.Errorf("Expected ErrInvalidRef for 0xFFFFFFFF ref, got: %v", err)
	}

	t.Log("Correctly rejected invalid references")
}

// Test_RealHive_DeleteKey_Root tests that deleting root fails.
func Test_RealHive_DeleteKey_Root(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Try to delete root (should fail)
	err := keyEditor.DeleteKey(rootRef, true)
	if err == nil {
		t.Fatal("Expected error when deleting root key, got nil")
	}

	if !errors.Is(err, ErrCannotDeleteRoot) {
		t.Errorf("Expected ErrCannotDeleteRoot, got: %v", err)
	}

	t.Logf("Correctly rejected root deletion: %v", err)
}

// Test_RealHive_DeleteKey_WithBigData tests deleting a key with big-data values (>16KB).
func Test_RealHive_DeleteKey_WithBigData(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create test key
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_DeleteTest_BigData"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	// Add a big-data value (>16KB forces DB structure)
	bigData := bytes.Repeat([]byte{0xAB, 0xCD}, 10000) // 20KB
	err = valueEditor.UpsertValue(keyRef, "BigDataValue", format.REGBinary, bigData)
	if err != nil {
		t.Fatalf("UpsertValue for big data failed: %v", err)
	}

	// Verify value exists
	_, ok := idx.GetVK(keyRef, "bigdatavalue")
	if !ok {
		t.Error("Big data value not found in index")
	}

	// Delete the key (should free DB structure, blocklist, and data blocks)
	err = keyEditor.DeleteKey(keyRef, false)
	if err != nil {
		t.Fatalf("DeleteKey with big data failed: %v", err)
	}

	t.Log("Successfully deleted key with big-data value")
}

// Test_RealHive_DeleteKey_ErrorPaths tests error handling in delete operations.
func Test_RealHive_DeleteKey_ErrorPaths(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)

	// Test with very large invalid ref (beyond hive bounds)
	err := keyEditor.DeleteKey(0xFFFFFF00, false)
	if err == nil {
		t.Error("Expected error for out-of-bounds ref, got nil")
	}

	t.Logf("Correctly handled out-of-bounds ref: %v", err)
}

// Test_decodeNKName tests UTF-16 name decoding.
func Test_decodeNKName(t *testing.T) {
	tests := []struct {
		name       string
		nameBytes  []byte
		compressed bool
		expected   string
	}{
		{
			name:       "ASCII compressed",
			nameBytes:  []byte("TestKey"),
			compressed: true,
			expected:   "TestKey",
		},
		{
			name: "UTF-16LE uncompressed",
			nameBytes: []byte{
				0x54,
				0x00,
				0x65,
				0x00,
				0x73,
				0x00,
				0x74,
				0x00,
			}, // "Test" in UTF-16LE
			compressed: false,
			expected:   "Test",
		},
		{
			name:       "UTF-16LE odd length fallback",
			nameBytes:  []byte{0x54, 0x00, 0x65}, // Truncated UTF-16
			compressed: false,
			expected:   "T\x00e", // Falls back to ASCII
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeNKName(tt.nameBytes, tt.compressed)
			if result != tt.expected {
				t.Errorf("decodeNKName() = %q, want %q", result, tt.expected)
			}
		})
	}
}
