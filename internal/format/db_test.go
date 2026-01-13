package format

import (
	"encoding/binary"
	"testing"
)

func TestDecodeDB(t *testing.T) {
	// Create a db record
	buf := make([]byte, DBMinSize)

	// Signature "db"
	copy(buf[DBSignatureOffset:], DBSignature)

	// Number of blocks: 3
	binary.LittleEndian.PutUint16(buf[DBCountOffset:], 3)

	// Blocklist offset: 0x1000
	binary.LittleEndian.PutUint32(buf[DBListOffset:], 0x1000)

	// Unknown1: 0
	binary.LittleEndian.PutUint32(buf[DBUnknown1Offset:], 0)

	db, err := DecodeDB(buf)
	if err != nil {
		t.Fatalf("DecodeDB failed: %v", err)
	}

	if db.NumBlocks != 3 {
		t.Errorf("NumBlocks: expected 3, got %d", db.NumBlocks)
	}

	if db.BlocklistOffset != 0x1000 {
		t.Errorf("ListOffset: expected 0x1000, got 0x%x", db.BlocklistOffset)
	}

	if db.Unknown1 != 0 {
		t.Errorf("Unknown1: expected 0, got 0x%x", db.Unknown1)
	}
}

func TestDecodeDB_Truncated(t *testing.T) {
	// Too short - only signature, missing other fields
	buf := append([]byte{}, DBSignature...)
	if _, err := DecodeDB(buf); err == nil {
		t.Error("Expected error for truncated db record")
	}
}

func TestDecodeDB_BadSignature(t *testing.T) {
	buf := make([]byte, 16)
	buf[0] = 'x'
	buf[1] = 'x'
	binary.LittleEndian.PutUint16(buf[DBCountOffset:], 1)

	if _, err := DecodeDB(buf); err == nil {
		t.Error("Expected error for bad signature")
	}
}

func TestDecodeDB_TruncatedRecord(t *testing.T) {
	// Buffer too small for full db record (needs DBMinSize bytes minimum)
	buf := make([]byte, 8)
	copy(buf[DBSignatureOffset:], DBSignature)
	binary.LittleEndian.PutUint16(buf[DBCountOffset:], 3)
	binary.LittleEndian.PutUint32(buf[DBListOffset:], 0x1000)
	// Missing unknown1 field

	if _, err := DecodeDB(buf); err == nil {
		t.Error("Expected error for truncated db record")
	}
}

func TestIsDBRecord(t *testing.T) {
	// Helper to create test data with signature + padding
	withPadding := func(sig []byte) []byte {
		return append(append([]byte{}, sig...), 0x00, 0x00)
	}

	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"valid db", withPadding(DBSignature), true},
		{"not db - vk", withPadding(VKSignature), false},
		{"too short", []byte{'d'}, false},
		{"empty", []byte{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsDBRecord(tc.data)
			if result != tc.expected {
				t.Errorf("IsDBRecord: expected %v, got %v", tc.expected, result)
			}
		})
	}
}
