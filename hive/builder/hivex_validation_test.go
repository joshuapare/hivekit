package builder

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// TestBuilder_HivexValidation validates that hives created by the builder
// can be successfully opened and read by hivex (via Go bindings).
func TestBuilder_HivexValidation(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Build a hive with the builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Add various data
	err = b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.SetDWORD([]string{"Software", "MyApp", "Settings"}, "Timeout", 30)
	require.NoError(t, err)

	err = b.SetBinary([]string{"Software", "MyApp"}, "Config", []byte{0x01, 0x02, 0x03})
	require.NoError(t, err)

	err = b.SetMultiString([]string{"Software", "MyApp"}, "Features", []string{"A", "B", "C"})
	require.NoError(t, err)

	err = b.SetQWORD([]string{"Software", "MyApp"}, "Counter", 9876543210)
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Validate with hivex (via Go bindings)
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseReader: true,
	}))
	defer v.Close()

	// We expect:
	// - Root
	// - Software
	// - Software\MyApp
	// - Software\MyApp\Settings
	// = 4 keys
	v.AssertKeyCount(t, 4)

	// We expect 5 values under Software\MyApp
	v.AssertValueCount(t, 5)

	t.Logf("Hivex validation passed!")
}

// TestBuilder_HivexValidation_ComplexStructure tests a more complex hive structure.
func TestBuilder_HivexValidation_ComplexStructure(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "complex.hive")

	// Build a complex hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create a deeper hierarchy
	paths := [][]string{
		{"Software", "Company1", "App1"},
		{"Software", "Company1", "App2"},
		{"Software", "Company2", "App1"},
		{"System", "Config", "Settings"},
		{"System", "Config", "Advanced", "Options"},
	}

	for i, path := range paths {
		err = b.SetString(path, "Name", "Test"+string(rune('A'+i)))
		require.NoError(t, err)

		err = b.SetDWORD(path, "Index", uint32(i))
		require.NoError(t, err)
	}

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Validate with hivex
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseReader: true,
	}))
	defer v.Close()

	// We expect: Root + 11 keys from the paths above
	// Root, Software, Company1, App1, App2, Company2, App1, System, Config, Settings, Advanced, Options
	// = 12 keys total
	keyCount, valueCount, err := v.CountTree()
	require.NoError(t, err)
	require.GreaterOrEqual(t, keyCount, 12, "should have at least 12 keys")

	// We expect 2 values per path (Name + Index) = 10 values
	require.Equal(t, 10, valueCount, "should have 10 values")

	t.Logf("Complex hive hivex validation passed!")
}

// TestBuilder_HivexValidation_AllValueTypes validates all value types are readable by hivex.
func TestBuilder_HivexValidation_AllValueTypes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "alltypes.hive")

	// Build hive with all value types
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

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

	err = b.SetMultiString([]string{"Test"}, "MultiSzValue", []string{"A", "B"})
	require.NoError(t, err)

	err = b.SetQWORD([]string{"Test"}, "QWordValue", 9876543210)
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Validate with hivex
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseReader: true,
	}))
	defer v.Close()

	// We expect 8 values
	v.AssertValueCount(t, 8)

	t.Logf("All value types validated by hivex!")
}
