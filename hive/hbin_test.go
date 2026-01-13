package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

func TestHBINIterator_OK(t *testing.T) {
	// build: 4K REGF + 4K HBIN
	buf := make([]byte, format.HeaderSize+format.HBINAlignment)

	// REGF
	copy(buf[0:4], []byte("regf"))
	// root rel = 0x20
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	// data size = 0x1000
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(format.HBINAlignment))

	// HBIN at 0x1000
	copy(buf[format.HeaderSize:format.HeaderSize+4], []byte("hbin"))
	// size = 0x1000
	format.PutU32(buf, format.HeaderSize+format.HBINSizeOffset, uint32(format.HBINAlignment))

	h := &Hive{
		data: buf,
		size: int64(len(buf)),
		base: &BaseBlock{raw: buf[:format.HeaderSize]},
	}

	it, err := h.HBINs()
	require.NoError(t, err)

	hb, err := it.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(format.HeaderSize), hb.Offset)
	require.Equal(t, uint32(format.HBINAlignment), hb.Size)
}

func TestHBINIterator_Truncated(t *testing.T) {
	// We declare 1 HBIN of size 0x1000 but give the file less.
	full := make([]byte, format.HeaderSize+format.HBINAlignment)
	copy(full[0:4], []byte("regf"))
	format.PutU32(full, format.REGFRootCellOffset, 0x20)
	format.PutU32(full, format.REGFDataSizeOffset, uint32(format.HBINAlignment))

	// HBIN header (will be truncated)
	copy(full[format.HeaderSize:format.HeaderSize+4], []byte("hbin"))
	format.PutU32(full, format.HeaderSize+format.HBINSizeOffset, uint32(format.HBINAlignment))

	// now truncate to 4608 bytes
	trunc := full[:4608]

	h := &Hive{
		data: trunc,
		size: int64(len(trunc)),
		base: &BaseBlock{raw: trunc[:format.HeaderSize]},
	}

	it, err := h.HBINs()
	require.NoError(t, err)

	_, err = it.Next()
	require.Error(t, err)
	require.Contains(t, err.Error(), "HBIN")
}
