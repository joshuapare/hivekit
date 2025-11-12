//go:build linux || darwin

package alloc

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_Fuzz_RandomAllocFree_GuardInvariants performs random alloc/free and validates invariants
// This is Test #22 from DEBUG.md: "Fuzz_RandomAllocFree_GuardInvariants".
func Test_Fuzz_RandomAllocFree_GuardInvariants(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 8192)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(42)) // Fixed seed for reproducibility
	allocations := make(map[CellRef]int)

	// Perform random operations
	for i := range 100 {
		op := rng.Intn(3) // 0=alloc, 1=free, 2=grow

		switch op {
		case 0: // Allocate
			size := 64 + rng.Intn(512)
			ref, _, allocErr := fa.Alloc(int32(size), ClassNK)
			if allocErr == nil {
				allocations[ref] = size
				t.Logf("Step %d: Allocated %d bytes at 0x%X", i, size, ref)
			} else {
				t.Logf("Step %d: Alloc failed (expected if no space): %v", i, allocErr)
			}

		case 1: // Free
			if len(allocations) > 0 {
				// Pick random allocation to free
				for ref := range allocations {
					freeErr := fa.Free(ref)
					require.NoError(t, freeErr, "Step %d: Free failed", i)
					delete(allocations, ref)
					t.Logf("Step %d: Freed cell at 0x%X", i, ref)
					break
				}
			}

		case 2: // Grow
			growSize := 4096 + rng.Intn(8192)
			growErr := fa.Grow(int32(growSize))
			require.NoError(t, growErr, "Step %d: Grow failed", i)
			t.Logf("Step %d: Grew by %d bytes", i, growSize)
		}

		// Validate invariants after each step
		validateErr := validateHiveInvariants(t, h)
		require.NoError(t, validateErr, "Step %d: Invariant check failed", i)
	}

	t.Logf("100 random operations completed, all invariants held")
	t.Logf("Final state: %d active allocations", len(allocations))
}

// validateHiveInvariants checks core invariants.
func validateHiveInvariants(t *testing.T, h *hive.Hive) error {
	t.Helper()
	data := h.Bytes()

	// 1. File size matches header
	headerDataSize := int(getU32(data, format.REGFDataSizeOffset))
	expectedFileSize := format.HeaderSize + headerDataSize
	if len(data) != expectedFileSize {
		t.Errorf("File size mismatch: expected 0x%X, got 0x%X", expectedFileSize, len(data))
		return &InvariantError{"file size mismatch"}
	}

	// 2. All HBINs are valid and contiguous
	pos := format.HeaderSize
	for pos < len(data) {
		if pos+format.HBINHeaderSize > len(data) {
			break
		}

		sig := string(data[pos : pos+4])
		if sig != string(format.HBINSignature) {
			break
		}

		// Check offset field
		offsetField := int(getU32(data, pos+format.HBINFileOffsetField))
		expectedOffset := pos - format.HeaderSize
		if offsetField != expectedOffset {
			t.Errorf("HBIN at 0x%X has wrong offset field: 0x%X (expected 0x%X)",
				pos, offsetField, expectedOffset)
			return &InvariantError{"HBIN offset mismatch"}
		}

		// Check size
		hbinSize := int(getU32(data, pos+format.HBINSizeOffset))
		if hbinSize <= 0 || hbinSize%format.HBINAlignment != 0 {
			t.Errorf("HBIN at 0x%X has invalid size: 0x%X", pos, hbinSize)
			return &InvariantError{"HBIN size invalid"}
		}

		// 3. Check cells within this HBIN don't cross boundary
		hbinEnd := pos + hbinSize
		cellPos := pos + format.HBINHeaderSize

		for cellPos < hbinEnd {
			if cellPos+format.CellHeaderSize > hbinEnd {
				break
			}

			rawSize := getI32(data, cellPos)
			absSize := rawSize
			if absSize < 0 {
				absSize = -absSize
			}

			if absSize <= 0 {
				break // Invalid or end
			}

			// Ensure cell doesn't cross HBIN boundary
			cellEnd := cellPos + int(absSize)
			if cellEnd > hbinEnd {
				t.Errorf("Cell at 0x%X (size %d) crosses HBIN boundary at 0x%X",
					cellPos, absSize, hbinEnd)
				return &InvariantError{"cell crosses HBIN boundary"}
			}

			cellPos += format.Align8(int(absSize))
		}

		pos += hbinSize
	}

	return nil
}

type InvariantError struct {
	msg string
}

func (e *InvariantError) Error() string {
	return "invariant violation: " + e.msg
}

// Test_Fuzz_StressAllocFree performs intensive alloc/free cycles.
func Test_Fuzz_StressAllocFree(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 16384)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(12345))

	// Stress test: rapid alloc/free cycles
	for round := range 10 {
		// Allocate many cells
		refs := []CellRef{}
		for range 50 {
			size := 64 + rng.Intn(256)
			ref, _, allocErr := fa.Alloc(int32(size), ClassNK)
			if allocErr != nil {
				// Try growing
				_ = fa.GrowByPages(2) // Add 8KB HBIN
				ref, _, allocErr = fa.Alloc(int32(size), ClassNK)
			}
			if allocErr == nil {
				refs = append(refs, ref)
			}
		}

		// Free all
		for _, ref := range refs {
			freeErr := fa.Free(ref)
			require.NoError(t, freeErr)
		}

		// Validate
		validateErr := validateHiveInvariants(t, h)
		require.NoError(t, validateErr, "Round %d: Invariant check failed", round)
	}

	t.Logf("Stress test: 10 rounds of 50 alloc/free cycles completed")
}
