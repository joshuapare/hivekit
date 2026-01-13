package builder

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/stretchr/testify/require"
)

// TestFindParts_WithHiveRootPrefix verifies that FindParts correctly strips
// hive root prefixes, matching the user's exact use case.
func TestFindParts_WithHiveRootPrefix(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	b, err := New(out, DefaultOptions())
	require.NoError(t, err)
	defer b.Close()

	// Simulate user's assembler code: manually split path with HKEY_LOCAL_MACHINE
	fullKey := "HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp"
	pathSegments := strings.Split(fullKey, "\\")
	// Result: ["HKEY_LOCAL_MACHINE", "SOFTWARE", "TestApp"]

	err = b.SetString(pathSegments, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.SetString(pathSegments, "Publisher", "Acme Corp")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	b.Close()

	// Reopen and test FindParts
	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	// Test FindParts with various path formats (all should work!)
	testCases := []struct {
		name     string
		parts    []string
		caseName string // What case variation
	}{
		{
			name:     "exact_match",
			parts:    []string{"HKEY_LOCAL_MACHINE", "SOFTWARE", "TestApp"},
			caseName: "exact case",
		},
		{
			name:     "uppercase_key",
			parts:    []string{"HKEY_LOCAL_MACHINE", "SOFTWARE", "TESTAPP"},
			caseName: "uppercase key name",
		},
		{
			name:     "short_prefix",
			parts:    []string{"HKLM", "SOFTWARE", "TestApp"},
			caseName: "short HKLM prefix",
		},
		{
			name:     "no_prefix",
			parts:    []string{"SOFTWARE", "TestApp"},
			caseName: "no prefix",
		},
		{
			name:     "lowercase_all",
			parts:    []string{"hkey_local_machine", "software", "testapp"},
			caseName: "all lowercase",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// FindParts should find the key
			node, err := h.FindParts(tc.parts)
			require.NoError(t, err, "FindParts(%v) should work (%s)", tc.parts, tc.caseName)
			require.NotZero(t, node)

			// Should be able to list values
			pathStr := JoinPath(tc.parts)
			vals, err := h.ListValues(pathStr)
			require.NoError(t, err, "ListValues should work for %s", tc.caseName)
			require.Equal(t, 2, len(vals), "Should have 2 values")

			// Should be able to get values
			version, err := h.GetString(pathStr, "Version")
			require.NoError(t, err)
			require.Equal(t, "1.0.0", version)

			publisher, err := h.GetString(pathStr, "Publisher")
			require.NoError(t, err)
			require.Equal(t, "Acme Corp", publisher)

			t.Logf("✓ FindParts with %s works!", tc.caseName)
		})
	}
}

// TestFindParts_EmptyKey verifies FindParts works for empty keys too.
func TestFindParts_EmptyKey(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	b, err := New(out, DefaultOptions())
	require.NoError(t, err)
	defer b.Close()

	// Create a key with no values
	fullKey := "HKEY_LOCAL_MACHINE\\SOFTWARE\\EmptyKey"
	pathSegments := strings.Split(fullKey, "\\")

	err = b.EnsureKey(pathSegments)
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	b.Close()

	// Reopen and find the empty key
	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	// FindParts should find it even though it has no values
	node, err := h.FindParts([]string{"HKEY_LOCAL_MACHINE", "SOFTWARE", "EmptyKey"})
	require.NoError(t, err, "FindParts should find empty key")
	require.NotZero(t, node)

	// ListValues should return empty list, not error
	vals, err := h.ListValues("SOFTWARE\\EmptyKey")
	require.NoError(t, err, "ListValues on empty key should not error")
	require.Equal(t, 0, len(vals), "Empty key should have 0 values")

	t.Log("✓ FindParts works for empty keys")
}
