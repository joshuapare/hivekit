package hive_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestParseRegFile tests parsing a .reg file without applying changes.
func TestParseRegFile(t *testing.T) {
	tempDir := t.TempDir()
	regFile := filepath.Join(tempDir, "test.reg")

	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Test]",
		"\"StringValue\"=\"test\"",
		"\"DwordValue\"=dword:00000042",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Test\\SubKey]",
		"\"NestedValue\"=\"nested\"",
		"",
		"[-HKEY_LOCAL_MACHINE\\SOFTWARE\\ToDelete]",
	}, "\r\n")

	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to write test .reg file: %v", err)
	}

	// Parse with auto-prefix
	ops, err := hive.ParseRegFile(regFile, hive.RegParseOptions{
		AutoPrefix: true,
	})
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	// Verify operations
	// Parser creates: 2 CreateKey (Test, Test\SubKey), 2 SetValue, 1 DeleteKey = 6 ops
	// (nested value doesn't get separate CreateKey since parent key creates it)
	expectedOps := 6
	if len(ops) != expectedOps {
		t.Errorf("Expected %d operations, got %d", expectedOps, len(ops))
	}

	// Verify operation types
	var keysCreated, valuesSet, keysDeleted int
	for _, op := range ops {
		switch op.(type) {
		case hive.OpCreateKey:
			keysCreated++
		case hive.OpSetValue:
			valuesSet++
		case hive.OpDeleteKey:
			keysDeleted++
		}
	}

	if keysCreated != 2 {
		t.Errorf("Expected 2 keys created, got %d", keysCreated)
	}
	if valuesSet != 3 {
		t.Errorf("Expected 3 values set, got %d", valuesSet)
	}
	if keysDeleted != 1 {
		t.Errorf("Expected 1 key deleted, got %d", keysDeleted)
	}
}

// TestParseRegString tests parsing .reg content from a string.
func TestParseRegString(t *testing.T) {
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\MyApp]
"Version"="1.0"
"Count"=dword:0000002a
`

	ops, err := hive.ParseRegString(regContent, hive.RegParseOptions{})
	if err != nil {
		t.Fatalf("ParseRegString failed: %v", err)
	}

	// Should have 1 CreateKey + 2 SetValue
	if len(ops) != 3 {
		t.Errorf("Expected 3 operations, got %d", len(ops))
	}
}

// TestParseRegBytes tests parsing .reg content from bytes.
func TestParseRegBytes(t *testing.T) {
	regContent := []byte(strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Test]",
		"\"Value\"=\"data\"",
	}, "\r\n"))

	ops, err := hive.ParseRegBytes(regContent, hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
	})
	if err != nil {
		t.Fatalf("ParseRegBytes failed: %v", err)
	}

	if len(ops) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(ops))
	}

	// Verify prefix was stripped
	if createKey, ok := ops[0].(hive.OpCreateKey); ok {
		if createKey.Path != "Test" {
			t.Errorf("Expected path 'Test', got %q", createKey.Path)
		}
	} else {
		t.Errorf("First operation should be OpCreateKey, got %T", ops[0])
	}
}

// TestParseRegFile_FileNotFound tests error when file doesn't exist.
func TestParseRegFile_FileNotFound(t *testing.T) {
	_, err := hive.ParseRegFile("nonexistent.reg", hive.RegParseOptions{})
	if err == nil {
		t.Fatal("Expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestMergeRegFile_WithStats tests that merge operations return accurate statistics.
func TestMergeRegFile_WithStats(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Create hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create .reg with known operations
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp]",
		"\"Version\"=\"1.0\"",
		"\"Build\"=dword:0000000a",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp\\Settings]",
		"\"Enabled\"=dword:00000001",
		"",
	}, "\r\n")

	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to write .reg file: %v", err)
	}

	// Merge and get stats
	stats, err := hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	// Verify stats
	if stats == nil {
		t.Fatal("Expected stats, got nil")
	}

	// Should have 2 CreateKey + 3 SetValue = 5 total operations
	if stats.OperationsTotal != 5 {
		t.Errorf("Expected 5 total operations, got %d", stats.OperationsTotal)
	}

	if stats.KeysCreated != 2 {
		t.Errorf("Expected 2 keys created, got %d", stats.KeysCreated)
	}

	if stats.ValuesSet != 3 {
		t.Errorf("Expected 3 values set, got %d", stats.ValuesSet)
	}

	if stats.KeysDeleted != 0 {
		t.Errorf("Expected 0 keys deleted, got %d", stats.KeysDeleted)
	}

	if stats.ValuesDeleted != 0 {
		t.Errorf("Expected 0 values deleted, got %d", stats.ValuesDeleted)
	}

	if stats.OperationsFailed != 0 {
		t.Errorf("Expected 0 failed operations, got %d", stats.OperationsFailed)
	}

	if stats.BytesWritten == 0 {
		t.Error("Expected non-zero bytes written")
	}
}

// TestMergeRegFile_WithDeleteOperations tests stats with deletions.
func TestMergeRegFile_WithDeleteOperations(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Create hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// First, create some data to delete
	setupReg := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\ToDelete]
"Value"="data"
`
	_, err = hive.MergeRegString(hiveFile, setupReg, nil)
	if err != nil {
		t.Fatalf("Setup merge failed: %v", err)
	}

	// Now create a .reg that deletes
	regContent := strings.Join([]string{
		"Windows Registry Editor Version 5.00",
		"",
		"[-HKEY_LOCAL_MACHINE\\SOFTWARE\\ToDelete]",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Keep]",
		"\"DeleteThis\"=\"value\"",
		"",
		"[HKEY_LOCAL_MACHINE\\SOFTWARE\\Keep]",
		"\"DeleteThis\"=-",
	}, "\r\n")

	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to write .reg file: %v", err)
	}

	// Merge and get stats
	stats, err := hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	// Verify stats
	if stats.KeysDeleted != 1 {
		t.Errorf("Expected 1 key deleted, got %d", stats.KeysDeleted)
	}

	if stats.ValuesDeleted != 1 {
		t.Errorf("Expected 1 value deleted, got %d", stats.ValuesDeleted)
	}
}

// TestMergeRegFiles_AggregatedStats tests that multi-file merges aggregate stats correctly.
func TestMergeRegFiles_AggregatedStats(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Create hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create two .reg files
	reg1 := filepath.Join(tempDir, "file1.reg")
	reg1Content := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\App1]
"Key1"="value1"
`
	if err := os.WriteFile(reg1, []byte(reg1Content), 0644); err != nil {
		t.Fatalf("Failed to write reg1: %v", err)
	}

	reg2 := filepath.Join(tempDir, "file2.reg")
	reg2Content := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\App2]
"Key2"="value2"
"Key3"=dword:00000001
`
	if err := os.WriteFile(reg2, []byte(reg2Content), 0644); err != nil {
		t.Fatalf("Failed to write reg2: %v", err)
	}

	// Merge both files
	stats, err := hive.MergeRegFiles(hiveFile, []string{reg1, reg2}, nil)
	if err != nil {
		t.Fatalf("MergeRegFiles failed: %v", err)
	}

	// Verify aggregated stats
	// file1: 1 CreateKey + 1 SetValue = 2 ops
	// file2: 1 CreateKey + 2 SetValue = 3 ops
	// Total: 5 ops
	if stats.OperationsTotal != 5 {
		t.Errorf("Expected 5 total operations, got %d", stats.OperationsTotal)
	}

	if stats.KeysCreated != 2 {
		t.Errorf("Expected 2 keys created, got %d", stats.KeysCreated)
	}

	if stats.ValuesSet != 3 {
		t.Errorf("Expected 3 values set, got %d", stats.ValuesSet)
	}
}

// TestMergeRegFile_DryRunStats tests that dry run still collects stats but doesn't write.
func TestMergeRegFile_DryRunStats(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Create hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	originalSize := len(minimalHive)
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Create .reg
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\SOFTWARE\Test]
"Value"="data"
`
	if err := os.WriteFile(regFile, []byte(regContent), 0644); err != nil {
		t.Fatalf("Failed to write .reg file: %v", err)
	}

	// Dry run merge
	stats, err := hive.MergeRegFile(hiveFile, regFile, &hive.MergeOptions{
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("MergeRegFile dry run failed: %v", err)
	}

	// Stats should still be collected
	if stats.OperationsTotal != 2 {
		t.Errorf("Expected 2 operations in dry run, got %d", stats.OperationsTotal)
	}

	// But file should not have been modified
	hiveData, err := os.ReadFile(hiveFile)
	if err != nil {
		t.Fatalf("Failed to read hive after dry run: %v", err)
	}

	if len(hiveData) != originalSize {
		t.Errorf("Hive file was modified during dry run: size changed from %d to %d", originalSize, len(hiveData))
	}
}
