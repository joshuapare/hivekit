package dirty

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// setupTestHive creates a minimal test hive for testing.
func setupTestHive(t testing.TB) (*hive.Hive, func()) {
	t.Helper()

	// Create temp directory
	tmpDir := t.TempDir()
	hivePath := filepath.Join(tmpDir, "test.hiv")

	// Create minimal hive file (8KB for testing)
	data := make([]byte, 8192)
	// Write REGF signature
	copy(data[0:4], []byte("regf"))

	// Write to file
	if err := os.WriteFile(hivePath, data, 0644); err != nil {
		t.Fatalf("Failed to write test hive: %v", err)
	}

	// Open hive
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("Failed to open test hive: %v", err)
	}

	cleanup := func() {
		h.Close()
	}

	return h, cleanup
}

// Test 1: Page Alignment.
func Test_DirtyTracker_PageAlignment(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add a range that's NOT page-aligned (offset 100, length 200)
	tracker.Add(100, 200)

	// Coalesce
	coalesced := tracker.coalesce()

	// Should be aligned to page boundaries
	// Start: 100 rounds down to 0
	// End: 100+200=300 rounds up to 4096
	if len(coalesced) != 1 {
		t.Fatalf("Expected 1 coalesced range, got %d", len(coalesced))
	}

	if coalesced[0].Off != 0 {
		t.Errorf("Start not aligned: got %d, want 0", coalesced[0].Off)
	}

	if coalesced[0].Len != 4096 {
		t.Errorf("Length not aligned: got %d, want 4096", coalesced[0].Len)
	}
}

// Test 2: Coalescing Adjacent Ranges.
func Test_DirtyTracker_Coalesce_Adjacent(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add two adjacent page-aligned ranges
	tracker.Add(4096, 4096) // Pages 1-2
	tracker.Add(8192, 4096) // Pages 2-3 (adjacent to first)

	coalesced := tracker.coalesce()

	// Should merge into single range
	if len(coalesced) != 1 {
		t.Fatalf("Expected 1 merged range, got %d", len(coalesced))
	}

	if coalesced[0].Off != 4096 {
		t.Errorf("Merged range start: got %d, want 4096", coalesced[0].Off)
	}

	if coalesced[0].Len != 8192 {
		t.Errorf("Merged range length: got %d, want 8192", coalesced[0].Len)
	}
}

// Test 3: Coalescing Overlapping Ranges.
func Test_DirtyTracker_Coalesce_Overlapping(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add overlapping ranges
	tracker.Add(0, 8192)    // Pages 0-1
	tracker.Add(4096, 8192) // Pages 1-2 (overlaps with first)

	coalesced := tracker.coalesce()

	// Should merge into single range covering 0-12288
	if len(coalesced) != 1 {
		t.Fatalf("Expected 1 merged range, got %d", len(coalesced))
	}

	if coalesced[0].Off != 0 {
		t.Errorf("Merged range start: got %d, want 0", coalesced[0].Off)
	}

	if coalesced[0].Len != 12288 {
		t.Errorf("Merged range length: got %d, want 12288", coalesced[0].Len)
	}
}

// Test 4: Non-Overlapping Ranges.
func Test_DirtyTracker_Coalesce_Separate(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add two ranges with a gap between them
	tracker.Add(0, 4096)     // Page 0
	tracker.Add(20480, 4096) // Page 5 (gap of 3 pages)

	coalesced := tracker.coalesce()

	// Should remain as two separate ranges
	if len(coalesced) != 2 {
		t.Fatalf("Expected 2 separate ranges, got %d", len(coalesced))
	}

	// First range
	if coalesced[0].Off != 0 || coalesced[0].Len != 4096 {
		t.Errorf("First range: got (%d, %d), want (0, 4096)",
			coalesced[0].Off, coalesced[0].Len)
	}

	// Second range
	if coalesced[1].Off != 20480 || coalesced[1].Len != 4096 {
		t.Errorf("Second range: got (%d, %d), want (20480, 4096)",
			coalesced[1].Off, coalesced[1].Len)
	}
}

// Test 5: Flush Data Only (excludes header).
func Test_DirtyTracker_FlushDataOnly_ExcludesHeader(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add header range and data range
	tracker.Add(0, 100)    // Header
	tracker.Add(4096, 100) // Data

	// FlushDataOnly should skip header
	err := tracker.FlushDataOnly(context.Background())
	if err != nil {
		t.Fatalf("FlushDataOnly() failed: %v", err)
	}

	// Ranges should be cleared
	if len(tracker.ranges) != 0 {
		t.Errorf("Ranges not cleared after flush: got %d, want 0", len(tracker.ranges))
	}
}

// Test 6: Flush Header.
func Test_DirtyTracker_FlushHeader(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// FlushHeaderAndMeta should not error
	err := tracker.FlushHeaderAndMeta(context.Background(), FlushAuto)
	if err != nil {
		t.Fatalf("FlushHeaderAndMeta() failed: %v", err)
	}
}

// Test 7: Reset.
func Test_DirtyTracker_Reset(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add multiple ranges
	tracker.Add(0, 100)
	tracker.Add(4096, 200)
	tracker.Add(8192, 300)

	if len(tracker.ranges) != 3 {
		t.Fatalf("Expected 3 ranges before reset, got %d", len(tracker.ranges))
	}

	// Reset
	tracker.Reset()

	// Should be empty
	if len(tracker.ranges) != 0 {
		t.Errorf("Ranges not cleared after reset: got %d, want 0", len(tracker.ranges))
	}
}

// Test 8: Empty Flush.
func Test_DirtyTracker_FlushDataOnly_Empty(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Flush with no ranges added
	err := tracker.FlushDataOnly(context.Background())
	if err != nil {
		t.Fatalf("FlushDataOnly() on empty tracker failed: %v", err)
	}
}

// Test 9: Large Range Count.
func Test_DirtyTracker_Coalesce_ManyRanges(t *testing.T) {
	h, cleanup := setupTestHive(t)
	defer cleanup()

	tracker := NewTracker(h)

	// Add 100 random ranges (some will overlap/be adjacent)
	// Create a pattern: every other page for 100 pages
	for i := range 100 {
		off := i * 8192 // Every 2 pages
		tracker.Add(off, 4096)
	}

	coalesced := tracker.coalesce()

	// Should be sorted
	for i := 1; i < len(coalesced); i++ {
		if coalesced[i].Off <= coalesced[i-1].Off {
			t.Errorf("Ranges not sorted: range %d offset %d <= range %d offset %d",
				i, coalesced[i].Off, i-1, coalesced[i-1].Off)
		}
	}

	// Verify no overlaps
	for i := 1; i < len(coalesced); i++ {
		prevEnd := coalesced[i-1].Off + coalesced[i-1].Len
		if coalesced[i].Off < prevEnd {
			t.Errorf("Overlapping ranges: range %d starts at %d, but range %d ends at %d",
				i, coalesced[i].Off, i-1, prevEnd)
		}
	}

	t.Logf("Coalesced %d ranges into %d", 100, len(coalesced))
}

// Test 10: FlushMode variations.
func Test_DirtyTracker_FlushModes(t *testing.T) {
	tests := []struct {
		name string
		mode FlushMode
	}{
		{"FlushAuto", FlushAuto},
		{"FlushDataOnly", FlushDataOnly},
		{"FlushFull", FlushFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cleanup := setupTestHive(t)
			defer cleanup()

			tracker := NewTracker(h)

			// Should not error
			err := tracker.FlushHeaderAndMeta(context.Background(), tt.mode)
			if err != nil {
				t.Errorf("FlushHeaderAndMeta(%v) failed: %v", tt.mode, err)
			}
		})
	}
}

// Benchmark: Add() performance.
func Benchmark_DirtyTracker_Add(b *testing.B) {
	h, cleanup := setupTestHive(b)
	defer cleanup()

	tracker := NewTracker(h)

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		tracker.Add(4096*i, 4096)
	}
}

// Benchmark: Coalesce 100 ranges.
func Benchmark_DirtyTracker_Coalesce_100Ranges(b *testing.B) {
	h, cleanup := setupTestHive(b)
	defer cleanup()

	tracker := NewTracker(h)

	// Pre-populate with 100 ranges
	for i := range 100 {
		tracker.Add(i*4096, 4096)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_ = tracker.coalesce()
	}
}

// Benchmark: Full Add + Coalesce cycle.
func Benchmark_DirtyTracker_AddAndCoalesce(b *testing.B) {
	h, cleanup := setupTestHive(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		tracker := NewTracker(h)
		for j := range 10 {
			tracker.Add(j*4096, 4096)
		}
		_ = tracker.coalesce()
	}
}
