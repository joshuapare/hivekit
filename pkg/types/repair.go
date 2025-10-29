package types

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// RepairOptions configures how repairs are applied
type RepairOptions struct {
	DryRun         bool   // Preview repairs without applying
	AutoOnly       bool   // Only apply auto-repairable fixes
	MaxRisk        RiskLevel // Maximum risk level to apply
	BackupSuffix   string // Suffix for backup file (default: ".backup")
	NoBackup       bool   // Skip creating backup (dangerous!)
	Verbose        bool   // Enable verbose logging
}

// RepairResult describes the outcome of a repair operation
type RepairResult struct {
	Applied        int       // Number of repairs applied
	Skipped        int       // Number of repairs skipped
	Failed         int       // Number of repairs that failed
	BackupPath     string    // Path to backup file
	DryRun         bool      // Whether this was a dry-run
	Duration       time.Duration
	Diagnostics    []RepairDiagnostic
}

// RepairDiagnostic describes what happened during a repair
type RepairDiagnostic struct {
	Offset      uint64
	Description string
	Applied     bool
	Error       string // Empty if successful
}

// ApplyRepairs applies repair actions from a diagnostic report
//
// Note: This is a placeholder method. The actual implementation is in internal/reader.ApplyRepairs()
// which should be called directly from CLI/application code to avoid import cycles.
// This method is kept here for API documentation purposes.
func (report *DiagnosticReport) ApplyRepairs(hivePath string, opts RepairOptions) (*RepairResult, error) {
	return nil, errors.New("ApplyRepairs must be called through internal/reader.ApplyRepairs() - see documentation")
}

// RestoreFromBackup restores a hive from its backup file
func RestoreFromBackup(hivePath string, backupSuffix string) error {
	if backupSuffix == "" {
		backupSuffix = ".backup"
	}
	backupPath := hivePath + backupSuffix

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	// Copy backup over original
	if err := copyFile(backupPath, hivePath); err != nil {
		return fmt.Errorf("failed to restore from backup: %w", err)
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Create destination file
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Ensure all writes are flushed
	if err := dstFile.Sync(); err != nil {
		return err
	}

	return nil
}

// ValidateBackup checks if a backup file exists and is valid
func ValidateBackup(hivePath string, backupSuffix string) error {
	if backupSuffix == "" {
		backupSuffix = ".backup"
	}
	backupPath := hivePath + backupSuffix

	// Check if backup exists
	backupInfo, err := os.Stat(backupPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}
	if err != nil {
		return fmt.Errorf("failed to stat backup: %w", err)
	}

	// Check if backup is a regular file
	if !backupInfo.Mode().IsRegular() {
		return fmt.Errorf("backup is not a regular file: %s", backupPath)
	}

	// Check if backup has reasonable size
	if backupInfo.Size() < 4096 {
		return fmt.Errorf("backup file is too small (< 4KB): %s", backupPath)
	}

	return nil
}

// ListBackups finds all backup files for a given hive path
func ListBackups(hivePath string) ([]string, error) {
	dir := filepath.Dir(hivePath)
	base := filepath.Base(hivePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	backups := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Check if this is a backup of our hive
		if len(name) > len(base) && name[:len(base)] == base && name[len(base)] == '.' {
			suffix := name[len(base):]
			if suffix == ".backup" || suffix[:7] == ".backup" {
				backups = append(backups, filepath.Join(dir, name))
			}
		}
	}

	return backups, nil
}
