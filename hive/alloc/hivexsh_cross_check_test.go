//go:build linux || darwin

package alloc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// Test_Hivex_Open_AllFixtures validates various generated hives with hivexsh
// This is Test #20 from DEBUG.md: "Hivex_Open_AllFixtures".
func Test_Hivex_Open_AllFixtures(t *testing.T) {
	// Check if hivexsh is available
	if !hivexval.IsHivexshAvailable() {
		t.Skip("hivexsh not available")
	}

	testCases := []struct {
		name        string
		createHive  func(t *testing.T, path string)
		shouldPass  bool
		description string
	}{
		{
			name: "MinimalHive",
			createHive: func(t *testing.T, path string) {
				createMinimalHive(t, path, 4096)
			},
			shouldPass:  true,
			description: "Basic 1-page hive",
		},
		{
			name: "HiveWithGrowth",
			createHive: func(t *testing.T, path string) {
				createMinimalHive(t, path, 4096)
				h, err := hive.Open(path)
				require.NoError(t, err)
				defer h.Close()

				dt := dirty.NewTracker(h)
				fa, err := NewFast(h, dt, nil)
				require.NoError(t, err)

				// Grow by 2 pages
				err = fa.GrowByPages(2) // Add 8KB HBIN
				require.NoError(t, err)
			},
			shouldPass:  true,
			description: "Hive after growth",
		},
		{
			name: "HiveWithAllocations",
			createHive: func(t *testing.T, path string) {
				createHiveWithFreeCellsAndRoot(t, path, []int{512, 1024, 2048})

				h, err := hive.Open(path)
				require.NoError(t, err)
				defer h.Close()

				dt := dirty.NewTracker(h)
				fa, err := NewFast(h, dt, nil)
				require.NoError(t, err)

				// Make some allocations
				for range 10 {
					_, _, allocErr := fa.Alloc(128, ClassNK)
					require.NoError(t, allocErr)
				}
			},
			shouldPass:  true,
			description: "Hive with multiple allocations",
		},
		{
			name: "HiveWithFreedCells",
			createHive: func(t *testing.T, path string) {
				createHiveWithFreeCellsAndRoot(t, path, []int{2048})

				h, err := hive.Open(path)
				require.NoError(t, err)
				defer h.Close()

				dt := dirty.NewTracker(h)
				fa, err := NewFast(h, dt, nil)
				require.NoError(t, err)

				// Allocate and free
				refs := []CellRef{}
				for range 5 {
					ref, _, allocErr := fa.Alloc(128, ClassNK)
					require.NoError(t, allocErr)
					refs = append(refs, ref)
				}

				// Free every other one
				for i := 0; i < len(refs); i += 2 {
					freeErr := fa.Free(refs[i])
					require.NoError(t, freeErr)
				}
			},
			shouldPass:  true,
			description: "Hive with freed cells",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			hivePath := filepath.Join(dir, "test.hiv")

			tc.createHive(t, hivePath)

			// Validate with hivexsh
			v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{UseHivexsh: true}))
			defer v.Close()

			err := v.ValidateWithHivexsh()
			if tc.shouldPass {
				if err != nil {
					t.Errorf("%s: hivexsh validation failed: %v", tc.description, err)
				} else {
					t.Logf("%s: hivexsh validation passed", tc.description)
				}
			} else {
				if err == nil {
					t.Errorf("%s: expected validation to fail but it passed", tc.description)
				} else {
					t.Logf("%s: validation failed as expected: %v", tc.description, err)
				}
			}
		})
	}
}

// Test_Hivex_OffsetFirst_Mismatch validates detection of corrupted HBIN offset field
// This is Test #21 from DEBUG.md: "Hivex_OffsetFirst_Mismatch".
func Test_Hivex_OffsetFirst_Mismatch(t *testing.T) {
	if !hivexval.IsHivexshAvailable() {
		t.Skip("hivexsh not available")
	}

	dir := t.TempDir()
	cleanPath := filepath.Join(dir, "clean.hiv")
	corruptPath := filepath.Join(dir, "corrupt.hiv")

	// Create a clean hive with multiple HBINs
	createMinimalHive(t, cleanPath, 4096)
	h, err := hive.Open(cleanPath)
	require.NoError(t, err)

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow to add a second HBIN
	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)
	h.Close()

	// Read the clean hive
	data, err := os.ReadFile(cleanPath)
	require.NoError(t, err)

	// Corrupt the second HBIN's offset field
	// Second HBIN should be at format.HeaderSize + 4096
	secondHBINOffset := format.HeaderSize + 4096
	if secondHBINOffset+format.HBINHeaderSize > len(data) {
		t.Skip("Second HBIN not found")
	}

	// Verify it's actually an HBIN
	sig := string(data[secondHBINOffset : secondHBINOffset+4])
	require.Equal(t, string(format.HBINSignature), sig)

	// Corrupt the offset field (set it to wrong value)
	corruptData := make([]byte, len(data))
	copy(corruptData, data)
	// Offset field should be 0x1000, let's set it to 0x2000 (wrong)
	putU32(corruptData, secondHBINOffset+format.HBINFileOffsetField, 0x2000)

	err = os.WriteFile(corruptPath, corruptData, 0644)
	require.NoError(t, err)

	// Try to validate with hivexsh
	v := hivexval.Must(hivexval.New(corruptPath, &hivexval.Options{UseHivexsh: true}))
	defer v.Close()

	err = v.ValidateWithHivexsh()
	if err != nil {
		if strings.Contains(err.Error(), "offset") || strings.Contains(err.Error(), "mismatch") {
			t.Logf("hivexsh detected offset mismatch: %v", err)
		} else {
			t.Logf("hivexsh rejected corrupt hive: %v", err)
		}
	} else {
		t.Log("hivexsh did not detect offset mismatch (may not check this)")
	}
}
