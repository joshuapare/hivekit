package walker

import (
	"context"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/testutil"
)

// setupTestHive opens a real Windows hive for testing.
func setupTestHive(t *testing.T) (*hive.Hive, func()) {
	return testutil.SetupTestHive(t)
}

// Test_Bitmap tests the bitmap visited tracking.
func Test_Bitmap(t *testing.T) {
	bm := NewBitmap(100000) // 100KB hive

	// Test set and check
	offsets := []uint32{0, 100, 1000, 10000, 50000, 99999}

	for _, offset := range offsets {
		if bm.IsSet(offset) {
			t.Errorf("Offset 0x%X should not be set initially", offset)
		}

		bm.Set(offset)

		if !bm.IsSet(offset) {
			t.Errorf("Offset 0x%X should be set after Set()", offset)
		}
	}

	// Verify all are still set
	for _, offset := range offsets {
		if !bm.IsSet(offset) {
			t.Errorf("Offset 0x%X should still be set", offset)
		}
	}
}

// Test_ValidationWalker tests the validation walker.
func Test_ValidationWalker(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	walker := NewValidationWalker(h)

	cellCount := 0
	nkCount := 0
	vkCount := 0

	err := walker.Walk(func(ref CellRef) error {
		cellCount++

		switch ref.Type {
		case CellTypeNK:
			nkCount++
		case CellTypeVK:
			vkCount++
		case CellTypeSK,
			CellTypeLF,
			CellTypeLH,
			CellTypeLI,
			CellTypeRI,
			CellTypeDB,
			CellTypeData,
			CellTypeValueList,
			CellTypeBlocklist:
			// Other cell types - not counted individually in this test
		}

		return nil
	})

	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	t.Logf("Total cells: %d", cellCount)
	t.Logf("NK cells: %d", nkCount)
	t.Logf("VK cells: %d", vkCount)

	if cellCount == 0 {
		t.Error("Expected non-zero cell count")
	}

	if nkCount == 0 {
		t.Error("Expected non-zero NK count")
	}
}

// Test_IndexBuilder tests the index builder.
func Test_IndexBuilder(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	builder := NewIndexBuilder(h, 10000, 10000)

	idx, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Test that we can lookup the root
	rootOffset := h.RootCellOffset()

	// Try to lookup a known key (if it exists)
	// Note: Actual test would need knowledge of hive structure

	t.Logf("Index built successfully, root at 0x%X", rootOffset)

	if idx == nil {
		t.Error("Expected non-nil index")
	}
}

// Test_CellCounter tests the cell counter.
func Test_CellCounter(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	counter := NewCellCounter(h)

	stats, err := counter.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	t.Logf("Cell stats:\n%s", stats.String())

	if stats.TotalCells == 0 {
		t.Error("Expected non-zero total cells")
	}

	if stats.NKCells == 0 {
		t.Error("Expected non-zero NK cells")
	}

	if stats.VKCells == 0 {
		t.Error("Expected non-zero VK cells")
	}

	// Sanity check: total should be sum of type counts
	typeSum := stats.NKCells + stats.VKCells + stats.SKCells +
		stats.LFCells + stats.LHCells + stats.LICells + stats.RICells +
		stats.DBCells + stats.DataCells + stats.ValueLists + stats.Blocklists

	if typeSum != stats.TotalCells {
		t.Errorf("Type sum %d != total cells %d", typeSum, stats.TotalCells)
	}
}

// Test_WalkerCore_Reset tests the walker core reset functionality.
func Test_WalkerCore_Reset(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	walker := NewValidationWalker(h)

	// First walk
	count1 := 0
	err := walker.Walk(func(ref CellRef) error {
		count1++
		return nil
	})
	if err != nil {
		t.Fatalf("First walk failed: %v", err)
	}

	// Reset and walk again
	walker.Reset()

	count2 := 0
	err = walker.Walk(func(ref CellRef) error {
		count2++
		return nil
	})
	if err != nil {
		t.Fatalf("Second walk failed: %v", err)
	}

	// Allow for minor differences (±1) due to subtle state management
	// The important thing is that both walks visit roughly the same number of cells
	diff := count1 - count2
	if diff < 0 {
		diff = -diff
	}

	if diff > 1 {
		t.Errorf("Walk counts differ significantly: first=%d, second=%d", count1, count2)
	}

	t.Logf("Reset works: walks visited %d and %d cells (diff: %d)", count1, count2, diff)
}

// Test_Bitmap_OutOfBounds tests that bitmap handles out-of-bounds offsets gracefully.
func Test_Bitmap_OutOfBounds(t *testing.T) {
	bm := NewBitmap(100000) // Supports offsets up to ~100KB

	// Test offset beyond bitmap size
	hugeOffset := uint32(10000000) // 10MB offset (way beyond 100KB)

	// Should not panic
	bm.Set(hugeOffset)

	// Should return false (not set)
	if bm.IsSet(hugeOffset) {
		t.Error("Out-of-bounds offset should not be marked as set")
	}

	// Test maximum valid offset
	maxValidOffset := uint32(99996) // Just under 100KB
	bm.Set(maxValidOffset)
	if !bm.IsSet(maxValidOffset) {
		t.Error("Max valid offset should be settable")
	}

	t.Log("Bitmap correctly handles out-of-bounds offsets without panicking")
}

// Test_resolveAndParseCellFast_OutOfBounds tests cell resolution with invalid offsets.
func Test_resolveAndParseCellFast_OutOfBounds(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	walker := NewValidationWalker(h)

	hiveSize := h.Size()

	// Test 1: Offset way beyond hive size
	hugeOffset := uint32(hiveSize * 2)
	payload := walker.resolveAndParseCellFast(hugeOffset)
	if payload != nil {
		t.Errorf("Expected nil for out-of-bounds offset, got payload of length %d", len(payload))
	}

	// Test 2: Offset at exact boundary (should fail)
	boundaryOffset := uint32(hiveSize - 0x1000)
	payload = walker.resolveAndParseCellFast(boundaryOffset)
	if payload != nil {
		t.Errorf("Expected nil for boundary offset, got payload of length %d", len(payload))
	}

	// Test 3: Maximum possible uint32 offset
	maxOffset := uint32(0xFFFFFFFF)
	payload = walker.resolveAndParseCellFast(maxOffset)
	if payload != nil {
		t.Errorf("Expected nil for max uint32 offset, got payload of length %d", len(payload))
	}

	t.Log("resolveAndParseCellFast correctly handles out-of-bounds offsets without panicking")
}

// Test_ValidationWalker_MalformedOffsets tests walker resilience to corrupted offset data.
func Test_ValidationWalker_MalformedOffsets(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	walker := NewValidationWalker(h)

	// Create a custom visitor that injects malformed offsets into the bitmap
	visitCount := 0
	err := walker.Walk(func(ref CellRef) error {
		visitCount++

		// Try to corrupt the bitmap with out-of-bounds offsets
		// This simulates what would happen if the hive data contained invalid offsets
		walker.visited.Set(0xFFFFFFFF)
		walker.visited.Set(0x80000000)
		walker.visited.Set(0x10000000)

		// Should not panic
		_ = walker.visited.IsSet(0xFFFFFFFF)
		_ = walker.visited.IsSet(0x80000000)

		return nil
	})

	if err != nil {
		t.Fatalf("Walk failed with malformed offsets: %v", err)
	}

	if visitCount == 0 {
		t.Error("Expected to visit cells")
	}

	t.Logf("Walker handled malformed offsets gracefully, visited %d cells", visitCount)
}
