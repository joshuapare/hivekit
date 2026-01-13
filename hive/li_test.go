package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- li tests ---

func TestLI_ParseOK(t *testing.T) {
	const n = 3
	buf := mkHeader(format.LISignature, n)
	// append n uint32 entries
	for i := range n {
		buf = append(buf, 0, 0, 0, 0)
		format.PutU32(buf, format.IdxListOffset+i*format.LIEntrySize, uint32(0x1000+4*i))
	}

	li, err := ParseLI(buf)
	require.NoError(t, err)
	require.Equal(t, n, li.Count())

	// zero-copy slice length
	raw := li.RawList()
	require.Len(t, raw, n*format.LIEntrySize)

	// per-entry
	require.Equal(t, uint32(0x1000), li.CellIndexAt(0))
	require.Equal(t, uint32(0x1004), li.CellIndexAt(1))
	require.Equal(t, uint32(0x1008), li.CellIndexAt(2))
}

func TestLI_ZeroCount_HeaderOnly(t *testing.T) {
	buf := mkHeader(format.LISignature, 0)
	li, err := ParseLI(buf)
	require.NoError(t, err)
	require.Equal(t, 0, li.Count())
	require.Empty(t, li.RawList())
}

func TestLI_BadSignature(t *testing.T) {
	buf := mkHeader([]byte("zz"), 1)
	_, err := ParseLI(buf)
	require.Error(t, err)
}

func TestLI_TruncatedHeader(t *testing.T) {
	// less than 0x04 bytes
	_, err := ParseLI([]byte{format.LISignature[0], format.LISignature[1], 0x01})
	require.Error(t, err)
}

func TestLI_TruncatedList(t *testing.T) {
	// count=2 but only one entry present
	buf := mkHeader(format.LISignature, 2)
	buf = append(buf, 0, 0, 0, 0) // 1 entry instead of 2
	_, err := ParseLI(buf)
	require.Error(t, err)
}
