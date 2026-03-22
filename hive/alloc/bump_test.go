package alloc

import (
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestBumpMode_BasicAllocation enables bump mode, allocates a few cells, and
// verifies that offsets are sequential within the bump region.
func TestBumpMode_BasicAllocation(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	// Enable bump mode with 4KB (one full page worth of bump space)
	if err := fa.EnableBumpMode(4096); err != nil {
		t.Fatalf("EnableBumpMode failed: %v", err)
	}

	if !fa.bump.active {
		t.Fatal("bump mode should be active after Enable")
	}

	// Allocate three 64-byte cells (each 64 bytes total = 4-byte header + 60 payload)
	refs := make([]CellRef, 3)
	for i := range refs {
		ref, payload, err := fa.Alloc(64, ClassNK)
		if err != nil {
			t.Fatalf("Alloc[%d] failed: %v", i, err)
		}
		refs[i] = ref
		// Payload should be 60 bytes (64 - 4 header)
		if len(payload) != 60 {
			t.Fatalf("Alloc[%d]: expected payload len 60, got %d", i, len(payload))
		}
	}

	// Verify sequential offsets: each allocation should be exactly 64 bytes apart
	for i := 1; i < len(refs); i++ {
		diff := int32(refs[i]) - int32(refs[i-1])
		if diff != 64 {
			t.Fatalf("refs[%d]-refs[%d] = %d, expected 64 (sequential bump)", i, i-1, diff)
		}
	}

	// Verify each cell is marked as allocated (negative size) in the hive data
	data := h.Bytes()
	for i, ref := range refs {
		absOff := int(ref) + format.HeaderSize
		raw := format.ReadI32(data, absOff)
		if raw >= 0 {
			t.Fatalf("refs[%d] at offset 0x%x: expected negative size (allocated), got %d", i, absOff, raw)
		}
		if raw != -64 {
			t.Fatalf("refs[%d]: expected size -64, got %d", i, raw)
		}
	}

	// Finalize bump mode
	if err := fa.FinalizeBumpMode(); err != nil {
		t.Fatalf("FinalizeBumpMode failed: %v", err)
	}

	if fa.bump.active {
		t.Fatal("bump mode should be inactive after Finalize")
	}
}

// TestBumpMode_FallbackOnExhaustion enables bump mode with small space, then
// verifies that once the bump region is exhausted, allocations fall back to
// the normal allocator path and still succeed.
func TestBumpMode_FallbackOnExhaustion(t *testing.T) {
	h := newTestHive(t, 1) // 1 HBIN = 4064 bytes free
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	// Enable bump mode with only 128 bytes of bump space
	if err := fa.EnableBumpMode(128); err != nil {
		t.Fatalf("EnableBumpMode failed: %v", err)
	}

	// Allocate cells until bump is exhausted — each is 64 bytes
	// 128 bytes of usable bump space should fit 1 cell of 64 bytes
	// (the HBIN header uses 32 bytes, and the bump region capacity
	// depends on how many pages the 128 bytes round up to minus the header)
	allocCount := 0
	for i := 0; i < 200; i++ {
		_, _, err := fa.Alloc(64, ClassVK)
		if err != nil {
			t.Fatalf("Alloc[%d] failed even after fallback: %v", i, err)
		}
		allocCount++
	}

	// We should have been able to allocate many cells (some bump, rest via normal path)
	if allocCount < 200 {
		t.Fatalf("expected 200 allocations to succeed, got %d", allocCount)
	}

	if err := fa.FinalizeBumpMode(); err != nil {
		t.Fatalf("FinalizeBumpMode failed: %v", err)
	}
}

// TestBumpMode_FinalizeFreeCell verifies that FinalizeBumpMode writes a
// trailing free cell for any unused bump space.
func TestBumpMode_FinalizeFreeCell(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	// Enable bump mode with enough space for several cells
	if err := fa.EnableBumpMode(4096); err != nil {
		t.Fatalf("EnableBumpMode failed: %v", err)
	}

	// Record the bump region info before allocating
	bumpStart := fa.bump.startOff
	bumpCap := fa.bump.capacity

	// Allocate only one 64-byte cell, leaving a large unused tail
	_, _, err := fa.Alloc(64, ClassNK)
	if err != nil {
		t.Fatalf("Alloc failed: %v", err)
	}

	cursorAfterAlloc := fa.bump.cursor

	// Finalize — should write a trailing free cell
	if err := fa.FinalizeBumpMode(); err != nil {
		t.Fatalf("FinalizeBumpMode failed: %v", err)
	}

	// The trailing free cell should start right after the allocated cell
	trailingOff := bumpStart + cursorAfterAlloc
	trailingSize := bumpCap - cursorAfterAlloc

	if trailingSize < minCellSize {
		t.Skip("trailing space too small for a free cell")
	}

	// Read the trailing cell header — should be positive (free)
	data := h.Bytes()
	raw := format.ReadI32(data, int(trailingOff))
	if raw <= 0 {
		t.Fatalf("trailing cell at offset 0x%x: expected positive (free), got %d", trailingOff, raw)
	}
	if raw != trailingSize {
		t.Fatalf("trailing free cell: expected size %d, got %d", trailingSize, raw)
	}
}

// TestBumpMode_Alignment verifies that bump allocations are 8-byte aligned.
func TestBumpMode_Alignment(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	if err := fa.EnableBumpMode(4096); err != nil {
		t.Fatalf("EnableBumpMode failed: %v", err)
	}

	// Request sizes that are not 8-byte aligned — the allocator should align them
	sizes := []int32{12, 20, 36, 44, 52}
	for _, sz := range sizes {
		ref, _, err := fa.Alloc(sz, ClassNK)
		if err != nil {
			t.Fatalf("Alloc(%d) failed: %v", sz, err)
		}
		absOff := int32(ref) + int32(format.HeaderSize)
		if absOff%format.CellAlignment != 0 {
			t.Fatalf("Alloc(%d) returned offset 0x%x not 8-aligned", sz, absOff)
		}
	}

	if err := fa.FinalizeBumpMode(); err != nil {
		t.Fatalf("FinalizeBumpMode failed: %v", err)
	}
}

// TestBumpMode_DoubleEnable verifies that calling EnableBumpMode twice returns an error.
func TestBumpMode_DoubleEnable(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	if err := fa.EnableBumpMode(4096); err != nil {
		t.Fatalf("first EnableBumpMode failed: %v", err)
	}

	// Second call should error
	if err := fa.EnableBumpMode(4096); err == nil {
		t.Fatal("expected error on second EnableBumpMode, got nil")
	}

	if err := fa.FinalizeBumpMode(); err != nil {
		t.Fatalf("FinalizeBumpMode failed: %v", err)
	}
}

// TestBumpMode_FinalizeWithoutEnable verifies that calling FinalizeBumpMode
// without EnableBumpMode returns an error.
func TestBumpMode_FinalizeWithoutEnable(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	if err := fa.FinalizeBumpMode(); err == nil {
		t.Fatal("expected error on FinalizeBumpMode without Enable, got nil")
	}
}

// TestBumpMode_NormalAllocUnaffected verifies that when bump mode is NOT active,
// the normal allocation path works exactly as before.
func TestBumpMode_NormalAllocUnaffected(t *testing.T) {
	h := newTestHive(t, 1)
	fa := newFastAllocatorForTest(t, h, newMockDirtyTracker())

	// Allocate without bump mode
	ref1, payload1, err := fa.Alloc(64, ClassNK)
	if err != nil {
		t.Fatalf("normal Alloc failed: %v", err)
	}

	if ref1 == 0 {
		t.Fatal("expected non-zero ref")
	}
	if len(payload1) != 60 {
		t.Fatalf("expected payload len 60, got %d", len(payload1))
	}

	// Verify cell is marked allocated
	data := h.Bytes()
	absOff := int(ref1) + format.HeaderSize
	raw := format.ReadI32(data, absOff)
	if raw >= 0 {
		t.Fatalf("expected negative size (allocated), got %d", raw)
	}
}

// BenchmarkMicro_Alloc compares normal allocation vs bump allocation performance.
func BenchmarkMicro_Alloc(b *testing.B) {
	b.Run("Normal_500", func(b *testing.B) {
		for range b.N {
			h := newTestHive(b, 1)
			fa := newFastAllocatorForTest(b, h, nil)

			for i := 0; i < 500; i++ {
				_, _, err := fa.Alloc(64, ClassNK)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})

	b.Run("BumpMode_500", func(b *testing.B) {
		for range b.N {
			h := newTestHive(b, 1)
			fa := newFastAllocatorForTest(b, h, nil)

			// Pre-grow with bump mode: 500 cells * 64 bytes = 32000 bytes
			if err := fa.EnableBumpMode(32000); err != nil {
				b.Fatal(err)
			}

			for i := 0; i < 500; i++ {
				_, _, err := fa.Alloc(64, ClassNK)
				if err != nil {
					b.Fatal(err)
				}
			}

			if err := fa.FinalizeBumpMode(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
