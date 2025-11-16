package builder

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
)

// TestSetValue_RootKey verifies that we can set values on the root key
// using an empty path ([]string{}).
func TestSetValue_RootKey(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set a value on the root key using empty path
	err = b.SetString([]string{}, "RootValue", "TestData")
	require.NoError(t, err)

	// Set another value with different type
	err = b.SetDWORD([]string{}, "RootDWORD", 42)
	require.NoError(t, err)

	// Also set a regular nested value to ensure normal operations still work
	err = b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify values were written by reading them back
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Read root value using empty path
	rootValue, err := h.GetString("", "RootValue")
	require.NoError(t, err)
	require.Equal(t, "TestData", rootValue)

	// Read root DWORD
	rootDWORD, err := h.GetDWORD("", "RootDWORD")
	require.NoError(t, err)
	require.Equal(t, uint32(42), rootDWORD)

	// Verify nested value still works
	version, err := h.GetString("Software\\MyApp", "Version")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", version)
}

// TestSetValue_RootKeyWithNormalization verifies that root key operations
// still work after path normalization (e.g., stripping HKLM prefix).
func TestSetValue_RootKeyWithNormalization(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set a value using HKLM prefix only (should become empty path after normalization)
	err = b.SetString([]string{"HKEY_LOCAL_MACHINE"}, "NormalizedRoot", "Test")
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify value was written to root
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Should be accessible at root
	value, err := h.GetString("", "NormalizedRoot")
	require.NoError(t, err)
	require.Equal(t, "Test", value)
}

// TestDeleteValue_RootKey verifies that we can delete values from the root key.
func TestDeleteValue_RootKey(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set a root value
	err = b.SetString([]string{}, "ToDelete", "Test")
	require.NoError(t, err)

	// Set another root value that we'll keep
	err = b.SetString([]string{}, "ToKeep", "Keep")
	require.NoError(t, err)

	// Delete the first value
	err = b.DeleteValue([]string{}, "ToDelete")
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify deleted value is gone and kept value remains
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Deleted value should not exist
	_, err = h.GetString("", "ToDelete")
	require.Error(t, err, "deleted value should not exist")

	// Kept value should exist
	kept, err := h.GetString("", "ToKeep")
	require.NoError(t, err)
	require.Equal(t, "Keep", kept)
}

// TestEnsureKey_RootKey verifies that EnsureKey with empty path is now allowed
// (even though it's a no-op since root always exists).
func TestEnsureKey_RootKey(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Ensure root key (should be allowed as no-op)
	err = b.EnsureKey([]string{})
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify hive is valid
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Root should exist (always does)
	nid, err := h.Find("")
	require.NoError(t, err)
	require.NotEqual(t, 0, nid, "root should exist")
}

// TestSetValueFromString_RootKey verifies the helper function works with root key.
func TestSetValueFromString_RootKey(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set value using string path (empty or just separator)
	err = b.SetValueFromString("", "RootHelper", "REG_SZ", []byte("T\x00e\x00s\x00t\x00\x00\x00"))
	require.NoError(t, err)

	// Also try with just backslash
	err = b.SetValueFromString("\\", "RootHelper2", "REG_DWORD", []byte{0x2A, 0x00, 0x00, 0x00})
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify values exist at root
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	value1, err := h.GetString("", "RootHelper")
	require.NoError(t, err)
	require.Equal(t, "Test", value1)

	value2, err := h.GetDWORD("", "RootHelper2")
	require.NoError(t, err)
	require.Equal(t, uint32(42), value2)
}

// TestMultipleRootValues verifies we can set multiple values on root.
func TestMultipleRootValues(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set various types of values on root
	err = b.SetString([]string{}, "StringVal", "Hello")
	require.NoError(t, err)

	err = b.SetDWORD([]string{}, "DWORDVal", 123)
	require.NoError(t, err)

	err = b.SetQWORD([]string{}, "QWORDVal", 9876543210)
	require.NoError(t, err)

	err = b.SetBinary([]string{}, "BinaryVal", []byte{0x01, 0x02, 0x03, 0x04})
	require.NoError(t, err)

	err = b.SetMultiString([]string{}, "MultiVal", []string{"Line1", "Line2", "Line3"})
	require.NoError(t, err)

	err = b.SetNone([]string{}, "NoneVal")
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify all values
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	stringVal, err := h.GetString("", "StringVal")
	require.NoError(t, err)
	require.Equal(t, "Hello", stringVal)

	dwordVal, err := h.GetDWORD("", "DWORDVal")
	require.NoError(t, err)
	require.Equal(t, uint32(123), dwordVal)

	qwordVal, err := h.GetQWORD("", "QWORDVal")
	require.NoError(t, err)
	require.Equal(t, uint64(9876543210), qwordVal)

	_, binaryVal, err := h.GetValue("", "BinaryVal")
	require.NoError(t, err)
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, binaryVal)

	multiVal, err := h.GetMultiString("", "MultiVal")
	require.NoError(t, err)
	require.Equal(t, []string{"Line1", "Line2", "Line3"}, multiVal)

	// GetValue should work for REG_NONE
	_, noneVal, err := h.GetValue("", "NoneVal")
	require.NoError(t, err)
	require.Empty(t, noneVal)
}
