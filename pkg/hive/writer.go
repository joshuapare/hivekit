package hive

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileWriter writes hive bytes to a filesystem path atomically.
// The write is performed via temp file + rename to ensure atomicity.
type FileWriter struct {
	Path string
}

// WriteHive implements the Writer interface, writing the hive to the configured path.
func (w *FileWriter) WriteHive(buf []byte) error {
	// Create temp file in same directory to ensure atomic rename
	dir := filepath.Dir(w.Path)
	tmpFile, createErr := os.CreateTemp(dir, ".gohivex-tmp-*")
	if createErr != nil {
		return fmt.Errorf("create temp file: %w", createErr)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	// Write data
	if _, writeErr := tmpFile.Write(buf); writeErr != nil {
		return fmt.Errorf("write temp file: %w", writeErr)
	}

	// Sync to disk
	if syncErr := tmpFile.Sync(); syncErr != nil {
		return fmt.Errorf("sync temp file: %w", syncErr)
	}

	// Close before rename
	if closeErr := tmpFile.Close(); closeErr != nil {
		return fmt.Errorf("close temp file: %w", closeErr)
	}
	tmpFile = nil // Don't clean up in defer

	// Atomic rename
	if renameErr := os.Rename(tmpPath, w.Path); renameErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", renameErr)
	}

	return nil
}

// SimpleFileWriter writes hive bytes directly to a file without atomicity guarantees.
// This matches hivex behavior: open, write, close (no sync, no temp file).
// Use FileWriter for production code that needs atomic writes.
type SimpleFileWriter struct {
	Path string
}

// WriteHive implements the Writer interface, writing directly to the file.
func (w *SimpleFileWriter) WriteHive(buf []byte) error {
	// Direct write to path (matches hivex behavior)
	if err := os.WriteFile(w.Path, buf, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// MemWriter captures hive bytes in memory.
// Useful for testing or when the hive data needs to be processed further.
type MemWriter struct {
	Buf []byte
}

// WriteHive implements the Writer interface, storing the hive in memory.
func (w *MemWriter) WriteHive(buf []byte) error {
	w.Buf = append(w.Buf[:0], buf...)
	return nil
}
