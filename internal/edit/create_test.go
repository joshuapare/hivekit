package edit

import (
	"encoding/binary"
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
)

// testWriter is a simple Writer implementation for testing that captures the written bytes.
type testWriter struct {
	data []byte
}

func (w *testWriter) WriteHive(buf []byte) error {
	w.data = make([]byte, len(buf))
	copy(w.data, buf)
	return nil
}

// TestCreateFromScratch tests creating a hive from scratch with nil reader.
func TestCreateFromScratch(t *testing.T) {
	// Create editor with nil reader
	ed := NewEditor(nil)
	tx := ed.Begin()

	// Create keys with CreateParents
	if err := tx.CreateKey("Software\\MyApp", types.CreateKeyOptions{CreateParents: true}); err != nil {
		t.Fatalf("CreateKey failed: %v", err)
	}

	// Set a value
	valueData := []byte("test")
	if err := tx.SetValue("Software\\MyApp", "TestValue", types.REG_SZ, valueData); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Commit and build the hive
	writer := &testWriter{}
	if err := tx.Commit(writer, types.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	data := writer.data
	if len(data) == 0 {
		t.Fatal("Built hive is empty")
	}

	// Verify REGF signature
	if string(data[0:4]) != "regf" {
		t.Errorf("Invalid signature: got %q, want %q", data[0:4], "regf")
	}

	t.Logf("Successfully created hive from scratch: %d bytes", len(data))
}

// TestCreateWithCreateParents tests creating keys with CreateParents flag.
func TestCreateWithCreateParents(t *testing.T) {
	ed := NewEditor(nil)
	tx := ed.Begin()

	// Create deep path with CreateParents
	if err := tx.CreateKey("Software\\Microsoft\\Windows\\CurrentVersion", types.CreateKeyOptions{CreateParents: true}); err != nil {
		t.Fatalf("CreateKey with CreateParents failed: %v", err)
	}

	// Should be able to set value on deep path
	if err := tx.SetValue("Software\\Microsoft\\Windows\\CurrentVersion", "Test", types.REG_DWORD, make([]byte, 4)); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	writer := &testWriter{}
	if err := tx.Commit(writer, types.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	data := writer.data
	if len(data) == 0 {
		t.Fatal("Built hive is empty")
	}

	t.Logf("Created hive with deep paths: %d bytes", len(data))
}

// TestCreateWithTypedValues tests creating different registry value types.
func TestCreateWithTypedValues(t *testing.T) {
	ed := NewEditor(nil)
	tx := ed.Begin()

	if err := tx.CreateKey("TestKey", types.CreateKeyOptions{}); err != nil {
		t.Fatalf("CreateKey failed: %v", err)
	}

	tests := []struct {
		name     string
		typ      types.RegType
		makeData func() []byte
	}{
		{
			name: "StringValue",
			typ:  types.REG_SZ,
			makeData: func() []byte {
				s := "TestString"
				data := make([]byte, len(s)*2)
				for i, r := range s {
					binary.LittleEndian.PutUint16(data[i*2:], uint16(r))
				}
				return data
			},
		},
		{
			name: "DwordValue",
			typ:  types.REG_DWORD,
			makeData: func() []byte {
				data := make([]byte, 4)
				binary.LittleEndian.PutUint32(data, 0x12345678)
				return data
			},
		},
		{
			name: "QwordValue",
			typ:  types.REG_QWORD,
			makeData: func() []byte {
				data := make([]byte, 8)
				binary.LittleEndian.PutUint64(data, 0x123456789ABCDEF0)
				return data
			},
		},
		{
			name: "MultiStringValue",
			typ:  types.REG_MULTI_SZ,
			makeData: func() []byte {
				s := "String1\x00String2\x00String3\x00"
				data := make([]byte, len(s)*2)
				for i, r := range s {
					binary.LittleEndian.PutUint16(data[i*2:], uint16(r))
				}
				return data
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.makeData()
			if err := tx.SetValue("TestKey", tt.name, tt.typ, data); err != nil {
				t.Errorf("SetValue(%s) failed: %v", tt.name, err)
			}
		})
	}

	writer := &testWriter{}
	if err := tx.Commit(writer, types.WriteOptions{}); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	data := writer.data
	if len(data) == 0 {
		t.Fatal("Built hive is empty")
	}

	t.Logf("Created hive with typed values: %d bytes", len(data))
}

// TestNewHiveValidation tests validation when creating from scratch.
func TestNewHiveValidation(t *testing.T) {
	ed := NewEditor(nil)
	tx := ed.Begin()

	// Should fail: setting value on non-existent key
	err := tx.SetValue("NonExistent", "Value", types.REG_SZ, []byte("test"))
	if err == nil {
		t.Error("SetValue on non-existent key should fail")
	}

	// Should fail: deleting non-existent key
	err = tx.DeleteKey("NonExistent", types.DeleteKeyOptions{})
	if err == nil {
		t.Error("DeleteKey on non-existent key should fail")
	}

	// Should succeed: creating and setting value
	if createErr := tx.CreateKey("TestKey", types.CreateKeyOptions{}); createErr != nil {
		t.Fatalf("CreateKey failed: %v", createErr)
	}

	if setErr := tx.SetValue("TestKey", "Value", types.REG_SZ, []byte("test")); setErr != nil {
		t.Errorf("SetValue after CreateKey failed: %v", setErr)
	}
}

// TestEmptyHive tests creating and committing a completely empty hive.
func TestEmptyHive(t *testing.T) {
	ed := NewEditor(nil)
	tx := ed.Begin()

	// Don't create any keys or values, just commit
	writer := &testWriter{}
	if err := tx.Commit(writer, types.WriteOptions{}); err != nil {
		t.Fatalf("Commit empty hive failed: %v", err)
	}

	data := writer.data
	if len(data) == 0 {
		t.Fatal("Built hive is empty")
	}

	// Should have valid REGF header even for empty hive
	if string(data[0:4]) != "regf" {
		t.Errorf("Invalid signature: got %q, want %q", data[0:4], "regf")
	}

	// Verify checksum is non-zero
	checksum := binary.LittleEndian.Uint32(data[0x1FC:0x200])
	if checksum == 0 {
		t.Error("Checksum is zero - checksum calculation may not be working")
	}

	t.Logf("Created empty hive: %d bytes, checksum=0x%08X", len(data), checksum)
}
