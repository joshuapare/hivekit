package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/walker"
)

func TestNew_CreateFromScratch(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create builder (should create new hive)
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	require.NotNil(t, b)
	defer b.Close()

	// Verify file was created
	_, err = os.Stat(hivePath)
	require.NoError(t, err, "hive file should exist")

	// Commit and close
	err = b.Commit()
	require.NoError(t, err)

	// Verify we can open the created hive
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Verify it's a valid hive
	require.Positive(t, h.Size())
}

func TestBuilder_SetString(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set some string values
	err = b.SetString([]string{"Software", "MyApp"}, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.SetString([]string{"Software", "MyApp"}, "Name", "TestApp")
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify values were written
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Walk to the key and verify values
	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	// Find the Software\MyApp key
	rootNK := h.RootCellOffset()
	softwareNK, found := idx.GetNK(rootNK, "Software")
	require.True(t, found, "Software key should exist")

	myAppNK, found := idx.GetNK(softwareNK, "MyApp")
	require.True(t, found, "MyApp key should exist")

	// Verify values exist
	versionVK, found := idx.GetVK(myAppNK, "Version")
	require.True(t, found, "Version value should exist")
	require.NotEqual(t, uint32(0), versionVK)

	nameVK, found := idx.GetVK(myAppNK, "Name")
	require.True(t, found, "Name value should exist")
	require.NotEqual(t, uint32(0), nameVK)
}

func TestBuilder_SetDWORD(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set DWORD values
	err = b.SetDWORD([]string{"Software", "MyApp"}, "Timeout", 30)
	require.NoError(t, err)

	err = b.SetDWORD([]string{"Software", "MyApp"}, "MaxRetries", 5)
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	rootNK := h.RootCellOffset()
	softwareNK, _ := idx.GetNK(rootNK, "Software")
	myAppNK, _ := idx.GetNK(softwareNK, "MyApp")

	timeoutVK, found := idx.GetVK(myAppNK, "Timeout")
	require.True(t, found)
	require.NotEqual(t, uint32(0), timeoutVK)
}

func TestBuilder_MultipleValueTypes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set various value types
	err = b.SetString([]string{"Test"}, "StringValue", "Hello")
	require.NoError(t, err)

	err = b.SetDWORD([]string{"Test"}, "DWordValue", 12345)
	require.NoError(t, err)

	err = b.SetQWORD([]string{"Test"}, "QWordValue", 9876543210)
	require.NoError(t, err)

	err = b.SetBinary([]string{"Test"}, "BinaryValue", []byte{0x01, 0x02, 0x03, 0x04})
	require.NoError(t, err)

	err = b.SetMultiString([]string{"Test"}, "MultiStringValue", []string{"A", "B", "C"})
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	rootNK := h.RootCellOffset()
	testNK, found := idx.GetNK(rootNK, "Test")
	require.True(t, found)

	// Verify all values exist
	_, found = idx.GetVK(testNK, "StringValue")
	require.True(t, found)

	_, found = idx.GetVK(testNK, "DWordValue")
	require.True(t, found)

	_, found = idx.GetVK(testNK, "QWordValue")
	require.True(t, found)

	_, found = idx.GetVK(testNK, "BinaryValue")
	require.True(t, found)

	_, found = idx.GetVK(testNK, "MultiStringValue")
	require.True(t, found)
}

func TestBuilder_DeleteValue(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Set values
	err = b.SetString([]string{"Test"}, "Keep", "KeepThis")
	require.NoError(t, err)

	err = b.SetString([]string{"Test"}, "Delete", "DeleteThis")
	require.NoError(t, err)

	// Delete one value
	err = b.DeleteValue([]string{"Test"}, "Delete")
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	rootNK := h.RootCellOffset()
	testNK, _ := idx.GetNK(rootNK, "Test")

	// Keep should exist
	_, found := idx.GetVK(testNK, "Keep")
	require.True(t, found, "Keep value should still exist")

	// Delete should not exist
	_, found = idx.GetVK(testNK, "Delete")
	require.False(t, found, "Delete value should not exist")
}

func TestBuilder_DeleteKey(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and populate hive
	b, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Create keys
	err = b.SetString([]string{"Keep"}, "Value", "KeepThis")
	require.NoError(t, err)

	err = b.SetString([]string{"Delete", "SubKey"}, "Value", "DeleteThis")
	require.NoError(t, err)

	// Delete the Delete key and its subkeys
	err = b.DeleteKey([]string{"Delete"})
	require.NoError(t, err)

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	rootNK := h.RootCellOffset()

	// Keep should exist
	_, found := idx.GetNK(rootNK, "Keep")
	require.True(t, found, "Keep key should still exist")

	// Delete should not exist
	_, found = idx.GetNK(rootNK, "Delete")
	require.False(t, found, "Delete key should not exist")
}

func TestBuilder_ProgressiveFlush(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create builder with low flush threshold
	opts := DefaultOptions()
	opts.AutoFlushThreshold = 5 // Flush every 5 operations

	b, err := New(hivePath, opts)
	require.NoError(t, err)
	defer b.Close()

	// Add 20 operations (should trigger 4 flushes)
	for i := range 20 {
		err = b.SetDWORD([]string{"Test"}, "Value"+string(rune('A'+i)), uint32(i))
		require.NoError(t, err)
	}

	// Commit
	err = b.Commit()
	require.NoError(t, err)

	// Verify all values were written
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	rootNK := h.RootCellOffset()
	testNK, found := idx.GetNK(rootNK, "Test")
	require.True(t, found)

	// Verify at least some values exist (index doesn't expose count directly)
	_, found = idx.GetVK(testNK, "ValueA")
	require.True(t, found)

	_, found = idx.GetVK(testNK, "ValueT")
	require.True(t, found)
}

func TestBuilder_OpenExisting(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create initial hive
	b1, err := New(hivePath, nil)
	require.NoError(t, err)

	err = b1.SetString([]string{"Initial"}, "Value", "First")
	require.NoError(t, err)

	err = b1.Commit()
	require.NoError(t, err)

	// Open existing hive and add more data
	b2, err := New(hivePath, nil)
	require.NoError(t, err)
	defer b2.Close()

	err = b2.SetString([]string{"Second"}, "Value", "Second")
	require.NoError(t, err)

	err = b2.Commit()
	require.NoError(t, err)

	// Verify both keys exist
	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	builder := walker.NewIndexBuilder(h, 1000, 1000)
	idx, err := builder.Build()
	require.NoError(t, err)

	rootNK := h.RootCellOffset()

	_, found := idx.GetNK(rootNK, "Initial")
	require.True(t, found, "Initial key should exist")

	_, found = idx.GetNK(rootNK, "Second")
	require.True(t, found, "Second key should exist")
}

func TestBuilder_ClosedBuilder(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create and close builder
	b, err := New(hivePath, nil)
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Try to use closed builder
	err = b.SetString([]string{"Test"}, "Value", "Test")
	require.Error(t, err, "should error on closed builder")
}
