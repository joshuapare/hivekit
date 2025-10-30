package merge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestMergeRegFile_EmptyOperations tests merging a .reg file with no operations (just header).
func TestMergeRegFile_EmptyOperations(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "empty.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create empty .reg (just header, no operations)
	regContent := "Windows Registry Editor Version 5.00\n"
	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to create .reg file: %v", err)
	}

	// Merge should succeed with zero operations
	stats, err := hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats, got nil")
	}

	if stats.OperationsTotal != 0 {
		t.Errorf("Expected 0 operations, got %d", stats.OperationsTotal)
	}

	if stats.KeysCreated != 0 {
		t.Errorf("Expected 0 keys created, got %d", stats.KeysCreated)
	}
}

// TestMergeRegFile_InputEncoding tests that InputEncoding option is passed through correctly.
func TestMergeRegFile_InputEncoding(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create simple .reg file
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
"Value"="Data"
`
	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to create .reg file: %v", err)
	}

	// Test with explicit UTF-8 encoding
	opts := &hive.MergeOptions{
		InputEncoding: "UTF-8",
	}
	stats, err := hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("MergeRegFile with UTF-8 encoding failed: %v", err)
	}

	if stats.OperationsTotal != 2 {
		t.Errorf("Expected 2 operations, got %d", stats.OperationsTotal)
	}
}

// TestMergeRegFile_OperationsFailed tests that failed operations are tracked correctly.
func TestMergeRegFile_OperationsFailed(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create .reg with operations that might fail
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
"Value1"="Data1"
"Value2"="Data2"
"Value3"="Data3"
`

	var errorCount int
	var failedOps []string

	opts := &hive.MergeOptions{
		OnError: func(op hive.EditOp, err error) bool {
			errorCount++
			// Track which operation failed
			switch v := op.(type) {
			case hive.OpSetValue:
				failedOps = append(failedOps, v.Name)
			}
			// Continue on error
			return true
		},
	}

	stats, err := hive.MergeRegString(hiveFile, regContent, opts)
	if err != nil {
		t.Fatalf("MergeRegString failed: %v", err)
	}

	// Note: In normal operation, these should all succeed
	// But the OnError callback should be invoked for any failures
	if stats.OperationsFailed != errorCount {
		t.Errorf("Expected %d failed operations in stats, got %d", errorCount, stats.OperationsFailed)
	}
}

// TestParseRegString_EmptyString tests parsing an empty string.
func TestParseRegString_EmptyString(t *testing.T) {
	ops, err := hive.ParseRegString("", hive.RegParseOptions{})
	// Parser should error on empty input OR return empty operations
	if err != nil {
		// Expected error - test passes
		return
	}
	// If no error, should return empty operations
	if len(ops) != 0 {
		t.Errorf("Expected 0 operations for empty string, got %d", len(ops))
	}
}

// TestParseRegBytes_EmptyBytes tests parsing empty bytes.
func TestParseRegBytes_EmptyBytes(t *testing.T) {
	ops, err := hive.ParseRegBytes([]byte{}, hive.RegParseOptions{})
	// Parser should error on empty input OR return empty operations
	if err != nil {
		// Expected error - test passes
		return
	}
	// If no error, should return empty operations
	if len(ops) != 0 {
		t.Errorf("Expected 0 operations for empty bytes, got %d", len(ops))
	}
}

// TestMergeRegFile_NilOptions tests that nil options are handled correctly.
func TestMergeRegFile_NilOptions(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create simple .reg file
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
"Value"="Data"
`
	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to create .reg file: %v", err)
	}

	// Test with nil options (should use defaults)
	stats, err := hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Fatalf("MergeRegFile with nil options failed: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats, got nil")
	}

	if stats.OperationsTotal != 2 {
		t.Errorf("Expected 2 operations, got %d", stats.OperationsTotal)
	}
}

// TestMergeRegString_WithComments tests that comments are handled correctly.
func TestMergeRegString_WithComments(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create .reg with lots of comments
	regContent := `Windows Registry Editor Version 5.00

; This is a comment
; Another comment

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
; Comment before value
"Value"="Data"
; Comment after value
`

	stats, err := hive.MergeRegString(hiveFile, regContent, nil)
	if err != nil {
		t.Fatalf("MergeRegString with comments failed: %v", err)
	}

	// Should have 2 operations (CreateKey + SetValue), comments ignored
	if stats.OperationsTotal != 2 {
		t.Errorf("Expected 2 operations, got %d", stats.OperationsTotal)
	}
}

// TestMergeStats_AllFields tests that all MergeStats fields are populated correctly.
func TestMergeStats_AllFields(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create .reg with various operations
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
"Value1"="Data1"
"Value2"="Data2"

[-HKEY_LOCAL_MACHINE\SOFTWARE\OldKey]

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
"DeleteThis"=-
`

	stats, err := hive.MergeRegString(hiveFile, regContent, nil)
	if err != nil {
		t.Fatalf("MergeRegString failed: %v", err)
	}

	// Verify all fields are set
	if stats.OperationsTotal == 0 {
		t.Error("OperationsTotal should not be 0")
	}
	if stats.KeysCreated == 0 {
		t.Error("KeysCreated should not be 0")
	}
	if stats.ValuesSet == 0 {
		t.Error("ValuesSet should not be 0")
	}
	if stats.KeysDeleted == 0 {
		t.Error("KeysDeleted should not be 0")
	}
	if stats.ValuesDeleted == 0 {
		t.Error("ValuesDeleted should not be 0")
	}
	if stats.BytesWritten == 0 {
		t.Error("BytesWritten should not be 0")
	}
	// OperationsFailed is 0 for successful operations
	if stats.OperationsFailed != 0 {
		t.Errorf("OperationsFailed should be 0, got %d", stats.OperationsFailed)
	}

	// Verify totals match
	expectedTotal := stats.KeysCreated + stats.ValuesSet + stats.KeysDeleted + stats.ValuesDeleted
	if stats.OperationsTotal != expectedTotal {
		t.Errorf("OperationsTotal %d != sum of operation types %d", stats.OperationsTotal, expectedTotal)
	}
}
