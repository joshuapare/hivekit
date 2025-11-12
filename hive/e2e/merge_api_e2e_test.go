package e2e

import (
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_MergePlan_OneLiner tests the simple one-liner API for applying plans.
func Test_MergePlan_OneLiner(t *testing.T) {
	// Setup: Copy test hive to temp location
	tempDir := t.TempDir()
	hivePath := filepath.Join(tempDir, "test.hive")
	if err := copyFile("../../testdata/suite/windows-2003-server-system", hivePath); err != nil {
		t.Fatalf("Failed to copy test hive: %v", err)
	}

	// Create a plan
	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "APITest", "Simple"})
	plan.AddSetValue([]string{"Software", "APITest", "Simple"}, "TestValue", format.REGSZ, []byte("Hello\x00"))
	plan.AddSetValue(
		[]string{"Software", "APITest", "Simple"},
		"Count",
		format.REGDWORD,
		[]byte{0x2A, 0x00, 0x00, 0x00},
	)

	// Apply plan using one-liner
	applied, err := merge.MergePlan(hivePath, plan, nil)
	if err != nil {
		t.Fatalf("MergePlan failed: %v", err)
	}

	// Verify statistics
	if applied.KeysCreated != 3 {
		t.Errorf("Expected 3 keys created (Software, APITest, Simple), got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 2 {
		t.Errorf("Expected 2 values set, got %d", applied.ValuesSet)
	}

	// Verify by reopening hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Verify key exists
	rootRef := h.RootCellOffset()
	if !verifyKeyExists(t, h, rootRef, []string{"Software", "APITest", "Simple"}) {
		t.Error("Expected key was not found after merge")
	}

	// Verify values exist
	if !verifyValueExists(t, h, rootRef, []string{"Software", "APITest", "Simple"}, "TestValue") {
		t.Error("TestValue was not found after merge")
	}
	if !verifyValueExists(t, h, rootRef, []string{"Software", "APITest", "Simple"}, "Count") {
		t.Error("Count value was not found after merge")
	}
}

// Test_MergeRegText_OneLiner tests parsing .reg text and applying it.
func Test_MergeRegText_OneLiner(t *testing.T) {
	// Setup: Copy test hive to temp location
	tempDir := t.TempDir()
	hivePath := filepath.Join(tempDir, "test.hive")
	if err := copyFile("../../testdata/suite/windows-2003-server-system", hivePath); err != nil {
		t.Fatalf("Failed to copy test hive: %v", err)
	}

	// .reg text to apply
	regText := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\RegTextTest]
"StringValue"="Test String"
"DwordValue"=dword:00000042
"BinaryValue"=hex:01,02,03,04,05

[HKEY_LOCAL_MACHINE\Software\RegTextTest\SubKey]
"NestedValue"="Nested"
`

	// Apply using one-liner
	applied, err := merge.MergeRegText(hivePath, regText, nil)
	if err != nil {
		t.Fatalf("MergeRegText failed: %v", err)
	}

	// Verify statistics
	if applied.KeysCreated != 3 {
		t.Errorf("Expected 3 keys created (Software, RegTextTest, SubKey), got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 4 {
		t.Errorf("Expected 4 values set, got %d", applied.ValuesSet)
	}

	// Verify by reopening hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Verify keys exist
	rootRef := h.RootCellOffset()
	if !verifyKeyExists(t, h, rootRef, []string{"Software", "RegTextTest"}) {
		t.Error("RegTextTest key was not found")
	}
	if !verifyKeyExists(t, h, rootRef, []string{"Software", "RegTextTest", "SubKey"}) {
		t.Error("SubKey was not found")
	}

	// Verify values exist
	if !verifyValueExists(t, h, rootRef, []string{"Software", "RegTextTest"}, "StringValue") {
		t.Error("StringValue was not found")
	}
	if !verifyValueExists(t, h, rootRef, []string{"Software", "RegTextTest"}, "DwordValue") {
		t.Error("DwordValue was not found")
	}
	if !verifyValueExists(t, h, rootRef, []string{"Software", "RegTextTest"}, "BinaryValue") {
		t.Error("BinaryValue was not found")
	}
	if !verifyValueExists(t, h, rootRef, []string{"Software", "RegTextTest", "SubKey"}, "NestedValue") {
		t.Error("NestedValue was not found")
	}
}

// Test_WithSession_MultipleOps tests the callback pattern for multiple operations.
func Test_WithSession_MultipleOps(t *testing.T) {
	// Setup: Copy test hive to temp location
	tempDir := t.TempDir()
	hivePath := filepath.Join(tempDir, "test.hive")
	if err := copyFile("../../testdata/suite/windows-2003-server-system", hivePath); err != nil {
		t.Fatalf("Failed to copy test hive: %v", err)
	}

	// Use WithSession to perform multiple operations
	var totalKeysCreated int
	var totalValuesSet int

	err := merge.WithSession(hivePath, nil, func(s *merge.Session) error {
		// Operation 1: Create first key tree
		plan1 := merge.NewPlan()
		plan1.AddEnsureKey([]string{"Software", "WithSessionTest", "Op1"})
		plan1.AddSetValue([]string{"Software", "WithSessionTest", "Op1"}, "Value1", format.REGSZ, []byte("First\x00"))

		applied1, err := s.ApplyWithTx(plan1)
		if err != nil {
			return err
		}
		totalKeysCreated += applied1.KeysCreated
		totalValuesSet += applied1.ValuesSet

		// Operation 2: Create second key tree
		plan2 := merge.NewPlan()
		plan2.AddEnsureKey([]string{"Software", "WithSessionTest", "Op2"})
		plan2.AddSetValue([]string{"Software", "WithSessionTest", "Op2"}, "Value2", format.REGSZ, []byte("Second\x00"))

		applied2, err := s.ApplyWithTx(plan2)
		if err != nil {
			return err
		}
		totalKeysCreated += applied2.KeysCreated
		totalValuesSet += applied2.ValuesSet

		// Operation 3: Update existing key
		plan3 := merge.NewPlan()
		plan3.AddSetValue(
			[]string{"Software", "WithSessionTest", "Op1"},
			"Value1Updated",
			format.REGDWORD,
			[]byte{0xFF, 0x00, 0x00, 0x00},
		)

		applied3, err := s.ApplyWithTx(plan3)
		if err != nil {
			return err
		}
		totalKeysCreated += applied3.KeysCreated
		totalValuesSet += applied3.ValuesSet

		return nil
	})

	if err != nil {
		t.Fatalf("WithSession failed: %v", err)
	}

	// Verify statistics
	// First plan creates: Software, WithSessionTest, Op1 (3 keys)
	// Second plan creates: Op2 (1 key, Software and WithSessionTest already exist)
	// Third plan creates: 0 keys (all exist)
	// Total: 4 keys
	if totalKeysCreated != 4 {
		t.Errorf("Expected 4 total keys created, got %d", totalKeysCreated)
	}
	if totalValuesSet != 3 {
		t.Errorf("Expected 3 total values set, got %d", totalValuesSet)
	}

	// Verify by reopening hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Verify all keys exist
	rootRef := h.RootCellOffset()
	if !verifyKeyExists(t, h, rootRef, []string{"Software", "WithSessionTest", "Op1"}) {
		t.Error("Op1 key was not found")
	}
	if !verifyKeyExists(t, h, rootRef, []string{"Software", "WithSessionTest", "Op2"}) {
		t.Error("Op2 key was not found")
	}

	// Verify all values exist
	if !verifyValueExists(t, h, rootRef, []string{"Software", "WithSessionTest", "Op1"}, "Value1") {
		t.Error("Value1 was not found")
	}
	if !verifyValueExists(t, h, rootRef, []string{"Software", "WithSessionTest", "Op1"}, "Value1Updated") {
		t.Error("Value1Updated was not found")
	}
	if !verifyValueExists(t, h, rootRef, []string{"Software", "WithSessionTest", "Op2"}, "Value2") {
		t.Error("Value2 was not found")
	}
}

// Test_PlanFromRegText_PathParsing tests that .reg paths are parsed correctly.
func Test_PlanFromRegText_PathParsing(t *testing.T) {
	tests := []struct {
		name         string
		regText      string
		expectedOps  int
		expectedKeys [][]string // Expected key paths
	}{
		{
			name: "HKEY_LOCAL_MACHINE prefix",
			regText: `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Test]
"Value"="Test"
`,
			expectedOps:  2, // CreateKey + SetValue
			expectedKeys: [][]string{{"Software", "Test"}},
		},
		{
			name: "HKLM prefix",
			regText: `Windows Registry Editor Version 5.00

[HKLM\System\Test]
"Value"="Test"
`,
			expectedOps:  2,
			expectedKeys: [][]string{{"System", "Test"}},
		},
		{
			name: "Multiple levels",
			regText: `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Company\Product\Version]
"Major"=dword:00000001
"Minor"=dword:00000000
`,
			expectedOps:  3, // CreateKey + 2 SetValues
			expectedKeys: [][]string{{"Software", "Company", "Product", "Version"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := merge.PlanFromRegText(tt.regText)
			if err != nil {
				t.Fatalf("PlanFromRegText failed: %v", err)
			}

			if len(plan.Ops) != tt.expectedOps {
				t.Errorf("Expected %d operations, got %d", tt.expectedOps, len(plan.Ops))
			}

			// Verify key paths were parsed correctly
			for _, expectedPath := range tt.expectedKeys {
				found := false
				for _, op := range plan.Ops {
					if op.Type == merge.OpEnsureKey {
						if pathsEqual(op.KeyPath, expectedPath) {
							found = true
							break
						}
					}
				}
				if !found {
					t.Errorf("Expected key path %v not found in plan", expectedPath)
				}
			}
		})
	}
}

// Test_MergePlan_ErrorHandling tests error handling in the API.
func Test_MergePlan_ErrorHandling(t *testing.T) {
	// Test 1: Non-existent hive file
	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "Test"})

	_, err := merge.MergePlan("/nonexistent/path/to/hive", plan, nil)
	if err == nil {
		t.Error("Expected error for non-existent hive file")
	}

	// Test 2: Malformed .reg text
	tempDir := t.TempDir()
	hivePath := filepath.Join(tempDir, "test.hive")
	if copyErr := copyFile("../../testdata/suite/windows-2003-server-system", hivePath); copyErr != nil {
		t.Fatalf("Failed to copy test hive: %v", copyErr)
	}

	malformedReg := `This is not valid .reg text`
	_, err = merge.MergeRegText(hivePath, malformedReg, nil)
	if err == nil {
		t.Error("Expected error for malformed .reg text")
	}
}

// Test_MergePlan_LargeValue tests that large values work with the API.
func Test_MergePlan_LargeValue(t *testing.T) {
	// Setup: Copy test hive to temp location
	tempDir := t.TempDir()
	hivePath := filepath.Join(tempDir, "test.hive")
	if err := copyFile("../../testdata/suite/windows-2003-server-system", hivePath); err != nil {
		t.Fatalf("Failed to copy test hive: %v", err)
	}

	// Create a plan with a large value (25KB - requires DB format)
	largeData := make([]byte, 25*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	plan := merge.NewPlan()
	plan.AddEnsureKey([]string{"Software", "LargeValueTest"})
	plan.AddSetValue([]string{"Software", "LargeValueTest"}, "BigData", format.REGBinary, largeData)

	// Apply using MergePlan
	applied, err := merge.MergePlan(hivePath, plan, nil)
	if err != nil {
		t.Fatalf("MergePlan with large value failed: %v", err)
	}

	if applied.KeysCreated != 2 {
		t.Errorf("Expected 2 keys created (Software, LargeValueTest), got %d", applied.KeysCreated)
	}
	if applied.ValuesSet != 1 {
		t.Errorf("Expected 1 value set, got %d", applied.ValuesSet)
	}

	// Verify by reopening hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h.Close()

	// Verify key exists
	rootRef := h.RootCellOffset()
	if !verifyKeyExists(t, h, rootRef, []string{"Software", "LargeValueTest"}) {
		t.Error("LargeValueTest key was not found")
	}

	// Verify value exists
	if !verifyValueExists(t, h, rootRef, []string{"Software", "LargeValueTest"}, "BigData") {
		t.Error("BigData value was not found")
	}
}

// Helper function to compare two path slices.
func pathsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
