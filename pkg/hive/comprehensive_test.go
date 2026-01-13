package hive_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestMergeRegFile_FileNotFound tests error when hive file doesn't exist.
func TestMergeRegFile_FileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	regFile := filepath.Join(tempDir, "test.reg")

	// Create .reg file but not hive
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Test]
"Value"="Data"
`
	os.WriteFile(regFile, []byte(regContent), 0644)

	// Should fail - hive doesn't exist
	err := hive.MergeRegFile("nonexistent.hive", regFile, nil)
	if err == nil {
		t.Error("Expected error for non-existent hive file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestMergeRegFile_RegFileNotFound tests error when .reg file doesn't exist.
func TestMergeRegFile_RegFileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Create hive but not .reg file
	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	// Should fail - .reg doesn't exist
	err := hive.MergeRegFile(hiveFile, "nonexistent.reg", nil)
	if err == nil {
		t.Error("Expected error for non-existent .reg file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// TestMergeRegString_HiveNotFound tests error when hive doesn't exist.
func TestMergeRegString_HiveNotFound(t *testing.T) {
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Test]
"Value"="Data"
`
	err := hive.MergeRegString("nonexistent.hive", regContent, nil)
	if err == nil {
		t.Error("Expected error for non-existent hive file")
	}
}

// TestMergeRegFile_EmptyRegFile tests merging an empty .reg file.
func TestMergeRegFile_EmptyRegFile(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "empty.reg")

	// Create hive
	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	// Create empty .reg file (just header)
	regContent := `Windows Registry Editor Version 5.00
`
	os.WriteFile(regFile, []byte(regContent), 0644)

	// Should succeed (nothing to merge)
	err := hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Errorf("Empty .reg file should succeed: %v", err)
	}
}

// TestMergeRegFiles_FileNotFound tests batch merge with missing file.
func TestMergeRegFiles_FileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	regFiles := []string{
		filepath.Join(tempDir, "exists.reg"),
		filepath.Join(tempDir, "notexist.reg"),
	}

	// Create first file
	os.WriteFile(regFiles[0], []byte("Windows Registry Editor Version 5.00\n"), 0644)

	// Should fail on second file
	err := hive.MergeRegFiles(hiveFile, regFiles, nil)
	if err == nil {
		t.Error("Expected error for non-existent .reg file in batch")
	}
}

// TestMergeRegFiles_HiveNotFound tests batch merge with missing.
func TestMergeRegFiles_HiveNotFound(t *testing.T) {
	err := hive.MergeRegFiles("nonexistent.hive", []string{"test.reg"}, nil)
	if err == nil {
		t.Error("Expected error for non-existent hive")
	}
}

// TestExportReg_HiveNotFound tests export with missing.
func TestExportReg_HiveNotFound(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "output.reg")

	err := hive.ExportReg("nonexistent.hive", outputFile, nil)
	if err == nil {
		t.Error("Expected error for non-existent hive")
	}
}

// TestExportRegString_HiveNotFound tests string export with missing.
func TestExportRegString_HiveNotFound(t *testing.T) {
	_, err := hive.ExportRegString("nonexistent.hive", nil)
	if err == nil {
		t.Error("Expected error for non-existent hive")
	}
}

// TestExportReg_WithSubtree tests exporting a specific subtree.
func TestExportReg_WithSubtree(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "merge.reg")
	outputFile := filepath.Join(tempDir, "export.reg")

	// Create hive
	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	// Merge to create a subtree with specific values
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\Test]
"TestValue"="TestData"

[HKEY_LOCAL_MACHINE\Software\Other]
"OtherValue"="OtherData"
`
	os.WriteFile(regFile, []byte(regContent), 0644)
	hive.MergeRegFile(hiveFile, regFile, nil)

	// Export just the Software\Test subtree
	opts := &hive.ExportOptions{
		SubtreePath: "Software\\Test",
	}
	err := hive.ExportReg(hiveFile, outputFile, opts)
	if err != nil {
		t.Fatalf("ExportReg with subtree failed: %v", err)
	}

	// Verify output has correct content
	exportData, _ := os.ReadFile(outputFile)
	exportStr := string(exportData)

	// Should contain the header
	if !strings.Contains(exportStr, "Windows Registry Editor") {
		t.Error("Export should contain registry header")
	}

	// Should contain the Test subtree and its value
	if !strings.Contains(exportStr, "Test") {
		t.Error("Export should contain Test key")
	}
	if !strings.Contains(exportStr, "TestValue") {
		t.Error("Export should contain TestValue from subtree")
	}
	if !strings.Contains(exportStr, "TestData") {
		t.Error("Export should contain TestData from subtree")
	}

	// Should NOT contain the Other key (it's outside the subtree)
	if strings.Contains(exportStr, "OtherValue") {
		t.Error("Export should NOT contain OtherValue (outside subtree)")
	}
	if strings.Contains(exportStr, "OtherData") {
		t.Error("Export should NOT contain OtherData (outside subtree)")
	}
}

// TestExportReg_WithUTF16LE tests exporting with UTF-16LE encoding.
func TestExportReg_WithUTF16LE(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "export.reg")

	opts := &hive.ExportOptions{
		Encoding: "UTF-16LE",
		WithBOM:  true,
	}

	err := hive.ExportReg("../../testdata/minimal", outputFile, opts)
	if err != nil {
		t.Fatalf("ExportReg with UTF-16LE failed: %v", err)
	}

	// Verify BOM is present
	data, _ := os.ReadFile(outputFile)
	if len(data) < 2 {
		t.Error("Export file too small")
	}
	// UTF-16LE BOM is 0xFF 0xFE
	if data[0] != 0xFF || data[1] != 0xFE {
		t.Error("Expected UTF-16LE BOM")
	}
}

// TestExportRegString_WithSubtree tests string export with subtree.
func TestExportRegString_WithSubtree(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "merge.reg")

	// Create hive with multiple subtrees
	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\MyApp]
"Version"="1.0"
"Enabled"=dword:00000001

[HKEY_LOCAL_MACHINE\Software\OtherApp]
"Name"="Other"
`
	os.WriteFile(regFile, []byte(regContent), 0644)
	hive.MergeRegFile(hiveFile, regFile, nil)

	// Export just MyApp subtree
	opts := &hive.ExportOptions{
		SubtreePath: "Software\\MyApp",
	}
	regStr, err := hive.ExportRegString(hiveFile, opts)
	if err != nil {
		t.Fatalf("ExportRegString with subtree failed: %v", err)
	}

	// Should contain MyApp and its values
	if !strings.Contains(regStr, "MyApp") {
		t.Error("Export should contain MyApp")
	}
	if !strings.Contains(regStr, "Version") {
		t.Error("Export should contain Version value")
	}
	if !strings.Contains(regStr, "Enabled") {
		t.Error("Export should contain Enabled value")
	}

	// Should NOT contain OtherApp (outside subtree)
	if strings.Contains(regStr, "OtherApp") {
		t.Error("Export should NOT contain OtherApp (outside subtree)")
	}
	if strings.Contains(regStr, "Name") && !strings.Contains(regStr, "MyApp") {
		t.Error("Export should NOT contain Name from OtherApp")
	}
}

// TestExportReg_InvalidSubtree tests export with non-existent subtree.
func TestExportReg_InvalidSubtree(t *testing.T) {
	tempDir := t.TempDir()
	outputFile := filepath.Join(tempDir, "output.reg")

	opts := &hive.ExportOptions{
		SubtreePath: "NonExistent\\Path",
	}

	err := hive.ExportReg("../../testdata/minimal", outputFile, opts)
	if err == nil {
		t.Error("Expected error for non-existent subtree")
	}
}

// TestDefragment_FileNotFound tests defragment with missing file.
func TestDefragment_FileNotFound(t *testing.T) {
	err := hive.Defragment("nonexistent.hive")
	if err == nil {
		t.Error("Expected error for non-existent hive")
	}
}

// TestValidateHive_FileNotFound tests validation with missing file.
func TestValidateHive_FileNotFound(t *testing.T) {
	err := hive.ValidateHive("nonexistent.hive", hive.DefaultLimits())
	if err == nil {
		t.Error("Expected error for non-existent hive")
	}
}

// TestValidateHive_AllLimitPresets tests all limit presets.
func TestValidateHive_AllLimitPresets(t *testing.T) {
	testCases := []struct {
		name   string
		limits hive.Limits
	}{
		{"Default", hive.DefaultLimits()},
		{"Relaxed", hive.RelaxedLimits()},
		{"Strict", hive.StrictLimits()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := hive.ValidateHive("../../testdata/minimal", tc.limits)
			if err != nil {
				t.Errorf("Validation with %s limits failed: %v", tc.name, err)
			}
		})
	}
}

// TestHiveInfo_FileNotFound tests info retrieval with missing file.
func TestHiveInfo_FileNotFound(t *testing.T) {
	_, err := hive.HiveStats("nonexistent.hive")
	if err == nil {
		t.Error("Expected error for non-existent hive")
	}
}

// TestHiveInfo_AllFields tests that all expected fields are present.
func TestHiveInfo_AllFields(t *testing.T) {
	info, err := hive.HiveStats("../../testdata/minimal")
	if err != nil {
		t.Fatalf("HiveInfo failed: %v", err)
	}

	requiredFields := []string{"root_keys", "file_size"}
	for _, field := range requiredFields {
		if _, ok := info[field]; !ok {
			t.Errorf("Expected field %s in HiveInfo result", field)
		}
	}
}

// TestMergeRegFile_AllOperationTypes tests all operation types.
func TestMergeRegFile_AllOperationTypes(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	// Create hive
	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	// Create .reg with various operation types
	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\OpTest]
"StringValue"="test string"
"DwordValue"=dword:00000042
"BinaryValue"=hex:01,02,03,04
"MultiSzValue"=hex(7):74,00,65,00,73,00,74,00,00,00,00,00

[HKEY_LOCAL_MACHINE\Software\OpTest\Subkey]
"NestedValue"="nested"

[-HKEY_LOCAL_MACHINE\Software\ToDelete]
`
	os.WriteFile(regFile, []byte(regContent), 0644)

	// Merge
	err := hive.MergeRegFile(hiveFile, regFile, nil)
	if err != nil {
		t.Fatalf("Merge with all operation types failed: %v", err)
	}
}

// TestMergeRegFile_WithDefragment tests merge with defragmentation.
func TestMergeRegFile_WithDefragment(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\DefragTest]
"Value"="Data"
`
	os.WriteFile(regFile, []byte(regContent), 0644)

	opts := &hive.MergeOptions{
		Defragment: true,
	}

	err := hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("Merge with defragment failed: %v", err)
	}
}

// TestMergeRegFile_WithCustomLimits tests merge with custom limits.
func TestMergeRegFile_WithCustomLimits(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")
	regFile := filepath.Join(tempDir, "test.reg")

	minimalHive, _ := os.ReadFile("../../testdata/minimal")
	os.WriteFile(hiveFile, minimalHive, 0644)

	regContent := `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\CustomLimits]
"Value"="Data"
`
	os.WriteFile(regFile, []byte(regContent), 0644)

	opts := &hive.MergeOptions{
		Limits: func() *hive.Limits {
			l := hive.RelaxedLimits()
			return &l
		}(),
	}

	err := hive.MergeRegFile(hiveFile, regFile, opts)
	if err != nil {
		t.Fatalf("Merge with custom limits failed: %v", err)
	}
}

// TestDefragment_RealHive tests defragmentation on various hives.
func TestDefragment_RealHive(t *testing.T) {
	testCases := []string{
		"../../testdata/minimal",
		"../../testdata/large",
		"../../testdata/special",
	}

	for _, hivePath := range testCases {
		t.Run(filepath.Base(hivePath), func(t *testing.T) {
			tempDir := t.TempDir()
			hiveFile := filepath.Join(tempDir, "test.hive")

			// Copy hive to temp
			data, err := os.ReadFile(hivePath)
			if err != nil {
				t.Skipf("Skipping %s: %v", hivePath, err)
			}
			os.WriteFile(hiveFile, data, 0644)

			// Defragment
			err = hive.Defragment(hiveFile)
			if err != nil {
				t.Errorf("Defragment failed on %s: %v", hivePath, err)
			}

			// Verify backup exists
			backupFile := hiveFile + ".bak"
			if _, statErr := os.Stat(backupFile); statErr != nil {
				t.Error("Backup not created")
			}
		})
	}
}

// TestValidateHive_RealHives tests validation on various hives.
func TestValidateHive_RealHives(t *testing.T) {
	testCases := []string{
		"../../testdata/minimal",
		"../../testdata/large",
		"../../testdata/special",
		"../../testdata/rlenvalue_test_hive",
	}

	for _, hivePath := range testCases {
		t.Run(filepath.Base(hivePath), func(t *testing.T) {
			// Should pass with default limits
			err := hive.ValidateHive(hivePath, hive.DefaultLimits())
			if err != nil {
				t.Errorf("Validation failed on %s: %v", hivePath, err)
			}

			// Should pass with relaxed limits
			err = hive.ValidateHive(hivePath, hive.RelaxedLimits())
			if err != nil {
				t.Errorf("Validation with relaxed limits failed on %s: %v", hivePath, err)
			}
		})
	}
}

// TestExportReg_RealHives tests export on various hives.
func TestExportReg_RealHives(t *testing.T) {
	testCases := []string{
		"../../testdata/minimal",
		"../../testdata/special",
	}

	for _, hivePath := range testCases {
		t.Run(filepath.Base(hivePath), func(t *testing.T) {
			tempDir := t.TempDir()
			outputFile := filepath.Join(tempDir, "export.reg")

			err := hive.ExportReg(hivePath, outputFile, nil)
			if err != nil {
				t.Errorf("Export failed on %s: %v", hivePath, err)
			}

			// Verify output
			data, _ := os.ReadFile(outputFile)
			if len(data) == 0 {
				t.Error("Export file is empty")
			}
		})
	}
}
