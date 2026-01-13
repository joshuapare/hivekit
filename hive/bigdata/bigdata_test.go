package bigdata

import (
	"bytes"
	"errors"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/testutil"
)

// noopDT is a no-op dirty tracker for tests.
type noopDT struct{}

func (noopDT) Add(_, _ int) {}

// Test_WriteDBHeader tests writing DB headers.
func Test_WriteDBHeader(t *testing.T) {
	tests := []struct {
		name         string
		count        uint16
		blocklistRef uint32
	}{
		{"single block", 1, 0x1000},
		{"two blocks", 2, 0x2000},
		{"many blocks", 100, 0x5000},
		{"max count", 65535, 0xFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, DBHeaderSize)

			err := WriteDBHeader(buf, tt.count, tt.blocklistRef)
			if err != nil {
				t.Fatalf("WriteDBHeader failed: %v", err)
			}

			// Verify signature
			if buf[0] != 'd' || buf[1] != 'b' {
				t.Errorf("Invalid signature: %c%c", buf[0], buf[1])
			}

			// Verify count
			count := uint16(buf[2]) | uint16(buf[3])<<8
			if count != tt.count {
				t.Errorf("Count = %d, want %d", count, tt.count)
			}

			// Verify blocklist ref
			ref := uint32(buf[4]) |
				uint32(buf[5])<<8 |
				uint32(buf[6])<<16 |
				uint32(buf[7])<<24
			if ref != tt.blocklistRef {
				t.Errorf("Blocklist ref = 0x%X, want 0x%X", ref, tt.blocklistRef)
			}

			// Verify reserved is zero
			reserved := uint32(buf[8]) |
				uint32(buf[9])<<8 |
				uint32(buf[10])<<16 |
				uint32(buf[11])<<24
			if reserved != 0 {
				t.Errorf("Reserved = 0x%X, want 0", reserved)
			}
		})
	}
}

// Test_WriteDBHeader_Truncated tests error handling.
func Test_WriteDBHeader_Truncated(t *testing.T) {
	buf := make([]byte, 10) // Too small

	err := WriteDBHeader(buf, 1, 0x1000)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

// Test_ReadDBHeader tests reading DB headers.
func Test_ReadDBHeader(t *testing.T) {
	// Create a valid DB header
	buf := make([]byte, DBHeaderSize)
	buf[0] = 'd'
	buf[1] = 'b'
	buf[2] = 5 // count = 5
	buf[3] = 0
	buf[4] = 0x00 // blocklist = 0x2000
	buf[5] = 0x20
	buf[6] = 0x00
	buf[7] = 0x00

	header, err := ReadDBHeader(buf)
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	if header.Count != 5 {
		t.Errorf("Count = %d, want 5", header.Count)
	}

	if header.Blocklist != 0x2000 {
		t.Errorf("Blocklist = 0x%X, want 0x2000", header.Blocklist)
	}

	if header.Reserved != 0 {
		t.Errorf("Reserved = 0x%X, want 0", header.Reserved)
	}
}

// Test_ReadDBHeader_InvalidSignature tests error handling.
func Test_ReadDBHeader_InvalidSignature(t *testing.T) {
	buf := make([]byte, DBHeaderSize)
	buf[0] = 'x'
	buf[1] = 'x'

	_, err := ReadDBHeader(buf)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Expected ErrInvalidSignature, got %v", err)
	}
}

// Test_ReadDBHeader_Truncated tests error handling.
func Test_ReadDBHeader_Truncated(t *testing.T) {
	buf := make([]byte, 10) // Too small

	_, err := ReadDBHeader(buf)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

// Test_WriteBlocklist tests writing blocklists.
func Test_WriteBlocklist(t *testing.T) {
	refs := []uint32{0x1000, 0x2000, 0x3000, 0x4000}
	buf := make([]byte, len(refs)*4)

	err := WriteBlocklist(buf, refs)
	if err != nil {
		t.Fatalf("WriteBlocklist failed: %v", err)
	}

	// Verify each reference
	for i, expectedRef := range refs {
		offset := i * 4
		ref := uint32(buf[offset]) |
			uint32(buf[offset+1])<<8 |
			uint32(buf[offset+2])<<16 |
			uint32(buf[offset+3])<<24

		if ref != expectedRef {
			t.Errorf("refs[%d] = 0x%X, want 0x%X", i, ref, expectedRef)
		}
	}
}

// Test_WriteBlocklist_Truncated tests error handling.
func Test_WriteBlocklist_Truncated(t *testing.T) {
	refs := []uint32{0x1000, 0x2000}
	buf := make([]byte, 4) // Too small for 2 refs

	err := WriteBlocklist(buf, refs)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

// Test_ReadBlocklist tests reading blocklists.
func Test_ReadBlocklist(t *testing.T) {
	// Create a blocklist with 3 refs
	buf := make([]byte, 12)
	// Ref 0: 0x1000
	buf[0] = 0x00
	buf[1] = 0x10
	buf[2] = 0x00
	buf[3] = 0x00
	// Ref 1: 0x2000
	buf[4] = 0x00
	buf[5] = 0x20
	buf[6] = 0x00
	buf[7] = 0x00
	// Ref 2: 0x3000
	buf[8] = 0x00
	buf[9] = 0x30
	buf[10] = 0x00
	buf[11] = 0x00

	refs, err := ReadBlocklist(buf, 3)
	if err != nil {
		t.Fatalf("ReadBlocklist failed: %v", err)
	}

	expected := []uint32{0x1000, 0x2000, 0x3000}
	for i, exp := range expected {
		if refs[i] != exp {
			t.Errorf("refs[%d] = 0x%X, want 0x%X", i, refs[i], exp)
		}
	}
}

// Test_ReadBlocklist_Truncated tests error handling.
func Test_ReadBlocklist_Truncated(t *testing.T) {
	buf := make([]byte, 4) // Only 1 ref

	_, err := ReadBlocklist(buf, 3) // Claim 3 refs
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

// Test_Roundtrip_DBHeader tests write and read roundtrip.
func Test_Roundtrip_DBHeader(t *testing.T) {
	buf := make([]byte, DBHeaderSize)

	// Write
	err := WriteDBHeader(buf, 42, 0xABCD)
	if err != nil {
		t.Fatalf("WriteDBHeader failed: %v", err)
	}

	// Read
	header, err := ReadDBHeader(buf)
	if err != nil {
		t.Fatalf("ReadDBHeader failed: %v", err)
	}

	// Verify
	if header.Count != 42 {
		t.Errorf("Count = %d, want 42", header.Count)
	}

	if header.Blocklist != 0xABCD {
		t.Errorf("Blocklist = 0x%X, want 0xABCD", header.Blocklist)
	}

	if header.Reserved != 0 {
		t.Errorf("Reserved should be 0, got 0x%X", header.Reserved)
	}
}

// Test_Roundtrip_Blocklist tests write and read roundtrip.
func Test_Roundtrip_Blocklist(t *testing.T) {
	original := []uint32{0x1000, 0x2000, 0x3000, 0x4000, 0x5000}
	buf := make([]byte, len(original)*4)

	// Write
	err := WriteBlocklist(buf, original)
	if err != nil {
		t.Fatalf("WriteBlocklist failed: %v", err)
	}

	// Read
	refs, err := ReadBlocklist(buf, uint16(len(original)))
	if err != nil {
		t.Fatalf("ReadBlocklist failed: %v", err)
	}

	// Verify
	if len(refs) != len(original) {
		t.Fatalf("Length mismatch: got %d, want %d", len(refs), len(original))
	}

	for i, exp := range original {
		if refs[i] != exp {
			t.Errorf("refs[%d] = 0x%X, want 0x%X", i, refs[i], exp)
		}
	}
}

// Test_ChunkCalculation tests chunking logic.
func Test_ChunkCalculation(t *testing.T) {
	tests := []struct {
		name           string
		dataSize       int
		expectedBlocks int
	}{
		{"single byte", 1, 1},
		{"max block size", MaxBlockSize, 1},
		{"max block + 1", MaxBlockSize + 1, 2},
		{"two full blocks", MaxBlockSize * 2, 2},
		{"two blocks + partial", MaxBlockSize*2 + 100, 3},
		{"large data", MaxBlockSize * 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			numBlocks := (tt.dataSize + MaxBlockSize - 1) / MaxBlockSize
			if numBlocks != tt.expectedBlocks {
				t.Errorf("Calculated %d blocks, want %d", numBlocks, tt.expectedBlocks)
			}
		})
	}
}

// Test_MaxBlockSize validates the constant.
func Test_MaxBlockSize(t *testing.T) {
	if MaxBlockSize != 16344 {
		t.Errorf("MaxBlockSize = %d, want 16344", MaxBlockSize)
	}
}

// Test_DBHeaderSize validates the constant.
func Test_DBHeaderSize(t *testing.T) {
	if DBHeaderSize != 12 {
		t.Errorf("DBHeaderSize = %d, want 12", DBHeaderSize)
	}
}

// Test_EmptyBlocklist tests handling of empty blocklists.
func Test_EmptyBlocklist(t *testing.T) {
	buf := make([]byte, 0)

	// Write empty blocklist
	err := WriteBlocklist(buf, []uint32{})
	if err != nil {
		t.Errorf("Empty blocklist should succeed, got %v", err)
	}

	// Read empty blocklist
	refs, err := ReadBlocklist(buf, 0)
	if err != nil {
		t.Errorf("Reading empty blocklist should succeed, got %v", err)
	}

	if len(refs) != 0 {
		t.Errorf("Expected 0 refs, got %d", len(refs))
	}
}

// Test_LargeBlocklist tests handling of large blocklists.
func Test_LargeBlocklist(t *testing.T) {
	// Create a large blocklist (1000 blocks)
	refs := make([]uint32, 1000)
	for i := range refs {
		refs[i] = uint32(0x1000 + i*0x100)
	}

	buf := make([]byte, len(refs)*4)

	// Write
	err := WriteBlocklist(buf, refs)
	if err != nil {
		t.Fatalf("WriteBlocklist failed: %v", err)
	}

	// Read
	readRefs, err := ReadBlocklist(buf, uint16(len(refs)))
	if err != nil {
		t.Fatalf("ReadBlocklist failed: %v", err)
	}

	// Verify
	if !bytes.Equal(refsToBytes(refs), refsToBytes(readRefs)) {
		t.Error("Blocklist roundtrip mismatch")
	}
}

// Helper to convert refs to bytes for comparison.
func refsToBytes(refs []uint32) []byte {
	buf := make([]byte, len(refs)*4)
	for i, ref := range refs {
		offset := i * 4
		buf[offset] = byte(ref)
		buf[offset+1] = byte(ref >> 8)
		buf[offset+2] = byte(ref >> 16)
		buf[offset+3] = byte(ref >> 24)
	}
	return buf
}

// setupTestHive creates a minimal test hive with an allocator for testing writer functions.
func setupTestHive(t *testing.T) (*hive.Hive, *alloc.FastAllocator, func()) {
	return testutil.SetupTestHiveWithAllocator(t)
}

// Test_Writer_Store tests the Writer.Store function with various data sizes.
func Test_Writer_Store(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	writer := NewWriter(h, allocator, noopDT{})

	tests := []struct {
		name           string
		dataSize       int
		expectedBlocks int // Approximate expected block count
	}{
		{"17KB - min big-data", 17 * 1024, 2},
		{"20KB", 20 * 1024, 3},
		{"50KB", 50 * 1024, 7},
		{"100KB", 100 * 1024, 13},
		{"200KB", 200 * 1024, 26},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			testData := make([]byte, tt.dataSize)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Store the data
			dbRef, err := writer.Store(testData)
			if err != nil {
				t.Fatalf("Store() error = %v", err)
			}

			if dbRef == 0 || dbRef == 0xFFFFFFFF {
				t.Errorf("Store() returned invalid ref: 0x%X", dbRef)
			}

			// Verify we can read back the DB header
			dbPayload, err := h.ResolveCellPayload(dbRef)
			if err != nil {
				t.Fatalf("Failed to resolve DB cell: %v", err)
			}

			header, err := ReadDBHeader(dbPayload)
			if err != nil {
				t.Fatalf("Failed to read DB header: %v", err)
			}

			// Verify signature
			if string(header.Signature[:]) != "db" {
				t.Errorf("DB signature = %s, want 'db'", string(header.Signature[:]))
			}

			// Verify block count is reasonable
			if header.Count < 2 {
				t.Errorf("Block count too low: %d", header.Count)
			}

			// Verify blocklist reference
			if header.Blocklist == 0 || header.Blocklist == 0xFFFFFFFF {
				t.Errorf("Invalid blocklist ref: 0x%X", header.Blocklist)
			}
		})
	}
}

// Test_Writer_Store_EdgeCases tests edge cases for Writer.Store.
func Test_Writer_Store_EdgeCases(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	writer := NewWriter(h, allocator, noopDT{})

	// Test with exact MaxBlockSize
	t.Run("exact_maxblocksize", func(t *testing.T) {
		testData := make([]byte, MaxBlockSize)
		for i := range testData {
			testData[i] = 0xAA
		}

		dbRef, err := writer.Store(testData)
		if err != nil {
			t.Fatalf("Store() error = %v", err)
		}

		if dbRef == 0 {
			t.Error("Store() returned zero ref")
		}
	})

	// Test with MaxBlockSize + 1
	t.Run("maxblocksize_plus_one", func(t *testing.T) {
		testData := make([]byte, MaxBlockSize+1)
		for i := range testData {
			testData[i] = 0xBB
		}

		dbRef, err := writer.Store(testData)
		if err != nil {
			t.Fatalf("Store() error = %v", err)
		}

		if dbRef == 0 {
			t.Error("Store() returned zero ref")
		}

		// Should create 2 blocks
		dbPayload, _ := h.ResolveCellPayload(dbRef)
		header, _ := ReadDBHeader(dbPayload)

		if header.Count < 2 {
			t.Errorf(
				"Expected at least 2 blocks for %d bytes, got %d",
				MaxBlockSize+1,
				header.Count,
			)
		}
	})
}
