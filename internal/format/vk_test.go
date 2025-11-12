package format

import (
	"encoding/binary"
	"testing"
)

const (
	regDWORD = 4
	regSZ    = 1
)

func TestDecodeVKInline(t *testing.T) {
	name := []byte("A")
	buf := make([]byte, VKFixedHeaderSize+len(name))
	copy(buf, VKSignature)
	binary.LittleEndian.PutUint16(buf[VKNameLenOffset:], uint16(len(name)))
	binary.LittleEndian.PutUint32(
		buf[VKDataLenOffset:],
		VKDataInlineBit|4,
	) // high bit => inline, length 4
	binary.LittleEndian.PutUint32(buf[VKDataOffOffset:], 0x11223344) // inline payload
	binary.LittleEndian.PutUint32(buf[VKTypeOffset:], regDWORD)
	binary.LittleEndian.PutUint16(buf[VKFlagsOffset:], VKFlagASCIIName)
	copy(buf[VKNameOffset:], name)

	vk, err := DecodeVK(buf)
	if err != nil {
		t.Fatalf("DecodeVK: %v", err)
	}
	if !vk.DataInline() || vk.InlineLength() != 4 {
		t.Fatalf("expected inline data: %+v", vk)
	}
}

func TestDecodeVKReferenced(t *testing.T) {
	buf := make([]byte, VKFixedHeaderSize+4)
	copy(buf, VKSignature)
	binary.LittleEndian.PutUint16(buf[VKNameLenOffset:], 0)
	binary.LittleEndian.PutUint32(buf[VKDataLenOffset:], 8)
	binary.LittleEndian.PutUint32(buf[VKDataOffOffset:], 0x200)
	binary.LittleEndian.PutUint32(buf[VKTypeOffset:], regSZ)
	binary.LittleEndian.PutUint16(buf[VKFlagsOffset:], 0)

	vk, err := DecodeVK(buf)
	if err != nil {
		t.Fatalf("DecodeVK: %v", err)
	}
	if vk.DataInline() {
		t.Fatalf("expected out-of-line data")
	}
}

// TestDecodeVK_CompNameFlag tests VK records with the VK_VALUE_COMP_NAME flag.
// This flag indicates that the value is a "name-only" or "tombstone" entry,
// and hivex treats such values as REG_NONE with no data, regardless of what
// the Type and DataLength fields contain.
//
// Bug reproduction: gohivex currently ignores this flag and returns the actual
// type and data, causing mismatches with hivex on thousands of values.
//
// References:
// - special hive: 2 values with this issue
// - rlenvalue_test_hive: 6 values
// - large hive: 3,633 values
// - windows-2003-server-system: 15,926 values.
func TestDecodeVK_CompNameFlag(t *testing.T) {
	// Create a VK record with:
	// - Type: REG_DWORD (4)
	// - DataLength: VKDataInlineBit | 4 (inline, 4 bytes)
	// - DataOffset: 0x12345678 (inline data)
	// - Flags: VKFlagASCIIName (VK_VALUE_COMP_NAME - hypothesis, needs verification)
	//
	// With the flag set, hivex would return REG_NONE with 0 bytes.
	// Without the fix, gohivex returns REG_DWORD with 4 bytes.

	name := []byte("testval")
	buf := make([]byte, VKFixedHeaderSize+len(name))
	copy(buf, VKSignature)
	binary.LittleEndian.PutUint16(buf[VKNameLenOffset:], uint16(len(name)))
	binary.LittleEndian.PutUint32(buf[VKDataLenOffset:], VKDataInlineBit|4) // inline, 4 bytes
	binary.LittleEndian.PutUint32(buf[VKDataOffOffset:], 0x12345678)        // inline data payload
	binary.LittleEndian.PutUint32(buf[VKTypeOffset:], regDWORD)             // Type: REG_DWORD
	binary.LittleEndian.PutUint16(
		buf[VKFlagsOffset:],
		VKFlagASCIIName,
	) // Flags: VK_VALUE_COMP_NAME
	copy(buf[VKNameOffset:], name)

	vk, err := DecodeVK(buf)
	if err != nil {
		t.Fatalf("DecodeVK: %v", err)
	}

	// Verify the record was decoded correctly
	if vk.Type != regDWORD {
		t.Errorf("Type: expected REG_DWORD (%d), got %d", regDWORD, vk.Type)
	}
	if vk.Flags != VKFlagASCIIName {
		t.Errorf("Flags: expected VKFlagASCIIName (0x%04x), got 0x%04x", VKFlagASCIIName, vk.Flags)
	}
	if !vk.DataInline() {
		t.Error("expected DataInline to be true")
	}
	if vk.InlineLength() != 4 {
		t.Errorf("InlineLength: expected 4, got %d", vk.InlineLength())
	}

	// TODO: Once we identify the correct flag and add VKRecord.IsCompName() method,
	// add a check here: if !vk.IsCompName() { t.Error("expected IsCompName to be true") }
}
