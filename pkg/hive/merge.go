package hive

import (
	"bytes"
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
)

// MergeBackend specifies which merge implementation to use.
type MergeBackend string

const (
	// MergeBackendNew uses the new fast mmap-based merge (default, 4-119x faster).
	MergeBackendNew MergeBackend = "new"
	// MergeBackendOld uses the legacy full-rebuild merge (for compatibility testing).
	MergeBackendOld MergeBackend = "old"
)

// MergeRegFile merges a .reg file into a registry
//
// The hive file is modified in-place. Registry limits are enforced by default
// (use opts.Limits to customize). The operation is atomic - if any error occurs
// (and OnError is not set), no changes are made to the
//
// Example:
//
//	err := ops.MergeRegFile("system.hive", "delta.reg", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// With options:
//
//	opts := &ops.MergeOptions{
//	    OnProgress: func(current, total int) {
//	        fmt.Printf("Progress: %d/%d\n", current, total)
//	    },
//	    Defragment: true,
//	    CreateBackup: true,
//	}
//	err := ops.MergeRegFile("system.hive", "delta.reg", opts)
func MergeRegFile(hivePath, regPath string, opts *MergeOptions) error {
	// Validate inputs
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}
	if !fileExists(regPath) {
		return fmt.Errorf(".reg file not found: %s", regPath)
	}

	// Read .reg file
	regData, err := os.ReadFile(regPath)
	if err != nil {
		return fmt.Errorf("failed to read .reg file %s: %w", regPath, err)
	}

	return mergeRegBytes(hivePath, regData, opts)
}

// MergeRegString merges registry operations from a string.
//
// The string should be in Windows .reg format. This is useful for
// programmatically generated registry changes or embedded data.
//
// Example:
//
//	regContent := `Windows Registry Editor Version 5.00
//
//	[HKEY_LOCAL_MACHINE\Software\MyApp]
//	"Version"="1.0"
//	`
//	err := ops.MergeRegString("software.hive", regContent, nil)
func MergeRegString(hivePath string, regContent string, opts *MergeOptions) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	return mergeRegBytes(hivePath, []byte(regContent), opts)
}

// MergeRegFiles merges multiple .reg files into a hive in order.
//
// Each file is merged sequentially. If OnProgress is set, it's called
// after each file completes. If any merge fails and OnError is not set,
// the operation stops immediately.
//
// Example:
//
//	regFiles := []string{"base.reg", "patch1.reg", "patch2.reg"}
//	err := ops.MergeRegFiles("system.hive", regFiles, &ops.MergeOptions{
//	    OnProgress: func(current, total int) {
//	        fmt.Printf("Merging file %d/%d\n", current, total)
//	    },
//	})
func MergeRegFiles(hivePath string, regPaths []string, opts *MergeOptions) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	total := len(regPaths)
	for i, regPath := range regPaths {
		// Report progress for file-level operations
		if opts != nil && opts.OnProgress != nil {
			opts.OnProgress(i+1, total)
		}

		// Create a copy of opts without progress callback for individual merges
		// (to avoid double progress reporting)
		fileOpts := &MergeOptions{}
		if opts != nil {
			*fileOpts = *opts
			fileOpts.OnProgress = nil // Disable operation-level progress
		}

		if err := MergeRegFile(hivePath, regPath, fileOpts); err != nil {
			return fmt.Errorf("failed to merge %s (file %d/%d): %w", regPath, i+1, total, err)
		}
	}

	return nil
}

// mergeRegBytes routes to the appropriate backend implementation.
func mergeRegBytes(hivePath string, regData []byte, opts *MergeOptions) error {
	// Set default backend to new
	if opts == nil {
		opts = &MergeOptions{}
	}
	if opts.Backend == "" {
		opts.Backend = MergeBackendNew
	}

	// Route to backend
	switch opts.Backend {
	case MergeBackendNew:
		return mergeRegBytesNew(hivePath, regData, opts)
	case MergeBackendOld:
		return mergeRegBytesOld(hivePath, regData, opts)
	default:
		return fmt.Errorf("unknown merge backend: %s", opts.Backend)
	}
}

// mergeRegBytesNew uses the new fast mmap-based implementation with query optimization.
func mergeRegBytesNew(hivePath string, regData []byte, opts *MergeOptions) error {
	// TODO: Add support for DryRun, OnProgress, OnError callbacks

	// Create backup if requested
	if opts.CreateBackup {
		backupPath := hivePath + ".bak"
		if err := copyFile(hivePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
		}
	}

	// Use optimized path (PlanFromRegTexts applies query optimization even for single files)
	// This automatically applies:
	//   - Deduplication (remove redundant ops within the .reg file)
	//   - Delete shadowing (remove ops under deleted subtrees)
	//   - I/O-efficient ordering (group by key, parents before children)
	regTexts := []string{string(regData)}
	plan, _, err := merge.PlanFromRegTexts(regTexts)
	if err != nil {
		return fmt.Errorf("parse and optimize: %w", err)
	}

	// Apply optimized plan
	_, err = merge.MergePlan(hivePath, plan, nil)
	return err
}

// mergeRegBytesOld uses the legacy full-rebuild implementation.
func mergeRegBytesOld(hivePath string, regData []byte, opts *MergeOptions) error {
	// Apply defaults
	if opts == nil {
		opts = &MergeOptions{}
	}
	if opts.Limits == nil {
		limits := DefaultLimits()
		opts.Limits = &limits
	}

	// Create backup if requested
	if opts.CreateBackup {
		backupPath := hivePath + ".bak"
		if err := copyFile(hivePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
		}
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{})
	if err != nil {
		return fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Parse .reg file
	codec := regtext.NewCodec()
	ops, err := codec.ParseReg(regData, RegParseOptions{})
	if err != nil {
		return fmt.Errorf("failed to parse .reg data: %w", err)
	}

	// Handle empty operation list
	if len(ops) == 0 {
		// Nothing to do, but not an error
		return nil
	}

	// Start transaction with limits
	ed := edit.NewEditor(r)
	tx := ed.BeginWithLimits(*opts.Limits)

	// Apply operations
	total := len(ops)
	for i, op := range ops {
		// Progress callback
		if opts.OnProgress != nil {
			opts.OnProgress(i+1, total)
		}

		// Apply operation
		if applyErr := applyOperation(tx, op); applyErr != nil {
			// Error callback
			if opts.OnError != nil {
				if !opts.OnError(op, applyErr) {
					// User requested abort
					if rbErr := tx.Rollback(); rbErr != nil {
						return fmt.Errorf(
							"operation aborted at %d/%d: %w (rollback error: %w)",
							i+1,
							total,
							applyErr,
							rbErr,
						)
					}
					return fmt.Errorf("operation aborted at %d/%d: %w", i+1, total, applyErr)
				}
				// User requested continue - skip this operation
				continue
			}
			// No error handler - abort on first error
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("operation %d/%d failed: %w (rollback error: %w)", i+1, total, applyErr, rbErr)
			}
			return fmt.Errorf("operation %d/%d failed: %w", i+1, total, applyErr)
		}
	}

	// Dry run? Don't commit
	if opts.DryRun {
		return tx.Rollback()
	}

	// Commit to buffer
	buf := &bytes.Buffer{}
	writeOpts := types.WriteOptions{Repack: opts.Defragment}
	if commitErr := tx.Commit(&bufWriter{buf}, writeOpts); commitErr != nil {
		return fmt.Errorf("failed to commit changes to %s: %w", hivePath, commitErr)
	}

	// Write to file atomically (write to temp, then rename)
	tempPath := hivePath + ".tmp"
	if writeErr := os.WriteFile(tempPath, buf.Bytes(), 0644); writeErr != nil {
		return fmt.Errorf("failed to write temporary file %s: %w", tempPath, writeErr)
	}

	if renameErr := os.Rename(tempPath, hivePath); renameErr != nil {
		// Clean up temp file on error
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace hive %s: %w", hivePath, renameErr)
	}

	return nil
}
