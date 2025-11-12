//go:build linux || darwin

package alloc

import (
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// mockDirtyTracker records Add() calls for testing.
type mockDirtyTracker struct {
	calls []addCall
}

type addCall struct {
	off    int
	length int
}

func (m *mockDirtyTracker) Add(off, length int) {
	m.calls = append(m.calls, addCall{off: off, length: length})
}

// Test_FastAlloc_Grow_MarksHeaderDirty verifies that Grow() marks the header
// as dirty after modifying the REGF "blocks" field at offset 0x28.
//
// This is CRITICAL for hivexsh compatibility. When we grow the hive:
// 1. We update the field at offset 0x28 (sum of HBIN sizes)
// 2. We MUST mark the header dirty so tx.Commit() flushes it
// 3. Otherwise hivexsh reads stale value and reports "trailing garbage"
//
// This test will FAIL before the fix is applied, proving the bug exists.
// After the fix, it will PASS, proving correct behavior.
func Test_FastAlloc_Grow_MarksHeaderDirty(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test-grow-dirty.hiv")

	// Create a minimal test hive
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create mock dirty tracker to capture Add() calls
	mockTracker := &mockDirtyTracker{}

	// Create allocator with dirty tracker
	// NOTE: This will fail to compile until we update NewFast() signature
	fa, err := NewFast(h, mockTracker, nil)
	if err != nil {
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Record initial state
	initialCalls := len(mockTracker.calls)

	// Call Grow() - this should mark the header dirty
	err = fa.GrowByPages(2) // Add 8KB HBIN // Request 8KB, will create ~8KB HBIN
	if err != nil {
		t.Fatalf("Grow() failed: %v", err)
	}

	// Verify that Add() was called to mark header dirty
	newCalls := mockTracker.calls[initialCalls:]

	if len(newCalls) == 0 {
		t.Fatalf("BUG: Grow() did not mark any ranges as dirty!\n" +
			"This means the header modification at offset 0x28 won't be flushed.\n" +
			"Result: hivexsh will report 'trailing garbage at end of file'")
	}

	// Verify the header range was marked dirty
	foundHeader := false
	for _, call := range newCalls {
		if call.off == 0 && call.length == format.HeaderSize {
			foundHeader = true
			break
		}
	}

	if !foundHeader {
		t.Errorf("BUG: Grow() did not mark header as dirty!\n"+
			"Expected: Add(0, %d) call\n"+
			"Got calls: %+v\n"+
			"This will cause hivexsh 'trailing garbage' errors!",
			format.HeaderSize, newCalls)
	}

	t.Logf("Grow() correctly marked header dirty: Add(0, %d)", format.HeaderSize)
}
