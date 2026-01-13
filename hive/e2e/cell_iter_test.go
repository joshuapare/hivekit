package e2e

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// CellCounts tracks the number of different cell types in a hive.
// This allows us to validate that cell iteration correctly identifies
// and counts all cells across all HBINs.
type CellCounts struct {
	// Total counts
	Total     int // Total cells visited (allocated + free)
	Allocated int // Cells with negative size (in-use)
	Free      int // Cells with positive size (free)

	// Cell types by signature (allocated cells only)
	NK int // "nk" - Node Key (registry keys)
	VK int // "vk" - Value Key (registry values)
	SK int // "sk" - Security descriptor
	LF int // "lf" - Leaf with name hints
	LH int // "lh" - Leaf with hash hints
	LI int // "li" - Leaf without hints
	RI int // "ri" - Index root
	DB int // "db" - Big data header

	// Cells without recognized signatures (allocated but unknown type)
	UnknownAllocated int

	// Size tracking (for validation)
	MinCellSize int // Smallest cell seen (should be >= 8 with header)
	MaxCellSize int // Largest cell seen
	TotalBytes  int // Sum of all cell sizes (should match data size)
}

// cellIterCase defines a test case for cell iteration.
type cellIterCase struct {
	Name    string
	Path    string
	Expect  *CellCounts // If non-nil, expect exact counts
	Comment string
}

func TestCell_Iterator_Comprehensive(t *testing.T) {
	cases := []cellIterCase{
		{
			Name: "windows-xp-system",
			Path: filepath.Join("suite", "windows-xp-system"),
			Expect: &CellCounts{
				Total:            113418,
				Allocated:        113410,
				Free:             8,
				NK:               18234,
				VK:               45345,
				SK:               211,
				LF:               0,
				LH:               3930,
				LI:               0,
				RI:               2,
				DB:               2,
				UnknownAllocated: 45686,
			},
			Comment: "Windows XP SYSTEM hive - clean, well-formed",
		},
		{
			Name: "windows-xp-software",
			Path: filepath.Join("suite", "windows-xp-software"),
			Expect: &CellCounts{
				Total:            60400,
				Allocated:        59709,
				Free:             691,
				NK:               8730,
				VK:               23849,
				SK:               45,
				LF:               0,
				LH:               1597,
				LI:               0,
				RI:               2,
				DB:               1,
				UnknownAllocated: 25485,
			},
			Comment: "Windows XP SOFTWARE hive",
		},
		{
			Name: "windows-xp-2-system",
			Path: filepath.Join("suite", "windows-xp-2-system"),
			Expect: &CellCounts{
				Total:            61498,
				Allocated:        60547,
				Free:             951,
				NK:               8862,
				VK:               24214,
				SK:               46,
				LF:               0,
				LH:               1664,
				LI:               0,
				RI:               2,
				DB:               2,
				UnknownAllocated: 25757,
			},
			Comment: "Windows XP 2 SYSTEM hive",
		},
		{
			Name: "windows-xp-2-software",
			Path: filepath.Join("suite", "windows-xp-2-software"),
			Expect: &CellCounts{
				Total:            185480,
				Allocated:        184334,
				Free:             1146,
				NK:               37833,
				VK:               53187,
				SK:               48,
				LF:               0,
				LH:               11926,
				LI:               0,
				RI:               3,
				DB:               0,
				UnknownAllocated: 81337,
			},
			Comment: "Windows XP 2 SOFTWARE hive",
		},
		{
			Name: "windows-2003-server-system",
			Path: filepath.Join("suite", "windows-2003-server-system"),
			Expect: &CellCounts{
				Total:            36465,
				Allocated:        35892,
				Free:             573,
				NK:               4579,
				VK:               15926,
				SK:               56,
				LF:               0,
				LH:               1525,
				LI:               0,
				RI:               0,
				DB:               0,
				UnknownAllocated: 13806,
			},
			Comment: "Windows 2003 Server SYSTEM hive",
		},
		{
			Name: "windows-2003-server-software",
			Path: filepath.Join("suite", "windows-2003-server-software"),
			Expect: &CellCounts{
				Total:            322583,
				Allocated:        320645,
				Free:             1938,
				NK:               65996,
				VK:               90612,
				SK:               57,
				LF:               0,
				LH:               21025,
				LI:               0,
				RI:               6,
				DB:               0,
				UnknownAllocated: 142949,
			},
			Comment: "Windows 2003 Server SOFTWARE hive - large file",
		},
		{
			Name: "windows-8-consumer-preview-system",
			Path: filepath.Join("suite", "windows-8-consumer-preview-system"),
			Expect: &CellCounts{
				Total:            113225,
				Allocated:        113218,
				Free:             7,
				NK:               18146,
				VK:               45462,
				SK:               202,
				LF:               0,
				LH:               4003,
				LI:               0,
				RI:               2,
				DB:               3,
				UnknownAllocated: 45400,
			},
			Comment: "Windows 8 Consumer Preview SYSTEM hive",
		},
		{
			Name: "windows-8-consumer-preview-software",
			Path: filepath.Join("suite", "windows-8-consumer-preview-software"),
			Expect: &CellCounts{
				Total:            768437,
				Allocated:        768230,
				Free:             207,
				NK:               151914,
				VK:               238228,
				SK:               338,
				LF:               0,
				LH:               57246,
				LI:               0,
				RI:               6,
				DB:               2,
				UnknownAllocated: 320496,
			},
			Comment: "Windows 8 Consumer Preview SOFTWARE hive - large file",
		},
		{
			Name: "windows-8-enterprise-system",
			Path: filepath.Join("suite", "windows-8-enterprise-system"),
			Expect: &CellCounts{
				Total:            113418,
				Allocated:        113410,
				Free:             8,
				NK:               18234,
				VK:               45345,
				SK:               211,
				LF:               0,
				LH:               3930,
				LI:               0,
				RI:               2,
				DB:               2,
				UnknownAllocated: 45686,
			},
			Comment: "Windows 8 Enterprise SYSTEM hive",
		},
		{
			Name: "windows-8-enterprise-software",
			Path: filepath.Join("suite", "windows-8-enterprise-software"),
			Expect: &CellCounts{
				Total:            472180,
				Allocated:        471960,
				Free:             220,
				NK:               92360,
				VK:               149143,
				SK:               289,
				LF:               0,
				LH:               35490,
				LI:               0,
				RI:               4,
				DB:               1,
				UnknownAllocated: 194673,
			},
			Comment: "Windows 8 Enterprise SOFTWARE hive - large file",
		},
		{
			Name: "windows-2012-system",
			Path: filepath.Join("suite", "windows-2012-system"),
			Expect: &CellCounts{
				Total:            157754,
				Allocated:        157528,
				Free:             226,
				NK:               25406,
				VK:               64076,
				SK:               195,
				LF:               0,
				LH:               5979,
				LI:               0,
				RI:               2,
				DB:               2,
				UnknownAllocated: 61868,
			},
			Comment: "Windows 2012 SYSTEM hive",
		},
		{
			Name: "windows-2012-software",
			Path: filepath.Join("suite", "windows-2012-software"),
			Expect: &CellCounts{
				Total:            642476,
				Allocated:        640914,
				Free:             1562,
				NK:               125596,
				VK:               203986,
				SK:               219,
				LF:               0,
				LH:               47673,
				LI:               0,
				RI:               8,
				DB:               2,
				UnknownAllocated: 263430,
			},
			Comment: "Windows 2012 SOFTWARE hive - very large file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			h, err := hive.Open(filepath.Join("..", "..", "testdata", tc.Path))
			require.NoError(t, err, "failed to open hive")

			// Iterate through all HBINs and their cells
			actual := countCells(t, h)

			// Log the results for debugging/analysis
			t.Logf("Cell counts for %s:", tc.Name)
			t.Logf("  Total:     %d", actual.Total)
			t.Logf("  Allocated: %d", actual.Allocated)
			t.Logf("  Free:      %d", actual.Free)
			t.Logf("  NK:        %d", actual.NK)
			t.Logf("  VK:        %d", actual.VK)
			t.Logf("  SK:        %d", actual.SK)
			t.Logf("  LF:        %d", actual.LF)
			t.Logf("  LH:        %d", actual.LH)
			t.Logf("  LI:        %d", actual.LI)
			t.Logf("  RI:        %d", actual.RI)
			t.Logf("  DB:        %d", actual.DB)
			t.Logf("  Unknown:   %d", actual.UnknownAllocated)
			t.Logf("  MinSize:   %d bytes", actual.MinCellSize)
			t.Logf("  MaxSize:   %d bytes", actual.MaxCellSize)
			t.Logf("  TotalBytes: %d bytes", actual.TotalBytes)

			// Invariant checks (always true regardless of expected counts)
			require.Equal(
				t,
				actual.Allocated+actual.Free,
				actual.Total,
				"allocated + free must equal total",
			)

			// Cell type counts should sum to allocated count
			typedCells := actual.NK + actual.VK + actual.SK + actual.LF + actual.LH +
				actual.LI + actual.RI + actual.DB + actual.UnknownAllocated
			require.Equal(
				t,
				actual.Allocated,
				typedCells,
				"sum of typed cells must equal allocated count",
			)

			// Size invariants
			// Minimum cell size is 8 bytes (4-byte header + 4 bytes data, 8-byte aligned)
			require.GreaterOrEqual(
				t,
				actual.MinCellSize,
				8,
				"minimum cell size should be at least 8 bytes (header + min data)",
			)
			require.Equal(
				t,
				0,
				actual.MinCellSize%format.CellAlignment,
				"minimum cell size must be 8-byte aligned",
			)
			require.Positive(t, actual.MaxCellSize, "max cell size should be positive")
			require.Positive(t, actual.TotalBytes, "total bytes should be positive")

			// If we have expected counts, validate them
			if tc.Expect != nil {
				// Only check counts that aren't -1 (meaning "to be determined")
				if tc.Expect.Total >= 0 {
					require.Equal(t, tc.Expect.Total, actual.Total, "total cell count mismatch")
				}
				if tc.Expect.Allocated >= 0 {
					require.Equal(
						t,
						tc.Expect.Allocated,
						actual.Allocated,
						"allocated cell count mismatch",
					)
				}
				if tc.Expect.Free >= 0 {
					require.Equal(t, tc.Expect.Free, actual.Free, "free cell count mismatch")
				}
				if tc.Expect.NK >= 0 {
					require.Equal(t, tc.Expect.NK, actual.NK, "NK cell count mismatch")
				}
				if tc.Expect.VK >= 0 {
					require.Equal(t, tc.Expect.VK, actual.VK, "VK cell count mismatch")
				}
				if tc.Expect.SK >= 0 {
					require.Equal(t, tc.Expect.SK, actual.SK, "SK cell count mismatch")
				}
				if tc.Expect.LF >= 0 {
					require.Equal(t, tc.Expect.LF, actual.LF, "LF cell count mismatch")
				}
				if tc.Expect.LH >= 0 {
					require.Equal(t, tc.Expect.LH, actual.LH, "LH cell count mismatch")
				}
				if tc.Expect.LI >= 0 {
					require.Equal(t, tc.Expect.LI, actual.LI, "LI cell count mismatch")
				}
				if tc.Expect.RI >= 0 {
					require.Equal(t, tc.Expect.RI, actual.RI, "RI cell count mismatch")
				}
				if tc.Expect.DB >= 0 {
					require.Equal(t, tc.Expect.DB, actual.DB, "DB cell count mismatch")
				}
				if tc.Expect.UnknownAllocated >= 0 {
					require.Equal(
						t,
						tc.Expect.UnknownAllocated,
						actual.UnknownAllocated,
						"Unknown cell count mismatch",
					)
				}
			}
		})
	}
}

// countCells iterates through all HBINs and their cells, counting types.
func countCells(t *testing.T, h *hive.Hive) CellCounts {
	t.Helper()

	counts := CellCounts{
		MinCellSize: int(^uint(0) >> 1), // max int
		MaxCellSize: 0,
	}

	hbinIter := h.NewHBINIterator()

	for {
		hbin, err := hbinIter.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "HBIN iteration failed")

		// Iterate through cells in this HBIN
		cellIter := hbin.Cells()
		for {
			cell, cellErr := cellIter.Next()
			if errors.Is(cellErr, io.EOF) {
				break
			}
			require.NoError(t, cellErr, "cell iteration failed")

			// Count total cells
			counts.Total++

			// Track size
			size := cell.SizeAbs()
			counts.TotalBytes += size
			if size < counts.MinCellSize {
				counts.MinCellSize = size
			}
			if size > counts.MaxCellSize {
				counts.MaxCellSize = size
			}

			// Check allocated vs free
			if cell.IsAllocated() {
				counts.Allocated++

				// Identify cell type by signature (first 2 bytes of payload)
				sig := cell.Signature2()
				if len(sig) >= 2 {
					switch {
					case bytes.Equal(sig, format.NKSignature):
						counts.NK++
					case bytes.Equal(sig, format.VKSignature):
						counts.VK++
					case bytes.Equal(sig, format.SKSignature):
						counts.SK++
					case bytes.Equal(sig, format.LFSignature):
						counts.LF++
					case bytes.Equal(sig, format.LHSignature):
						counts.LH++
					case bytes.Equal(sig, format.LISignature):
						counts.LI++
					case bytes.Equal(sig, format.RISignature):
						counts.RI++
					case bytes.Equal(sig, format.DBSignature):
						counts.DB++
					default:
						counts.UnknownAllocated++
					}
				} else {
					counts.UnknownAllocated++
				}
			} else {
				counts.Free++
			}
		}
	}

	return counts
}

// TestCell_Iterator_Alignment validates that all cells are properly aligned.
func TestCell_Iterator_Alignment(t *testing.T) {
	cases := []string{
		filepath.Join("suite", "windows-xp-system"),
		filepath.Join("suite", "windows-xp-software"),
	}

	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			h, err := hive.Open(filepath.Join("..", "..", "testdata", path))
			require.NoError(t, err)

			hbinIter := h.NewHBINIterator()

			for {
				hbin, hbinErr := hbinIter.Next()
				if errors.Is(hbinErr, io.EOF) {
					break
				}
				require.NoError(t, hbinErr)

				cellIter := hbin.Cells()
				prevEndOffset := int(format.HBINHeaderSize)

				for {
					cell, cellErr := cellIter.Next()
					if errors.Is(cellErr, io.EOF) {
						break
					}
					require.NoError(t, cellErr)

					// Cell sizes must be aligned to 8 bytes
					size := cell.SizeAbs()
					require.Equal(
						t,
						0,
						size%format.CellAlignment,
						"cell size %d not aligned to %d bytes",
						size,
						format.CellAlignment,
					)

					// Cells must be contiguous (accounting for alignment padding)
					// Note: We can't easily track absolute offsets from the iterator,
					// but we can validate sizes are sane
					require.GreaterOrEqual(
						t,
						size,
						format.CellHeaderSize,
						"cell size must be at least header size",
					)

					prevEndOffset += size
					if rem := prevEndOffset % format.CellAlignment; rem != 0 {
						prevEndOffset += format.CellAlignment - rem
					}
				}
			}
		})
	}
}

// TestCell_Iterator_SignatureValidation validates that recognized cell types
// have valid signatures and structures.
func TestCell_Iterator_SignatureValidation(t *testing.T) {
	h, err := hive.Open(filepath.Join("..", "..", "testdata", "suite", "windows-xp-system"))
	require.NoError(t, err)

	hbinIter := h.NewHBINIterator()

	nkCount := 0
	vkCount := 0
	skCount := 0

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

			if !cell.IsAllocated() {
				continue
			}

			sig := cell.Signature2()
			if len(sig) < 2 {
				continue
			}

			// For known cell types, validate they can be parsed
			switch {
			case bytes.Equal(sig, format.NKSignature):
				nkCount++
				// Should be parseable as NK
				_, parseErr := hive.ParseNK(cell.Payload())
				require.NoError(t, parseErr, "NK cell at offset should parse")

			case bytes.Equal(sig, format.VKSignature):
				vkCount++
				// Should be parseable as VK
				_, parseErr := hive.ParseVK(cell.Payload())
				require.NoError(t, parseErr, "VK cell should parse")

			case bytes.Equal(sig, format.SKSignature):
				skCount++
				// Should be parseable as SK
				_, parseErr := hive.ParseSK(cell.Payload())
				require.NoError(t, parseErr, "SK cell should parse")
			}
		}
	}

	// Sanity check: we should have found some of each major type
	t.Logf("Found and validated: %d NK, %d VK, %d SK cells", nkCount, vkCount, skCount)
	require.Positive(t, nkCount, "should have found at least one NK cell")
	require.Positive(t, vkCount, "should have found at least one VK cell")
	require.Positive(t, skCount, "should have found at least one SK cell")
}
