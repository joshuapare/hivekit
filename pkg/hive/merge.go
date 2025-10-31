package hive

import (
	"bytes"
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/types"
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
func MergeRegFile(hivePath, regPath string, opts *MergeOptions) (*MergeStats, error) {
	// Validate inputs
	if !fileExists(hivePath) {
		return nil, fmt.Errorf("hive file not found: %s", hivePath)
	}
	if !fileExists(regPath) {
		return nil, fmt.Errorf(".reg file not found: %s", regPath)
	}

	// Read .reg file
	regData, err := os.ReadFile(regPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .reg file %s: %w", regPath, err)
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
func MergeRegString(hivePath string, regContent string, opts *MergeOptions) (*MergeStats, error) {
	if !fileExists(hivePath) {
		return nil, fmt.Errorf("hive file not found: %s", hivePath)
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
func MergeRegFiles(hivePath string, regPaths []string, opts *MergeOptions) (*MergeStats, error) {
	if !fileExists(hivePath) {
		return nil, fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Aggregate stats from all files
	aggregatedStats := &MergeStats{}

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

		stats, err := MergeRegFile(hivePath, regPath, fileOpts)
		if err != nil {
			return aggregatedStats, fmt.Errorf("failed to merge %s (file %d/%d): %w", regPath, i+1, total, err)
		}

		// Aggregate stats
		if stats != nil {
			aggregatedStats.KeysCreated += stats.KeysCreated
			aggregatedStats.KeysDeleted += stats.KeysDeleted
			aggregatedStats.ValuesSet += stats.ValuesSet
			aggregatedStats.ValuesDeleted += stats.ValuesDeleted
			aggregatedStats.OperationsTotal += stats.OperationsTotal
			aggregatedStats.OperationsFailed += stats.OperationsFailed
			// BytesWritten from last merge is the final size
			aggregatedStats.BytesWritten = stats.BytesWritten
		}
	}

	return aggregatedStats, nil
}

// mergeRegBytes is the internal implementation that merges .reg data from bytes.
func mergeRegBytes(hivePath string, regData []byte, opts *MergeOptions) (*MergeStats, error) {
	// Apply defaults
	if opts == nil {
		opts = &MergeOptions{}
	}
	if opts.Limits == nil {
		limits := DefaultLimits()
		opts.Limits = &limits
	}

	// Initialize stats
	stats := &MergeStats{}

	// Create backup if requested
	if opts.CreateBackup {
		backupPath := hivePath + ".bak"
		if err := copyFile(hivePath, backupPath); err != nil {
			return nil, fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
		}
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{
		ZeroCopy: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Parse .reg file
	codec := regtext.NewCodec()
	parseOpts := RegParseOptions{
		Prefix:        opts.Prefix,
		AutoPrefix:    opts.AutoPrefix,
		InputEncoding: opts.InputEncoding,
	}
	ops, err := codec.ParseReg(regData, parseOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .reg data: %w", err)
	}

	// Handle empty operation list
	if len(ops) == 0 {
		// Nothing to do, but not an error
		return stats, nil
	}

	// Record total operations
	stats.OperationsTotal = len(ops)

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
		if err := applyOperation(tx, op); err != nil {
			stats.OperationsFailed++
			// Error callback
			if opts.OnError != nil {
				if !opts.OnError(op, err) {
					// User requested abort
					tx.Rollback()
					return stats, fmt.Errorf("operation aborted at %d/%d: %w", i+1, total, err)
				}
				// User requested continue - skip this operation
				continue
			}
			// No error handler - abort on first error
			tx.Rollback()
			return stats, fmt.Errorf("operation %d/%d failed: %w", i+1, total, err)
		}

		// Count successful operations by type
		switch op.(type) {
		case OpCreateKey:
			stats.KeysCreated++
		case OpDeleteKey:
			stats.KeysDeleted++
		case OpSetValue:
			stats.ValuesSet++
		case OpDeleteValue:
			stats.ValuesDeleted++
		}
	}

	// Dry run? Don't commit
	if opts.DryRun {
		tx.Rollback()
		return stats, nil
	}

	// Commit to buffer
	buf := &bytes.Buffer{}
	writeOpts := types.WriteOptions{Repack: opts.Defragment}
	if err := tx.Commit(&bufWriter{buf}, writeOpts); err != nil {
		return stats, fmt.Errorf("failed to commit changes to %s: %w", hivePath, err)
	}

	// Write to file atomically (write to temp, then rename)
	tempPath := hivePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		return stats, fmt.Errorf("failed to write temporary file %s: %w", tempPath, err)
	}

	if err := os.Rename(tempPath, hivePath); err != nil {
		// Clean up temp file on error
		os.Remove(tempPath)
		return stats, fmt.Errorf("failed to replace hive %s: %w", hivePath, err)
	}

	// Record bytes written
	stats.BytesWritten = int64(buf.Len())

	return stats, nil
}
