package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

func TestResolveRelCellPayload_OK(t *testing.T) {
	// Build a tiny hive with one allocated cell at rel 0x200
	hive := make([]byte, format.HiveDataBase+0x4000)
	rel := uint32(0x0200)
	abs := format.HiveDataBase + int(rel)

	payload := []byte{1, 2, 3, 4, 5}
	size := 4 + len(payload)
	if size%8 != 0 {
		size += 8 - (size % 8)
	} // common alignment
	format.PutI32(hive, abs, int32(-size)) // negative => allocated
	copy(hive[abs+4:], payload)

	got, err := resolveRelCellPayload(hive, rel)
	require.NoError(t, err)
	require.Equal(t, payload, got[:len(payload)])
}

func TestResolveRelCellPayload_Errors(t *testing.T) {
	// Empty hive
	_, err := resolveRelCellPayload(nil, 0x100)
	require.Error(t, err)

	// Truncated header
	hive := make([]byte, format.HiveDataBase+0x10)
	rel := uint32(0x0008)
	_, err = resolveRelCellPayload(hive, rel)
	require.Error(t, err)

	// Declared size > available
	abs := format.HiveDataBase + int(rel)
	format.PutI32(hive, abs, int32(-4096))
	_, err = resolveRelCellPayload(hive, rel)
	require.Error(t, err)
}
