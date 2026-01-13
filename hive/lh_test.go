package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- lh tests ---

func TestLH_ParseOK_AndEntries(t *testing.T) {
	const n = 3
	buf := mkHeader(format.LHSignature, n)
	buf = append(buf, make([]byte, n*format.LFFHEntrySize)...)

	// Fill cells and hash keys
	for i := range n {
		cell := 0x4000 + 0x10*uint32(i)
		hash := 0xAABBCCDD + uint32(i)
		off := format.IdxListOffset + i*format.LFFHEntrySize
		format.PutU32(buf, off, cell)
		format.PutU32(buf, off+4, hash)
	}

	lh, err := ParseLH(buf)
	require.NoError(t, err)
	require.Equal(t, n, lh.Count())

	raw := lh.RawList()
	require.Len(t, raw, n*format.LFFHEntrySize)

	// spot-check first & last
	e0 := lh.Entry(0)
	require.Equal(t, uint32(0x4000), e0.Cell())
	require.Equal(t, uint32(0xAABBCCDD), e0.HashKey())

	e2 := lh.Entry(2)
	require.Equal(t, uint32(0x4020), e2.Cell())
	require.Equal(t, uint32(0xAABBCCDF), e2.HashKey())
}

func TestLH_ZeroCount_HeaderOnly(t *testing.T) {
	buf := mkHeader(format.LHSignature, 0)
	lh, err := ParseLH(buf)
	require.NoError(t, err)
	require.Equal(t, 0, lh.Count())
	require.Empty(t, lh.RawList())
}

func TestLH_TruncatedList(t *testing.T) {
	// count=2 but only 1 entry present (8 bytes missing)
	buf := mkHeader(format.LHSignature, 2)
	buf = append(buf, make([]byte, format.LFFHEntrySize)...) // only 1 entry
	_, err := ParseLH(buf)
	require.Error(t, err)
}
