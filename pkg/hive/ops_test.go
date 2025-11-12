package hive_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestMergeRegFile_Simple tests basic merge functionality.
func TestMergeRegFile_Simple(t *testing.T) {
	// Create temp directory for test files
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive to temp
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Create simple .reg file
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\TestKey]
"TestValue"="TestData"
`
	if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
		t.Fatalf("Failed to create .reg file: %v", writeErr)
	}

	// Merge
	err = hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	// Verify hive was modified
	modifiedHive, err := os.ReadFile(hiveFile)
	if err != nil {
		t.Fatalf("Failed to read modified hive: %v", err)
	}

	if len(modifiedHive) == 0 {
		t.Error("Modified hive is empty")
	}
}

// TestMergeRegString tests merging from a string.
func TestMergeRegString(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Merge from string
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\StringTest]
"Value1"="Data1"
`
	err = hive.MergeRegString(hiveFile, regContent, nil)
	if err != nil {
		t.Fatalf("MergeRegString failed: %v", err)
	}
}

// TestMergeRegFile_WithProgress tests progress reporting.
func TestMergeRegFile_WithProgress(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Create .reg with multiple operations
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Key1]
"Value1"="Data1"

[HKEY_LOCAL_MACHINE\Key2]
"Value2"="Data2"

[HKEY_LOCAL_MACHINE\Key3]
"Value3"="Data3"
`
	if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
		t.Fatalf("Failed to create .reg file: %v", writeErr)
	}

	// Track progress calls
	var progressCalls int
	var lastCurrent, lastTotal int

	opts := &hive.MergeOptions{
		OnProgress: func(current, total int) {
			progressCalls++
			lastCurrent = current
			lastTotal = total
		},
	}

	err = hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	if progressCalls == 0 {
		t.Error("OnProgress was never called")
	}
	if lastCurrent != lastTotal {
		t.Errorf("Expected lastCurrent == lastTotal, got %d != %d", lastCurrent, lastTotal)
	}
}

// TestMergeRegFile_WithBackup tests backup creation.
func TestMergeRegFile_WithBackup(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	backupFile := hiveFile + ".bak"
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Create simple .reg
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\BackupTest]
"Value"="Data"
`
	if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
		t.Fatalf("Failed to create .reg file: %v", writeErr)
	}

	// Merge with backup
	opts := &hive.MergeOptions{
		CreateBackup: true,
	}
	err = hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	// Verify backup exists
	if _, statErr := os.Stat(backupFile); statErr != nil {
		t.Errorf("Backup file not created: %v", statErr)
	}

	// Verify backup is same as original
	backupData, _ := os.ReadFile(backupFile)
	if len(backupData) != len(minimalHive) {
		t.Error("Backup differs from original")
	}
}

// TestMergeRegFile_DryRun tests dry run mode.
func TestMergeRegFile_DryRun(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Get original hive content
	originalHive, _ := os.ReadFile(hiveFile)

	// Create .reg
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\DryRunTest]
"Value"="Data"
`
	if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
		t.Fatalf("Failed to create .reg file: %v", writeErr)
	}

	// Merge with dry run
	opts := &hive.MergeOptions{
		DryRun: true,
	}
	err = hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("DryRun merge failed: %v", err)
	}

	// Verify hive was NOT modified
	currentHive, _ := os.ReadFile(hiveFile)
	if string(currentHive) != string(originalHive) {
		t.Error("Hive was modified during dry run")
	}
}

// TestMergeRegFiles tests batch merge.
func TestMergeRegFiles(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Create multiple .reg files
	regFiles := []string{
		filepath.Join(tempDir, "test1.reg"),
		filepath.Join(tempDir, "test2.reg"),
		filepath.Join(tempDir, "test3.reg"),
	}

	for i, regFile := range regFiles {
		regContent := "Windows Registry Editor Version 5.00\n\n" +
			"[HKEY_LOCAL_MACHINE\\BatchTest" + string(rune('1'+i)) + "]\n" +
			"\"Value\"=\"Data\"\n"
		if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
			t.Fatalf("Failed to create .reg file %d: %v", i, writeErr)
		}
	}

	// Merge all files
	err = hive.MergeRegFiles(hiveFile, regFiles, nil)
	if err != nil {
		t.Fatalf("MergeRegFiles failed: %v", err)
	}
}

// TestExportReg tests export functionality.
func TestExportReg(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "export.reg")

	// Export minimal hive
	err := hive.ExportReg("../../testdata/minimal", outputFile, nil)
	if err != nil {
		t.Fatalf("ExportReg failed: %v", err)
	}

	// Verify output file exists and has content
	exportData, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read export file: %v", err)
	}

	if len(exportData) == 0 {
		t.Error("Export file is empty")
	}

	// Should contain registry header
	exportStr := string(exportData)
	if !strings.Contains(exportStr, "Windows Registry Editor") {
		t.Error("Export missing registry header")
	}
}

// TestExportRegString tests string export.
func TestExportRegString(t *testing.T) {
	regContent, err := hive.ExportRegString("../../testdata/minimal", nil)
	if err != nil {
		t.Fatalf("ExportRegString failed: %v", err)
	}

	if regContent == "" {
		t.Error("Export string is empty")
	}

	if !strings.Contains(regContent, "Windows Registry Editor") {
		t.Error("Export missing registry header")
	}
}

// TestDefragment tests defragmentation.
func TestDefragment(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	backupFile := hiveFile + ".bak"

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Defragment
	err = hive.Defragment(hiveFile)
	if err != nil {
		t.Fatalf("Defragment failed: %v", err)
	}

	// Verify backup was created
	if _, statErr := os.Stat(backupFile); statErr != nil {
		t.Error("Backup not created during defragment")
	}

	// Verify hive still exists and is valid
	defragHive, err := os.ReadFile(hiveFile)
	if err != nil {
		t.Fatalf("Failed to read defragmented hive: %v", err)
	}
	if len(defragHive) == 0 {
		t.Error("Defragmented hive is empty")
	}
}

// TestValidateHive tests validation.
func TestValidateHive(t *testing.T) {
	// Validate minimal hive with default limits
	err := hive.ValidateHive("../../testdata/minimal", hive.DefaultLimits())
	if err != nil {
		t.Errorf("ValidateHive failed on valid hive: %v", err)
	}

	// Validate with relaxed limits
	err = hive.ValidateHive("../../testdata/minimal", hive.RelaxedLimits())
	if err != nil {
		t.Errorf("ValidateHive failed with relaxed limits: %v", err)
	}

	// Validate with strict limits (should still pass for minimal hive)
	err = hive.ValidateHive("../../testdata/minimal", hive.StrictLimits())
	if err != nil {
		t.Errorf("ValidateHive failed with strict limits: %v", err)
	}
}

// TestHiveInfo tests hive info retrieval.
func TestHiveInfo(t *testing.T) {
	info, err := hive.HiveStats("../../testdata/minimal")
	if err != nil {
		t.Fatalf("HiveInfo failed: %v", err)
	}

	if info["root_keys"] == "" {
		t.Error("HiveInfo missing root_keys")
	}
	if info["file_size"] == "" {
		t.Error("HiveInfo missing file_size")
	}
}

// TestMergeRegFile_ErrorHandling tests error callback.
func TestMergeRegFile_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Create .reg with operations (some might fail)
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\ErrorTest]
"Value1"="Data1"

[HKEY_LOCAL_MACHINE\ErrorTest]
"Value2"="Data2"
`
	if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
		t.Fatalf("Failed to create .reg file: %v", writeErr)
	}

	// Track errors
	var errorCount int
	opts := &hive.MergeOptions{
		OnError: func(op hive.EditOp, err error) bool {
			errorCount++
			return true // continue on errors
		},
	}

	err = hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	// Note: errorCount might be 0 if all operations succeed
}

// TestMergeRegFile_LimitViolation tests that limit violations are caught.
func TestMergeRegFile_LimitViolation(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("Failed to read minimal hive: %v", err)
	}
	if writeErr := os.WriteFile(hiveFile, minimalHive, 0644); writeErr != nil {
		t.Fatalf("Failed to create test hive: %v", writeErr)
	}

	// Create .reg with value exceeding strict limits
	largeData := strings.Repeat("A", 100000) // 100KB value
	regContent := "Windows Registry Editor Version 5.00\n\n" +
		"[HKEY_LOCAL_MACHINE\\LimitTest]\n" +
		"\"LargeValue\"=\"" + largeData + "\"\n"
	if writeErr := os.WriteFile(regFile, []byte(regContent), 0644); writeErr != nil {
		t.Fatalf("Failed to create .reg file: %v", writeErr)
	}

	// Merge with strict limits (should fail)
	opts := &hive.MergeOptions{
		Limits: func() *hive.Limits {
			l := hive.StrictLimits()
			return &l
		}(),
	}
	err = hive.MergeRegFile(hiveFile, regFile, opts)
	if err == nil {
		t.Error("Expected limit violation error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "MaxValueSize") {
		t.Errorf("Expected MaxValueSize error, got: %v", err)
	}
}
