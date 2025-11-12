package e2e

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/bigdata"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/edit"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/values"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
)

const testHivePath = "../../testdata/suite/windows-2003-server-system"

// Test_Integration_CompleteWorkflow tests a complete end-to-end workflow:
// 1. Open real hive
// 2. Create key hierarchy
// 3. Add values of all types (inline, external, big-data)
// 4. Read values back
// 5. Update values
// 6. Delete values.
func Test_Integration_CompleteWorkflow(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Step 1: Create nested key hierarchy
	t.Log("Step 1: Creating key hierarchy")
	testPath := []string{"_IntegrationTest", "Level1", "Level2", "Level3"}
	leafRef, keysCreated, err := keyEditor.EnsureKeyPath(rootRef, testPath)
	if err != nil {
		t.Fatalf("EnsureKeyPath failed: %v", err)
	}
	if keysCreated == 0 {
		t.Log("Keys already existed (expected for unique path)")
	}

	// Verify each level exists in index
	currentRef := rootRef
	for i, segment := range testPath {
		nextRef, ok := idx.GetNK(currentRef, strings.ToLower(segment))
		if !ok {
			t.Fatalf("Key '%s' (level %d) not found in index", segment, i)
		}
		currentRef = nextRef
	}
	if currentRef != leafRef {
		t.Errorf("Final ref mismatch: got 0x%X, want 0x%X", currentRef, leafRef)
	}

	// Step 2: Add values of different types
	t.Log("Step 2: Adding values")

	// Inline value (≤4 bytes)
	err = valueEditor.UpsertValue(
		leafRef,
		"InlineDword",
		format.REGDWORD,
		[]byte{0x01, 0x02, 0x03, 0x04},
	)
	if err != nil {
		t.Fatalf("UpsertValue (inline) failed: %v", err)
	}

	// External value (5-1024 bytes)
	externalData := bytes.Repeat([]byte{0xAB}, 1024)
	err = valueEditor.UpsertValue(leafRef, "ExternalBinary", format.REGBinary, externalData)
	if err != nil {
		t.Fatalf("UpsertValue (external) failed: %v", err)
	}

	// Big-data value (>16KB)
	bigData := bytes.Repeat([]byte{0xCD}, 20*1024)
	err = valueEditor.UpsertValue(leafRef, "BigDataValue", format.REGBinary, bigData)
	if err != nil {
		t.Fatalf("UpsertValue (big-data) failed: %v", err)
	}

	// String value
	stringData := []byte("Test String Value\x00")
	err = valueEditor.UpsertValue(leafRef, "StringValue", format.REGSZ, stringData)
	if err != nil {
		t.Fatalf("UpsertValue (string) failed: %v", err)
	}

	// QWORD value
	qwordData := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	err = valueEditor.UpsertValue(leafRef, "QwordValue", format.REGQWORD, qwordData)
	if err != nil {
		t.Fatalf("UpsertValue (qword) failed: %v", err)
	}

	// Default value (empty name)
	err = valueEditor.UpsertValue(leafRef, "", format.REGSZ, []byte("Default Value\x00"))
	if err != nil {
		t.Fatalf("UpsertValue (default) failed: %v", err)
	}

	// Step 3: Verify all values exist in index
	t.Log("Step 3: Verifying values in index")
	expectedValues := []string{
		"inlinedword",
		"externalbinary",
		"bigdatavalue",
		"stringvalue",
		"qwordvalue",
		"",
	}
	for _, name := range expectedValues {
		_, ok := idx.GetVK(leafRef, name)
		if !ok {
			t.Errorf("Value '%s' not found in index", name)
		}
	}

	// Step 4: Update existing values
	t.Log("Step 4: Updating values")
	updatedData := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	err = valueEditor.UpsertValue(leafRef, "InlineDword", format.REGDWORD, updatedData)
	if err != nil {
		t.Fatalf("UpsertValue (update) failed: %v", err)
	}

	// Step 5: Delete a value
	t.Log("Step 5: Deleting value")
	err = valueEditor.DeleteValue(leafRef, "StringValue")
	if err != nil {
		t.Fatalf("DeleteValue failed: %v", err)
	}

	// Verify idempotency
	err = valueEditor.DeleteValue(leafRef, "StringValue")
	if err != nil {
		t.Fatalf("DeleteValue (idempotent) should succeed, got: %v", err)
	}

	// Step 6: Test no-op update
	t.Log("Step 6: Testing no-op update")
	err = valueEditor.UpsertValue(leafRef, "InlineDword", format.REGDWORD, updatedData)
	if err != nil {
		t.Fatalf("UpsertValue (no-op) failed: %v", err)
	}

	t.Log("Complete workflow test passed!")
}

// Test_Integration_SubkeysWriter tests subkeys writer functions.
func Test_Integration_SubkeysWriter(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create parent key
	parentRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_SubkeysTest"})
	if err != nil {
		t.Fatalf("Failed to create parent key: %v", err)
	}

	// Create multiple child keys to test different list formats
	t.Log("Creating child keys to test subkeys writer")

	// Create 15 children to force LH format (>12 entries)
	for i := range 15 {
		childName := []string{"_SubkeysTest", t.Name() + "_Child_" + string(rune('A'+i))}
		_, _, ensureErr := keyEditor.EnsureKeyPath(rootRef, childName)
		if ensureErr != nil {
			t.Fatalf("Failed to create child %d: %v", i, ensureErr)
		}
	}

	// Verify parent has 15 children
	nkBuf, err := h.ResolveCellPayload(parentRef)
	if err != nil {
		t.Fatalf("Failed to resolve parent NK: %v", err)
	}

	subkeyCount := format.ReadU32(nkBuf, format.NKSubkeyCountOffset)
	if subkeyCount != 15 {
		t.Errorf("Expected 15 subkeys, got %d", subkeyCount)
	}

	// Read subkey list and verify format
	subkeyListOff := format.ReadU32(nkBuf, format.NKSubkeyListOffset)
	if subkeyListOff == 0xFFFFFFFF {
		t.Fatal("Subkey list offset is nil")
	}

	listBuf, err := h.ResolveCellPayload(subkeyListOff)
	if err != nil {
		t.Fatalf("Failed to resolve subkey list: %v", err)
	}

	signature := string(listBuf[0:2])
	t.Logf("Subkey list format: %s", signature)

	// With 15 entries, should be LH format
	if signature != "lh" && signature != "lf" {
		t.Errorf("Expected LH or LF format with 15 entries, got %s", signature)
	}
}

// Test_Integration_ValuesWriter tests values writer functions.
func Test_Integration_ValuesWriter(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create test key
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_ValuesWriterTest"})
	if err != nil {
		t.Fatalf("Failed to create test key: %v", err)
	}

	// Test values.Write() by creating value list manually
	t.Log("Testing values writer")

	// Create multiple values
	for i := range 10 {
		valueName := t.Name() + "_Value_" + string(rune('A'+i))
		valueData := []byte{byte(i), byte(i + 1), byte(i + 2), byte(i + 3)}
		upsertErr := valueEditor.UpsertValue(keyRef, valueName, format.REGDWORD, valueData)
		if upsertErr != nil {
			t.Fatalf("Failed to create value %d: %v", i, upsertErr)
		}
	}

	// Read NK and verify value count
	nkBuf, err := h.ResolveCellPayload(keyRef)
	if err != nil {
		t.Fatalf("Failed to resolve NK: %v", err)
	}

	valueCount := format.ReadU32(nkBuf, format.NKValueCountOffset)
	if valueCount != 10 {
		t.Errorf("Expected 10 values, got %d", valueCount)
	}

	// Read value list
	valueListOff := format.ReadU32(nkBuf, format.NKValueListOffset)
	if valueListOff == 0xFFFFFFFF {
		t.Fatal("Value list offset is nil")
	}

	// Use values.Read() to read the list
	// First, wrap keyRef in an NK view
	nkBuf2, err := h.ResolveCellPayload(keyRef)
	if err != nil {
		t.Fatalf("Failed to resolve NK for values.Read(): %v", err)
	}
	nk, err := hive.ParseNK(nkBuf2)
	if err != nil {
		t.Fatalf("Failed to parse NK: %v", err)
	}

	valueList, err := values.Read(h, nk)
	if err != nil {
		t.Fatalf("values.Read() failed: %v", err)
	}

	if valueList.Len() != 10 {
		t.Errorf("Expected value list length 10, got %d", valueList.Len())
	}

	t.Log("Values writer test passed!")
}

// Test_Integration_BigDataWriter tests bigdata writer with various sizes.
func Test_Integration_BigDataWriter(t *testing.T) {
	h, allocator, _, dt, cleanup := setupRealHive(t)
	defer cleanup()

	writer := bigdata.NewWriter(h, allocator, dt)

	testCases := []struct {
		name string
		size int
	}{
		{"17KB", 17 * 1024},
		{"20KB", 20 * 1024},
		{"50KB", 50 * 1024},
		{"100KB", 100 * 1024},
		{"200KB", 200 * 1024},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test data
			testData := make([]byte, tc.size)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Store big-data
			dbRef, err := writer.Store(testData)
			if err != nil {
				t.Fatalf("bigdata.Store() failed for %s: %v", tc.name, err)
			}

			t.Logf("Stored %s at ref 0x%X", tc.name, dbRef)

			// Read DB header
			dbBuf, err := h.ResolveCellPayload(dbRef)
			if err != nil {
				t.Fatalf("Failed to resolve DB header: %v", err)
			}

			dbHeader, err := bigdata.ReadDBHeader(dbBuf)
			if err != nil {
				t.Fatalf("ReadDBHeader failed: %v", err)
			}

			// Verify block count is reasonable
			expectedBlocks := (tc.size + bigdata.MaxBlockSize - 1) / bigdata.MaxBlockSize
			if dbHeader.Count != uint16(expectedBlocks) {
				t.Errorf("Expected %d blocks, got %d", expectedBlocks, dbHeader.Count)
			}

			// Read blocklist
			blocklistBuf, err := h.ResolveCellPayload(dbHeader.Blocklist)
			if err != nil {
				t.Fatalf("Failed to resolve blocklist: %v", err)
			}

			blockRefs, err := bigdata.ReadBlocklist(blocklistBuf, dbHeader.Count)
			if err != nil {
				t.Fatalf("ReadBlocklist failed: %v", err)
			}

			if len(blockRefs) != int(dbHeader.Count) {
				t.Errorf("Expected %d block refs, got %d", dbHeader.Count, len(blockRefs))
			}

			// Verify each block is accessible
			for i, blockRef := range blockRefs {
				blockBuf, resolveErr := h.ResolveCellPayload(blockRef)
				if resolveErr != nil {
					t.Errorf("Failed to resolve block %d: %v", i, resolveErr)
				}
				if len(blockBuf) == 0 {
					t.Errorf("Block %d is empty", i)
				}
			}
		})
	}
}

// Test_Integration_Walker tests the walker functionality.
func Test_Integration_Walker(t *testing.T) {
	h, _, _, _, cleanup := setupRealHive(t)
	defer cleanup()

	t.Log("Testing walker on real hive")

	// Test WalkReferences
	cellCount := 0

	err := walker.NewValidationWalker(h).Walk(func(_ walker.CellRef) error {
		cellCount++
		// Just count cells for now
		return nil
	})
	if err != nil {
		t.Fatalf("WalkReferences failed: %v", err)
	}

	// Real hive should have many cells
	if cellCount == 0 {
		t.Error("Expected to find cells in real hive")
	}

	t.Logf("Walker visited %d cells", cellCount)
}

// Test_Integration_EdgeCases tests edge cases and boundary conditions.
func Test_Integration_EdgeCases(t *testing.T) {
	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	t.Run("EmptyValue", func(t *testing.T) {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", "EmptyValue"})
		if err != nil {
			t.Fatal(err)
		}

		// Create value with empty data
		err = valueEditor.UpsertValue(keyRef, "EmptyValue", format.REGSZ, []byte{})
		if err != nil {
			t.Fatalf("Failed to create empty value: %v", err)
		}
	})

	t.Run("MaxInlineValue", func(t *testing.T) {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", "MaxInline"})
		if err != nil {
			t.Fatal(err)
		}

		// Create value with exactly 4 bytes (max inline)
		err = valueEditor.UpsertValue(
			keyRef,
			"MaxInline",
			format.REGDWORD,
			[]byte{0xFF, 0xFF, 0xFF, 0xFF},
		)
		if err != nil {
			t.Fatalf("Failed to create max inline value: %v", err)
		}
	})

	t.Run("MinExternalValue", func(t *testing.T) {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", "MinExternal"})
		if err != nil {
			t.Fatal(err)
		}

		// Create value with exactly 5 bytes (min external)
		err = valueEditor.UpsertValue(
			keyRef,
			"MinExternal",
			format.REGBinary,
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05},
		)
		if err != nil {
			t.Fatalf("Failed to create min external value: %v", err)
		}
	})

	t.Run("BoundaryValue", func(t *testing.T) {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", "Boundary"})
		if err != nil {
			t.Fatal(err)
		}

		// Create value at exact boundary (16344 bytes)
		boundaryData := bytes.Repeat([]byte{0xAB}, 16344)
		err = valueEditor.UpsertValue(keyRef, "Boundary", format.REGBinary, boundaryData)
		if err != nil {
			t.Fatalf("Failed to create boundary value: %v", err)
		}
	})

	t.Run("BeyondBoundary", func(t *testing.T) {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", "BeyondBoundary"})
		if err != nil {
			t.Fatal(err)
		}

		// Create value just beyond boundary (16345 bytes = min big-data)
		beyondData := bytes.Repeat([]byte{0xCD}, 16345)
		err = valueEditor.UpsertValue(keyRef, "BeyondBoundary", format.REGBinary, beyondData)
		if err != nil {
			t.Fatalf("Failed to create beyond-boundary value: %v", err)
		}
	})

	t.Run("LongKeyName", func(t *testing.T) {
		// Create key with long name (Windows limit is 255 chars)
		longName := strings.Repeat("A", 200)
		_, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", longName})
		if err != nil {
			t.Fatalf("Failed to create key with long name: %v", err)
		}
	})

	t.Run("LongValueName", func(t *testing.T) {
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_EdgeCases", "LongValueName"})
		if err != nil {
			t.Fatal(err)
		}

		// Create value with long name
		longValueName := strings.Repeat("V", 200)
		err = valueEditor.UpsertValue(keyRef, longValueName, format.REGSZ, []byte("test\x00"))
		if err != nil {
			t.Fatalf("Failed to create value with long name: %v", err)
		}
	})

	t.Run("DeepNesting", func(t *testing.T) {
		// Create deeply nested key hierarchy (20 levels)
		path := make([]string, 20)
		for i := range 20 {
			path[i] = "_EdgeCases_Deep_" + string(rune('A'+i))
		}

		_, _, err := keyEditor.EnsureKeyPath(rootRef, path)
		if err != nil {
			t.Fatalf("Failed to create deeply nested keys: %v", err)
		}
	})
}

// Test_Integration_StressTest creates many keys and values to stress test the system.
func Test_Integration_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	h, allocator, idx, dt, cleanup := setupRealHive(t)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create stress test root
	stressRoot, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_StressTest"})
	if err != nil {
		t.Fatalf("Failed to create stress test root: %v", err)
	}

	t.Log("Creating 100 keys with 10 values each...")

	// Create 100 keys, each with 10 values
	for i := range 100 {
		keyName := t.Name() + "_Key_" + string(rune('0'+(i/10))) + string(rune('0'+(i%10)))
		keyRef, _, ensureErr := keyEditor.EnsureKeyPath(stressRoot, []string{keyName})
		if ensureErr != nil {
			t.Fatalf("Failed to create key %d: %v", i, ensureErr)
		}

		// Add 10 values to each key
		for j := range 10 {
			valueName := "Value" + string(rune('0'+j))
			valueData := []byte{byte(i), byte(j), 0x00, 0x00}
			upsertErr := valueEditor.UpsertValue(keyRef, valueName, format.REGDWORD, valueData)
			if upsertErr != nil {
				t.Fatalf("Failed to create value %d for key %d: %v", j, i, upsertErr)
			}
		}

		if i%25 == 0 {
			t.Logf("Progress: %d/100 keys created", i)
		}
	}

	t.Log("Stress test completed: 100 keys with 1000 values created!")
}

// setupRealHive opens a real Windows hive for testing.
func setupRealHive(t *testing.T) (*hive.Hive, *alloc.FastAllocator, index.Index, *dirty.Tracker, func()) {
	t.Helper()

	// Check if test hive exists
	if _, err := os.Stat(testHivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", testHivePath)
	}

	// Create a copy in temp dir
	tempDir := t.TempDir()
	tempHivePath := filepath.Join(tempDir, "test-hive")

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

	// Create dirty tracker
	dt := dirty.NewTracker(h)

	// Create allocator
	allocator, err := alloc.NewFast(h, dt, nil)
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Create index
	idx := index.NewStringIndex(10000, 10000)

	cleanup := func() {
		h.Close()
	}

	return h, allocator, idx, dt, cleanup
}
