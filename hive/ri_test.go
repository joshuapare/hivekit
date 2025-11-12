package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- ri tests ---

func TestRI_ParseOK(t *testing.T) {
	const n = 4
	buf := mkHeader(format.RISignature, n)
	buf = append(buf, make([]byte, n*format.LIEntrySize)...)

	for i := range n {
		format.PutU32(buf, format.IdxListOffset+i*format.LIEntrySize, 0x9000+0x100*uint32(i))
	}

	ri, err := ParseRI(buf)
	require.NoError(t, err)
	require.Equal(t, n, ri.Count())

	raw := ri.RawList()
	require.Len(t, raw, n*format.LIEntrySize)

	require.Equal(t, uint32(0x9000), ri.LeafCellAt(0))
	require.Equal(t, uint32(0x9100), ri.LeafCellAt(1))
	require.Equal(t, uint32(0x9200), ri.LeafCellAt(2))
	require.Equal(t, uint32(0x9300), ri.LeafCellAt(3))
}

func TestRI_ZeroCount_HeaderOnly(t *testing.T) {
	buf := mkHeader(format.RISignature, 0)
	ri, err := ParseRI(buf)
	require.NoError(t, err)
	require.Equal(t, 0, ri.Count())
	require.Empty(t, ri.RawList())
}

func TestRI_TruncatedList(t *testing.T) {
	// count=3 but only 2 entries present
	buf := mkHeader(format.RISignature, 3)
	buf = append(buf, make([]byte, 2*format.LIEntrySize)...)
	_, err := ParseRI(buf)
	require.Error(t, err)
}
