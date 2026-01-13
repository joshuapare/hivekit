package alloc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_FastAlloc_Adversarial_ZeroSizeCell tests handling of corrupted zero-size cells.
func Test_FastAlloc_Adversarial_ZeroSizeCell(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{256})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate a cell
	ref1, _, _ := fa.Alloc(128, ClassNK)

	// CORRUPT: Write zero size to a cell header (simulates corruption)
	data := h.Bytes()
	putI32(data, int32(int(ref1)+128), 0) // Write 0 size at next position

	// Try to allocate - should not hang
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan bool)
	go func() {
		_, _, _ = fa.Alloc(64, ClassNK)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't hang
		t.Log("Handled zero-size cell without hanging")
	case <-ctx.Done():
		t.Fatal("HUNG on zero-size cell - infinite loop detected!")
	}
}

// Test_FastAlloc_Adversarial_NegativeSizeCell tests handling of negative size cells.
func Test_FastAlloc_Adversarial_NegativeSizeCell(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{512})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	ref1, _, _ := fa.Alloc(128, ClassNK)

	// CORRUPT: Write negative size to next cell (simulates corruption)
	data := h.Bytes()
	putI32(data, int32(int(ref1)+128), -256)

	// Free the cell - backward coalesce should handle negative size gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- fa.Free(ref1)
	}()

	select {
	case freeErr := <-done:
		// Success - didn't hang
		if freeErr != nil {
			t.Logf("Rejected negative-size cell: %v", freeErr)
		} else {
			t.Log("Handled negative-size cell without hanging")
		}
	case <-ctx.Done():
		t.Fatal("HUNG on negative-size cell - infinite loop detected!")
	}
}

// Test_FastAlloc_Adversarial_OversizedCell tests handling of cells with size > HBIN.
func Test_FastAlloc_Adversarial_OversizedCell(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{256})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	ref1, _, _ := fa.Alloc(128, ClassNK)

	// CORRUPT: Write huge size that spans multiple HBINs
	data := h.Bytes()
	putI32(data, int32(int(ref1)+128), 0x100000) // 1MB size in a 4KB HBIN

	// Try to free - should not scan past HBIN boundary
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- fa.Free(ref1)
	}()

	select {
	case freeErr := <-done:
		if freeErr != nil {
			t.Logf("Rejected oversized cell: %v", freeErr)
		} else {
			t.Log("Handled oversized cell without scanning out of bounds")
		}
	case <-ctx.Done():
		t.Fatal("HUNG on oversized cell - likely scanned past HBIN boundary!")
	}
}

// Test_FastAlloc_Adversarial_CorruptedHBINSize tests handling of corrupted HBIN size.
func Test_FastAlloc_Adversarial_CorruptedHBINSize(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{256})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	ref1, _, _ := fa.Alloc(128, ClassNK)

	// CORRUPT: Corrupt HBIN size field
	data := h.Bytes()
	hbinStart := format.HeaderSize
	putU32(data, hbinStart+format.HBINSizeOffset, 0) // Zero HBIN size

	// Try to free - should handle gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- fa.Free(ref1)
	}()

	select {
	case freeErr := <-done:
		if freeErr != nil {
			t.Logf("Rejected corrupted HBIN: %v", freeErr)
		} else {
			t.Log("Handled corrupted HBIN size gracefully")
		}
	case <-ctx.Done():
		t.Fatal("HUNG on corrupted HBIN size!")
	}
}

// Test_FastAlloc_Adversarial_FragmentedFreeList tests deep free list chains.
func Test_FastAlloc_Adversarial_FragmentedFreeList(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{0x100000}) // 1MB

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create extreme fragmentation: allocate 1000 cells then free every other one
	refs := make([]CellRef, 1000)
	for i := range 1000 {
		ref, _, allocErr := fa.Alloc(64, ClassNK)
		if allocErr != nil {
			t.Fatalf("Alloc %d failed: %v", i, allocErr)
		}
		refs[i] = ref
	}

	// Free every other cell to create fragmented free list
	for i := 0; i < 1000; i += 2 {
		if freeErr := fa.Free(refs[i]); freeErr != nil {
			t.Fatalf("Free %d failed: %v", i, freeErr)
		}
	}

	// Now allocate with timeout - should not hang even with fragmented free list
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		_, _, allocErr := fa.Alloc(64, ClassNK)
		done <- allocErr
	}()

	select {
	case allocErr := <-done:
		if allocErr != nil {
			t.Fatalf("Alloc failed: %v", allocErr)
		}
		t.Log("Handled fragmented free list without hanging")
	case <-ctx.Done():
		t.Fatal("HUNG on fragmented free list - possible infinite loop in free list traversal!")
	}
}

// Test_FastAlloc_Adversarial_RapidAllocFree tests rapid alloc/free cycles.
func Test_FastAlloc_Adversarial_RapidAllocFree(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{0x100000}) // 1MB

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Rapid alloc/free for 10,000 iterations with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		for i := range 10000 {
			ref, _, allocErr := fa.Alloc(int32(64+i%128), ClassNK)
			if allocErr != nil {
				done <- allocErr
				return
			}
			if i%2 == 0 {
				if freeErr := fa.Free(ref); freeErr != nil {
					done <- freeErr
					return
				}
			}
		}
		done <- nil
	}()

	select {
	case cycleErr := <-done:
		if cycleErr != nil {
			t.Fatalf("Rapid alloc/free failed: %v", cycleErr)
		}
		t.Log("Completed 10,000 rapid alloc/free cycles without hanging")
	case <-ctx.Done():
		t.Fatal("HUNG during rapid alloc/free - possible infinite loop!")
	}
}

// Test_FastAlloc_Adversarial_CellAtHBINBoundary tests cell exactly at HBIN end.
func Test_FastAlloc_Adversarial_CellAtHBINBoundary(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{256})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate a cell
	ref1, _, _ := fa.Alloc(128, ClassNK)

	// CORRUPT: Create a cell header at the exact HBIN boundary
	data := h.Bytes()
	hbinStart := format.HeaderSize
	hbinSize := int(getI32(data, hbinStart+format.HBINSizeOffset))
	if hbinSize < 0 {
		hbinSize = -hbinSize
	}
	hbinEnd := hbinStart + hbinSize

	// Write cell size at boundary (should be caught by bounds check)
	if hbinEnd-4 < len(data) {
		putI32(data, int32(hbinEnd-4), 128)
	}

	// Try to free - should handle boundary case
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error)
	go func() {
		done <- fa.Free(ref1)
	}()

	select {
	case freeErr := <-done:
		if freeErr != nil {
			t.Logf("Handled boundary cell: %v", freeErr)
		} else {
			t.Log("Handled cell at HBIN boundary without reading past end")
		}
	case <-ctx.Done():
		t.Fatal("HUNG on boundary cell!")
	}
}

// Test_FastAlloc_Adversarial_AllOperationsTimeout ensures no operation takes > 1 second.
func Test_FastAlloc_Adversarial_AllOperationsTimeout(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{0x10000})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	maxDuration := 100 * time.Millisecond

	// Test Alloc timeout
	start := time.Now()
	_, _, _ = fa.Alloc(128, ClassNK)
	if time.Since(start) > maxDuration {
		t.Errorf("Alloc took %v (max: %v)", time.Since(start), maxDuration)
	}

	// Test Free timeout
	ref, _, _ := fa.Alloc(128, ClassNK)
	start = time.Now()
	_ = fa.Free(ref)
	if time.Since(start) > maxDuration {
		t.Errorf("Free took %v (max: %v)", time.Since(start), maxDuration)
	}

	// Test Grow timeout
	start = time.Now()
	_ = fa.GrowByPages(1) // Add 4KB HBIN
	if time.Since(start) > maxDuration {
		t.Errorf("Grow took %v (max: %v)", time.Since(start), maxDuration)
	}

	t.Log("All operations completed within timeout")
}
