package write

import (
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

func TestWriteNK_ValidPayload(t *testing.T) {
	name := "TestKey"
	payloadSize := NKPayloadSize(name)
	buf := make([]byte, payloadSize)

	parentRef := uint32(0x1000)
	skRef := uint32(0x2000)

	WriteNK(buf, name, parentRef, skRef)

	// Verify signature.
	if buf[0] != 'n' || buf[1] != 'k' {
		t.Errorf("bad signature: %c%c", buf[0], buf[1])
	}

	// Verify it parses correctly.
	nk, err := hive.ParseNK(buf)
	if err != nil {
		t.Fatalf("ParseNK failed: %v", err)
	}

	// Verify flags: compressed name.
	if nk.Flags()&format.NKFlagCompressedName == 0 {
		t.Error("expected compressed name flag")
	}

	// Verify parent.
	if nk.ParentOffsetRel() != parentRef {
		t.Errorf("parent ref: got 0x%X, want 0x%X", nk.ParentOffsetRel(), parentRef)
	}

	// Verify SK ref.
	if nk.SecurityOffsetRel() != skRef {
		t.Errorf("SK ref: got 0x%X, want 0x%X", nk.SecurityOffsetRel(), skRef)
	}

	// Verify subkey count is 0.
	if nk.SubkeyCount() != 0 {
		t.Errorf("subkey count: got %d, want 0", nk.SubkeyCount())
	}

	// Verify value count is 0.
	if nk.ValueCount() != 0 {
		t.Errorf("value count: got %d, want 0", nk.ValueCount())
	}

	// Verify subkey list is InvalidOffset.
	if nk.SubkeyListOffsetRel() != format.InvalidOffset {
		t.Errorf("subkey list: got 0x%X, want 0x%X", nk.SubkeyListOffsetRel(), format.InvalidOffset)
	}

	// Verify value list is InvalidOffset.
	if nk.ValueListOffsetRel() != format.InvalidOffset {
		t.Errorf("value list: got 0x%X, want 0x%X", nk.ValueListOffsetRel(), format.InvalidOffset)
	}

	// Verify name.
	nameBytes := nk.Name()
	if string(nameBytes) != name {
		t.Errorf("name: got %q, want %q", string(nameBytes), name)
	}

	// Verify name length.
	if nk.NameLength() != uint16(len(name)) {
		t.Errorf("name length: got %d, want %d", nk.NameLength(), len(name))
	}
}

func TestWriteNKWithCounts(t *testing.T) {
	name := "CountKey"
	payloadSize := NKPayloadSize(name)
	buf := make([]byte, payloadSize)

	WriteNKWithCounts(buf, name, 0x100, 0x200, 5, 0x300, 3, 0x400)

	nk, err := hive.ParseNK(buf)
	if err != nil {
		t.Fatalf("ParseNK failed: %v", err)
	}

	if nk.SubkeyCount() != 5 {
		t.Errorf("subkey count: got %d, want 5", nk.SubkeyCount())
	}
	if nk.SubkeyListOffsetRel() != 0x300 {
		t.Errorf("subkey list: got 0x%X, want 0x300", nk.SubkeyListOffsetRel())
	}
	if nk.ValueCount() != 3 {
		t.Errorf("value count: got %d, want 3", nk.ValueCount())
	}
	if nk.ValueListOffsetRel() != 0x400 {
		t.Errorf("value list: got 0x%X, want 0x400", nk.ValueListOffsetRel())
	}
}

func TestWriteVK_InlineData(t *testing.T) {
	name := "TestVal"
	data := []byte{0x01, 0x02, 0x03} // 3 bytes: fits inline

	payloadSize := VKPayloadSize(name)
	buf := make([]byte, payloadSize)

	WriteVK(buf, name, format.REGDWORD, data, 0)

	// Verify signature.
	if buf[0] != 'v' || buf[1] != 'k' {
		t.Errorf("bad signature: %c%c", buf[0], buf[1])
	}

	// Verify it parses.
	vk, err := hive.ParseVK(buf)
	if err != nil {
		t.Fatalf("ParseVK failed: %v", err)
	}

	// Verify type.
	if vk.Type() != format.REGDWORD {
		t.Errorf("type: got %d, want %d", vk.Type(), format.REGDWORD)
	}

	// Verify name.
	vkName := vk.Name()
	if string(vkName) != name {
		t.Errorf("name: got %q, want %q", string(vkName), name)
	}

	// Verify small data flag is set.
	if !vk.IsSmallData() {
		t.Error("expected inline/small data")
	}

	// Verify data length (masked).
	if vk.DataLen() != 3 {
		t.Errorf("data len: got %d, want 3", vk.DataLen())
	}
}

func TestWriteVK_ExternalData(t *testing.T) {
	name := "BigVal"
	data := []byte("Hello, this is some longer data that exceeds DWORD")

	payloadSize := VKPayloadSize(name)
	buf := make([]byte, payloadSize)
	dataRef := uint32(0x5000)

	WriteVK(buf, name, format.REGSZ, data, dataRef)

	vk, err := hive.ParseVK(buf)
	if err != nil {
		t.Fatalf("ParseVK failed: %v", err)
	}

	// Should NOT be small data.
	if vk.IsSmallData() {
		t.Error("expected external data, got inline")
	}

	// Data length.
	if vk.DataLen() != len(data) {
		t.Errorf("data len: got %d, want %d", vk.DataLen(), len(data))
	}

	// Data offset should point to the external cell.
	if vk.DataOffsetRel() != dataRef {
		t.Errorf("data offset: got 0x%X, want 0x%X", vk.DataOffsetRel(), dataRef)
	}
}

func TestWriteLHList_Roundtrip(t *testing.T) {
	entries := []subkeys.RawEntry{
		{NKRef: 100, Hash: 0xABCD1234},
		{NKRef: 200, Hash: 0xDEADBEEF},
		{NKRef: 300, Hash: 0xCAFEBABE},
	}

	payloadSize := LHListPayloadSize(len(entries))
	buf := make([]byte, payloadSize)

	WriteLHList(buf, entries)

	// Verify signature.
	if buf[0] != 'l' || buf[1] != 'h' {
		t.Errorf("bad signature: %c%c", buf[0], buf[1])
	}

	// Verify count.
	count := format.ReadU16(buf, 2)
	if count != 3 {
		t.Errorf("count: got %d, want 3", count)
	}

	// Verify each entry.
	for i, expected := range entries {
		off := format.ListHeaderSize + i*format.QWORDSize
		nkRef := format.ReadU32(buf, off)
		hash := format.ReadU32(buf, off+4)
		if nkRef != expected.NKRef {
			t.Errorf("entry[%d] NKRef: got 0x%X, want 0x%X", i, nkRef, expected.NKRef)
		}
		if hash != expected.Hash {
			t.Errorf("entry[%d] Hash: got 0x%X, want 0x%X", i, hash, expected.Hash)
		}
	}
}

func TestWriteValueList_Roundtrip(t *testing.T) {
	refs := []uint32{0x100, 0x200, 0x300, 0x400}

	payloadSize := ValueListPayloadSize(len(refs))
	buf := make([]byte, payloadSize)

	WriteValueList(buf, refs)

	// Read back.
	for i, expected := range refs {
		got := format.ReadU32(buf, i*format.DWORDSize)
		if got != expected {
			t.Errorf("ref[%d]: got 0x%X, want 0x%X", i, got, expected)
		}
	}
}

func TestWriteDataCell(t *testing.T) {
	data := []byte("registry value data payload")
	buf := make([]byte, len(data))

	WriteDataCell(buf, data)

	for i := range data {
		if buf[i] != data[i] {
			t.Errorf("byte[%d]: got 0x%02X, want 0x%02X", i, buf[i], data[i])
		}
	}
}

func TestNKPayloadSize(t *testing.T) {
	got := NKPayloadSize("Software")
	want := format.NKFixedHeaderSize + len("Software")
	if got != want {
		t.Errorf("NKPayloadSize: got %d, want %d", got, want)
	}
}

func TestVKPayloadSize(t *testing.T) {
	got := VKPayloadSize("Version")
	want := format.VKFixedHeaderSize + len("Version")
	if got != want {
		t.Errorf("VKPayloadSize: got %d, want %d", got, want)
	}
}
