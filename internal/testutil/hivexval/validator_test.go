package hivexval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive/builder"
)

// createTestHive creates a test hive with known structure for testing.
func createTestHive(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Build a test hive with known structure
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create known structure:
	// Root
	//   Software
	//     Company1
	//       App1
	//         Name = "TestApp1"
	//         Version = "1.0.0"
	//         Timeout = 30 (DWORD)
	//         Counter = 9876543210 (QWORD)
	//       App2
	//         Name = "TestApp2"
	//         Enabled = 1 (DWORD)
	//     Company2
	//       App1
	//         Features = ["A", "B", "C"] (MULTI_SZ)
	//   System
	//     Config
	//       Data = [0x01, 0x02, 0x03] (BINARY)

	err = b.SetString([]string{"Software", "Company1", "App1"}, "Name", "TestApp1")
	require.NoError(t, err)

	err = b.SetString([]string{"Software", "Company1", "App1"}, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.SetDWORD([]string{"Software", "Company1", "App1"}, "Timeout", 30)
	require.NoError(t, err)

	err = b.SetQWORD([]string{"Software", "Company1", "App1"}, "Counter", 9876543210)
	require.NoError(t, err)

	err = b.SetString([]string{"Software", "Company1", "App2"}, "Name", "TestApp2")
	require.NoError(t, err)

	err = b.SetDWORD([]string{"Software", "Company1", "App2"}, "Enabled", 1)
	require.NoError(t, err)

	err = b.SetMultiString([]string{"Software", "Company2", "App1"}, "Features", []string{"A", "B", "C"})
	require.NoError(t, err)

	err = b.SetBinary([]string{"System", "Config"}, "Data", []byte{0x01, 0x02, 0x03})
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	return hivePath
}

// TestValidator_New tests creating validators.
func TestValidator_New(t *testing.T) {
	hivePath := createTestHive(t)

	tests := []struct {
		name    string
		opts    *Options
		wantErr bool
	}{
		{
			name: "default options",
			opts: nil,
		},
		{
			name: "bindings only",
			opts: &Options{UseBindings: true},
		},
		{
			name: "reader only",
			opts: &Options{UseReader: true},
		},
		{
			name: "hivexsh only",
			opts: &Options{UseHivexsh: true},
		},
		{
			name: "all backends",
			opts: &Options{UseBindings: true, UseReader: true, UseHivexsh: true},
		},
		{
			name:    "no backends",
			opts:    &Options{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := New(hivePath, tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, v)
			defer v.Close()
		})
	}
}

// TestValidator_NewBytes tests creating validators from bytes.
func TestValidator_NewBytes(t *testing.T) {
	hivePath := createTestHive(t)
	data, err := os.ReadFile(hivePath)
	require.NoError(t, err)

	v, err := NewBytes(data, nil)
	require.NoError(t, err)
	require.NotNil(t, v)
	defer v.Close()

	// Verify it works
	root, err := v.Root()
	require.NoError(t, err)
	require.NotNil(t, root)
}

// TestValidator_Root tests getting root node.
func TestValidator_Root(t *testing.T) {
	hivePath := createTestHive(t)

	backends := []struct {
		name string
		opts *Options
	}{
		{"bindings", &Options{UseBindings: true}},
		{"reader", &Options{UseReader: true}},
	}

	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			v := Must(New(hivePath, backend.opts))
			defer v.Close()

			root, err := v.Root()
			require.NoError(t, err)
			require.NotNil(t, root)
		})
	}
}

// TestValidator_GetKey tests key navigation.
func TestValidator_GetKey(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	tests := []struct {
		name    string
		path    []string
		wantErr bool
	}{
		{"root", []string{}, false},
		{"Software", []string{"Software"}, false},
		{"Software\\Company1", []string{"Software", "Company1"}, false},
		{"Software\\Company1\\App1", []string{"Software", "Company1", "App1"}, false},
		{"nonexistent", []string{"NonExistent"}, true},
		{"deep nonexistent", []string{"Software", "Company1", "NonExistent"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := v.GetKey(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, key)
		})
	}
}

// TestValidator_GetKeyName tests getting key names.
func TestValidator_GetKeyName(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	tests := []struct {
		path     []string
		expected string
	}{
		{[]string{}, ""},                               // Root
		{[]string{"Software"}, "Software"},             // First level
		{[]string{"Software", "Company1"}, "Company1"}, // Second level
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			key, err := v.GetKey(tt.path)
			require.NoError(t, err)

			name, err := v.GetKeyName(key)
			require.NoError(t, err)
			require.Equal(t, tt.expected, name)
		})
	}
}

// TestValidator_GetSubkeys tests enumerating child keys.
func TestValidator_GetSubkeys(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	// Root should have Software and System
	root, err := v.GetKey([]string{})
	require.NoError(t, err)

	children, err := v.GetSubkeys(root)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(children), 2) // At least Software and System

	// Software should have Company1 and Company2
	software, err := v.GetKey([]string{"Software"})
	require.NoError(t, err)

	children, err = v.GetSubkeys(software)
	require.NoError(t, err)
	require.Len(t, children, 2) // Company1 and Company2
}

// TestValidator_CountTree tests counting keys and values.
func TestValidator_CountTree(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	keyCount, valueCount, err := v.CountTree()
	require.NoError(t, err)

	// Expected structure:
	// Root, Software, Company1, App1, App2, Company2, App1, System, Config
	// = 9 keys
	require.Equal(t, 9, keyCount)

	// Expected values:
	// App1: Name, Version, Timeout, Counter = 4
	// App2: Name, Enabled = 2
	// Company2\App1: Features = 1
	// System\Config: Data = 1
	// Total = 8
	require.Equal(t, 8, valueCount)
}

// TestValidator_GetValue tests finding values by name.
func TestValidator_GetValue(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	key, err := v.GetKey([]string{"Software", "Company1", "App1"})
	require.NoError(t, err)

	// Test getting existing value
	val, err := v.GetValue(key, "Name")
	require.NoError(t, err)
	require.NotNil(t, val)

	// Test getting nonexistent value
	_, err = v.GetValue(key, "NonExistent")
	require.Error(t, err)

	// Test case-insensitive
	val, err = v.GetValue(key, "name") // lowercase
	require.NoError(t, err)
	require.NotNil(t, val)
}

// TestValidator_GetValueString tests reading string values.
func TestValidator_GetValueString(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	key, err := v.GetKey([]string{"Software", "Company1", "App1"})
	require.NoError(t, err)

	val, err := v.GetValue(key, "Name")
	require.NoError(t, err)

	str, err := v.GetValueString(val)
	require.NoError(t, err)
	require.Equal(t, "TestApp1", str)
}

// TestValidator_GetValueDWORD tests reading DWORD values.
func TestValidator_GetValueDWORD(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	key, err := v.GetKey([]string{"Software", "Company1", "App1"})
	require.NoError(t, err)

	val, err := v.GetValue(key, "Timeout")
	require.NoError(t, err)

	dw, err := v.GetValueDWORD(val)
	require.NoError(t, err)
	require.Equal(t, uint32(30), dw)
}

// TestValidator_GetValueQWORD tests reading QWORD values.
func TestValidator_GetValueQWORD(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	key, err := v.GetKey([]string{"Software", "Company1", "App1"})
	require.NoError(t, err)

	val, err := v.GetValue(key, "Counter")
	require.NoError(t, err)

	qw, err := v.GetValueQWORD(val)
	require.NoError(t, err)
	require.Equal(t, uint64(9876543210), qw)
}

// TestValidator_GetValueStrings tests reading MULTI_SZ values.
func TestValidator_GetValueStrings(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	key, err := v.GetKey([]string{"Software", "Company2", "App1"})
	require.NoError(t, err)

	val, err := v.GetValue(key, "Features")
	require.NoError(t, err)

	strs, err := v.GetValueStrings(val)
	require.NoError(t, err)
	require.Equal(t, []string{"A", "B", "C"}, strs)
}

// TestValidator_GetValueData tests reading raw value data.
func TestValidator_GetValueData(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	key, err := v.GetKey([]string{"System", "Config"})
	require.NoError(t, err)

	val, err := v.GetValue(key, "Data")
	require.NoError(t, err)

	data, err := v.GetValueData(val)
	require.NoError(t, err)
	require.Equal(t, []byte{0x01, 0x02, 0x03}, data)
}

// TestValidator_GetValueType tests getting value types.
func TestValidator_GetValueType(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	tests := []struct {
		keyPath   []string
		valueName string
		expected  string
	}{
		{[]string{"Software", "Company1", "App1"}, "Name", "REG_SZ"},
		{[]string{"Software", "Company1", "App1"}, "Timeout", "REG_DWORD"},
		{[]string{"Software", "Company1", "App1"}, "Counter", "REG_QWORD"},
		{[]string{"Software", "Company2", "App1"}, "Features", "REG_MULTI_SZ"},
		{[]string{"System", "Config"}, "Data", "REG_BINARY"},
	}

	for _, tt := range tests {
		t.Run(tt.valueName, func(t *testing.T) {
			key, err := v.GetKey(tt.keyPath)
			require.NoError(t, err)

			val, err := v.GetValue(key, tt.valueName)
			require.NoError(t, err)

			typ, err := v.GetValueType(val)
			require.NoError(t, err)
			require.Equal(t, tt.expected, typ)
		})
	}
}

// TestValidator_WalkTree tests recursive tree traversal.
func TestValidator_WalkTree(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	keysSeen := 0
	valuesSeen := 0

	err := v.WalkTree(func(_ string, _ int, isValue bool) error {
		if isValue {
			valuesSeen++
		} else {
			keysSeen++
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 9, keysSeen)   // Expected key count
	require.Equal(t, 8, valuesSeen) // Expected value count
}

// TestValidator_ValidateStructure tests structure validation.
func TestValidator_ValidateStructure(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, nil))
	defer v.Close()

	err := v.ValidateStructure()
	require.NoError(t, err)
}

// TestValidator_ValidateWithHivexsh tests hivexsh validation.
func TestValidator_ValidateWithHivexsh(t *testing.T) {
	if !IsHivexshAvailable() {
		t.Skip("hivexsh not available")
	}

	hivePath := createTestHive(t)
	v := Must(New(hivePath, &Options{UseHivexsh: true}))
	defer v.Close()

	err := v.ValidateWithHivexsh()
	require.NoError(t, err)
}

// TestValidator_Validate tests comprehensive validation.
func TestValidator_Validate(t *testing.T) {
	hivePath := createTestHive(t)
	v := Must(New(hivePath, &Options{
		UseBindings: true,
		UseHivexsh:  IsHivexshAvailable(),
	}))
	defer v.Close()

	result, err := v.Validate()
	require.NoError(t, err)
	require.True(t, result.StructureValid)
	require.Equal(t, 9, result.KeyCount)
	require.Equal(t, 8, result.ValueCount)

	if IsHivexshAvailable() {
		require.True(t, result.HivexshPassed)
	}
}
