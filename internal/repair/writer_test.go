package repair

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriter_WriteAtomic(t *testing.T) {
	writer := NewWriter()

	// Create temp directory
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "test.dat")

	// Write data atomically
	testData := []byte("Hello, World!")
	if err := writer.WriteAtomic(targetPath, testData); err != nil {
		t.Fatalf("WriteAtomic failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("target file not created: %v", err)
	}

	// Verify content
	readData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Errorf("data mismatch\nexpected: %v\ngot:      %v", testData, readData)
	}
}

func TestWriter_WriteAtomic_Overwrite(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "test.dat")

	// Write initial data
	initialData := []byte("initial data")
	if err := writer.WriteAtomic(targetPath, initialData); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	// Overwrite with new data
	newData := []byte("new data that is longer")
	if err := writer.WriteAtomic(targetPath, newData); err != nil {
		t.Fatalf("overwrite failed: %v", err)
	}

	// Verify new content
	readData, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}

	if !bytes.Equal(readData, newData) {
		t.Errorf("data mismatch after overwrite\nexpected: %v\ngot:      %v", newData, readData)
	}
}

func TestWriter_WriteAtomic_InvalidPath(t *testing.T) {
	writer := NewWriter()

	// Try to write to non-existent directory
	invalidPath := "/nonexistent/directory/test.dat"
	err := writer.WriteAtomic(invalidPath, []byte("test"))

	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestWriter_WriteAtomic_EmptyData(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "empty.dat")

	// Write empty file
	if err := writer.WriteAtomic(targetPath, []byte{}); err != nil {
		t.Fatalf("WriteAtomic with empty data failed: %v", err)
	}

	// Verify file exists and is empty
	stat, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("target file not created: %v", err)
	}

	if stat.Size() != 0 {
		t.Errorf("expected empty file, got size %d", stat.Size())
	}
}

func TestWriter_CreateBackup(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source.dat")

	// Create source file
	sourceData := []byte("source data for backup")
	if err := os.WriteFile(sourcePath, sourceData, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Create backup
	backupPath, err := writer.CreateBackup(sourcePath, "bak")
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Verify backup path format
	if !strings.HasPrefix(backupPath, sourcePath) {
		t.Errorf("backup path should start with source path\nexpected prefix: %s\ngot: %s", sourcePath, backupPath)
	}

	if !strings.Contains(backupPath, ".bak.") {
		t.Error("backup path should contain suffix")
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}

	// Verify backup content matches source
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}

	if !bytes.Equal(backupData, sourceData) {
		t.Errorf("backup data mismatch\nexpected: %v\ngot:      %v", sourceData, backupData)
	}
}

func TestWriter_CreateBackup_NonexistentSource(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "nonexistent.dat")

	// Try to backup non-existent file
	_, err := writer.CreateBackup(sourcePath, "bak")
	if err == nil {
		t.Fatal("expected error when backing up non-existent file")
	}
}

func TestWriter_CreateBackup_LargeFile(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "large.dat")

	// Create large file (1MB)
	largeData := bytes.Repeat([]byte("A"), 1024*1024)
	if err := os.WriteFile(sourcePath, largeData, 0644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	// Create backup
	backupPath, err := writer.CreateBackup(sourcePath, "bak")
	if err != nil {
		t.Fatalf("CreateBackup failed for large file: %v", err)
	}

	// Verify backup size matches source
	sourceInfo, _ := os.Stat(sourcePath)
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("backup file not created: %v", err)
	}

	if sourceInfo.Size() != backupInfo.Size() {
		t.Errorf("backup size mismatch: expected %d, got %d", sourceInfo.Size(), backupInfo.Size())
	}
}

func TestWriter_RestoreBackup(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	originalPath := filepath.Join(tmpDir, "original.dat")
	backupPath := filepath.Join(tmpDir, "backup.dat")

	// Create original file
	originalData := []byte("original data")
	if err := os.WriteFile(originalPath, originalData, 0644); err != nil {
		t.Fatalf("failed to create original file: %v", err)
	}

	// Create backup
	backupData := []byte("backup data to restore")
	if err := os.WriteFile(backupPath, backupData, 0644); err != nil {
		t.Fatalf("failed to create backup file: %v", err)
	}

	// Restore backup
	if err := writer.RestoreBackup(originalPath, backupPath); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify original file now has backup content
	restoredData, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}

	if !bytes.Equal(restoredData, backupData) {
		t.Errorf("restored data mismatch\nexpected: %v\ngot:      %v", backupData, restoredData)
	}
}

func TestWriter_RestoreBackup_NonexistentBackup(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	originalPath := filepath.Join(tmpDir, "original.dat")
	backupPath := filepath.Join(tmpDir, "nonexistent.dat")

	// Try to restore from non-existent backup
	err := writer.RestoreBackup(originalPath, backupPath)
	if err == nil {
		t.Fatal("expected error when restoring from non-existent backup")
	}
}

func TestWriter_CopyFile(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.dat")
	dstPath := filepath.Join(tmpDir, "dest.dat")

	// Create source file
	srcData := []byte("source file content")
	if err := os.WriteFile(srcPath, srcData, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Copy file
	if err := writer.CopyFile(srcPath, dstPath); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(dstPath); err != nil {
		t.Fatalf("destination file not created: %v", err)
	}

	// Verify content matches
	dstData, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if !bytes.Equal(dstData, srcData) {
		t.Errorf("copied data mismatch\nexpected: %v\ngot:      %v", srcData, dstData)
	}
}

func TestWriter_CopyFile_NonexistentSource(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "nonexistent.dat")
	dstPath := filepath.Join(tmpDir, "dest.dat")

	// Try to copy non-existent file
	err := writer.CopyFile(srcPath, dstPath)
	if err == nil {
		t.Fatal("expected error when copying non-existent file")
	}
}

func TestWriter_CopyFile_OverwriteExisting(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.dat")
	dstPath := filepath.Join(tmpDir, "dest.dat")

	// Create source and destination
	srcData := []byte("new source data")
	oldDstData := []byte("old destination data")

	if err := os.WriteFile(srcPath, srcData, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	if err := os.WriteFile(dstPath, oldDstData, 0644); err != nil {
		t.Fatalf("failed to create destination file: %v", err)
	}

	// Copy should overwrite
	if err := writer.CopyFile(srcPath, dstPath); err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	// Verify destination has new content
	dstData, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if !bytes.Equal(dstData, srcData) {
		t.Errorf("destination not overwritten\nexpected: %v\ngot:      %v", srcData, dstData)
	}
}

func TestWriter_SetTempDir(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	customTempDir := filepath.Join(tmpDir, "custom-temp")

	if err := os.Mkdir(customTempDir, 0755); err != nil {
		t.Fatalf("failed to create custom temp dir: %v", err)
	}

	writer.SetTempDir(customTempDir)

	// Verify temp dir is set (we can't easily test this without exposing internals,
	// but at least verify the method doesn't panic)
	if writer.tempDir != customTempDir {
		t.Errorf("temp dir not set correctly")
	}
}

func TestWriter_NoTempFileLeftBehind(t *testing.T) {
	writer := NewWriter()

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "test.dat")

	// Count files before write
	beforeFiles, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read dir before write: %v", err)
	}

	// Write atomically
	testData := []byte("test data")
	if err := writer.WriteAtomic(targetPath, testData); err != nil {
		t.Fatalf("WriteAtomic failed: %v", err)
	}

	// Count files after write
	afterFiles, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read dir after write: %v", err)
	}

	// Should have exactly one more file (the target)
	if len(afterFiles) != len(beforeFiles)+1 {
		t.Errorf("expected 1 new file, got %d", len(afterFiles)-len(beforeFiles))
	}

	// Verify no .tmp files remain
	for _, f := range afterFiles {
		if strings.HasSuffix(f.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", f.Name())
		}
	}
}
