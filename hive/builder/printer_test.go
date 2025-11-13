package builder

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/printer"
	"github.com/stretchr/testify/require"
)

// TestPrinterNoDuplicateRoots verifies that the REG printer doesn't
// output duplicate root entries after the path normalization fix.
func TestPrinterNoDuplicateRoots(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	b, err := New(out, DefaultOptions())
	require.NoError(t, err)
	defer b.Close()

	// Create keys with HKEY_LOCAL_MACHINE prefix (simulating user's code)
	fullKey := "HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp"
	pathSegments := strings.Split(fullKey, "\\")

	err = b.SetString(pathSegments, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.SetString(pathSegments, "Publisher", "Acme Corp")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	b.Close()

	// Reopen and print
	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	opts := printer.DefaultOptions()
	opts.Format = printer.FormatReg

	var buf bytes.Buffer
	err = h.PrintTree(&buf, "", opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("REG Output:\n%s", output)

	// Check for duplicate root entries
	lines := strings.Split(output, "\n")
	rootCount := 0
	emptyBracketCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[\\]" || trimmed == "[]" {
			rootCount++
			t.Logf("Found root entry: %q", line)
		}
	}

	// Should have exactly ONE root entry (or possibly zero if printer skips it)
	require.LessOrEqual(t, rootCount, 1, "Should not have duplicate root entries")

	// Verify the actual key structure is correct
	require.Contains(t, output, "[\\SOFTWARE]", "Should contain SOFTWARE key")
	require.Contains(t, output, "[\\SOFTWARE\\TestApp]", "Should contain TestApp key")
	require.Contains(t, output, "Version", "Should contain Version value")
	require.Contains(t, output, "Publisher", "Should contain Publisher value")

	// Count how many times [\] appears
	for _, line := range lines {
		if strings.TrimSpace(line) == "[\\]" {
			emptyBracketCount++
		}
	}

	if emptyBracketCount > 1 {
		t.Errorf("DUPLICATE ROOT BUG: Found %d instances of [\\]", emptyBracketCount)
		t.Logf("Full output:\n%s", output)
	}
}

// TestPrinterBeforeNormalizationFix shows what the output looked like
// before the fix (for documentation purposes).
func TestPrinterWithManualPrefix(t *testing.T) {
	t.Skip("This test documents the old broken behavior - skip by default")

	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	opts := DefaultOptions()
	// Simulate old behavior by NOT normalizing
	// (Note: we can't actually test this anymore since we fixed it!)

	b, err := New(out, opts)
	require.NoError(t, err)
	defer b.Close()

	// This would have created HKEY_LOCAL_MACHINE as literal key
	err = b.SetString([]string{"SOFTWARE", "TestApp"}, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	b.Close()

	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	prOpts := printer.DefaultOptions()
	prOpts.Format = printer.FormatReg

	var buf bytes.Buffer
	err = h.PrintTree(&buf, "", prOpts)
	require.NoError(t, err)

	t.Logf("Correct Output (after fix):\n%s", buf.String())
}
