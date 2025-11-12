// Package writer exposes sinks for hive emission.
package writer

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileWriter writes hive bytes to a filesystem path atomically.
type FileWriter struct {
	Path string
}

// WriteHive writes buf to the configured path atomically via temp file + rename.
func (w *FileWriter) WriteHive(buf []byte) error {
	// Create temp file in same directory to ensure atomic rename
	dir := filepath.Dir(w.Path)
	tmpFile, err := os.CreateTemp(dir, ".gohivex-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
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
