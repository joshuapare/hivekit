package printer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
	"github.com/stretchr/testify/require"
)

// openTestHive opens a test hive file and returns a Reader.
func openTestHive(t *testing.T, name string) types.Reader {
	t.Helper()

	hivePath := filepath.Join("..", "..", "testdata", name)
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skipf("test hive %s not found", name)
	}

	r, err := reader.Open(hivePath, types.OpenOptions{})
	require.NoError(t, err)
	return r
}

func TestPrinter_PrintKey_Text(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatText
	opts.ShowValues = true

	p := New(r, &buf, opts)
	err := p.PrintKey("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Text output:\n%s", output)

	// Check that output contains expected elements
	require.Contains(t, output, "[")          // Key name in brackets
	require.Contains(t, output, "Subkeys:")   // Metadata
	require.Contains(t, output, "Values:")    // Metadata
}

func TestPrinter_PrintKey_JSON(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatJSON
	opts.ShowValues = true

	p := New(r, &buf, opts)
	err := p.PrintKey("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("JSON output:\n%s", output)

	// Verify it's valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Check structure
	require.Contains(t, result, "name")
	require.Contains(t, result, "subkeys")
	require.Contains(t, result, "values")
}

func TestPrinter_PrintKey_Reg(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatReg
	opts.ShowValues = true

	p := New(r, &buf, opts)
	err := p.PrintKey("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf(".reg output:\n%s", output)

	// Check .reg format header
	require.Contains(t, output, "Windows Registry Editor Version 5.00")
	require.Contains(t, output, "[")
	require.Contains(t, output, "]")
}

func TestPrinter_PrintTree_Text(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatText
	opts.ShowValues = true
	opts.MaxDepth = 2 // Limit depth for testing

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Tree output (truncated):\n%s", output[:min(len(output), 500)])

	// Should contain at least the root key
	require.Greater(t, len(output), 0)
}

func TestPrinter_PrintTree_JSON(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatJSON
	opts.ShowValues = true
	opts.MaxDepth = 2

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("JSON tree output (truncated):\n%s", output[:min(len(output), 500)])

	// Verify it's valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Check for children
	if children, ok := result["children"].([]interface{}); ok && len(children) > 0 {
		t.Logf("Found %d children in JSON tree", len(children))
	}
}

func TestPrinter_PrintTree_Reg(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatReg
	opts.ShowValues = true
	opts.MaxDepth = 2

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf(".reg tree output (truncated):\n%s", output[:min(len(output), 500)])

	// Check .reg format
	require.Contains(t, output, "Windows Registry Editor Version 5.00")

	// Should contain at least one key
	keyCount := strings.Count(output, "[")
	require.Greater(t, keyCount, 0, "should have at least one key")
	t.Logf("Found %d keys in .reg output", keyCount)
}

func TestPrinter_Options_ShowTimestamps(t *testing.T) {
	r := openTestHive(t, "minimal")

	// Without timestamps
	var buf1 bytes.Buffer
	opts1 := DefaultOptions()
	opts1.ShowTimestamps = false
	p1 := New(r, &buf1, opts1)
	err := p1.PrintKey("")
	require.NoError(t, err)

	// With timestamps
	var buf2 bytes.Buffer
	opts2 := DefaultOptions()
	opts2.ShowTimestamps = true
	p2 := New(r, &buf2, opts2)
	err = p2.PrintKey("")
	require.NoError(t, err)

	// Second output should contain timestamp info
	output1 := buf1.String()
	output2 := buf2.String()

	require.NotContains(t, output1, "Last Write")
	require.Contains(t, output2, "Last Write")
}

func TestPrinter_Options_ShowValueTypes(t *testing.T) {
	r := openTestHive(t, "minimal")

	// Get root to check if it has values
	root, err := r.Root()
	require.NoError(t, err)

	values, err := r.Values(root)
	require.NoError(t, err)

	if len(values) == 0 {
		t.Skip("Root has no values to test")
	}

	// Without value types
	var buf1 bytes.Buffer
	opts1 := DefaultOptions()
	opts1.ShowValues = true
	opts1.ShowValueTypes = false
	p1 := New(r, &buf1, opts1)
	err = p1.PrintKey("")
	require.NoError(t, err)

	// With value types
	var buf2 bytes.Buffer
	opts2 := DefaultOptions()
	opts2.ShowValues = true
	opts2.ShowValueTypes = true
	p2 := New(r, &buf2, opts2)
	err = p2.PrintKey("")
	require.NoError(t, err)

	t.Logf("Without types:\n%s", buf1.String())
	t.Logf("With types:\n%s", buf2.String())
}

func TestPrinter_Options_MaxDepth(t *testing.T) {
	r := openTestHive(t, "large")

	// Depth 1
	var buf1 bytes.Buffer
	opts1 := DefaultOptions()
	opts1.MaxDepth = 1
	p1 := New(r, &buf1, opts1)
	err := p1.PrintTree("")
	require.NoError(t, err)

	// Depth 3
	var buf2 bytes.Buffer
	opts2 := DefaultOptions()
	opts2.MaxDepth = 3
	p2 := New(r, &buf2, opts2)
	err = p2.PrintTree("")
	require.NoError(t, err)

	// Deeper tree should have more content
	output1 := buf1.String()
	output2 := buf2.String()

	t.Logf("Depth 1: %d bytes", len(output1))
	t.Logf("Depth 3: %d bytes", len(output2))

	require.Less(t, len(output1), len(output2), "deeper tree should have more content")
}

func TestPrinter_Options_MaxValueBytes(t *testing.T) {
	r := openTestHive(t, "minimal")

	// Find a value to test with
	root, err := r.Root()
	require.NoError(t, err)

	values, err := r.Values(root)
	require.NoError(t, err)

	if len(values) == 0 {
		t.Skip("No values to test")
	}

	// Use small MaxValueBytes
	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.ShowValues = true
	opts.MaxValueBytes = 8 // Very small limit
	p := New(r, &buf, opts)
	err = p.PrintKey("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Output with MaxValueBytes=8:\n%s", output)

	// If we have binary values, should see truncation
	if strings.Contains(output, "truncated") {
		t.Log("Successfully truncated large binary value")
	}
}

func TestPrinter_PrintValue(t *testing.T) {
	r := openTestHive(t, "minimal")

	// Get root and find a value
	root, err := r.Root()
	require.NoError(t, err)

	values, err := r.Values(root)
	require.NoError(t, err)

	if len(values) == 0 {
		t.Skip("No values to test")
	}

	// Get first value's metadata
	valMeta, err := r.StatValue(values[0])
	require.NoError(t, err)

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.ShowValueTypes = true
	p := New(r, &buf, opts)

	err = p.PrintValue("", valMeta.Name)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Value output:\n%s", output)

	require.Greater(t, len(output), 0)
}

func TestPrinter_AllValueTypes(t *testing.T) {
	// This test would require a hive with all value types
	// For now, we just test that the printer doesn't crash on various types

	r := openTestHive(t, "large")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.ShowValues = true
	opts.MaxDepth = 3

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Processed large hive: %d bytes output", len(output))

	// Just verify we got some output
	require.Greater(t, len(output), 100)
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	require.Equal(t, FormatText, opts.Format)
	require.Equal(t, 2, opts.IndentSize)
	require.Equal(t, 0, opts.MaxDepth)
	require.True(t, opts.ShowValues)
	require.False(t, opts.ShowTimestamps)
	require.True(t, opts.ShowValueTypes)
	require.False(t, opts.Recursive)
	require.Equal(t, 32, opts.MaxValueBytes)
}
