package hive

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// makeNKPayload builds a synthetic NK payload we control entirely,
// using the *spec-correct* offsets (the ones we just aligned).
func makeNKPayload(t *testing.T, mutate func([]byte)) []byte {
	t.Helper()

	// start with minimum size (fixed header) + room for name
	buf := make([]byte, format.NKFixedHeaderSize+32) // NKFixedHeaderSize should be 0x4C

	// signature "nk"
	copy(buf[format.NKSignatureOffset:format.NKSignatureOffset+2], format.NKSignature)

	// flags @ 0x02
	format.PutU16(buf, format.NKFlagsOffset, 0x1234)

	// access bits (optional) @ 0x0C
	format.PutU32(buf, format.NKAccessBitsOffset, 0xDEADBEEF)

	// parent offset @ 0x10
	format.PutU32(buf, format.NKParentOffset, 0x2000)

	// stable subkey count @ 0x14
	format.PutU32(buf, format.NKSubkeyCountOffset, 3)

	// volatile subkey count @ 0x18
	format.PutU32(buf, format.NKVolSubkeyCountOffset, 0)

	// stable subkey list @ 0x1C
	format.PutU32(buf, format.NKSubkeyListOffset, 0x3000)

	// volatile subkey list @ 0x20
	format.PutU32(buf, format.NKVolSubkeyListOffset, 0x0000)

	// value count @ 0x24
	format.PutU32(buf, format.NKValueCountOffset, 2)

	// value list @ 0x28
	format.PutU32(buf, format.NKValueListOffset, 0x4000)

	// security @ 0x2C
	format.PutU32(buf, format.NKSecurityOffset, 0x5000)

	// class @ 0x30
	format.PutU32(buf, format.NKClassNameOffset, 0x6000)

	// name length + name
	name := []byte("ControlSet001")
	format.PutU16(buf, format.NKNameLenOffset, uint16(len(name)))
	copy(buf[format.NKNameOffset:], name)

	if mutate != nil {
		mutate(buf)
	}

	return buf
}

func TestNK_ParseOK(t *testing.T) {
	payload := makeNKPayload(t, nil)

	nk, err := ParseNK(payload)
	require.NoError(t, err)

	// sig
	require.Equal(t, "nk", string(payload[0:2]))

	// flags
	require.Equal(t, uint16(0x1234), nk.Flags())

	// parent
	require.Equal(t, uint32(0x2000), nk.ParentOffsetRel())

	// subkeys
	require.Equal(t, uint32(3), nk.SubkeyCount())
	require.Equal(t, uint32(0x3000), nk.SubkeyListOffsetRel())

	// values
	require.Equal(t, uint32(2), nk.ValueCount())
	require.Equal(t, uint32(0x4000), nk.ValueListOffsetRel())

	// security + class
	require.Equal(t, uint32(0x5000), nk.SecurityOffsetRel())
	require.Equal(t, uint32(0x6000), nk.ClassNameOffsetRel())

	// name
	require.Equal(t, uint16(len("ControlSet001")), nk.NameLength())
	require.Equal(t, "ControlSet001", string(nk.Name()))
}

func TestNK_BadSig(t *testing.T) {
	payload := makeNKPayload(t, func(b []byte) {
		b[0] = 'x'
		b[1] = 'x'
	})

	_, err := ParseNK(payload)
	require.Error(t, err)
}

func TestNK_TooSmall(t *testing.T) {
	// smaller than NKFixedHeaderSize / NKMinSize
	payload := make([]byte, 10)
	copy(payload, format.NKSignature)

	_, err := ParseNK(payload)
	require.Error(t, err)
}
