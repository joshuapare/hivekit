package regtext

import (
	"os"
	"strings"
	"testing"
)

func TestParseRegFile(t *testing.T) {
	input := `Windows Registry Editor Version 5.00

[\]

[\ControlSet001]

[\ControlSet001\Control]
"CurrentUser"=hex(1):55,00,53,00,45,00
"SystemBootDevice"=hex(1):6d,00,75,00
@="DefaultValue"

[\ControlSet001\Control\AGP]
"102B0520"=hex(3):80,00,00,00
`

	stats, err := ParseRegFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	// Expect 4 keys: \, \ControlSet001, \ControlSet001\Control, \ControlSet001\Control\AGP
	if stats.KeyCount != 4 {
		t.Errorf("Expected 4 keys, got %d", stats.KeyCount)
	}

	// Expect 4 values: CurrentUser, SystemBootDevice, @ (default), 102B0520
	if stats.ValueCount != 4 {
		t.Errorf("Expected 4 values, got %d", stats.ValueCount)
	}

	// Check key paths
	expectedKeys := []string{`\`, `\ControlSet001`, `\ControlSet001\Control`, `\ControlSet001\Control\AGP`}
	if len(stats.Keys) != len(expectedKeys) {
		t.Fatalf("Expected %d keys, got %d", len(expectedKeys), len(stats.Keys))
	}

	for i, expected := range expectedKeys {
		if stats.Keys[i] != expected {
			t.Errorf("Key %d: expected %q, got %q", i, expected, stats.Keys[i])
		}
	}
}

func TestParseRegValue_BackslashEscaping(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		expectedName   string
		expectedType   string
		shouldBeNil    bool
	}{
		{
			name:         "Value name ending with backslash",
			line:         `"C:\\"=hex(1):2c,00,2c,00,35,00,00,00`,
			expectedName: `C:\`,
			expectedType: "hex(1)",
		},
		{
			name:         "Value name with multiple trailing backslashes",
			line:         `"\\\\"=hex(1):00,00`,
			expectedName: `\\`,
			expectedType: "hex(1)",
		},
		{
			name:         "Value name with escaped quote",
			line:         `"Test\"Quote"=dword:00000001`,
			expectedName: `Test"Quote`,
			expectedType: "dword",
		},
		{
			name:         "Value name with backslash then escaped quote",
			line:         `"Path\\\"Name"=dword:00000001`,
			expectedName: `Path\"Name`,
			expectedType: "dword",
		},
		{
			name:         "Normal value",
			line:         `"ProductSuite"="ServerNT"`,
			expectedName: "ProductSuite",
			expectedType: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRegValue(tt.line)
			if tt.shouldBeNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("parseRegValue returned nil")
			}
			if result.Name != tt.expectedName {
				t.Errorf("Name: expected %q, got %q", tt.expectedName, result.Name)
			}
			if result.Type != tt.expectedType {
				t.Errorf("Type: expected %q, got %q", tt.expectedType, result.Type)
			}
		})
	}
}

func TestParseRegFileReal(t *testing.T) {
	// Test with a real .reg file if available
	regPath := "../../testdata/suite/windows-2003-server-system.reg"
	if _, err := os.Stat(regPath); os.IsNotExist(err) {
		t.Skipf("Real .reg file not found: %s", regPath)
	}

	f, err := os.Open(regPath)
	if err != nil {
		t.Fatalf("Failed to open .reg file: %v", err)
	}
	defer f.Close()

	stats, err := ParseRegFile(f)
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	t.Logf("Real .reg file stats:")
	t.Logf("  Keys: %d", stats.KeyCount)
	t.Logf("  Values: %d", stats.ValueCount)

	// Sanity checks
	if stats.KeyCount == 0 {
		t.Error("Expected non-zero key count")
	}
	if stats.ValueCount == 0 {
		t.Error("Expected non-zero value count")
	}
}
