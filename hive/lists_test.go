package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// --- kind detection ---

func TestDetectListKind(t *testing.T) {
	type tc struct {
		sig  []byte
		kind SubkeyListKind
	}
	cases := []tc{
		{format.LISignature, ListLI},
		{format.LFSignature, ListLF},
		{format.LHSignature, ListLH},
		{format.RISignature, ListRI},
	}
	for _, c := range cases {
		buf := mkHeader(c.sig, 0)
		require.Equal(t, c.kind, DetectListKind(buf))
	}

	require.Equal(t, ListUnknown, DetectListKind([]byte("zzzz")))
}

// --- extra edge cases ---

func TestIndex_HeaderExactlyMinSize(t *testing.T) {
	// exactly 4 bytes header; count=0
	buf := mkHeader(format.LISignature, 0)
	_, err := ParseLI(buf)
	require.NoError(t, err)

	buf = mkHeader(format.LFSignature, 0)
	_, err = ParseLF(buf)
	require.NoError(t, err)

	buf = mkHeader(format.LHSignature, 0)
	_, err = ParseLH(buf)
	require.NoError(t, err)

	buf = mkHeader(format.RISignature, 0)
	_, err = ParseRI(buf)
	require.NoError(t, err)
}

func TestIndex_HeaderTooShort(t *testing.T) {
	short := []byte{format.LISignature[0], format.LISignature[1], 0x00} // 3 bytes only
	_, err := ParseLI(short)
	require.Error(t, err)
}
