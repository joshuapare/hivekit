package hive

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/ast"
	"github.com/joshuapare/hivekit/pkg/types"
)

// Defragment compacts a registry hive and rewrites it.
//
// Defragmentation improves performance and reduces file size by:
//   - Compacting free cells
//   - Rebucketing data for better locality
//   - Removing unused space
//
// The operation is safe and atomic - the original file is replaced only
// after successful compaction. A backup is created at <hivePath>.bak
// before modification.
//
// Example:
//
//	err := ops.Defragment("software.hive")
//	if err != nil {
//	    log.Fatal(err)
//	}
func Defragment(hivePath string) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Create backup
	backupPath := hivePath + ".bak"
	if err := copyFile(hivePath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup at %s: %w", backupPath, err)
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

	// Start transaction (no changes, just defragment on commit)
	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Commit with repack enabled
	buf := &bytes.Buffer{}
	writeOpts := types.WriteOptions{Repack: true}
	if commitErr := tx.Commit(&bufWriter{buf}, writeOpts); commitErr != nil {
		return fmt.Errorf("failed to defragment hive %s: %w", hivePath, commitErr)
	}

	// Write atomically (temp file, then rename)
	tempPath := hivePath + ".tmp"
	if writeErr := os.WriteFile(tempPath, buf.Bytes(), 0644); writeErr != nil {
		return fmt.Errorf("failed to write temporary file %s: %w", tempPath, writeErr)
	}

	if renameErr := os.Rename(tempPath, hivePath); renameErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to replace hive %s: %w", hivePath, renameErr)
	}

	return nil
}

// ValidateHive checks a registry hive against specified limits.
//
// This validates the hive structure without modifying it. Useful for:
//   - Checking if a hive will work on Windows
//   - Verifying hive integrity
//   - Pre-flight checks before deployment
//
// If limits is nil, DefaultLimits() is used.
//
// Example:
//
//	err := ops.ValidateHive("system.hive", ops.DefaultLimits())
//	if err != nil {
//	    log.Printf("Hive validation failed: %v", err)
//	    return
//	}
//	log.Println("Hive is valid")
//
// Example with strict limits:
//
//	err := ops.ValidateHive("system.hive", ops.StrictLimits())
//	if err != nil {
//	    log.Printf("Hive exceeds strict limits: %v", err)
//	}
func ValidateHive(hivePath string, limits Limits) error {
	if !fileExists(hivePath) {
		return fmt.Errorf("hive file not found: %s", hivePath)
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

	// For validation, we need to build a full AST of the current hive
	// Use an empty transaction (no changes)
	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Get base buffer from reader (for zero-copy AST building)
	var baseHive []byte
	if bb, ok := r.(interface{ BaseBuffer() []byte }); ok {
		baseHive = bb.BaseBuffer()
	}

	// Build AST (this represents the current state of the hive)
	// The empty transaction means we're building from the base hive only
	txWithMethods, ok := tx.(interface {
		GetCreatedKeys() map[string]bool
		GetDeletedKeys() map[string]bool
		GetSetValues() map[ast.ValueKey]ast.ValueData
		GetDeletedValues() map[ast.ValueKey]bool
	})
	if !ok {
		_ = tx.Rollback()
		return errors.New("transaction does not implement required methods")
	}

	tree, buildErr := ast.BuildIncremental(r, txWithMethods, baseHive)
	if buildErr != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("failed to build AST for validation: %w (rollback error: %w)", buildErr, rbErr)
		}
		return fmt.Errorf("failed to build AST for validation: %w", buildErr)
	}

	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		return fmt.Errorf("failed to rollback read-only transaction: %w", rollbackErr)
	}

	// Validate against limits
	if validateErr := tree.ValidateTree(limits); validateErr != nil {
		return fmt.Errorf("hive validation failed: %w", validateErr)
	}

	return nil
}

// HiveInfo returns basic information about a registry
//
// This includes:
//   - Root key count
//   - Total tree depth
//   - Estimated size
//
// Returns a map with string keys for flexibility in future additions.
//
// Example:
//
//	info, err := hive.HiveStats("system.hive")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Root keys: %s\n", info["root_keys"])
//	fmt.Printf("Max depth: %s\n", info["max_depth"])
func HiveStats(hivePath string) (map[string]string, error) {
	if !fileExists(hivePath) {
		return nil, fmt.Errorf("hive file not found: %s", hivePath)
	}

	// Open hive
	hiveData, err := os.ReadFile(hivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read hive %s: %w", hivePath, err)
	}

	r, err := reader.OpenBytes(hiveData, OpenOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to open hive %s: %w", hivePath, err)
	}
	defer r.Close()

	// Get root node
	rootNode, err := r.Root()
	if err != nil {
		return nil, fmt.Errorf("failed to get root node: %w", err)
	}

	// Count root keys
	subkeys, err := r.Subkeys(rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to get subkeys: %w", err)
	}

	// Build info map
	info := map[string]string{
		"root_keys": strconv.Itoa(len(subkeys)),
		"file_size": strconv.Itoa(len(hiveData)),
	}

	return info, nil
}
