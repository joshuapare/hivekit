package edit

import (
	"testing"
	"time"

	"github.com/joshuapare/hivekit/internal/format"
)

// Test_timeToFiletime tests the Windows FILETIME conversion.
func Test_timeToFiletime(t *testing.T) {
	// Test with a known time
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	filetime := timeToFiletime(testTime)

	// FILETIME should be non-zero
	allZero := true
	for _, b := range filetime {
		if b != 0 {
			allZero = false
			break
		}
	}

	if allZero {
		t.Error("Expected non-zero FILETIME")
	}
}

// Test_normalizeName tests case normalization.
func Test_normalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Software", "software"},
		{"SOFTWARE", "software"},
		{"MixedCase", "mixedcase"},
		{"already_lower", "already_lower"},
		{"With123Numbers", "with123numbers"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Test_ErrorConstants tests that error constants are defined.
func Test_ErrorConstants(t *testing.T) {
	errors := []error{
		ErrNotImplemented,
		ErrInvalidRef,
		ErrKeyNotFound,
		ErrValueNotFound,
		ErrInvalidKeyName,
		ErrInvalidValueName,
		ErrDataTooLarge,
		ErrCannotDeleteRoot,
		ErrKeyHasSubkeys,
	}

	for i, err := range errors {
		if err == nil {
			t.Errorf("Error constant %d is nil", i)
		}
		if err.Error() == "" {
			t.Errorf("Error constant %d has empty message", i)
		}
	}
}

// Test_ValueTypeConstants tests registry type constants.
func Test_ValueTypeConstants(t *testing.T) {
	// Just verify they have expected values
	if format.RegNone != 0 {
		t.Errorf("format.RegNone = %d, want 0", format.RegNone)
	}
	if format.REGSZ != 1 {
		t.Errorf("format.REGSZ = %d, want 1", format.REGSZ)
	}
	if format.REGDWORD != 4 {
		t.Errorf("format.REGDWORD = %d, want 4", format.REGDWORD)
	}
	if format.REGQWORD != 11 {
		t.Errorf("format.REGQWORD = %d, want 11", format.REGQWORD)
	}
}

// Test_SizeConstants tests storage size constants.
func Test_SizeConstants(t *testing.T) {
	if MaxInlineValueBytes != 4 {
		t.Errorf("MaxInlineValueBytes = %d, want 4", MaxInlineValueBytes)
	}
	if MaxExternalValueBytes != 16344 {
		t.Errorf("MaxExternalValueBytes = %d, want 16344", MaxExternalValueBytes)
	}
}
