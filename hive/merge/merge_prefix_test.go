package merge_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/merge"
)

func TestMergeRegTextWithPrefix_SimplePrefix(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext without any prefix
	regText := `Windows Registry Editor Version 5.00

[Microsoft\Windows]
"Version"="10.0"
"Build"=dword:00004e20
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)
	require.Greater(t, applied.ValuesSet, 0)

	// Verify the keys were created under SOFTWARE
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Check SOFTWARE\Microsoft\Windows exists
	nid, err := h.Find("SOFTWARE\\Microsoft\\Windows")
	require.NoError(t, err)
	require.NotEqual(t, 0, nid)

	// Verify values
	version, err := h.GetString("SOFTWARE\\Microsoft\\Windows", "Version")
	require.NoError(t, err)
	require.Equal(t, "10.0", version)

	build, err := h.GetDWORD("SOFTWARE\\Microsoft\\Windows", "Build")
	require.NoError(t, err)
	require.Equal(t, uint32(20000), build)
}

func TestMergeRegTextWithPrefix_NestedPrefix(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with simple paths
	regText := `Windows Registry Editor Version 5.00

[CurrentVersion\SideBySide]
"Enabled"=dword:00000001
`

	// Apply with nested prefix
	applied, err := merge.MergeRegTextWithPrefix(
		context.Background(),
		hivePath,
		regText,
		"SOFTWARE\\Microsoft\\Windows",
		nil,
	)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify the key structure
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Should be at SOFTWARE\Microsoft\Windows\CurrentVersion\SideBySide
	enabled, err := h.GetDWORD("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\SideBySide", "Enabled")
	require.NoError(t, err)
	require.Equal(t, uint32(1), enabled)
}

func TestMergeRegTextWithPrefix_MultipleKeys(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with multiple keys
	regText := `Windows Registry Editor Version 5.00

[App1]
"Version"="1.0"

[App2]
"Version"="2.0"

[App3\SubKey]
"Data"="test"
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 2) // At least App1, App2, App3

	// Verify all keys exist
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	app1Version, err := h.GetString("SOFTWARE\\App1", "Version")
	require.NoError(t, err)
	require.Equal(t, "1.0", app1Version)

	app2Version, err := h.GetString("SOFTWARE\\App2", "Version")
	require.NoError(t, err)
	require.Equal(t, "2.0", app2Version)

	app3Data, err := h.GetString("SOFTWARE\\App3\\SubKey", "Data")
	require.NoError(t, err)
	require.Equal(t, "test", app3Data)
}

func TestMergeRegTextWithPrefix_ExistingHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with existing data
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"SOFTWARE", "Existing"}, "Value", "Old")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Merge new data with SOFTWARE prefix
	regText := `Windows Registry Editor Version 5.00

[New]
"Value"="Fresh"
`

	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify both old and new data exist
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	oldValue, err := h.GetString("SOFTWARE\\Existing", "Value")
	require.NoError(t, err)
	require.Equal(t, "Old", oldValue)

	newValue, err := h.GetString("SOFTWARE\\New", "Value")
	require.NoError(t, err)
	require.Equal(t, "Fresh", newValue)
}

func TestMergeRegTextWithPrefix_EmptyPrefix(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext
	regText := `Windows Registry Editor Version 5.00

[TestKey]
"Value"="Data"
`

	// Apply with empty prefix (no prepending)
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify key is at root level
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	value, err := h.GetString("TestKey", "Value")
	require.NoError(t, err)
	require.Equal(t, "Data", value)
}

func TestMergeRegTextWithPrefix_PrefixWithHiveRoot(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext
	regText := `Windows Registry Editor Version 5.00

[Test]
"Value"="Data"
`

	// Apply with prefix that includes hive root (should be stripped)
	applied, err := merge.MergeRegTextWithPrefix(
		context.Background(),
		hivePath,
		regText,
		"HKEY_LOCAL_MACHINE\\SOFTWARE",
		nil,
	)
	require.NoError(t, err)
	require.Greater(t, applied.KeysCreated, 0)

	// Verify key is under SOFTWARE (not HKEY_LOCAL_MACHINE\SOFTWARE)
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	value, err := h.GetString("SOFTWARE\\Test", "Value")
	require.NoError(t, err)
	require.Equal(t, "Data", value)
}

func TestMergeRegTextWithPrefix_DeleteOperations(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with existing data
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"SOFTWARE", "ToDelete"}, "Value", "Remove")
	require.NoError(t, err)
	err = b.SetString([]string{"SOFTWARE", "ToKeep"}, "Value", "Keep")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with delete operations
	regText := `Windows Registry Editor Version 5.00

[-ToDelete]
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.KeysDeleted, 0)

	// Verify ToDelete is gone and ToKeep remains
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	_, err = h.Find("SOFTWARE\\ToDelete")
	require.Error(t, err, "deleted key should not exist")

	keepValue, err := h.GetString("SOFTWARE\\ToKeep", "Value")
	require.NoError(t, err)
	require.Equal(t, "Keep", keepValue)
}

func TestMergeRegTextWithPrefix_AllValueTypes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Regtext with various basic value types
	regText := `Windows Registry Editor Version 5.00

[Types]
"String"="Hello"
"Binary"=hex:01,02,03,04
"DWORD"=dword:12345678
`

	// Apply with SOFTWARE prefix
	applied, err := merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.NoError(t, err)
	require.Greater(t, applied.ValuesSet, 2)

	// Verify values
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	str, err := h.GetString("SOFTWARE\\Types", "String")
	require.NoError(t, err)
	require.Equal(t, "Hello", str)

	_, binaryData, err := h.GetValue("SOFTWARE\\Types", "Binary")
	require.NoError(t, err)
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, binaryData)

	dw, err := h.GetDWORD("SOFTWARE\\Types", "DWORD")
	require.NoError(t, err)
	require.Equal(t, uint32(0x12345678), dw)
}

func TestMergeRegTextWithPrefix_InvalidRegText(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Invalid regtext (no header)
	regText := `[InvalidKey]
"Value"="Data"
`

	// Should error during parsing
	_, err = merge.MergeRegTextWithPrefix(context.Background(), hivePath, regText, "SOFTWARE", nil)
	require.Error(t, err, "should error on invalid regtext")
}

func TestMergeRegTextWithPrefix_InvalidHivePath(t *testing.T) {
	regText := `Windows Registry Editor Version 5.00

[Test]
"Value"="Data"
`

	// Should error on non-existent hive
	_, err := merge.MergeRegTextWithPrefix(context.Background(), "/nonexistent/hive", regText, "SOFTWARE", nil)
	require.Error(t, err, "should error on invalid hive path")
}
