package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// helper: write a cell at relOff with negative size header and payload.
func writeCell(hive []byte, relOff uint32, payload []byte) {
	abs := format.HiveDataBase + int(relOff)
	size := 4 + len(payload)
	if r := size & 7; r != 0 {
		size += (8 - r)
	}
	format.PutI32(hive, abs, int32(-size))
	copy(hive[abs+4:], payload)
}

func TestVK_SmallData_Inline(t *testing.T) {
	// Build a single VK payload with 3-byte inline data and compressed name "Val"
	name := []byte("Val")
	data := []byte{0xDE, 0xAD, 0xBE}
	pl := make([]byte, format.VKFixedHeaderSize+len(name))
	copy(pl[format.VKSignatureOffset:], format.VKSignature)
	format.PutU16(pl, format.VKNameLenOffset, uint16(len(name)))
	format.PutU32(pl, format.VKDataLenOffset, uint32(len(data))|format.VKSmallDataMask)
	// Inline bytes go into DataOff field; remaining byte ignored.
	tmp := []byte{0, 0, 0, 0}
	copy(tmp, data)
	format.PutU32(
		pl,
		format.VKDataOffOffset,
		uint32(tmp[0])|uint32(tmp[1])<<8|uint32(tmp[2])<<16|uint32(tmp[3])<<24,
	)
	format.PutU32(pl, format.VKTypeOffset, format.RegBinary)
	format.PutU16(pl, format.VKFlagsOffset, format.VKFlagNameCompressed)
	copy(pl[format.VKNameOffset:], name)
	pl = pl[:format.VKNameOffset+len(name)]

	vk, err := ParseVK(pl)
	require.NoError(t, err)
	require.Equal(t, "Val", string(vk.Name()))
	require.True(t, vk.IsSmallData())
	require.Equal(t, 3, vk.DataLen())

	// hiveBuf is not needed for inline; pass nil
	got, err := vk.Data(nil)
	require.NoError(t, err)
	require.Equal(t, data, got)
}

func TestVK_ExternalData_Cell(t *testing.T) {
	// Create a hive buffer and write an external data cell with 6 bytes.
	hive := make([]byte, format.HiveDataBase+0x4000)
	data := []byte{1, 2, 3, 4, 5, 6}
	const dataRel = 0x0200
	writeCell(hive, dataRel, data)

	// VK points to that dataRel, with name "Foo"
	name := []byte("Foo")
	pl := make([]byte, format.VKFixedHeaderSize+len(name))
	copy(pl[format.VKSignatureOffset:], format.VKSignature)
	format.PutU16(pl, format.VKNameLenOffset, uint16(len(name)))
	format.PutU32(pl, format.VKDataLenOffset, uint32(len(data))) // NOT small
	format.PutU32(pl, format.VKDataOffOffset, dataRel)           // external HCELL index
	format.PutU32(pl, format.VKTypeOffset, format.RegBinary)
	format.PutU16(pl, format.VKFlagsOffset, format.VKFlagNameCompressed)
	copy(pl[format.VKNameOffset:], name)
	pl = pl[:format.VKNameOffset+len(name)]

	vk, err := ParseVK(pl)
	require.NoError(t, err)
	require.False(t, vk.IsSmallData())

	got, err := vk.Data(hive)
	require.NoError(t, err)
	require.Equal(t, data, got)
}
