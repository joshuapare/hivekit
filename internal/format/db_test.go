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
	binary.LittleEndian.PutUint16(buf[DBNumBlocksOffset:], 3)

	// Blocklist offset: 0x1000
	binary.LittleEndian.PutUint32(buf[DBBlocklistOffset:], 0x1000)

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
		t.Errorf("BlocklistOffset: expected 0x1000, got 0x%x", db.BlocklistOffset)
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
	binary.LittleEndian.PutUint16(buf[DBNumBlocksOffset:], 1)

	if _, err := DecodeDB(buf); err == nil {
		t.Error("Expected error for bad signature")
	}
}

func TestDecodeDB_TruncatedRecord(t *testing.T) {
	// Buffer too small for full db record (needs DBMinSize bytes minimum)
	buf := make([]byte, 8)
	copy(buf[DBSignatureOffset:], DBSignature)
	binary.LittleEndian.PutUint16(buf[DBNumBlocksOffset:], 3)
	binary.LittleEndian.PutUint32(buf[DBBlocklistOffset:], 0x1000)
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

func TestEncodeDB(t *testing.T) {
	numBlocks := uint16(5)
	blocklistOffset := uint32(0x2000)

	buf := EncodeDB(numBlocks, blocklistOffset)

	if len(buf) != DBMinSize {
		t.Fatalf("EncodeDB: expected %d bytes, got %d", DBMinSize, len(buf))
	}

	// Verify signature
	if buf[0] != 'd' || buf[1] != 'b' {
		t.Errorf("EncodeDB: invalid signature, got %c%c", buf[0], buf[1])
	}

	// Verify numBlocks
	gotNumBlocks := binary.LittleEndian.Uint16(buf[DBNumBlocksOffset:])
	if gotNumBlocks != numBlocks {
		t.Errorf("EncodeDB: numBlocks expected %d, got %d", numBlocks, gotNumBlocks)
	}

	// Verify blocklistOffset
	gotBlocklistOffset := binary.LittleEndian.Uint32(buf[DBBlocklistOffset:])
	if gotBlocklistOffset != blocklistOffset {
		t.Errorf("EncodeDB: blocklistOffset expected 0x%x, got 0x%x", blocklistOffset, gotBlocklistOffset)
	}

	// Verify unknown1 is zero
	gotUnknown1 := binary.LittleEndian.Uint32(buf[DBUnknown1Offset:])
	if gotUnknown1 != 0 {
		t.Errorf("EncodeDB: unknown1 expected 0, got 0x%x", gotUnknown1)
	}

	// Round-trip test: encode then decode
	decoded, err := DecodeDB(buf)
	if err != nil {
		t.Fatalf("DecodeDB failed on EncodeDB output: %v", err)
	}

	if decoded.NumBlocks != numBlocks {
		t.Errorf("Round-trip numBlocks: expected %d, got %d", numBlocks, decoded.NumBlocks)
	}

	if decoded.BlocklistOffset != blocklistOffset {
		t.Errorf("Round-trip blocklistOffset: expected 0x%x, got 0x%x", blocklistOffset, decoded.BlocklistOffset)
	}
}

func TestCalculateDBBlocks(t *testing.T) {
	tests := []struct {
		name              string
		dataLen           int
		expectedNumBlocks int
		expectedLastSize  int
	}{
		{"empty", 0, 0, 0},
		{"small (1 block)", 1000, 1, 1000},
		{"exactly 1 block", DBBlockMaxSize, 1, DBBlockMaxSize},
		{"slightly over 1 block", DBBlockMaxSize + 1, 2, 1},
		{"20KB (2 blocks)", 20480, 2, 20480 - DBBlockMaxSize},
		{"50KB (4 blocks)", 51200, 4, 51200 - 3*DBBlockMaxSize},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			numBlocks, blockSizes := CalculateDBBlocks(tc.dataLen)

			if numBlocks != tc.expectedNumBlocks {
				t.Errorf("numBlocks: expected %d, got %d", tc.expectedNumBlocks, numBlocks)
			}

			if tc.dataLen == 0 {
				if blockSizes != nil {
					t.Errorf("blockSizes should be nil for empty data")
				}
				return
			}

			if len(blockSizes) != numBlocks {
				t.Errorf("blockSizes length: expected %d, got %d", numBlocks, len(blockSizes))
			}

			// Verify all blocks except last are max size
			for i := 0; i < numBlocks-1; i++ {
				if blockSizes[i] != DBBlockMaxSize {
					t.Errorf("block %d size: expected %d, got %d", i, DBBlockMaxSize, blockSizes[i])
				}
			}

			// Verify last block size
			if blockSizes[numBlocks-1] != tc.expectedLastSize {
				t.Errorf("last block size: expected %d, got %d", tc.expectedLastSize, blockSizes[numBlocks-1])
			}

			// Verify total size
			totalSize := 0
			for _, size := range blockSizes {
				totalSize += size
			}
			if totalSize != tc.dataLen {
				t.Errorf("total size: expected %d, got %d", tc.dataLen, totalSize)
			}
		})
	}
}
