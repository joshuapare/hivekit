/*
Package hivex provides a high-level, ergonomic API for Windows registry hive operations.

# Quick Start

Merge a .reg file into a hive:

	err := ops.MergeRegFile("system.hive", "delta.reg", nil)

# Features

  - Simple one-function merge API
  - Automatic registry limits validation
  - Progress reporting and error handling
  - Batch operations
  - Export to .reg format
  - Defragmentation support
  - Transaction safety
  - Zero-copy performance

# Basic Usage

Merge a registry delta file:

	err := ops.MergeRegFile("software.hive", "changes.reg", nil)
	if err != nil {
	    log.Fatal(err)
	}

Merge with progress reporting:

	opts := &ops.MergeOptions{
	    OnProgress: func(current, total int) {
	        fmt.Printf("Progress: %d/%d\n", current, total)
	    },
	    Defragment: true,
	}
	err := ops.MergeRegFile("system.hive", "delta.reg", opts)

Merge multiple files:

	regFiles := []string{"base.reg", "patch1.reg", "patch2.reg"}
	err := ops.MergeRegFiles("system.hive", regFiles, nil)

Export hive to .reg format:

	err := ops.ExportReg("software.hive", "backup.reg", nil)

# Error Handling

By default, the first error aborts the operation. For more control:

	var errors []error
	opts := &ops.MergeOptions{
	    OnError: func(op ops.EditOp, err error) bool {
	        errors = append(errors, err)
	        return true // continue despite errors
	    },
	}
	ops.MergeRegFile("system.hive", "delta.reg", opts)

# Registry Limits

Registry limits are enforced by default to prevent corruption:

	// Use default Windows limits (safe)
	ops.MergeRegFile("system.hive", "delta.reg", nil)

	// Use relaxed limits for system keys
	opts := &ops.MergeOptions{
	    Limits: ops.RelaxedLimits(),
	}
	ops.MergeRegFile("system.hive", "delta.reg", opts)

	// Use strict limits for constrained environments
	opts := &ops.MergeOptions{
	    Limits: ops.StrictLimits(),
	}
	ops.MergeRegFile("system.hive", "delta.reg", opts)

# Dry Run and Validation

Test changes without applying them:

	opts := &ops.MergeOptions{
	    DryRun: true,
	    Limits: ops.StrictLimits(),
	}
	if err := ops.MergeRegFile("system.hive", "risky.reg", opts); err != nil {
	    log.Printf("Validation failed: %v", err)
	    return
	}
	log.Println("Changes are safe to apply")

# Advanced Usage

For advanced users who need fine-grained control, use the low-level API:

	import (
	    "github.com/joshuapare/hivekit/internal/reader"
	    "github.com/joshuapare/hivekit/internal/edit"
	    "github.com/joshuapare/hivekit/pkg/types"
	)

	r, _ := reader.Open("system.hive", OpenOptions{ZeroCopy: true})
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.Begin()
	tx.CreateKey("Software\\MyApp", CreateKeyOptions{})
	// ... fine-grained operations

# Performance

gohivex is optimized for speed:
  - 597x faster than libhivex for small changes
  - Zero-copy reads when safe
  - Incremental AST for efficient merges
  - <2% overhead for limits validation

# Safety

  - Registry limits enforced by default
  - Transaction-based edits (atomic commit or rollback)
  - Automatic validation before commit
  - Optional backup creation
*/
package hive
