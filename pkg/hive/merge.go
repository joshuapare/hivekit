package hive

import (
	"context"
	"fmt"
	"os"

	"github.com/joshuapare/hivekit/hive/merge"
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
		// Merge each file in sequence
		if err := MergeRegFile(hivePath, regPath, opts); err != nil {
			return fmt.Errorf("failed to merge %s (file %d/%d): %w", regPath, i+1, total, err)
		}
	}

	return nil
}

// mergeRegBytes merges .reg file data into a hive using the mmap-based implementation.
func mergeRegBytes(hivePath string, regData []byte, opts *MergeOptions) error {
	// Apply defaults
	if opts == nil {
		opts = &MergeOptions{}
	}

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
	_, err = merge.MergePlan(context.Background(), hivePath, plan, nil)
	return err
}
