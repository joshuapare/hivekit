package e2e

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

func sPtr(s string) *string { return &s }

type parseHBINCase struct {
	Name    string
	Path    string
	Abs     uint32           // absolute start of HBIN to parse (usually 0x1000)
	Mutate  func([]byte)     // optional in-memory modification before parse
	Expect  *parseHBINExpect // nil => expect success, then sanity asserts
	Comment string
}

type parseHBINExpect struct {
	ErrSubstr *string // if non-nil, ParseHBINAt MUST error and contain this substring

	// If no error expected, we can assert some invariants/values:
	ExpectAligned bool   // require 4KiB-aligned start & size
	WantMinSize   uint32 // e.g., >= 0x1000
}

func TestHBIN_ParseHBINAt_Table(t *testing.T) {
	cases := []parseHBINCase{
		{
			Name: "good_xp_system_first_bin",
			Path: filepath.Join("suite", "windows-xp-system"),
			Abs:  uint32(format.HeaderSize),
			Expect: &parseHBINExpect{
				ErrSubstr:     nil,
				ExpectAligned: true,
				WantMinSize:   0x1000,
			},
		},
		// Corrupted HBINs from your README directory
		{
			Name: "corrupt_hbin_signature",
			Path: filepath.Join("corrupted", "corrupt_hbin_signature"),
			Abs:  uint32(format.HeaderSize),
			Expect: &parseHBINExpect{
				ErrSubstr: sPtr("HBIN bad signature"),
			},
		},
		{
			Name: "corrupt_hbin_size_zero",
			Path: filepath.Join("corrupted", "corrupt_hbin_size_zero"),
			Abs:  uint32(format.HeaderSize),
			Expect: &parseHBINExpect{
				ErrSubstr: sPtr("size 0"),
			},
		},
		{
			Name: "corrupt_hbin_size_unaligned",
			Path: filepath.Join("corrupted", "corrupt_hbin_size_unaligned"),
			Abs:  uint32(format.HeaderSize),
			Expect: &parseHBINExpect{
				ErrSubstr: sPtr("size 0x"),
			},
		},
		{
			Name: "corrupt_hbin_size_overflow",
			Path: filepath.Join("corrupted", "corrupt_hbin_size_overflow"),
			Abs:  uint32(format.HeaderSize),
			Expect: &parseHBINExpect{
				ErrSubstr: sPtr("exceeds file"),
			},
		},
		// Synthetic: force offset-echo mismatch at +0x04
		{
			Name: "hbin_offset_echo_mismatch",
			Path: filepath.Join("suite", "windows-xp-system"),
			Abs:  uint32(format.HeaderSize),
			Mutate: func(b []byte) {
				off := uint32(format.HeaderSize) + uint32(format.HBINOffsetEchoOffset)
				// Flip a bit inside the offset echo
				b[off] ^= 0x01
			},
			Expect: &parseHBINExpect{
				// we do NOT error on this since it's valid to not have it
				ErrSubstr: nil,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := loadHiveFile(filepath.Join("testdata", tc.Path))
			require.NoError(t, err, "unable to read test hive")

			// in-memory mutate (does not touch disk)
			if tc.Mutate != nil {
				tc.Mutate(raw)
			}

			hb, perr := hive.ParseHBINAt(raw, tc.Abs)
			if tc.Expect != nil && tc.Expect.ErrSubstr != nil {
				require.Error(t, perr, "ParseHBINAt should have failed")
				require.Contains(t, perr.Error(), *tc.Expect.ErrSubstr)
				return
			}
			require.NoError(t, perr, "ParseHBINAt failed on %s", tc.Name)

			// Sanity asserts on helpers/invariants:
			// Header() and Payload() are zero-copy slices into Data
			require.True(
				t,
				bytes.Equal(hb.Header(), hb.Data[:format.HBINHeaderSize]),
				"Header slice mismatch",
			)
			require.True(
				t,
				bytes.Equal(hb.Payload(), hb.Data[format.HBINHeaderSize:]),
				"Payload slice mismatch",
			)

			if tc.Expect != nil && tc.Expect.ExpectAligned {
				require.Equal(
					t,
					uint32(0),
					hb.Offset%uint32(format.HeaderSize),
					"HBIN start not 4KiB aligned",
				)
				require.Equal(
					t,
					uint32(0),
					hb.Size%uint32(format.HeaderSize),
					"HBIN size not 4KiB aligned",
				)
			}
			if tc.Expect != nil && tc.Expect.WantMinSize > 0 {
				require.GreaterOrEqual(t, hb.Size, tc.Expect.WantMinSize, "HBIN size too small")
			}

			require.Equal(
				t,
				hb.Offset+uint32(format.HBINHeaderSize),
				hb.FirstCellAbs(),
				"FirstCellAbs mismatch",
			)
			require.Equal(t, hb.Offset+hb.Size, hb.EndAbs(), "EndAbs mismatch")

			// Signature must be "hbin" at header
			require.Equal(t, "hbin", string(hb.Header()[:4]))
		})
	}
}
