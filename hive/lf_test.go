package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- helpers ---

func mkHeader(sig []byte, count uint16) []byte {
	buf := make([]byte, format.IdxMinHeader)
	buf[format.IdxSignatureOffset+0] = sig[0]
	buf[format.IdxSignatureOffset+1] = sig[1]
	format.PutU16(buf, format.IdxCountOffset, count)
	return buf
}

// --- lf tests ---

func TestLF_ParseOK_AndEntries(t *testing.T) {
	const n = 2
	buf := mkHeader(format.LFSignature, n)
	// Each entry: Cell(uint32) + Hint(uint32)
	buf = append(buf, make([]byte, n*format.LFFHEntrySize)...)

	// Entry 0
	format.PutU32(buf, format.IdxListOffset+0, 0x2000)                       // Cell
	copy(buf[format.IdxListOffset+4:format.IdxListOffset+8], []byte("abcd")) // Hint

	// Entry 1
	format.PutU32(buf, format.IdxListOffset+8, 0x3000)                         // Cell
	copy(buf[format.IdxListOffset+12:format.IdxListOffset+16], []byte("WXYZ")) // Hint (raw bytes)

	lf, err := ParseLF(buf)
	require.NoError(t, err)
	require.Equal(t, n, lf.Count())

	raw := lf.RawList()
	require.Len(t, raw, n*format.LFFHEntrySize)

	e0 := lf.Entry(0)
	require.Equal(t, uint32(0x2000), e0.Cell())
	require.Equal(t, []byte("abcd"), e0.HintBytes())

	e1 := lf.Entry(1)
	require.Equal(t, uint32(0x3000), e1.Cell())
	require.Equal(t, []byte("WXYZ"), e1.HintBytes())
}

func TestLF_ZeroCount_HeaderOnly(t *testing.T) {
	buf := mkHeader(format.LFSignature, 0)
	lf, err := ParseLF(buf)
	require.NoError(t, err)
	require.Equal(t, 0, lf.Count())
	require.Empty(t, lf.RawList())
}

func TestLF_BadSignature(t *testing.T) {
	buf := mkHeader([]byte("lF"), 1) // case-sensitive mismatch
	_, err := ParseLF(buf)
	require.Error(t, err)
}

func TestLF_TruncatedList(t *testing.T) {
	// count=1 but not enough for 8-byte entry
	buf := mkHeader(format.LFSignature, 1)
	// append fewer than 8 bytes
	buf = append(buf, 0xAA, 0xBB, 0xCC)
	_, err := ParseLF(buf)
	require.Error(t, err)
}
