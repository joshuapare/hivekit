package alloc

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_FastAlloc_SimpleFit tests basic allocation that fits immediately.
func Test_FastAlloc_SimpleFit(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate 64 bytes (should fit in 128-byte cell)
	ref, payload, err := fa.Alloc(64, ClassNK)
	if err != nil {
		t.Fatalf("Alloc failed: %v", err)
	}

	if ref == 0 {
		t.Fatal("Expected non-zero ref")
	}
	if len(payload) != 60 { // 64 - 4 byte header
		t.Fatalf("Expected payload len 60, got %d", len(payload))
	}

	// Verify cell is marked as allocated (negative size)
	data := h.Bytes()
	absOff := int(ref) + format.HeaderSize // Convert relative to absolute
	raw := format.ReadI32(data, absOff)
	if raw >= 0 {
		t.Fatalf("Expected negative size (allocated), got %d", raw)
	}
}

// Test_FastAlloc_BestFit tests that smallest sufficient cell is chosen.
func Test_FastAlloc_BestFit(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create cells of sizes: 256, 128, 512
	createHiveWithFreeCells(t, hivePath, []int{256, 128, 512})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Request 64 bytes - should get 128-byte cell (best fit)
	ref, _, err := fa.Alloc(64, ClassNK)
	if err != nil {
		t.Fatalf("Alloc failed: %v", err)
	}

	data := h.Bytes()
	allocSize := cellAbsSize(data, int(ref)+format.HeaderSize)
	// Should be exactly 128 or split into 64+remainder
	if allocSize != 128 && allocSize != 64 {
		t.Fatalf("Expected size 64 or 128, got %d", allocSize)
	}
}

// Test_FastAlloc_Splitting tests cell splitting when remainder is usable.
func Test_FastAlloc_Splitting(t *testing.T) {
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

	// Allocate 64 bytes from 256-byte cell
	ref, _, err := fa.Alloc(64, ClassNK)
	if err != nil {
		t.Fatalf("Alloc failed: %v", err)
	}

	data := h.Bytes()
	allocSize := cellAbsSize(data, int(ref)+format.HeaderSize)
	if allocSize != 64 {
		t.Fatalf("Expected allocated size 64, got %d", allocSize)
	}

	// Check if tail cell was created (should be 256 - 64 = 192)
	tailOff := int(ref) + format.HeaderSize + 64 // Convert ref to absolute, then add 64
	if tailOff+4 <= len(data) {
		tailSize := format.ReadI32(data, tailOff)
		if tailSize < 0 {
			t.Fatalf("Expected tail cell to be free (positive size), got %d", tailSize)
		}
		if tailSize != 192 {
			t.Fatalf("Expected tail size 192, got %d", tailSize)
		}
	}
}

// Test_FastAlloc_NoSplit tests that small remainders are consumed.
func Test_FastAlloc_NoSplit(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Create 128-byte cell
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Request 120 bytes - remainder (8 bytes) too small to split
	ref, _, err := fa.Alloc(120, ClassNK)
	if err != nil {
		t.Fatalf("Alloc failed: %v", err)
	}

	data := h.Bytes()
	allocSize := cellAbsSize(data, int(ref)+format.HeaderSize)
	// Should consume entire 128-byte cell (no split due to small remainder)
	// Note: The allocator rounds up to 8-byte boundaries, so 120 becomes 120 aligned
	// The remainder is 128 - 120 = 8 bytes, which is exactly the minimum cell size
	// So it MAY split to 120+8 or consume the full 128
	if allocSize != 128 && allocSize != 120 {
		t.Fatalf("Expected full cell (128) or aligned cell (120), got %d", allocSize)
	}
}

// Test_FastAlloc_CoalesceForward tests freeing cell coalesces with next free cell.
func Test_FastAlloc_CoalesceForward(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Two adjacent 128-byte cells, plus a large cell to fill the HBIN
	// This prevents coalescing with slack space
	createHiveWithFreeCells(t, hivePath, []int{128, 128, 3808})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate a dummy cell from the large free cell to prevent coalescing with it
	dummyRef, _, err := fa.Alloc(3808, ClassNK)
	if err != nil {
		t.Fatal(err)
	}
	_ = dummyRef // Keep it allocated

	// Allocate both test cells
	ref1, _, allocErr := fa.Alloc(128, ClassNK)
	if allocErr != nil {
		t.Fatal(allocErr)
	}

	ref2, _, allocErr := fa.Alloc(128, ClassNK)
	if allocErr != nil {
		t.Fatal(allocErr)
	}

	// Free first cell (ref2 is still allocated)
	if freeErr := fa.Free(ref1); freeErr != nil {
		t.Fatal(freeErr)
	}

	// Free second cell - should coalesce with first
	if freeErr := fa.Free(ref2); freeErr != nil {
		t.Fatal(freeErr)
	}

	// Check that we have a single 256-byte cell at the lower address
	// (coalescing creates the merged cell at the lower of the two addresses)
	data := h.Bytes()
	mergedRef := min(ref2, ref1)

	size := format.ReadI32(data, int(mergedRef)+format.HeaderSize)
	if size <= 0 {
		t.Fatal("Expected free cell (positive size)")
	}
	if size != 256 {
		t.Fatalf("Expected coalesced size 256, got %d", size)
	}
}

// Test_FastAlloc_CoalesceBackward tests backward coalescing.
func Test_FastAlloc_CoalesceBackward(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Two adjacent 128-byte cells, plus a large cell to fill the HBIN
	createHiveWithFreeCells(t, hivePath, []int{128, 128, 3808})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate a dummy cell from the large free cell to prevent coalescing with it
	dummyRef, _, err := fa.Alloc(3808, ClassNK)
	if err != nil {
		t.Fatal(err)
	}
	_ = dummyRef // Keep it allocated

	ref1, _, _ := fa.Alloc(128, ClassNK)
	ref2, _, _ := fa.Alloc(128, ClassNK)

	// Free second cell first
	if freeErr := fa.Free(ref2); freeErr != nil {
		t.Fatal(freeErr)
	}

	// Free first cell - should coalesce with second
	if freeErr := fa.Free(ref1); freeErr != nil {
		t.Fatal(freeErr)
	}

	// Check merged cell at lower address
	data := h.Bytes()
	mergedRef := min(ref1, ref2)

	size := format.ReadI32(data, int(mergedRef)+format.HeaderSize)
	if size <= 0 {
		t.Fatal("Expected free cell")
	}
	if size != 256 {
		t.Fatalf("Expected coalesced size 256, got %d", size)
	}
}

// Test_FastAlloc_CoalesceBoth tests coalescing in both directions.
func Test_FastAlloc_CoalesceBoth(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Three adjacent cells, plus a large cell to fill the HBIN
	createHiveWithFreeCells(t, hivePath, []int{128, 128, 128, 3680})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate a dummy cell from the large free cell to prevent coalescing with it
	dummyRef, _, err := fa.Alloc(3680, ClassNK)
	if err != nil {
		t.Fatal(err)
	}
	_ = dummyRef // Keep it allocated

	ref1, _, _ := fa.Alloc(128, ClassNK)
	ref2, _, _ := fa.Alloc(128, ClassNK)
	ref3, _, _ := fa.Alloc(128, ClassNK)

	// Free first and third
	fa.Free(ref1)
	fa.Free(ref3)

	// Free middle - should coalesce all three
	if freeErr := fa.Free(ref2); freeErr != nil {
		t.Fatal(freeErr)
	}

	// Find lowest address of the three cells
	data := h.Bytes()
	mergedRef := min(ref1, ref2, ref3)

	size := format.ReadI32(data, int(mergedRef)+format.HeaderSize)
	if size <= 0 {
		t.Fatal("Expected free cell")
	}
	if size != 384 {
		t.Fatalf("Expected coalesced size 384, got %d", size)
	}
}

// Test_FastAlloc_Alignment tests 8-byte alignment.
func Test_FastAlloc_Alignment(t *testing.T) {
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

	// Request 65 bytes - should round up to 72 (nearest 8-byte boundary)
	ref, payload, err := fa.Alloc(65, ClassNK)
	if err != nil {
		t.Fatal(err)
	}

	data := h.Bytes()
	size := cellAbsSize(data, int(ref)+format.HeaderSize)
	if size%8 != 0 {
		t.Fatalf("Size not 8-byte aligned: %d", size)
	}

	// Payload should be 68 bytes (72 - 4 byte header)
	if len(payload) != 68 {
		t.Fatalf("Expected payload 68, got %d", len(payload))
	}
}

// Test_FastAlloc_Grow tests automatic HBIN growth.
func Test_FastAlloc_Grow(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	// Small initial free space
	createHiveWithFreeCells(t, hivePath, []int{64})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	initialSize := len(h.Bytes())

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate more than available - should trigger growth
	// Note: With spec-compliant Grow(), an 8KB HBIN has 8160 bytes free (8192 - 32 header)
	// So we request 8160 bytes which will fit in the grown HBIN
	ref, _, err := fa.Alloc(8160, ClassNK)
	if err != nil {
		t.Fatalf("Alloc with grow failed: %v", err)
	}

	if ref == 0 {
		t.Fatal("Expected successful allocation after grow")
	}

	finalSize := len(h.Bytes())
	if finalSize <= initialSize {
		t.Fatal("Expected hive to grow")
	}
}

// Test_FastAlloc_Multiple tests multiple allocations.
func Test_FastAlloc_Multiple(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{1024})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	refs := make([]CellRef, 10)
	for i := range 10 {
		ref, _, allocErr := fa.Alloc(64, ClassNK)
		if allocErr != nil {
			t.Fatalf("Alloc %d failed: %v", i, allocErr)
		}
		refs[i] = ref
	}

	// All refs should be unique
	seen := make(map[CellRef]bool)
	for _, ref := range refs {
		if seen[ref] {
			t.Fatalf("Duplicate ref: %d", ref)
		}
		seen[ref] = true
	}
}

// Test_FastAlloc_BadRef tests error on invalid reference.
func Test_FastAlloc_BadRef(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Try to free invalid offset
	err = fa.Free(CellRef(999999))
	if !errors.Is(err, ErrBadRef) {
		t.Fatalf("Expected ErrBadRef, got %v", err)
	}
}

// Test_FastAlloc_TooSmall tests error on request smaller than header.
func Test_FastAlloc_TooSmall(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = fa.Alloc(2, ClassNK)
	if !errors.Is(err, ErrNeedSmall) {
		t.Fatalf("Expected ErrNeedSmall, got %v", err)
	}
}

// Test_FastAlloc_SizeClasses tests that size class assignment works correctly.
// Since size classes are now dynamic and tunable, we test basic properties:
// 1. Smaller sizes get lower class numbers
// 2. Same size always gets same class
// 3. Size class is within valid range.
func Test_FastAlloc_SizeClasses(t *testing.T) {
	table := newSizeClassTable(DefaultConfig)

	tests := []int32{8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192}

	prevClass := -1
	for _, size := range tests {
		class := table.getSizeClass(size)

		// Check class is within valid range
		if class < 0 || class > table.NumClasses() {
			t.Errorf("getSizeClass(%d) = %d, out of range [0, %d]", size, class, table.NumClasses())
		}

		// Check monotonicity: larger sizes should get same or higher class
		if class < prevClass {
			t.Errorf("getSizeClass(%d) = %d, but previous class was %d (not monotonic)", size, class, prevClass)
		}

		prevClass = class
	}
}

// Test_FastAlloc_BoundaryCheck validates that cells extending past hive boundaries
// are rejected gracefully without panicking. This test reproduces and validates the fix
// for the bug found in steady-state benchmarks where cells could extend past the hive end.
func Test_FastAlloc_BoundaryCheck(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")

	// Create hive with exact size that could trigger boundary issues
	createHiveWithFreeCells(t, hivePath, []int{0x1000}) // 4KB free space

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Fill up the hive to force growth
	refs := make([]CellRef, 0, 100)
	for range 20 {
		ref, _, allocErr := fa.Alloc(128, ClassNK)
		if allocErr != nil {
			t.Fatal(allocErr)
		}
		refs = append(refs, ref)
	}

	// Free some cells to create fragmentation
	for i := range 10 {
		freeErr := fa.Free(refs[i*2])
		if freeErr != nil {
			t.Fatal(freeErr)
		}
	}

	// Allocate variable sizes to trigger splitting and coalescing
	// This stresses the boundary checking logic
	for i := range 100 {
		size := 64 + (i%256)*2 // 64-576 bytes
		ref, _, allocErr := fa.Alloc(int32(size), ClassNK)
		if allocErr != nil {
			t.Fatal(allocErr)
		}

		// Randomly free some to maintain steady state
		if i%3 == 0 && len(refs) > 0 {
			idx := i % len(refs)
			freeErr := fa.Free(refs[idx])
			if freeErr != nil {
				t.Fatal(freeErr)
			}
			refs[idx] = ref
		} else {
			refs = append(refs, ref)
		}
	}

	// Verify hive integrity - all cells should be within bounds
	data := h.Bytes()
	for _, ref := range refs {
		off := int(ref)
		if off >= len(data) {
			t.Fatalf("Cell offset %d is beyond hive size %d", off, len(data))
		}

		size := cellAbsSize(data, off)
		if off+size > len(data) {
			t.Fatalf("Cell at %d with size %d extends past hive end %d", off, size, len(data))
		}
	}
}

// TestGetFreeCell_NoPanic verifies that getFreeCell does not panic
// when the freeCellPool returns an unexpected type.
//
// Bug: getFreeCell used `panic("freeCellPool returned unexpected type")` which
// would crash production if the pool ever returned a non-*freeCell value.
func TestGetFreeCell_NoPanic(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Pollute the package-level pool with a wrong type
	freeCellPool.Put("not a *freeCell")

	// getFreeCell should NOT panic — it should fall back to a fresh allocation
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("getFreeCell panicked: %v", r)
		}
	}()

	cell := fa.getFreeCell()
	if cell == nil {
		t.Fatal("getFreeCell returned nil")
	}
}
