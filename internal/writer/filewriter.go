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
	if _, err := tmpFile.Write(buf); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	// Close before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	tmpFile = nil // Don't clean up in defer

	// Atomic rename
	if err := os.Rename(tmpPath, w.Path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}
