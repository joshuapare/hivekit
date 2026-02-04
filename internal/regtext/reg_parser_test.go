package regtext

import (
	"os"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/pkg/types"
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
		name         string
		line         string
		expectedName string
		expectedType string
		shouldBeNil  bool
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

// Line Continuation Tests

func TestParseRegFile_LineContinuation_Single(t *testing.T) {
	input := `Windows Registry Editor Version 5.00

[\TestKey]
"LongValue"=hex(1):41,00,42,00,43,00,44,00,45,00,\
  46,00,47,00,48,00,49,00,4a,00,4b,00
`
	stats, err := ParseRegFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	if stats.ValueCount != 1 {
		t.Errorf("Expected 1 value, got %d", stats.ValueCount)
	}

	// Verify the value data is concatenated correctly
	value := stats.Structure[0].Values[0]
	expected := "hex(1):41,00,42,00,43,00,44,00,45,00,46,00,47,00,48,00,49,00,4a,00,4b,00"
	if value.Data != expected {
		t.Errorf("Expected data %q, got %q", expected, value.Data)
	}
}

func TestParseRegFile_LineContinuation_Multiple(t *testing.T) {
	input := `Windows Registry Editor Version 5.00

[\TestKey]
"VeryLongValue"=hex(7):41,00,42,00,43,00,44,00,\
  45,00,46,00,47,00,48,00,49,00,4a,00,\
  4b,00,4c,00,4d,00,4e,00,4f,00,00,00
`
	stats, err := ParseRegFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	value := stats.Structure[0].Values[0]
	expected := "hex(7):41,00,42,00,43,00,44,00,45,00,46,00,47,00,48,00,49,00,4a,00,4b,00,4c,00,4d,00,4e,00,4f,00,00,00"
	if value.Data != expected {
		t.Errorf("Expected data %q, got %q", expected, value.Data)
	}
}

func TestParseRegFile_LineContinuation_None(t *testing.T) {
	input := `Windows Registry Editor Version 5.00

[\TestKey]
"ShortValue"=hex(1):41,00,42,00
`
	stats, err := ParseRegFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	value := stats.Structure[0].Values[0]
	expected := "hex(1):41,00,42,00"
	if value.Data != expected {
		t.Errorf("Expected data %q, got %q", expected, value.Data)
	}
}

func TestParseRegFile_LineContinuation_Mixed(t *testing.T) {
	input := `Windows Registry Editor Version 5.00

[\TestKey]
"Short"=dword:00000001
"Long"=hex(1):41,00,42,00,43,00,44,00,45,00,\
  46,00,47,00,48,00,49,00,4a,00
"AnotherShort"="test"
`
	stats, err := ParseRegFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	if stats.ValueCount != 3 {
		t.Errorf("Expected 3 values, got %d", stats.ValueCount)
	}
}

func TestParseRegFile_LineContinuation_BackslashInName(t *testing.T) {
	input := `Windows Registry Editor Version 5.00

[\TestKey]
"C:\\"=dword:00000001
`
	stats, err := ParseRegFile(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	value := stats.Structure[0].Values[0]
	if value.Name != `C:\` {
		t.Errorf("Expected name 'C:\\', got %q", value.Name)
	}
}

// =============================================================================
// ParseReg AllowMissingHeader Tests
// =============================================================================

func TestParseReg_MissingHeader_Allowed(t *testing.T) {
	// Regtext without header - should succeed when AllowMissingHeader is true
	regText := `[Software\Test]
"Value"="Data"
`
	ops, err := ParseReg([]byte(regText), types.RegParseOptions{
		AllowMissingHeader: true,
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("Expected non-empty ops")
	}

	// Should have at least a CreateKey and SetValue operation
	var hasCreateKey, hasSetValue bool
	for _, op := range ops {
		switch op.(type) {
		case types.OpCreateKey:
			hasCreateKey = true
		case types.OpSetValue:
			hasSetValue = true
		}
	}
	if !hasCreateKey {
		t.Error("Expected OpCreateKey in result")
	}
	if !hasSetValue {
		t.Error("Expected OpSetValue in result")
	}
}

func TestParseReg_MissingHeader_NotAllowed(t *testing.T) {
	// Regtext without header - should fail when AllowMissingHeader is false
	regText := `[Software\Test]
"Value"="Data"
`
	_, err := ParseReg([]byte(regText), types.RegParseOptions{
		AllowMissingHeader: false,
	})
	if err == nil {
		t.Fatal("Expected error for missing header")
	}
	if !strings.Contains(err.Error(), "missing header") {
		t.Errorf("Expected 'missing header' error, got: %v", err)
	}
}

func TestParseReg_MissingHeader_DefaultBehavior(t *testing.T) {
	// Regtext without header - default (zero value) should require header
	regText := `[Software\Test]
"Value"="Data"
`
	_, err := ParseReg([]byte(regText), types.RegParseOptions{})
	if err == nil {
		t.Fatal("Expected error for missing header with default options")
	}
	if !strings.Contains(err.Error(), "missing header") {
		t.Errorf("Expected 'missing header' error, got: %v", err)
	}
}

func TestParseReg_WithHeader_StillWorks(t *testing.T) {
	// Regtext with header - should work regardless of AllowMissingHeader setting
	regText := `Windows Registry Editor Version 5.00

[Software\Test]
"Value"="Data"
`
	// With AllowMissingHeader false (default behavior)
	ops1, err := ParseReg([]byte(regText), types.RegParseOptions{
		AllowMissingHeader: false,
	})
	if err != nil {
		t.Fatalf("ParseReg with header failed (AllowMissingHeader=false): %v", err)
	}
	if len(ops1) == 0 {
		t.Error("Expected non-empty ops with AllowMissingHeader=false")
	}

	// With AllowMissingHeader true
	ops2, err := ParseReg([]byte(regText), types.RegParseOptions{
		AllowMissingHeader: true,
	})
	if err != nil {
		t.Fatalf("ParseReg with header failed (AllowMissingHeader=true): %v", err)
	}
	if len(ops2) == 0 {
		t.Error("Expected non-empty ops with AllowMissingHeader=true")
	}
}

func TestParseReg_MissingHeader_MultipleKeys(t *testing.T) {
	// Test parsing multiple keys without header
	regText := `[Key1]
"Val1"="Data1"

[Key2]
"Val2"=dword:00000001

[Key3\SubKey]
"Val3"="Data3"
`
	ops, err := ParseReg([]byte(regText), types.RegParseOptions{
		AllowMissingHeader: true,
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	// Count operations
	var createKeys, setValues int
	for _, op := range ops {
		switch op.(type) {
		case types.OpCreateKey:
			createKeys++
		case types.OpSetValue:
			setValues++
		}
	}

	if createKeys < 3 {
		t.Errorf("Expected at least 3 CreateKey ops, got %d", createKeys)
	}
	if setValues != 3 {
		t.Errorf("Expected 3 SetValue ops, got %d", setValues)
	}
}

func TestParseReg_MissingHeader_DeleteOperations(t *testing.T) {
	// Test delete operations without header
	regText := `[-DeletedKey]

[ExistingKey]
"ToDelete"=-
`
	ops, err := ParseReg([]byte(regText), types.RegParseOptions{
		AllowMissingHeader: true,
	})
	if err != nil {
		t.Fatalf("ParseReg failed: %v", err)
	}

	var hasDeleteKey, hasDeleteValue bool
	for _, op := range ops {
		switch op.(type) {
		case types.OpDeleteKey:
			hasDeleteKey = true
		case types.OpDeleteValue:
			hasDeleteValue = true
		}
	}

	if !hasDeleteKey {
		t.Error("Expected OpDeleteKey in result")
	}
	if !hasDeleteValue {
		t.Error("Expected OpDeleteValue in result")
	}
}

// =============================================================================
// ParseReg String Value Quote Handling Tests (parser.go path)
// =============================================================================

func TestParseReg_StringValueQuoteHandling(t *testing.T) {
	tests := []struct {
		name        string
		line        string // value line within a reg file
		wantValue   string // expected unescaped value
		wantErr     bool
		errContains string
	}{
		{
			name:      "simple string value",
			line:      `"Name"="hello"`,
			wantValue: "hello",
		},
		{
			name:      "string value ending with escaped backslash",
			line:      `"Name"="C:\\"`,
			wantValue: `C:\`,
		},
		{
			name:      "string value ending with escaped quote",
			line:      `"Name"="test\""`,
			wantValue: `test"`,
		},
		{
			name:      "string value with escaped backslash+quote sequence",
			line:      `"Name"="test\\\""`,
			wantValue: `test\"`,
		},
		{
			name:        "genuinely unterminated string",
			line:        `"Name"="test\"`,
			wantErr:     true,
			errContains: "unterminated string",
		},
		{
			name:      "string value with escaped chars in middle",
			line:      `"Name"="a\\b\"c"`,
			wantValue: `a\b"c`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regText := "Windows Registry Editor Version 5.00\r\n\r\n[TestKey]\r\n" + tt.line + "\r\n"
			ops, err := ParseReg([]byte(regText), types.RegParseOptions{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got: %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Find the SetValue op
			var found bool
			for _, op := range ops {
				if sv, ok := op.(types.OpSetValue); ok {
					found = true
					// Decode the UTF-16LE data back to string for comparison
					gotValue := decodeUTF16LEString(sv.Data)
					if gotValue != tt.wantValue {
						t.Errorf("value mismatch: got %q, want %q", gotValue, tt.wantValue)
					}
				}
			}
			if !found {
				t.Fatal("no OpSetValue found in ops")
			}
		})
	}
}

// =============================================================================
// Codec convertRegValue String Quote Handling Tests (codec.go path)
// =============================================================================

func TestConvertRegValue_StringQuoteHandling(t *testing.T) {
	tests := []struct {
		name      string
		data      string // raw Data field in RegValue (includes surrounding quotes)
		wantValue string
	}{
		{
			name:      "simple string value",
			data:      `"hello"`,
			wantValue: "hello",
		},
		{
			name:      "string ending with escaped backslash",
			data:      `"C:\\"`,
			wantValue: `C:\`,
		},
		{
			name:      "string ending with escaped quote",
			data:      `"test\""`,
			wantValue: `test"`,
		},
		{
			name:      "string with escaped backslash+quote sequence",
			data:      `"test\\\""`,
			wantValue: `test\"`,
		},
		{
			name:      "string with escaped chars in middle",
			data:      `"a\\b\"c"`,
			wantValue: `a\b"c`,
		},
		{
			name:      "genuinely unterminated string should not corrupt data",
			data:      `"test\"`,
			wantValue: `"test"`, // no valid closing quote found; data kept as-is, then unescapeRegString runs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := &RegValue{
				Name: "Test",
				Type: ValueTypeString,
				Data: tt.data,
			}
			regType, data, err := convertRegValue(value)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if regType != types.REG_SZ {
				t.Errorf("expected REG_SZ, got %v", regType)
			}
			gotValue := decodeUTF16LEString(data)
			if gotValue != tt.wantValue {
				t.Errorf("value mismatch: got %q, want %q", gotValue, tt.wantValue)
			}
		})
	}
}

// =============================================================================
// Round-trip Tests (escape → parse → compare)
// =============================================================================

func TestRoundTrip_EscapeAndParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"trailing backslash", `C:\`},
		{"trailing quote", `test"`},
		{"trailing backslash-quote", `test\"`},
		{"middle escapes", `a\b"c`},
		{"double backslash", `\\`},
		{"no escapes", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			escaped := escapeString(tt.input)
			regLine := `"Name"="` + escaped + `"`

			regText := "Windows Registry Editor Version 5.00\r\n\r\n[TestKey]\r\n" + regLine + "\r\n"
			ops, err := ParseReg([]byte(regText), types.RegParseOptions{})
			if err != nil {
				t.Fatalf("ParseReg failed: %v", err)
			}
			for _, op := range ops {
				if sv, ok := op.(types.OpSetValue); ok {
					gotValue := decodeUTF16LEString(sv.Data)
					if gotValue != tt.input {
						t.Errorf("round-trip mismatch: input=%q escaped=%q got=%q", tt.input, escaped, gotValue)
					}
					return
				}
			}
			t.Fatal("no OpSetValue found in ops")
		})
	}
}

// =============================================================================
// ParseReg Line Continuation Tests
// =============================================================================

func TestParseReg_LineContinuation(t *testing.T) {
	t.Run("single continuation", func(t *testing.T) {
		regText := "Windows Registry Editor Version 5.00\r\n\r\n[TestKey]\r\n" +
			"\"LongValue\"=hex(1):41,00,42,00,43,00,\\\r\n" +
			"  44,00,45,00,46,00\r\n"
		ops, err := ParseReg([]byte(regText), types.RegParseOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var found bool
		for _, op := range ops {
			if sv, ok := op.(types.OpSetValue); ok {
				found = true
				if sv.Type != types.REG_SZ {
					t.Errorf("expected REG_SZ, got %v", sv.Type)
				}
			}
		}
		if !found {
			t.Fatal("no OpSetValue found")
		}
	})

	t.Run("multiple continuations", func(t *testing.T) {
		regText := "Windows Registry Editor Version 5.00\r\n\r\n[TestKey]\r\n" +
			"\"LongValue\"=hex(7):41,00,42,00,\\\r\n" +
			"  43,00,44,00,\\\r\n" +
			"  00,00,00,00\r\n"
		ops, err := ParseReg([]byte(regText), types.RegParseOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var found bool
		for _, op := range ops {
			if sv, ok := op.(types.OpSetValue); ok {
				found = true
				if sv.Type != types.REG_MULTI_SZ {
					t.Errorf("expected REG_MULTI_SZ, got %v", sv.Type)
				}
			}
		}
		if !found {
			t.Fatal("no OpSetValue found")
		}
	})

	t.Run("no continuation", func(t *testing.T) {
		regText := "Windows Registry Editor Version 5.00\r\n\r\n[TestKey]\r\n" +
			"\"Short\"=hex:01,02,03\r\n"
		ops, err := ParseReg([]byte(regText), types.RegParseOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var found bool
		for _, op := range ops {
			if _, ok := op.(types.OpSetValue); ok {
				found = true
			}
		}
		if !found {
			t.Fatal("no OpSetValue found")
		}
	})

	t.Run("continuation does not affect string values", func(t *testing.T) {
		// A string value ending with \\ should NOT be treated as continuation
		regText := "Windows Registry Editor Version 5.00\r\n\r\n[TestKey]\r\n" +
			`"Path"="C:\\"` + "\r\n" +
			`"Next"="value"` + "\r\n"
		ops, err := ParseReg([]byte(regText), types.RegParseOptions{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var setValues int
		for _, op := range ops {
			if _, ok := op.(types.OpSetValue); ok {
				setValues++
			}
		}
		if setValues != 2 {
			t.Errorf("expected 2 SetValue ops, got %d", setValues)
		}
	})
}

// decodeUTF16LEString decodes UTF-16LE bytes (with null terminator) to a Go string.
func decodeUTF16LEString(data []byte) string {
	// Remove null terminator if present
	if len(data) >= 2 && data[len(data)-1] == 0 && data[len(data)-2] == 0 {
		data = data[:len(data)-2]
	}
	if len(data) == 0 {
		return ""
	}
	u16 := make([]uint16, len(data)/2)
	for i := range u16 {
		u16[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
	}
	return string(utf16.Decode(u16))
}
