//go:build linux || darwin

package alloc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/tx"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// Test_FastAlloc_Grow_Integration_DirtyTracking is a REAL integration test
// that validates the complete flow:
//  1. Use REAL dirty.Tracker (not mocked)
//  2. Use REAL FastAllocator.Grow()
//  3. Use REAL tx.Manager to commit
//  4. Verify field at 0x28 is updated on disk
//  5. Verify hivexsh can parse the result
//
// This test will FAIL without the fix, proving the bug exists at integration level.
func Test_FastAlloc_Grow_Integration_DirtyTracking(t *testing.T) {
	// Use a REAL test hive instead of synthetic one
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test-grow-integration.hiv")

	// Copy the real test hive to temp location
	srcData, err := os.ReadFile(testHivePath)
	if err != nil {
		t.Skipf("Test hive not found: %v", err)
	}
	if writeErr := os.WriteFile(hivePath, srcData, 0644); writeErr != nil {
		t.Fatalf("Failed to copy test hive: %v", writeErr)
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Read INITIAL value at offset 0x28 (before grow)
	data := h.Bytes()
	initialDataSize := format.ReadU32(data, format.REGFDataSizeOffset)
	initialFileSize := len(data)
	expectedInitialSize := int(initialDataSize) + format.HeaderSize
	t.Logf("Initial data size at 0x28: 0x%X", initialDataSize)
	t.Logf("Initial file size: 0x%X (expected: 0x%X)", initialFileSize, expectedInitialSize)
	if initialFileSize != expectedInitialSize {
		t.Logf("WARNING: Initial file size (0x%X) != header field + 0x1000 (0x%X), delta: 0x%X",
			initialFileSize, expectedInitialSize, initialFileSize-expectedInitialSize)
	}

	// Create REAL dirty tracker
	dt := dirty.NewTracker(h)

	// Create REAL transaction manager
	txMgr := tx.NewManager(h, dt, dirty.FlushAuto)

	// Begin transaction
	if beginErr := txMgr.Begin(context.Background()); beginErr != nil {
		t.Fatalf("Begin() failed: %v", beginErr)
	}

	// Create allocator with REAL dirty tracker
	fa, err := NewFast(h, dt, nil)
	if err != nil {
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Call GrowByPages() - this should mark header dirty
	t.Logf("Requesting GrowByPages(2) = 8KB")
	growErr := fa.GrowByPages(2) // Add 8KB HBIN
	if growErr != nil {
		t.Fatalf("GrowByPages() failed: %v", growErr)
	}

	// Read UPDATED value at offset 0x28 (after grow, before commit)
	data = h.Bytes()
	afterGrowDataSize := format.ReadU32(data, format.REGFDataSizeOffset)
	afterGrowFileSize := len(data)
	expectedAfterGrowSize := int(afterGrowDataSize) + format.HeaderSize
	t.Logf("After grow data size at 0x28: 0x%X", afterGrowDataSize)
	t.Logf("After grow file size: 0x%X (expected: 0x%X)", afterGrowFileSize, expectedAfterGrowSize)
	t.Logf("File grew by: 0x%X bytes (requested: 2 pages = 8KB)", afterGrowFileSize-initialFileSize)
	if afterGrowFileSize != expectedAfterGrowSize {
		t.Logf("WARNING: After-grow file size (0x%X) != header field + 0x1000 (0x%X), delta: 0x%X",
			afterGrowFileSize, expectedAfterGrowSize, afterGrowFileSize-expectedAfterGrowSize)
	}

	// Verify field was updated in memory
	if afterGrowDataSize <= initialDataSize {
		t.Fatalf("BUG: Field at 0x28 was NOT updated after Grow()!\n"+
			"Before: 0x%X, After: 0x%X", initialDataSize, afterGrowDataSize)
	}

	// Commit transaction - this should flush header to disk
	if commitErr := txMgr.Commit(context.Background()); commitErr != nil {
		t.Fatalf("Commit() failed: %v", commitErr)
	}

	// Close hive to ensure all data is flushed
	h.Close()

	// ============================================================
	// CRITICAL TEST: Reopen hive and verify 0x28 was persisted
	// ============================================================

	h, reopenErr := hive.Open(hivePath)
	if reopenErr != nil {
		t.Fatalf("Failed to reopen hive: %v", reopenErr)
	}
	defer h.Close()

	// Read value at 0x28 from DISK
	data = h.Bytes()
	onDiskDataSize := format.ReadU32(data, format.REGFDataSizeOffset)
	onDiskFileSize := len(data)
	expectedOnDiskSize := int(onDiskDataSize) + format.HeaderSize
	t.Logf("On-disk data size at 0x28: 0x%X", onDiskDataSize)
	t.Logf("On-disk file size: 0x%X (expected: 0x%X)", onDiskFileSize, expectedOnDiskSize)
	if onDiskFileSize != expectedOnDiskSize {
		t.Logf("🔴 BUG: On-disk file size (0x%X) != header field + 0x1000 (0x%X), delta: 0x%X",
			onDiskFileSize, expectedOnDiskSize, onDiskFileSize-expectedOnDiskSize)
		t.Logf("    This is why hivexsh reports 'trailing garbage at end of file'!")
	}

	// THIS IS THE CRITICAL ASSERTION
	// If this fails, it means the header wasn't marked dirty and thus wasn't flushed
	if onDiskDataSize != afterGrowDataSize {
		t.Fatalf("BUG CONFIRMED: Header field at 0x28 was NOT flushed to disk!\n"+
			"In-memory after grow: 0x%X\n"+
			"On disk after reopen: 0x%X\n"+
			"This means Grow() did NOT mark the header dirty.\n"+
			"Result: hivexsh will report 'trailing garbage at end of file'",
			afterGrowDataSize, onDiskDataSize)
	}

	t.Logf("Field at 0x28 correctly persisted to disk: 0x%X", onDiskDataSize)

	// ============================================================
	// BONUS: Validate with hivexsh if available
	// ============================================================

	if !hivexval.IsHivexshAvailable() {
		t.Logf("hivexsh not available, skipping external validation")
		return
	}

	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{UseHivexsh: true}))
	defer v.Close()

	v.AssertHivexshValid(t)

	t.Logf("hivexsh successfully parsed the grown hive!")
	t.Logf("   This confirms the fix works end-to-end.")
}
