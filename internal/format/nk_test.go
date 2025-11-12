package format

import (
	"encoding/binary"
	"testing"
)

func TestDecodeNKCompressedName(t *testing.T) {
	buf := make([]byte, NKFixedHeaderSize+4)
	copy(buf, NKSignature)
	binary.LittleEndian.PutUint16(buf[NKFlagsOffset:], NKFlagCompressedName)
	binary.LittleEndian.PutUint64(buf[NKLastWriteOffset:], 0xfeedface)
	binary.LittleEndian.PutUint32(buf[NKAccessBitsOffset:], 0)
	binary.LittleEndian.PutUint32(buf[NKParentOffset:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(buf[NKSubkeyCountOffset:], 1)
	binary.LittleEndian.PutUint32(buf[NKVolSubkeyCountOffset:], 0)
	binary.LittleEndian.PutUint32(buf[NKSubkeyListOffset:], 0x200)
	binary.LittleEndian.PutUint32(buf[NKVolSubkeyListOffset:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(buf[NKValueCountOffset:], 2)
	binary.LittleEndian.PutUint32(buf[NKValueListOffset:], 0x300)
	name := []byte("ROOT")
	binary.LittleEndian.PutUint16(buf[NKNameLenOffset:], uint16(len(name)))
	copy(buf[NKNameOffset:], name)

	nk, err := DecodeNK(buf)
	if err != nil {
		t.Fatalf("DecodeNK: %v", err)
	}
	if string(nk.NameRaw) != "ROOT" || !nk.NameIsCompressed() {
		t.Fatalf("unexpected name: %+v", nk)
	}
	if nk.SubkeyCount != 1 || nk.ValueCount != 2 {
		t.Fatalf("unexpected counts: %+v", nk)
	}
}

func TestDecodeNKTruncated(t *testing.T) {
	buf := make([]byte, 2)
	copy(buf, NKSignature)
	if _, err := DecodeNK(buf); err == nil {
		t.Fatalf("expected truncation error")
	}
}

// TestDecodeNK_UTF16Name tests NK records with UTF-16LE encoded names.
// When the compressed name flag (NKFlagCompressedName) is NOT set, the name should be
// decoded as UTF-16LE, not as ASCII/CP-1252.
//
// Bug reproduction: gohivex produces mojibake for non-ASCII UTF-16LE names.
// Example from hivex comparison on 'special' hive:
//   - hivex:   "abcd_äöüß"
//   - gohivex: "abcd_����" (mojibake)
//
// References:
// - special hive: 1 node with UTF-16LE name
// - windows-2003-server-software: 2 nodes with UTF-16LE names.
func TestDecodeNK_UTF16Name(t *testing.T) {
	// Test name "abcd_äöüß" in UTF-16LE encoding
	// UTF-16LE bytes for "abcd_äöüß":
	//   a=0x61, b=0x62, c=0x63, d=0x64, _=0x5F
	//   ä=0xE4, ö=0xF6, ü=0xFC, ß=0xDF
	nameUTF16LE := []byte{
		0x61, 0x00, // 'a'
		0x62, 0x00, // 'b'
		0x63, 0x00, // 'c'
		0x64, 0x00, // 'd'
		0x5F, 0x00, // '_'
		0xE4, 0x00, // 'ä'
		0xF6, 0x00, // 'ö'
		0xFC, 0x00, // 'ü'
		0xDF, 0x00, // 'ß'
	}

	buf := make([]byte, NKFixedHeaderSize+len(nameUTF16LE))
	copy(buf, NKSignature)
	binary.LittleEndian.PutUint16(buf[NKFlagsOffset:], 0x0000) // flags: 0x0000 = NOT compressed (UTF-16LE)
	binary.LittleEndian.PutUint64(buf[NKLastWriteOffset:], 0xfeedface)
	binary.LittleEndian.PutUint32(buf[NKAccessBitsOffset:], 0)
	binary.LittleEndian.PutUint32(buf[NKParentOffset:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(buf[NKSubkeyCountOffset:], 0)
	binary.LittleEndian.PutUint32(buf[NKVolSubkeyCountOffset:], 0)
	binary.LittleEndian.PutUint32(buf[NKSubkeyListOffset:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(buf[NKVolSubkeyListOffset:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(buf[NKValueCountOffset:], 0)
	binary.LittleEndian.PutUint32(buf[NKValueListOffset:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint16(buf[NKNameLenOffset:], uint16(len(nameUTF16LE)))
	copy(buf[NKNameOffset:], nameUTF16LE)

	nk, err := DecodeNK(buf)
	if err != nil {
		t.Fatalf("DecodeNK: %v", err)
	}

	// Verify the name was stored correctly
	if len(nk.NameRaw) != len(nameUTF16LE) {
		t.Errorf("NameRaw length: expected %d, got %d", len(nameUTF16LE), len(nk.NameRaw))
	}

	// Verify the compressed flag is NOT set
	if nk.NameIsCompressed() {
		t.Error("expected NameIsCompressed to be false for UTF-16LE name")
	}

	// TODO: Once we fix the UTF-16LE decoding in reader.go, verify that
	// the decoded string is "abcd_äöüß", not mojibake like "abcd_����"
}

// TestDecodeNK_CompressedVsUTF16 tests the difference between compressed (ASCII)
// and UTF-16LE name encoding based on the flags field.
func TestDecodeNK_CompressedVsUTF16(t *testing.T) {
	tests := []struct {
		name       string
		flags      uint16
		compressed bool
	}{
		{"compressed (ASCII)", 0x0020, true},
		{"UTF-16LE", 0x0000, false},
		{"UTF-16LE with other flags", 0x0004, false}, // other flags, but not 0x0020
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nameData := []byte("TEST")
			buf := make([]byte, NKFixedHeaderSize+len(nameData))
			copy(buf, NKSignature)
			binary.LittleEndian.PutUint16(buf[NKFlagsOffset:], tc.flags)
			binary.LittleEndian.PutUint64(buf[NKLastWriteOffset:], 0)
			binary.LittleEndian.PutUint32(buf[NKAccessBitsOffset:], 0)
			binary.LittleEndian.PutUint32(buf[NKParentOffset:], 0xFFFFFFFF)
			binary.LittleEndian.PutUint32(buf[NKSubkeyCountOffset:], 0)
			binary.LittleEndian.PutUint32(buf[NKVolSubkeyCountOffset:], 0)
			binary.LittleEndian.PutUint32(buf[NKSubkeyListOffset:], 0xFFFFFFFF)
			binary.LittleEndian.PutUint32(buf[NKVolSubkeyListOffset:], 0xFFFFFFFF)
			binary.LittleEndian.PutUint32(buf[NKValueCountOffset:], 0)
			binary.LittleEndian.PutUint32(buf[NKValueListOffset:], 0xFFFFFFFF)
			binary.LittleEndian.PutUint16(buf[NKNameLenOffset:], uint16(len(nameData)))
			copy(buf[NKNameOffset:], nameData)

			nk, err := DecodeNK(buf)
			if err != nil {
				t.Fatalf("DecodeNK: %v", err)
			}

			if nk.NameIsCompressed() != tc.compressed {
				t.Errorf("NameIsCompressed: expected %v, got %v", tc.compressed, nk.NameIsCompressed())
			}
		})
	}
}
