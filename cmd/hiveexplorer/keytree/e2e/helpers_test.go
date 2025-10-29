package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Test helper functions for e2e tests

// assertContains verifies output contains expected substring
func assertContains(t *testing.T, output, expected string) {
	t.Helper()
	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain %q, but it doesn't.\nGot: %s", expected, output)
	}
}

// assertNotContains verifies output does NOT contain substring
func assertNotContains(t *testing.T, output, unwanted string) {
	t.Helper()
	if strings.Contains(output, unwanted) {
		t.Errorf("Expected output NOT to contain %q, but it does.\nGot: %s", unwanted, output)
	}
}

// assertHasIcon verifies output contains specific icon
func assertHasIcon(t *testing.T, output, icon string) {
	t.Helper()
	assertContains(t, output, icon)
}

// assertHasPrefix verifies output has diff prefix at the start
func assertHasPrefix(t *testing.T, output string, prefix rune) {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if len(trimmed) == 0 {
		t.Error("Output is empty, cannot check prefix")
		return
	}
	if rune(trimmed[0]) != prefix {
		t.Errorf("Expected prefix %q but got %q in output: %s", prefix, trimmed[0], output)
	}
}

// assertHasBookmark verifies output has bookmark indicator (★)
func assertHasBookmark(t *testing.T, output string) {
	t.Helper()
	assertContains(t, output, "★")
}

// assertNoBookmark verifies output does NOT have bookmark indicator
func assertNoBookmark(t *testing.T, output string) {
	t.Helper()
	assertNotContains(t, output, "★")
}

// assertIndentation verifies indentation level (depth * 2 spaces)
func assertIndentation(t *testing.T, output string, depth int) {
	t.Helper()
	// After stripping ANSI codes and prefix, count leading spaces before icon
	// Expected: prefix + leftIndicator + space + indent + icon
	// For depth N, we expect N*2 spaces of indentation

	expectedIndent := strings.Repeat("  ", depth)

	// Look for the pattern: some prefix chars, then the indent, then an icon
	// We'll check if the expected indentation appears before a known icon
	icons := []string{"▼", "▶", "•"}
	found := false
	for _, icon := range icons {
		if strings.Contains(output, expectedIndent+icon) {
			found = true
			break
		}
	}

	if !found && depth > 0 {
		t.Errorf("Expected indentation of %d spaces (depth %d) but pattern not found in: %s",
			depth*2, depth, output)
	}
}

// assertHasANSI verifies ANSI styling is present (basic check for escape codes)
func assertHasANSI(t *testing.T, output string) {
	t.Helper()
	if !strings.Contains(output, "\x1b[") {
		t.Error("Expected ANSI escape codes in output for styling, but found none")
	}
}

// ItemOption is a functional option for building test items
type ItemOption func(*keytree.Item)

// newTestItem creates a test Item with sensible defaults
func newTestItem(name string, opts ...ItemOption) keytree.Item {
	item := keytree.Item{
		Name:       name,
		Path:       "TEST\\" + name,
		Depth:      0,
		DiffStatus: hive.DiffUnchanged,
		NodeID:     1,
	}

	for _, opt := range opts {
		opt(&item)
	}

	return item
}

// withDepth sets the depth of a test item
func withDepth(d int) ItemOption {
	return func(i *keytree.Item) {
		i.Depth = d
	}
}

// withChildren sets HasChildren, Expanded state, and SubkeyCount
func withChildren(count int, expanded bool) ItemOption {
	return func(i *keytree.Item) {
		i.HasChildren = true
		i.SubkeyCount = count
		i.Expanded = expanded
	}
}

// withDiffStatus sets the DiffStatus
func withDiffStatus(status hive.DiffStatus) ItemOption {
	return func(i *keytree.Item) {
		i.DiffStatus = status
	}
}

// withTimestamp sets the LastWrite timestamp
func withTimestamp(t time.Time) ItemOption {
	return func(i *keytree.Item) {
		i.LastWrite = t
	}
}

// withPath sets a custom path
func withPath(path string) ItemOption {
	return func(i *keytree.Item) {
		i.Path = path
	}
}
