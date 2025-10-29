package reader

import (
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

func TestDecodeKeyName(t *testing.T) {
	tests := []struct {
		name         string
		nameRaw      []byte
		flags        uint16 // 0x20 = compressed (Windows-1252), otherwise UTF-16LE
		expectedName string
		expectError  bool
	}{
		{
			name:         "Empty name",
			nameRaw:      []byte{},
			flags:        0x20,
			expectedName: "",
			expectError:  false,
		},
		{
			name:         "ASCII only (compressed)",
			nameRaw:      []byte("TestKey"),
			flags:        0x20,
			expectedName: "TestKey",
			expectError:  false,
		},
		{
			name:         "German umlauts (Windows-1252 compressed)",
			nameRaw:      []byte{0x61, 0x62, 0x63, 0x64, 0x5f, 0xe4, 0xf6, 0xfc, 0xdf}, // abcd_äöüß in Windows-1252
			flags:        0x20,
			expectedName: "abcd_äöüß", // Should be converted to UTF-8
			expectError:  false,
		},
		{
			name:         "Spanish accents (Windows-1252 compressed)",
			nameRaw:      []byte{0xe1, 0xe9, 0xed, 0xf3, 0xfa, 0xf1}, // áéíóúñ in Windows-1252
			flags:        0x20,
			expectedName: "áéíóúñ", // Should be converted to UTF-8
			expectError:  false,
		},
		{
			name:         "French accents (Windows-1252 compressed)",
			nameRaw:      []byte{0xe0, 0xe8, 0xe9, 0xea, 0xeb, 0xe7}, // àèéêëç in Windows-1252
			flags:        0x20,
			expectedName: "àèéêëç", // Should be converted to UTF-8
			expectError:  false,
		},
		{
			name:         "Trademark symbol (Windows-1252)",
			nameRaw:      []byte{0x77, 0x65, 0x69, 0x72, 0x64, 0x99}, // weird™ in Windows-1252
			flags:        0x20,
			expectedName: "weird™", // ™ = 0x99 in Windows-1252, should convert to UTF-8
			expectError:  false,
		},
		{
			name:         "Euro sign (Windows-1252)",
			nameRaw:      []byte{0x70, 0x72, 0x69, 0x63, 0x65, 0x80}, // price€ in Windows-1252
			flags:        0x20,
			expectedName: "price€", // € = 0x80 in Windows-1252
			expectError:  false,
		},
		{
			name:         "ASCII only (UTF-16LE uncompressed)",
			nameRaw:      []byte{0x54, 0x00, 0x65, 0x00, 0x73, 0x00, 0x74, 0x00}, // "Test" in UTF-16LE
			flags:        0x00,
			expectedName: "Test",
			expectError:  false,
		},
		{
			name:         "Unicode emoji (UTF-16LE uncompressed)",
			nameRaw:      []byte{0x3d, 0xd8, 0x00, 0xde}, // 😀 in UTF-16LE
			flags:        0x00,
			expectedName: "😀",
			expectError:  false,
		},
		{
			name:         "Mixed ASCII and unicode (UTF-16LE)",
			nameRaw:      []byte{0x48, 0x00, 0x69, 0x00, 0x3d, 0xd8, 0x00, 0xde}, // "Hi😀" in UTF-16LE
			flags:        0x00,
			expectedName: "Hi😀",
			expectError:  false,
		},
		{
			name:        "Invalid UTF-16LE (odd length)",
			nameRaw:     []byte{0x54, 0x00, 0x65},
			flags:       0x00,
			expectError: true,
		},
		{
			name:         "Null bytes in name (compressed)",
			nameRaw:      []byte{0x7a, 0x65, 0x72, 0x6f, 0x00, 0x6b, 0x65, 0x79}, // zero\x00key
			flags:        0x20,
			expectedName: "zero\x00key",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal NKRecord with the test data
			nk := format.NKRecord{
				Flags:      tt.flags,
				NameLength: uint16(len(tt.nameRaw)),
				NameRaw:    tt.nameRaw,
			}

			result, err := DecodeKeyName(nk)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expectedName {
				t.Errorf("Name mismatch:\n  got:      %q (bytes: % x)\n  expected: %q (bytes: % x)",
					result, []byte(result),
					tt.expectedName, []byte(tt.expectedName))
			}
		})
	}
}
