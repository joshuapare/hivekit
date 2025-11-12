package e2e

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

type iterCase struct {
	Name          string
	Path          string
	ExpectMinBins int     // at least N bins
	ExpectBins    *int    // at least N bins
	ExpectEOF     bool    // Next() returns io.EOF after last
	ExpectError   *string // if non-nil, expect error from Next() mid-iteration
	Comment       string
}

func TestHBIN_Iterator_Basics(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	cases := []iterCase{
		{
			Name:          "good_xp_system_iterates",
			Path:          filepath.Join("suite", "windows-xp-system"),
			ExpectMinBins: 1,
			ExpectEOF:     true,
			Comment:       "Iterates through all bins, then EOF",
		},
		{
			Name:      "corrupt_hbin_signature_treated_as_end_by_iterator",
			Path:      filepath.Join("corrupted", "corrupt_hbin_signature"),
			ExpectEOF: true, // iterator treats non-hbin as end-of-bins (padding)
			Comment:   "Strict detection covered by ParseHBINAt; iterator ends silently.",
		},
		// Suite test files with exact HBIN counts
		{
			Name:       "windows-2003-server-software",
			Path:       filepath.Join("suite", "windows-2003-server-software"),
			ExpectBins: intPtr(3776),
			ExpectEOF:  true,
			Comment:    "Windows 2003 Server SOFTWARE hive",
		},
		{
			Name:       "windows-2003-server-system",
			Path:       filepath.Join("suite", "windows-2003-server-system"),
			ExpectBins: intPtr(485),
			ExpectEOF:  true,
			Comment:    "Windows 2003 Server SYSTEM hive",
		},
		{
			Name:       "windows-2012-software",
			Path:       filepath.Join("suite", "windows-2012-software"),
			ExpectBins: intPtr(8320),
			ExpectEOF:  true,
			Comment:    "Windows 2012 SOFTWARE hive",
		},
		{
			Name:       "windows-2012-system",
			Path:       filepath.Join("suite", "windows-2012-system"),
			ExpectBins: intPtr(2143),
			ExpectEOF:  true,
			Comment:    "Windows 2012 SYSTEM hive",
		},
		{
			Name:       "windows-8-consumer-preview-software",
			Path:       filepath.Join("suite", "windows-8-consumer-preview-software"),
			ExpectBins: intPtr(9470),
			ExpectEOF:  true,
			Comment:    "Windows 8 Consumer Preview SOFTWARE hive",
		},
		{
			Name:       "windows-8-consumer-preview-system",
			Path:       filepath.Join("suite", "windows-8-consumer-preview-system"),
			ExpectBins: intPtr(1602),
			ExpectEOF:  true,
			Comment:    "Windows 8 Consumer Preview SYSTEM hive",
		},
		{
			Name:       "windows-8-enterprise-software",
			Path:       filepath.Join("suite", "windows-8-enterprise-software"),
			ExpectBins: intPtr(5898),
			ExpectEOF:  true,
			Comment:    "Windows 8 Enterprise SOFTWARE hive",
		},
		{
			Name:       "windows-8-enterprise-system",
			Path:       filepath.Join("suite", "windows-8-enterprise-system"),
			ExpectBins: intPtr(1586),
			ExpectEOF:  true,
			Comment:    "Windows 8 Enterprise SYSTEM hive",
		},
		{
			Name:       "windows-xp-2-software",
			Path:       filepath.Join("suite", "windows-xp-2-software"),
			ExpectBins: intPtr(2187),
			ExpectEOF:  true,
			Comment:    "Windows XP 2 SOFTWARE hive",
		},
		{
			Name:       "windows-xp-2-system",
			Path:       filepath.Join("suite", "windows-xp-2-system"),
			ExpectBins: intPtr(689),
			ExpectEOF:  true,
			Comment:    "Windows XP 2 SYSTEM hive",
		},
		{
			Name:       "windows-xp-software",
			Path:       filepath.Join("suite", "windows-xp-software"),
			ExpectBins: intPtr(674),
			ExpectEOF:  true,
			Comment:    "Windows XP SOFTWARE hive",
		},
		{
			Name:       "windows-xp-system",
			Path:       filepath.Join("suite", "windows-xp-system"),
			ExpectBins: intPtr(1586),
			ExpectEOF:  true,
			Comment:    "Windows XP SYSTEM hive",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			h, err := hive.Open(filepath.Join("..", "..", "testdata", tc.Path))
			require.NoError(t, err)

			it := h.NewHBINIterator()

			count := 0
			var lastOff uint32
			for {
				hb, iterErr := it.Next()
				if errors.Is(iterErr, io.EOF) {
					require.True(t, tc.ExpectEOF, "unexpected EOF handling")
					break
				}
				if tc.ExpectError != nil {
					require.Error(t, iterErr)
					require.Contains(t, iterErr.Error(), *tc.ExpectError)
					return
				}
				require.NoError(t, iterErr)

				// Monotonic, aligned, sane:
				if count > 0 {
					require.Greater(t, hb.Offset, lastOff, "HBIN offsets must be increasing")
				}
				require.Equal(
					t,
					uint32(0),
					hb.Offset%uint32(format.HeaderSize),
					"HBIN start alignment",
				)
				require.Equal(
					t,
					uint32(0),
					hb.Size%uint32(format.HeaderSize),
					"HBIN size alignment",
				)
				require.Equal(t, hb.Offset+uint32(format.HBINHeaderSize), hb.FirstCellAbs())
				require.Equal(t, "hbin", string(hb.Header()[:4]))

				lastOff = hb.Offset
				count++
			}

			if tc.ExpectMinBins > 0 {
				require.GreaterOrEqual(
					t,
					count,
					tc.ExpectMinBins,
					"expected at least %d HBINs",
					tc.ExpectMinBins,
				)
			}
			if tc.ExpectBins != nil {
				require.Equal(
					t,
					*tc.ExpectBins,
					count,
					"expected %d HBINs visited, visited %d",
					*tc.ExpectBins,
					count,
				)
			}
		})
	}
}
