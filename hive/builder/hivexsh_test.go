package builder

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// TestBuilder_HivexshValidation validates that hives created by the builder
// can be successfully opened and parsed by hivexsh.
func TestBuilder_HivexshValidation(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Build a hive
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

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Validate with hivexsh
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseHivexsh: true,
	}))
	defer v.Close()

	v.AssertHivexshValid(t)
	t.Log("hivexsh validation successful!")
}

// TestBuilder_HivexshValidation_MinimalHive validates the smallest possible hive.
func TestBuilder_HivexshValidation_MinimalHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "minimal.hive")

	// Create minimal hive (just create and commit, no values)
	b, err := New(hivePath, nil)
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Validate with hivexsh
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseHivexsh: true,
	}))
	defer v.Close()

	v.AssertHivexshValid(t)
	t.Log("Minimal hive hivexsh validation successful!")
}

// TestBuilder_HivexshValidation_ComplexStructure validates a complex hive.
func TestBuilder_HivexshValidation_ComplexStructure(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "complex.hive")

	// Build a complex hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create deep hierarchy
	paths := [][]string{
		{"Software", "Company1", "App1"},
		{"Software", "Company1", "App2"},
		{"Software", "Company2", "App1"},
		{"System", "Config", "Settings"},
		{"System", "Config", "Advanced", "Options"},
	}

	for i, path := range paths {
		err = b.SetString(path, "Name", fmt.Sprintf("Test%d", i))
		require.NoError(t, err)

		err = b.SetDWORD(path, "Index", uint32(i))
		require.NoError(t, err)
	}

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Validate with hivexsh
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseHivexsh: true,
	}))
	defer v.Close()

	v.AssertHivexshValid(t)
	t.Log("Complex hive hivexsh validation successful!")
}

// TestBuilder_HivexshValidation_AllValueTypes validates all registry value types.
func TestBuilder_HivexshValidation_AllValueTypes(t *testing.T) {
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

	// Validate with hivexsh
	v := hivexval.Must(hivexval.New(hivePath, &hivexval.Options{
		UseHivexsh: true,
	}))
	defer v.Close()

	v.AssertHivexshValid(t)
	t.Log("All value types hivexsh validation successful!")
}
