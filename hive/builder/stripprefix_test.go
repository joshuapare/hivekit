package builder

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/pkg/types"
	"github.com/stretchr/testify/require"
)

// TestStripHiveRootPrefixes_False reproduces the user's issue where
// StripHiveRootPrefixes=false doesn't work as expected when using raw path arrays.
func TestStripHiveRootPrefixes_False(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	opts := DefaultOptions()
	opts.StripHiveRootPrefixes = false

	b, err := New(out, opts)
	require.NoError(t, err)
	defer b.Close()

	// Simulate what the user's assembler does:
	// Manually split "HKEY_LOCAL_MACHINE\SOFTWARE\TestApp" by backslash
	fullKey := "HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp"
	pathSegments := strings.Split(fullKey, "\\")
	t.Logf("Path segments from manual split: %v", pathSegments)
	// Expected: ["HKEY_LOCAL_MACHINE", "SOFTWARE", "TestApp"]

	// Set a value using the manually split path
	err = b.SetString(pathSegments, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	err = b.Close()
	require.NoError(t, err)

	// Now try to read it back
	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	// Print the tree to see what was actually created
	t.Log("=== Tree Structure ===")
	err = h.Walk("", func(id types.NodeID, meta types.KeyMeta) error {
		t.Logf("Key: %s", meta.Name)
		return nil
	})
	require.NoError(t, err)

	// Try different query patterns to find the key
	testCases := []struct {
		name  string
		query string
	}{
		{"exact_case", "HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp"},
		{"uppercase", "HKEY_LOCAL_MACHINE\\SOFTWARE\\TESTAPP"},
		{"no_prefix", "SOFTWARE\\TestApp"},
		{"no_prefix_upper", "SOFTWARE\\TESTAPP"},
		{"full_path", "HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vals, err := h.ListValues(tc.query)
			if err != nil {
				t.Logf("ListValues(%q) failed: %v", tc.query, err)
			} else {
				t.Logf("ListValues(%q) returned %d values", tc.query, len(vals))
				for _, v := range vals {
					t.Logf("  - %s (type %d)", v.Name, v.Type)
				}
			}
		})
	}
}

// TestStripHiveRootPrefixes_ManualArrayVsString tests the difference between
// manually creating path arrays vs using string-based methods.
func TestStripHiveRootPrefixes_ManualArrayVsString(t *testing.T) {
	tmp := t.TempDir()

	// Test 1: StripHiveRootPrefixes=false with manual array
	t.Run("manual_array_nostrip", func(t *testing.T) {
		out := filepath.Join(tmp, "manual_nostrip.hiv")
		opts := DefaultOptions()
		opts.StripHiveRootPrefixes = false

		b, err := New(out, opts)
		require.NoError(t, err)
		defer b.Close()

		// Manual split (what user is doing)
		pathSegments := strings.Split("HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp", "\\")
		err = b.SetString(pathSegments, "Test1", "value1")
		require.NoError(t, err)

		err = b.Commit()
		require.NoError(t, err)
		b.Close()

		h, err := hive.Open(out)
		require.NoError(t, err)
		defer h.Close()

		t.Log("=== Manual Array, NoStrip ===")
		dumpTree(t, h)
	})

	// Test 2: StripHiveRootPrefixes=false with SetValueFromString
	t.Run("string_method_nostrip", func(t *testing.T) {
		out := filepath.Join(tmp, "string_nostrip.hiv")
		opts := DefaultOptions()
		opts.StripHiveRootPrefixes = false

		b, err := New(out, opts)
		require.NoError(t, err)
		defer b.Close()

		// Using SetValueFromString (which calls SplitPath internally)
		err = b.SetValueFromString("HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp", "Test2", "REG_SZ", EncodeStringHelper("value2"))
		require.NoError(t, err)

		err = b.Commit()
		require.NoError(t, err)
		b.Close()

		h, err := hive.Open(out)
		require.NoError(t, err)
		defer h.Close()

		t.Log("=== String Method, NoStrip ===")
		dumpTree(t, h)
	})

	// Test 3: StripHiveRootPrefixes=true (default) with manual array
	t.Run("manual_array_strip", func(t *testing.T) {
		out := filepath.Join(tmp, "manual_strip.hiv")
		opts := DefaultOptions()
		opts.StripHiveRootPrefixes = true

		b, err := New(out, opts)
		require.NoError(t, err)
		defer b.Close()

		// Manual split - note: this bypasses the stripping logic!
		pathSegments := strings.Split("HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp", "\\")
		err = b.SetString(pathSegments, "Test3", "value3")
		require.NoError(t, err)

		err = b.Commit()
		require.NoError(t, err)
		b.Close()

		h, err := hive.Open(out)
		require.NoError(t, err)
		defer h.Close()

		t.Log("=== Manual Array, Strip ===")
		dumpTree(t, h)
	})
}

// TestSplitPathBehavior tests the difference between SplitPath and manual splitting.
func TestSplitPathBehavior(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "with_hklm_prefix",
			input:    "HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp",
			expected: []string{"SOFTWARE", "TestApp"},
		},
		{
			name:     "with_hklm_short",
			input:    "HKLM\\SOFTWARE\\TestApp",
			expected: []string{"SOFTWARE", "TestApp"},
		},
		{
			name:     "no_prefix",
			input:    "SOFTWARE\\TestApp",
			expected: []string{"SOFTWARE", "TestApp"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test SplitPath (always strips)
			splitResult := SplitPath(tc.input)
			t.Logf("SplitPath(%q) = %v", tc.input, splitResult)
			require.Equal(t, tc.expected, splitResult, "SplitPath should always strip prefixes")

			// Show what manual split does
			manualSplit := strings.Split(tc.input, "\\")
			t.Logf("strings.Split(%q, \"\\\\\") = %v", tc.input, manualSplit)
		})
	}
}

// Helper to dump tree structure
func dumpTree(t *testing.T, h *hive.Hive) {
	err := h.Walk("", func(id types.NodeID, meta types.KeyMeta) error {
		vals, _ := h.ListValues(JoinPath([]string{meta.Name}))
		t.Logf("  Key: %s (%d values)", meta.Name, len(vals))
		for _, v := range vals {
			t.Logf("    - %s", v.Name)
		}
		return nil
	})
	if err != nil {
		t.Logf("Walk error: %v", err)
	}
}
