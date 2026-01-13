package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- small header builder (keeps tests readable) ---

type regfOpts struct {
	seq1, seq2         uint32
	fileTime           uint64
	major, minor       uint32
	typ, fmt           uint32
	rootCellRel        uint32
	dataSize           uint32 // sum of HBIN sizes (must be 4K aligned)
	flags              uint32
	fileName           []byte // up to 64 bytes
	guidSig            uint32
	lastReorg          uint64
	bootType, bootRecv uint32
	// mutate raw header before checksum (for negative tests)
	mutate func(h []byte)
}

func makeHeader(t *testing.T, o regfOpts) []byte {
	t.Helper()

	if o.dataSize == 0 {
		o.dataSize = 0x2000 // 8 KiB of HBINs by default
	}
	h := make([]byte, format.HeaderSize)

	// Signature
	copy(h[format.REGFSignatureOffset:], format.REGFSignature)

	// Basic fields
	format.PutU32(h, format.REGFPrimarySeqOffset, o.seq1)
	format.PutU32(h, format.REGFSecondarySeqOffset, o.seq2)
	format.PutU64(h, format.REGFTimeStampOffset, o.fileTime)
	format.PutU32(h, format.REGFMajorVersionOffset, o.major)
	format.PutU32(h, format.REGFMinorVersionOffset, o.minor)
	format.PutU32(h, format.REGFTypeOffset, o.typ)
	format.PutU32(h, format.REGFFormatOffset, o.fmt)
	format.PutU32(h, format.REGFRootCellOffset, o.rootCellRel)
	format.PutU32(h, format.REGFDataSizeOffset, o.dataSize)
	format.PutU32(h, format.REGFClusterOffset, 1) // sane default
	if len(o.fileName) > 0 {
		copy(
			h[format.REGFFileNameOffset:format.REGFFileNameOffset+format.REGFFileNameSize],
			o.fileName,
		)
	}
	format.PutU32(h, format.REGFFlagsOffset, o.flags)
	format.PutU32(h, format.REGFGuidSigOffset, o.guidSig)
	format.PutU64(h, format.REGFLastReorgTimeOffset, o.lastReorg)
	format.PutU32(h, format.REGFBootTypeOffset, o.bootType)
	format.PutU32(h, format.REGFBootRecovOffset, o.bootRecv)

	// Caller mutation before checksum (used to corrupt specific bytes)
	if o.mutate != nil {
		o.mutate(h)
	}

	// Compute checksum over first 508 bytes and store at 0x1FC
	sum := regfChecksum(h[:format.REGFChecksumRegionLen])
	format.PutU32(h, format.REGFCheckSumOffset, sum)

	return h
}

// wraps header with trailing HBIN bytes according to DataSize to simulate real file size.
func withFile(hdr []byte, dataSize uint32) []byte {
	fileLen := format.HeaderSize + int(dataSize)
	f := make([]byte, fileLen)
	copy(f, hdr)
	return f
}

// --- tests ---

func TestREGF_Validate_OK(t *testing.T) {
	opts := regfOpts{
		seq1:        0x1111_2222,
		seq2:        0x1111_2222,        // clean hive (equal ok, unequal also allowed)
		fileTime:    0x01DFFFFFFFFFFFFF, // arbitrary FILETIME
		major:       1,
		minor:       5,
		typ:         0,      // common
		fmt:         1,      // common
		rootCellRel: 0x1000, // within data area
		dataSize:    0x4000, // 16 KiB (aligned)
		flags:       0,
		fileName:    []byte("SYSTEM"),
	}
	h := makeHeader(t, opts)
	whole := withFile(h, opts.dataSize)

	bb, err := ParseBaseBlock(whole)
	require.NoError(t, err)
	require.True(t, bb.ChecksumOK())
	require.NoError(t, bb.Validate(len(whole)))

	require.True(t, bb.IsClean())
	require.Equal(t, uint32(1), bb.Major())
	require.Equal(t, uint32(5), bb.Minor())
	require.Equal(t, "regf", string(bb.Signature()))
	require.Equal(t, uint32(0x1000), bb.RootCellOffset())
	require.Equal(t, uint32(0x4000), bb.DataSize())
	require.Equal(t, len(whole), bb.HiveLength())
}

func TestREGF_Validate_DirtyHive_NotError(t *testing.T) {
	opts := regfOpts{
		seq1:        0xAA55AA55,
		seq2:        0xCAFEBABE, // differ => dirty, but not validation error
		major:       1,
		minor:       6,
		rootCellRel: 0x2000,
		dataSize:    0x4000,
	}
	h := makeHeader(t, opts)
	whole := withFile(h, opts.dataSize)

	bb, err := ParseBaseBlock(whole)
	require.NoError(t, err)
	require.NoError(t, bb.Validate(len(whole)))
	require.False(t, bb.IsClean())
}

func TestREGF_Validate_Errors(t *testing.T) {
	type tc struct {
		name       string
		mutate     func([]byte) // mutate AFTER we build a valid header (but BEFORE checksum)
		opts       regfOpts     // used to make an initially-valid header
		fileShrink int          // shrink file to trigger size errors
		wantSubstr string
	}

	base := regfOpts{
		seq1:        1,
		seq2:        1,
		major:       1,
		minor:       5,
		rootCellRel: 0x1000,
		dataSize:    0x3000,
	}

	tests := []tc{
		{
			name: "bad-signature",
			opts: base,
			mutate: func(h []byte) {
				off := format.REGFSignatureOffset
				h[off+0], h[off+1], h[off+2], h[off+3] = 'b', 'a', 'd', '!'
			},
			wantSubstr: "bad signature",
		},
		{
			name: "checksum-mismatch",
			opts: base,
			mutate: func(_ []byte) {
				// Flip a byte in the checksum region; we'll recompute checksum later but we WONT
				// (because we call mutate BEFORE computing checksum in makeHeader, this would be fixed).
				// So to force a real mismatch, we mutate AFTER makeHeader below in test body.
			},
			wantSubstr: "checksum mismatch",
		},
		{
			name:       "data-size-not-4k-aligned",
			opts:       func() regfOpts { o := base; o.dataSize = 0x3001; return o }(),
			wantSubstr: "data size not 4KiB-aligned",
		},
		{
			name: "reported-length-exceeds-file",
			opts: base,
			// shrink the file by 4 KiB so HiveLength > fileSize
			fileShrink: 0x1000,
			wantSubstr: "reported hive length",
		},
		{
			name:       "root-outside-data",
			opts:       func() regfOpts { o := base; o.rootCellRel = 0x4000; return o }(),
			wantSubstr: "root cell offset",
		},
		{
			name:       "root-zero",
			opts:       func() regfOpts { o := base; o.rootCellRel = 0; return o }(),
			wantSubstr: "root cell offset is zero",
		},
		{
			name:       "bad-major",
			opts:       func() regfOpts { o := base; o.major = 0; return o }(),
			wantSubstr: "unsupported major version",
		},
		{
			name:       "bad-minor-low",
			opts:       func() regfOpts { o := base; o.minor = 2; return o }(),
			wantSubstr: "unsupported minor version",
		},
		{
			name:       "bad-minor-high",
			opts:       func() regfOpts { o := base; o.minor = 7; return o }(),
			wantSubstr: "unsupported minor version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.opts
			if tt.mutate != nil {
				opts.mutate = tt.mutate // <-- wire it in
			}
			h := makeHeader(t, opts)

			// Special case: force checksum mismatch by corrupting a covered byte AFTER checksum.
			if tt.name == "checksum-mismatch" {
				h[format.REGFFileNameOffset] ^= 0xFF // inside first 508 bytes
			}

			fileBytes := withFile(h, opts.dataSize)
			if tt.fileShrink > 0 {
				fileBytes = fileBytes[:len(fileBytes)-tt.fileShrink]
			}

			bb, err := ParseBaseBlock(fileBytes)
			// Only signature/size issues should bubble here; others show up in Validate.
			if tt.name == "bad-signature" {
				require.Error(t, err)
				require.Contains(t, err.Error(), "bad REGF signature")
				return
			}
			require.NoError(t, err)

			err = bb.Validate(len(fileBytes))
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantSubstr)
		})
	}
}

func TestREGF_FileNameAndGUIDSlices_NoAlloc(t *testing.T) {
	opts := regfOpts{
		major:       1,
		minor:       3,
		rootCellRel: 0x1000,
		dataSize:    0x2000,
		fileName:    []byte("SOFTWARE\x00"), // can contain zeros; no decoding here
	}
	h := makeHeader(t, opts)
	whole := withFile(h, opts.dataSize)

	bb, err := ParseBaseBlock(whole)
	require.NoError(t, err)
	require.NoError(t, bb.Validate(len(whole)))

	// Verify we can take zero-copy slices safely
	fn := bb.FileName()
	require.Len(t, fn, 64)
	require.Equal(t, byte('S'), fn[0])

	rm := bb.RmID()
	log := bb.LogID()
	tm := bb.TmID()
	require.Len(t, rm, format.GUIDSize)
	require.Len(t, log, format.GUIDSize)
	require.Len(t, tm, format.GUIDSize)
}
