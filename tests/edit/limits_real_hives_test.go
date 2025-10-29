package edit_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/pkg/ast"
	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestLimitsValidation_RealHives tests that real, valid Windows registry hives
// pass validation with DefaultLimits.
func TestLimitsValidation_RealHives(t *testing.T) {
	testCases := []struct {
		name     string
		hivePath string
	}{
		{"minimal", "../../testdata/minimal"},
		{"large", "../../testdata/large"},
		{"special", "../../testdata/special"},
		{"rlenvalue", "../../testdata/rlenvalue_test_hive"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Fatalf("Failed to read test hive: %v", err)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			// Create editor with default limits
			ed := edit.NewEditor(r)
			tx := ed.Begin() // Uses DefaultLimits

			// Make a simple modification
			if err := tx.CreateKey("TestKey", hive.CreateKeyOptions{}); err != nil {
				t.Fatalf("Failed to create key: %v", err)
			}
			tx.SetValue("TestKey", "TestValue", hive.REG_SZ, []byte("test data"))

			// Should succeed with valid hive and minimal changes
			buf := &bytes.Buffer{}
			if err := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); err != nil {
				t.Fatalf("Commit failed on real hive %s: %v", tc.name, err)
			}

			// Verify we got output
			if buf.Len() == 0 {
				t.Error("Expected non-empty buffer after commit")
			}
		})
	}
}

// TestLimitsValidation_LargeHive specifically tests the large hive
// to ensure it validates correctly with relaxed limits.
func TestLimitsValidation_LargeHive(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Fatalf("Failed to read large hive: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer r.Close()

	// Test with relaxed limits (should pass)
	ed := edit.NewEditor(r)
	tx := ed.BeginWithLimits(ast.RelaxedLimits())

	// Make multiple modifications
	for i := 0; i < 50; i++ {
		keyName := "BulkTestKey" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		if err := tx.CreateKey(keyName, hive.CreateKeyOptions{}); err != nil {
			t.Fatalf("Failed to create key %s: %v", keyName, err)
		}
		tx.SetValue(keyName, "Value1", hive.REG_SZ, []byte("test data"))
		tx.SetValue(keyName, "Value2", hive.REG_DWORD, []byte{0x01, 0x02, 0x03, 0x04})
	}

	// Should succeed with relaxed limits
	buf := &bytes.Buffer{}
	if err := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed with relaxed limits: %v", err)
	}
}

// TestLimitsValidation_StrictLimits_LargeHive tests that strict limits
// catch violations on large hives with many modifications.
func TestLimitsValidation_StrictLimits_LargeHive(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Fatalf("Failed to read large hive: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer r.Close()

	// Test with strict limits
	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxTotalSize = 100 << 10 // 100 KB - very small for the large hive

	tx := ed.BeginWithLimits(limits)

	// Try to add many keys (should eventually hit size limit)
	for i := 0; i < 1000; i++ {
		keyName := "Key" + string(rune('A'+i%26)) + string(rune('0'+(i/26)%10)) + string(rune('0'+i/260))
		if err := tx.CreateKey(keyName, hive.CreateKeyOptions{}); err != nil {
			// Continue even if create fails
			continue
		}
		// Add multiple large values
		tx.SetValue(keyName, "LargeValue1", hive.REG_BINARY, make([]byte, 1000))
		tx.SetValue(keyName, "LargeValue2", hive.REG_BINARY, make([]byte, 1000))
	}

	// Should fail due to size limit
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxTotalSize limit")
	}

	if !strings.Contains(err.Error(), "MaxTotalSize") {
		t.Errorf("Expected MaxTotalSize error, got: %v", err)
	}
}

// TestLimitsValidation_ValueNameLength tests that value name length
// limits are enforced on real hives.
func TestLimitsValidation_ValueNameLength(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.DefaultLimits()
	limits.MaxValueNameLen = 20 // Short limit
	tx := ed.BeginWithLimits(limits)

	if err := tx.CreateKey("TestKey", hive.CreateKeyOptions{}); err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	// Add value with name exceeding limit
	longValueName := strings.Repeat("a", 21)
	tx.SetValue("TestKey", longValueName, hive.REG_SZ, []byte("data"))

	// Should fail
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxValueNameLen limit")
	}

	if !strings.Contains(err.Error(), "MaxValueNameLen") {
		t.Errorf("Expected MaxValueNameLen error, got: %v", err)
	}
}

// TestLimitsValidation_MultipleViolations tests that we get the first
// violation encountered in the tree walk.
func TestLimitsValidation_MultipleViolations(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxSubkeys = 2
	limits.MaxValues = 1
	limits.MaxValueSize = 50
	tx := ed.BeginWithLimits(limits)

	// Create multiple violations
	if err := tx.CreateKey("Key1", hive.CreateKeyOptions{}); err != nil {
		t.Fatalf("Failed to create key1: %v", err)
	}
	if err := tx.CreateKey("Key2", hive.CreateKeyOptions{}); err != nil {
		t.Fatalf("Failed to create key2: %v", err)
	}
	if err := tx.CreateKey("Key3", hive.CreateKeyOptions{}); err != nil {
		t.Fatalf("Failed to create key3: %v", err)
	}

	// Add too many values to Key1
	tx.SetValue("Key1", "Val1", hive.REG_SZ, []byte("data"))
	tx.SetValue("Key1", "Val2", hive.REG_SZ, []byte("data"))

	// Add too large value to Key2
	tx.SetValue("Key2", "LargeVal", hive.REG_BINARY, make([]byte, 51))

	// Should fail - we should get at least one error
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to limit violations")
	}

	// Should contain one of: MaxSubkeys, MaxValues, or MaxValueSize
	errStr := err.Error()
	hasLimitError := strings.Contains(errStr, "MaxSubkeys") ||
		strings.Contains(errStr, "MaxValues") ||
		strings.Contains(errStr, "MaxValueSize")

	if !hasLimitError {
		t.Errorf("Expected limit error, got: %v", err)
	}
}

// TestLimitsValidation_NoChanges tests that validation works even
// when there are no changes (validates base hive).
func TestLimitsValidation_NoChanges(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.Begin() // Default limits

	// Commit without any changes
	buf := &bytes.Buffer{}
	if err := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed with no changes: %v", err)
	}

	// Should succeed
	if buf.Len() == 0 {
		t.Error("Expected non-empty buffer")
	}
}

// TestLimitsValidation_DeepNesting tests tree depth limits.
func TestLimitsValidation_DeepNesting(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.DefaultLimits()
	limits.MaxTreeDepth = 10 // Shallow limit
	tx := ed.BeginWithLimits(limits)

	// Create a deep path
	deepPath := "L1\\L2\\L3\\L4\\L5\\L6\\L7\\L8\\L9\\L10\\L11"
	if err := tx.CreateKey(deepPath, hive.CreateKeyOptions{CreateParents: true}); err != nil {
		t.Fatalf("Failed to create deep path: %v", err)
	}

	// Should fail due to depth
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxTreeDepth limit")
	}

	if !strings.Contains(err.Error(), "MaxTreeDepth") {
		t.Errorf("Expected MaxTreeDepth error, got: %v", err)
	}
}

// TestLimitsValidation_ExactAtLimit tests that we can use exactly the limit.
func TestLimitsValidation_ExactAtLimit(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxSubkeys = 5
	limits.MaxValues = 3
	tx := ed.BeginWithLimits(limits)

	// Create exactly MaxSubkeys subkeys
	for i := 0; i < 5; i++ {
		keyName := "Key" + string(rune('0'+i))
		if err := tx.CreateKey(keyName, hive.CreateKeyOptions{}); err != nil {
			t.Fatalf("Failed to create key%d: %v", i, err)
		}
	}

	// Add exactly MaxValues values to first key
	tx.SetValue("Key0", "Val0", hive.REG_SZ, []byte("data"))
	tx.SetValue("Key0", "Val1", hive.REG_SZ, []byte("data"))
	tx.SetValue("Key0", "Val2", hive.REG_SZ, []byte("data"))

	// Should succeed (at limit, not over)
	buf := &bytes.Buffer{}
	if err := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed at exact limit: %v", err)
	}
}
