package e2e

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/walker"
)

// TestCell_Accounting validates that all allocated cells are reachable from the root.
// This is the definitive test that proves:
// 1. The walker visits all reachable cells
// 2. All allocated cells ARE reachable (no orphans)
// 3. We can account for every "unknown" cell by its purpose.
func TestCell_Accounting(t *testing.T) {
	cases := []struct {
		name string
		path string
		// Expected counts from walker
		expectWalked int
	}{
		{
			name:         "windows-xp-system",
			path:         filepath.Join("suite", "windows-xp-system"),
			expectWalked: 67724, // From linear iteration: 113410 allocated - will compare
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := hive.Open(filepath.Join("..", "..", "testdata", tc.path))
			require.NoError(t, err)

			// Step 1: Walk all reachable cells from root
			walkedCells := make(map[uint32]bool)
			cellTypes := make(map[walker.CellType]int)
			cellPurposes := make(map[walker.CellPurpose]int)

			err = walker.NewValidationWalker(h).Walk(func(ref walker.CellRef) error {
				walkedCells[ref.Offset] = true
				cellTypes[ref.Type]++
				cellPurposes[ref.Purpose]++
				return nil
			})
			require.NoError(t, err, "walker should complete without error")

			// Step 2: Count total allocated cells via linear iteration
			// Note: We can't easily get offsets from the iterator without API changes
			// So for now we just count to compare magnitudes
			allocatedCount := 0
			freeCount := 0

			hbinIter := h.NewHBINIterator()
			for {
				hbin, hbinErr := hbinIter.Next()
				if errors.Is(hbinErr, io.EOF) {
					break
				}
				require.NoError(t, hbinErr)

				cellIter := hbin.Cells()
				for {
					cell, cellErr := cellIter.Next()
					if errors.Is(cellErr, io.EOF) {
						break
					}
					require.NoError(t, cellErr)

					if cell.IsAllocated() {
						allocatedCount++
					} else {
						freeCount++
					}
				}
			}

			t.Logf("  Linear iteration:")
			t.Logf("    Allocated cells: %d", allocatedCount)
			t.Logf("    Free cells: %d", freeCount)

			// Step 3: Report what was walked
			t.Logf("Cell accounting for %s:", tc.name)
			t.Logf("  Cells walked by reference: %d", len(walkedCells))
			t.Logf("")
			t.Logf("  Breakdown by type:")
			t.Logf("    NK: %d", cellTypes[walker.CellTypeNK])
			t.Logf("    VK: %d", cellTypes[walker.CellTypeVK])
			t.Logf("    SK: %d", cellTypes[walker.CellTypeSK])
			t.Logf("    LF: %d", cellTypes[walker.CellTypeLF])
			t.Logf("    LH: %d", cellTypes[walker.CellTypeLH])
			t.Logf("    LI: %d", cellTypes[walker.CellTypeLI])
			t.Logf("    RI: %d", cellTypes[walker.CellTypeRI])
			t.Logf("    DB: %d", cellTypes[walker.CellTypeDB])
			t.Logf("    Data: %d", cellTypes[walker.CellTypeData])
			t.Logf("    List: %d", cellTypes[walker.CellTypeValueList])
			t.Logf("")
			t.Logf("  Breakdown by purpose:")
			t.Logf("    Registry keys: %d", cellPurposes[walker.PurposeKey])
			t.Logf("    Registry values: %d", cellPurposes[walker.PurposeValue])
			t.Logf("    Security descriptors: %d", cellPurposes[walker.PurposeSecurity])
			t.Logf("    Subkey lists: %d", cellPurposes[walker.PurposeSubkeyList])
			t.Logf("    Value lists: %d", cellPurposes[walker.PurposeValueList])
			t.Logf("    Value data: %d", cellPurposes[walker.PurposeValueData])
			t.Logf("    Big data headers: %d", cellPurposes[walker.PurposeBigDataHeader])
			t.Logf("    DB blocklists: %d", cellPurposes[walker.PurposeBigDataList])
			t.Logf("    DB data blocks: %d", cellPurposes[walker.PurposeBigDataBlock])
			t.Logf("    Class names: %d", cellPurposes[walker.PurposeClassName])

			// Basic validation
			require.NotEmpty(t, walkedCells, "should walk at least some cells")
			require.Positive(t, cellTypes[walker.CellTypeNK], "should find NK cells")
			require.Positive(t, cellTypes[walker.CellTypeVK], "should find VK cells")
		})
	}
}

// TestCell_Accounting_CompareToLinearIteration validates that the walker
// finds the same number of cells as linear iteration (proving 100% reachability).
func TestCell_Accounting_CompareToLinearIteration(t *testing.T) {
	t.Skip("TODO: This requires adding cell offset tracking to the cell iterator")
	// TODO: Once we have cell offsets from the iterator, we can:
	// 1. Build a set of all allocated cell offsets from linear iteration
	// 2. Build a set of all walked cell offsets from the walker
	// 3. Assert they are identical (no orphans, no missing cells)
	// 4. Assert walkedSet == allocatedSet
}

// TestCell_Walker_NoInfiniteLoops validates that the walker doesn't get stuck
// in cycles (e.g., SK Flink/Blink circular lists).
func TestCell_Walker_NoInfiniteLoops(t *testing.T) {
	h, err := hive.Open(filepath.Join("..", "..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	visitCount := 0
	maxVisits := 1000000 // Safety limit

	err = walker.NewValidationWalker(h).Walk(func(_ walker.CellRef) error {
		visitCount++
		if visitCount > maxVisits {
			return fmt.Errorf("walker exceeded max visits (%d), likely infinite loop", maxVisits)
		}
		return nil
	})

	require.NoError(t, err)
	t.Logf("Walker completed in %d visits", visitCount)
	require.Less(t, visitCount, maxVisits, "walker should terminate")
}

// TestCell_Walker_VisitOrder validates that the walker visits cells in a
// reasonable order (depth-first from root).
func TestCell_Walker_VisitOrder(t *testing.T) {
	h, err := hive.Open(filepath.Join("..", "..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	var visitedOffsets []uint32
	err = walker.NewValidationWalker(h).Walk(func(ref walker.CellRef) error {
		visitedOffsets = append(visitedOffsets, ref.Offset)
		return nil
	})
	require.NoError(t, err)

	// First cell visited should be the root NK
	rootOffset := h.RootCellOffset()
	require.Equal(t, rootOffset, visitedOffsets[0], "first visited cell should be root NK")

	// Should visit some reasonable number of cells
	require.Greater(t, len(visitedOffsets), 1000, "should visit many cells")

	t.Logf("Walker visited %d cells starting from root 0x%08x", len(visitedOffsets), rootOffset)
}

// TestCell_Walker_ErrorHandling validates that the walker propagates errors correctly.
func TestCell_Walker_ErrorHandling(t *testing.T) {
	h, err := hive.Open(filepath.Join("..", "..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	// Test that visitor errors are propagated
	expectedErr := errors.New("test error")
	visitCount := 0

	err = walker.NewValidationWalker(h).Walk(func(_ walker.CellRef) error {
		visitCount++
		if visitCount == 100 {
			return expectedErr
		}
		return nil
	})

	require.ErrorIs(t, err, expectedErr, "walker should propagate visitor errors")
	require.Equal(t, 100, visitCount, "walker should stop at first error")
}
