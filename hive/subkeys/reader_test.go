package subkeys

import (
	"errors"
	"testing"
)

// Test_readLFLH tests parsing LF/LH list payloads.
func Test_readLFLH(t *testing.T) {
	// Create a minimal LF list: signature (lf) + count (2) + 2 entries (8 bytes each)
	payload := make([]byte, 4+2*8)

	// Signature
	payload[0] = 'l'
	payload[1] = 'f'

	// Count = 2
	payload[2] = 2
	payload[3] = 0

	// Entry 0: NK offset = 0x1000, hash = 0x12345678
	payload[4] = 0x00
	payload[5] = 0x10
	payload[6] = 0x00
	payload[7] = 0x00
	payload[8] = 0x78
	payload[9] = 0x56
	payload[10] = 0x34
	payload[11] = 0x12

	// Entry 1: NK offset = 0x2000, hash = 0xABCDEF00
	payload[12] = 0x00
	payload[13] = 0x20
	payload[14] = 0x00
	payload[15] = 0x00
	payload[16] = 0x00
	payload[17] = 0xEF
	payload[18] = 0xCD
	payload[19] = 0xAB

	refs, err := readLFLH(payload, 2)
	if err != nil {
		t.Fatalf("readLFLH failed: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("Expected 2 refs, got %d", len(refs))
	}

	if refs[0] != 0x1000 {
		t.Errorf("Expected refs[0] = 0x1000, got 0x%X", refs[0])
	}

	if refs[1] != 0x2000 {
		t.Errorf("Expected refs[1] = 0x2000, got 0x%X", refs[1])
	}
}

// Test_readLFLH_Truncated tests error handling for truncated data.
func Test_readLFLH_Truncated(t *testing.T) {
	// Create truncated LF list (only header, no entries)
	payload := make([]byte, 4)
	payload[0] = 'l'
	payload[1] = 'f'
	payload[2] = 2 // Claim 2 entries
	payload[3] = 0

	_, err := readLFLH(payload, 2)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

// Test_readLI tests parsing LI list payloads.
func Test_readLI(t *testing.T) {
	// Create a minimal LI list: signature (li) + count (3) + 3 entries (4 bytes each)
	payload := make([]byte, 4+3*4)

	// Signature
	payload[0] = 'l'
	payload[1] = 'i'

	// Count = 3
	payload[2] = 3
	payload[3] = 0

	// Entry 0: 0x1000
	payload[4] = 0x00
	payload[5] = 0x10
	payload[6] = 0x00
	payload[7] = 0x00

	// Entry 1: 0x2000
	payload[8] = 0x00
	payload[9] = 0x20
	payload[10] = 0x00
	payload[11] = 0x00

	// Entry 2: 0x3000
	payload[12] = 0x00
	payload[13] = 0x30
	payload[14] = 0x00
	payload[15] = 0x00

	refs, err := readLI(payload, 3)
	if err != nil {
		t.Fatalf("readLI failed: %v", err)
	}

	if len(refs) != 3 {
		t.Fatalf("Expected 3 refs, got %d", len(refs))
	}

	expected := []uint32{0x1000, 0x2000, 0x3000}
	for i, exp := range expected {
		if refs[i] != exp {
			t.Errorf("refs[%d] = 0x%X, want 0x%X", i, refs[i], exp)
		}
	}
}

// Test_readLI_Truncated tests error handling.
func Test_readLI_Truncated(t *testing.T) {
	payload := make([]byte, 4) // Header only
	payload[0] = 'l'
	payload[1] = 'i'
	payload[2] = 5 // Claim 5 entries
	payload[3] = 0

	_, err := readLI(payload, 5)
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

// Test_isASCII tests ASCII detection.
func Test_isASCII(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"empty", []byte{}, true},
		{"pure ASCII", []byte("hello"), true},
		{"ASCII with numbers", []byte("test123"), true},
		{"extended char", []byte{0xFF}, false},
		{"mixed", []byte("hello\x80world"), false},
		{"boundary", []byte{0x7F}, true},
		{"over boundary", []byte{0x80}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isASCII(tt.data)
			if result != tt.expected {
				t.Errorf("isASCII(%v) = %v, want %v", tt.data, result, tt.expected)
			}
		})
	}
}

// Test_decodeCompressedName tests ASCII/Windows-1252 name decoding.
func Test_decodeCompressedName(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
		wantErr  bool
	}{
		{"pure ASCII", []byte("TestKey"), "TestKey", false},
		{"empty", []byte{}, "", false},
		{"with space", []byte("My Key"), "My Key", false},
		{"ASCII numbers", []byte("key123"), "key123", false},
		{"Windows-1252 extended", []byte{0x41, 0xE9, 0x42}, "A\u00e9B", false}, // A, é, B in Windows-1252
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeCompressedName(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeCompressedName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if result != tt.expected {
				t.Errorf("decodeCompressedName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test_decodeUTF16LEName tests UTF-16LE name decoding.
func Test_decodeUTF16LEName(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
		wantErr  bool
	}{
		{
			"simple ASCII",
			[]byte{
				'T', 0x00, 'e', 0x00, 's', 0x00, 't', 0x00,
			},
			"Test",
			false,
		},
		{
			"empty",
			[]byte{},
			"",
			false,
		},
		{
			"odd length",
			[]byte{'T', 0x00, 'e'},
			"",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeUTF16LEName(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeUTF16LEName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result != tt.expected {
				t.Errorf("decodeUTF16LEName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test_resolveCell tests cell resolution logic.
func Test_resolveCell_MockData(t *testing.T) {
	// This test uses mock hive data structure
	// Skip for now - requires full hive mock
	t.Skip("Requires hive mock - will be tested in integration")
}

// Test_readDirectList tests the direct list reading logic.
func Test_readDirectList_MockData(t *testing.T) {
	// This test requires a full Hive instance
	// Skip for now - will be tested in integration
	t.Skip("Requires hive instance - will be tested in integration")
}
