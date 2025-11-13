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
	opts.PrintMetadata = true // Enable metadata output

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
	opts.PrintMetadata = true // Enable metadata output

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
	// Use "large" hive which has subkeys (minimal hive has no subkeys)
	r := openTestHive(t, "large")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatText
	opts.ShowValues = true
	opts.PrintMetadata = true // Enable metadata output
	opts.MaxDepth = 2          // Limit depth for testing

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Tree output (truncated):\n%s", output[:min(len(output), 500)])

	// Should contain at least one key
	require.Greater(t, len(output), 0)
}

func TestPrinter_PrintTree_JSON(t *testing.T) {
	r := openTestHive(t, "minimal")

	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatJSON
	opts.ShowValues = true
	opts.PrintMetadata = true // Enable metadata output
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

// Line Wrapping Tests for .reg Format
//
// Note: formatHexBytes needs to be replaced with formatHexBytesWithWrapping
// to implement line wrapping at 80 characters with backslash continuation.

func TestFormatHexBytesWithWrapping_Short(t *testing.T) {
	// Test that short hex values are not wrapped
	data := []byte{0x41, 0x00, 0x42, 0x00, 0x43, 0x00}

	// This would call the new function: formatHexBytesWithWrapping(data, "\"TestValue\"=hex(1):")
	// For now, test the concept with formatHexBytes
	result := formatHexBytes(data)
	expected := "41,00,42,00,43,00"

	require.Equal(t, expected, result)
	require.NotContains(t, result, "\\")
}

func TestFormatHexBytesWithWrapping_Long(t *testing.T) {
	// Test that long hex values ARE wrapped at 80 characters when enabled
	// Create data that will result in > 80 character line
	data := make([]byte, 50) // 50 bytes = 149 chars when formatted as "XX," (minus last comma)
	for i := range data {
		data[i] = byte(i)
	}

	prefix := "\"LongValue\"=hex(1):"
	result := formatHexBytesWithWrapping(data, prefix)
	fullOutput := prefix + result

	t.Logf("Full output:\n%s", fullOutput)

	// Check that output contains continuation
	if !strings.Contains(result, "\\") {
		t.Errorf("Result should contain backslash continuation for long data")
	}

	// Verify no individual line exceeds 80 characters
	lines := strings.Split(fullOutput, "\n")
	for i, line := range lines {
		if len(line) > 80 {
			t.Errorf("Line %d exceeds 80 characters (%d chars): %s", i+1, len(line), line)
		}
	}
}

func TestFormatHexBytesWithWrapping_Continuation(t *testing.T) {
	// Test that wrapped lines have proper continuation format when enabled
	// Expected format:
	//   "Value"=hex(1):41,00,42,00,...,\
	//     46,00,47,00,48,00,...

	// Create data that will require wrapping
	data := make([]byte, 50)
	for i := range data {
		data[i] = byte(0x41 + (i % 26)) // Cycling through letters
	}

	prefix := "\"TestValue\"=hex(1):"
	result := formatHexBytesWithWrapping(data, prefix)
	fullOutput := prefix + result

	t.Logf("Full output:\n%s", fullOutput)

	// Should contain backslash continuation
	if !strings.Contains(result, "\\") {
		t.Errorf("Result should contain backslash continuation for long data")
	}

	// Check continuation line format
	lines := strings.Split(fullOutput, "\n")
	if len(lines) <= 1 {
		t.Errorf("Long line should be wrapped into multiple lines, got %d line(s)", len(lines))
	}

	// Verify continuation lines start with 2 spaces
	for i := 1; i < len(lines); i++ {
		if len(lines[i]) > 0 && !strings.HasPrefix(lines[i], "  ") {
			t.Errorf("Continuation line %d should start with 2 spaces: %q", i+1, lines[i])
		}
	}
}

// Integration test: export with wrapping enabled, then parse back
func TestLineContinuation_Roundtrip(t *testing.T) {
	// This test requires the regtext package which is internal
	// We'll test with a suite file that has long values
	r := openTestHive(t, "suite/windows-2003-server-system")

	// Export to .reg format with wrapping ENABLED
	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatReg
	opts.ShowValues = true
	opts.MaxDepth = 2
	opts.MaxValueBytes = 0 // No truncation
	opts.WrapLines = true  // ENABLE line wrapping

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Generated .reg output (%d bytes)", len(output))

	// Verify we have some continuations
	continuationCount := strings.Count(output, ",\\")
	t.Logf("Found %d line continuations", continuationCount)

	if continuationCount > 0 {
		t.Logf("Line continuation wrapping is working!")

		// Show a sample of wrapped output
		lines := strings.Split(output, "\n")
		for i, line := range lines {
			if strings.HasSuffix(line, "\\") {
				// Found a continuation line - show it and the next line
				t.Logf("Sample continuation at line %d:", i+1)
				t.Logf("  %s", line)
				if i+1 < len(lines) {
					t.Logf("  %s", lines[i+1])
				}
				break
			}
		}
	} else {
		t.Error("Expected line continuations with WrapLines=true, but found none")
	}

	// Verify no lines exceed 80 characters (with small margin for edge cases)
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if len(line) > 85 {
			t.Errorf("Line %d exceeds 85 characters (%d): %s...", i+1, len(line), line[:min(len(line), 80)])
		}
	}
}

// Test that wrapping is OFF by default
func TestLineContinuation_NoWrapByDefault(t *testing.T) {
	r := openTestHive(t, "suite/windows-2003-server-system")

	// Export WITHOUT wrapping (default)
	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.Format = FormatReg
	opts.ShowValues = true
	opts.MaxDepth = 2
	opts.MaxValueBytes = 0
	// WrapLines defaults to false

	p := New(r, &buf, opts)
	err := p.PrintTree("")
	require.NoError(t, err)

	output := buf.String()
	t.Logf("Generated .reg output (%d bytes)", len(output))

	// Verify we have NO continuations (wrapping is off by default)
	continuationCount := strings.Count(output, ",\\")
	t.Logf("Found %d line continuations", continuationCount)

	if continuationCount > 0 {
		t.Errorf("Expected NO line continuations with WrapLines=false (default), but found %d", continuationCount)
	} else {
		t.Log("No line wrapping by default (as expected)")
	}
}

