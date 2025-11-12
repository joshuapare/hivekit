package builder

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple path",
			input:    "Software\\MyApp",
			expected: []string{"Software", "MyApp"},
		},
		{
			name:     "HKLM prefix",
			input:    "HKEY_LOCAL_MACHINE\\Software\\MyApp",
			expected: []string{"Software", "MyApp"},
		},
		{
			name:     "HKLM short prefix",
			input:    "HKLM\\System\\CurrentControlSet",
			expected: []string{"System", "CurrentControlSet"},
		},
		{
			name:     "HKCU prefix",
			input:    "HKEY_CURRENT_USER\\Software\\MyApp",
			expected: []string{"Software", "MyApp"},
		},
		{
			name:     "HKCU short prefix",
			input:    "HKCU\\Software\\MyApp",
			expected: []string{"Software", "MyApp"},
		},
		{
			name:     "deep path",
			input:    "Software\\Microsoft\\Windows\\CurrentVersion\\Run",
			expected: []string{"Software", "Microsoft", "Windows", "CurrentVersion", "Run"},
		},
		{
			name:     "trailing backslash",
			input:    "Software\\MyApp\\",
			expected: []string{"Software", "MyApp"},
		},
		{
			name:     "single segment",
			input:    "Software",
			expected: []string{"Software"},
		},
		{
			name:     "empty path",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitPath(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseValueType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint32
		wantErr  bool
	}{
		{"REG_NONE", "REG_NONE", format.REGNone, false},
		{"NONE", "NONE", format.REGNone, false},
		{"REG_SZ", "REG_SZ", format.REGSZ, false},
		{"SZ", "SZ", format.REGSZ, false},
		{"STRING", "STRING", format.REGSZ, false},
		{"REG_EXPAND_SZ", "REG_EXPAND_SZ", format.REGExpandSZ, false},
		{"EXPAND_SZ", "EXPAND_SZ", format.REGExpandSZ, false},
		{"REG_BINARY", "REG_BINARY", format.REGBinary, false},
		{"BINARY", "BINARY", format.REGBinary, false},
		{"REG_DWORD", "REG_DWORD", format.REGDWORD, false},
		{"DWORD", "DWORD", format.REGDWORD, false},
		{"REG_DWORD_BIG_ENDIAN", "REG_DWORD_BIG_ENDIAN", format.REGDWORDBigEndian, false},
		{"DWORD_BE", "DWORD_BE", format.REGDWORDBigEndian, false},
		{"REG_LINK", "REG_LINK", format.REGLink, false},
		{"LINK", "LINK", format.REGLink, false},
		{"REG_MULTI_SZ", "REG_MULTI_SZ", format.REGMultiSZ, false},
		{"MULTI_SZ", "MULTI_SZ", format.REGMultiSZ, false},
		{"REG_QWORD", "REG_QWORD", format.REGQWORD, false},
		{"QWORD", "QWORD", format.REGQWORD, false},
		{"lowercase", "reg_sz", format.REGSZ, false},
		{"mixed case", "ReG_DwOrD", format.REGDWORD, false},
		{"unknown", "INVALID_TYPE", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseValueType(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBuilder_SetValueFromString(t *testing.T) {
	dir := t.TempDir()
	hivePath := dir + "/test.hive"

	// Create builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Use SetValueFromString with full paths
	err = b.SetValueFromString("HKLM\\Software\\MyApp", "Version", "REG_SZ", encodeString("1.0.0"))
	require.NoError(t, err)

	err = b.SetValueFromString("Software\\MyApp", "Timeout", "DWORD", encodeDWORD(30))
	require.NoError(t, err)

	err = b.SetValueFromString("HKEY_CURRENT_USER\\Software\\Settings", "Data", "BINARY", []byte{0x01, 0x02})
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)
}

func TestBuilder_SetAllValueTypes(t *testing.T) {
	dir := t.TempDir()
	hivePath := dir + "/test.hive"

	// Create builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Test all value types
	err = b.SetNone([]string{"Test"}, "NoneValue")
	require.NoError(t, err)

	err = b.SetString([]string{"Test"}, "StringValue", "Hello")
	require.NoError(t, err)

	err = b.SetExpandString([]string{"Test"}, "ExpandValue", "%PATH%")
	require.NoError(t, err)

	err = b.SetBinary([]string{"Test"}, "BinaryValue", []byte{0x01, 0x02})
	require.NoError(t, err)

	err = b.SetDWORD([]string{"Test"}, "DWordValue", 123)
	require.NoError(t, err)

	err = b.SetDWORDBigEndian([]string{"Test"}, "DWordBEValue", 456)
	require.NoError(t, err)

	err = b.SetLink([]string{"Test"}, "LinkValue", []byte("\\Registry\\Machine\\Software"))
	require.NoError(t, err)

	err = b.SetMultiString([]string{"Test"}, "MultiSzValue", []string{"A", "B"})
	require.NoError(t, err)

	err = b.SetResourceList([]string{"Test"}, "ResourceListValue", []byte{0x00})
	require.NoError(t, err)

	err = b.SetFullResourceDescriptor([]string{"Test"}, "FullResourceValue", []byte{0x00})
	require.NoError(t, err)

	err = b.SetResourceRequirementsList([]string{"Test"}, "ResourceReqValue", []byte{0x00})
	require.NoError(t, err)

	err = b.SetQWORD([]string{"Test"}, "QWordValue", 9876543210)
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)
}

func TestBuilder_SplitPath_WithStripOption(t *testing.T) {
	tmpFile := t.TempDir() + "/test.hive"

	// Test with stripping enabled (default)
	t.Run("with stripping enabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.StripHiveRootPrefixes = true

		b, err := New(tmpFile, opts)
		require.NoError(t, err)
		defer b.Close()

		// Test various inputs
		tests := []struct {
			input    string
			expected []string
		}{
			{"HKLM\\Software\\MyApp", []string{"Software", "MyApp"}},
			{"HKEY_LOCAL_MACHINE\\Software\\Test", []string{"Software", "Test"}},
			{"HKCU\\Software\\MyApp", []string{"Software", "MyApp"}},
			{"Software\\MyApp", []string{"Software", "MyApp"}},
		}

		for _, tt := range tests {
			result := b.splitPath(tt.input)
			require.Equal(t, tt.expected, result, "input: %s", tt.input)
		}
	})

	// Test with stripping disabled
	t.Run("with stripping disabled", func(t *testing.T) {
		opts := DefaultOptions()
		opts.StripHiveRootPrefixes = false

		b, err := New(tmpFile+"2", opts)
		require.NoError(t, err)
		defer b.Close()

		// Test various inputs - prefixes should be preserved
		tests := []struct {
			input    string
			expected []string
		}{
			{"HKLM\\Software\\MyApp", []string{"HKLM", "Software", "MyApp"}},
			{"HKEY_LOCAL_MACHINE\\Software\\Test", []string{"HKEY_LOCAL_MACHINE", "Software", "Test"}},
			{"HKCU\\Software\\MyApp", []string{"HKCU", "Software", "MyApp"}},
			{"Software\\MyApp", []string{"Software", "MyApp"}},
		}

		for _, tt := range tests {
			result := b.splitPath(tt.input)
			require.Equal(t, tt.expected, result, "input: %s", tt.input)
		}
	})
}
