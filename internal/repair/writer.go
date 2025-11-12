package repair

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Writer provides atomic file operations for repair operations.
// All write operations use the temp-file-then-rename pattern to ensure
// atomicity: either the entire file is written or nothing changes.
type Writer struct {
	// Optional: custom temp directory for atomic writes
	tempDir string
}

// NewWriter creates a new writer with default settings.
func NewWriter() *Writer {
	return &Writer{}
}

// SetTempDir sets a custom temporary directory for atomic writes.
// If not set, the system temp directory will be used.
func (w *Writer) SetTempDir(dir string) {
	w.tempDir = dir
}

// WriteAtomic writes data to a file atomically using temp-file-then-rename.
// This ensures that the target file is never left in a corrupted state.
//
// Steps:
//  1. Create temporary file in same directory as target
//  2. Write data to temp file
//  3. Fsync temp file to ensure data is on disk
//  4. Rename temp file to target (atomic operation)
//  5. Fsync parent directory to ensure rename is persisted
//
// If any step fails, the temp file is cleaned up and the original target
// file (if it exists) remains unchanged.
func (w *Writer) WriteAtomic(path string, data []byte) error {
	// Get absolute path to ensure we're working with the right directory
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	dir := filepath.Dir(absPath)

	// Create temp file in same directory as target (required for atomic rename)
	// Using same directory ensures we're on the same filesystem
	tmpFile, err := os.CreateTemp(dir, ".gohivex-repair-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Cleanup temp file if we fail
	cleanup := func() {
		tmpFile.Close()
		os.Remove(tmpPath)
	}

	// Write data to temp file
	if _, writeErr := tmpFile.Write(data); writeErr != nil {
		cleanup()
		return fmt.Errorf("writing to temp file: %w", writeErr)
	}

	// Fsync to ensure data is on disk before rename
	if syncErr := tmpFile.Sync(); syncErr != nil {
		cleanup()
		return fmt.Errorf("syncing temp file: %w", syncErr)
	}

	// Close before rename (required on Windows)
	if closeErr := tmpFile.Close(); closeErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	// Atomic rename
	if renameErr := os.Rename(tmpPath, absPath); renameErr != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", renameErr)
	}

	// Fsync parent directory to ensure rename is persisted
	// This is critical for crash consistency
	if syncDirErr := syncDir(dir); syncDirErr != nil {
		// Don't fail the operation since data is already written
		// Log this as a warning in production code
		_ = syncDirErr // Silence linter
	}

	return nil
}

// CreateBackup creates a backup of the file with a timestamped suffix.
// The backup is verified after creation to ensure it's valid.
//
// Returns the path to the backup file and any error encountered.
//
// Backup naming: <original>.<suffix>.<timestamp>
// Example: system.types.bak.20060102-150405.
func (w *Writer) CreateBackup(path, suffix string) (string, error) {
	// Check if source file exists
	stat, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}

	// Read source file
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading source file: %w", err)
	}

	// Generate backup path with timestamp
	timestamp := time.Now().Format("20060102-150405")
	// Don't add extra dot if suffix already starts with one
	var backupPath string
	if suffix != "" && suffix[0] == '.' {
		backupPath = fmt.Sprintf("%s%s.%s", path, suffix, timestamp)
	} else {
		backupPath = fmt.Sprintf("%s.%s.%s", path, suffix, timestamp)
	}

	// Write backup atomically
	if writeErr := w.WriteAtomic(backupPath, data); writeErr != nil {
		return "", fmt.Errorf("writing backup: %w", writeErr)
	}

	// Verify backup
	if verifyErr := verifyBackup(backupPath, stat.Size()); verifyErr != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("backup verification failed: %w", verifyErr)
	}

	return backupPath, nil
}

// RestoreBackup atomically restores a file from its backup.
// This replaces the original file with the backup file's contents.
func (w *Writer) RestoreBackup(path, backupPath string) error {
	// Check if backup exists
	_, err := os.Stat(backupPath)
	if err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Read backup file
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("reading backup file: %w", err)
	}

	// Write backup data to original path atomically
	if writeErr := w.WriteAtomic(path, data); writeErr != nil {
		return fmt.Errorf("restoring from backup: %w", writeErr)
	}

	return nil
}

// CopyFile copies a file from src to dst atomically.
// This is useful for creating backups or temporary working copies.
func (w *Writer) CopyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	// Read all data
	data, err := io.ReadAll(srcFile)
	if err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}

	// Write to destination atomically
	if writeErr := w.WriteAtomic(dst, data); writeErr != nil {
		return fmt.Errorf("writing destination file: %w", writeErr)
	}

	return nil
}

// syncDir fsyncs a directory to ensure metadata changes are persisted.
// This is necessary after rename operations to ensure crash consistency.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening directory: %w", err)
	}
	defer d.Close()

	if syncErr := d.Sync(); syncErr != nil {
		return fmt.Errorf("syncing directory: %w", syncErr)
	}

	return nil
}

// verifyBackup performs basic verification on a backup file.
// It checks that the file exists and has the expected size.
func verifyBackup(path string, expectedSize int64) error {
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	if stat.Size() != expectedSize {
		return fmt.Errorf("backup size mismatch: expected %d, got %d", expectedSize, stat.Size())
	}

	// Additional check: ensure file is readable
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("backup file not readable: %w", err)
	}
	f.Close()

	return nil
}
