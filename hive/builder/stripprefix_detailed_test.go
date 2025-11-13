package builder

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/pkg/types"
	"github.com/stretchr/testify/require"
)

// TestWhereAreTheValues investigates where values are actually stored
// when using manual path arrays with HKEY_LOCAL_MACHINE prefix.
func TestWhereAreTheValues(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	opts := DefaultOptions()
	opts.StripHiveRootPrefixes = false

	b, err := New(out, opts)
	require.NoError(t, err)
	defer b.Close()

	// Create keys with manual split (simulating user's code)
	pathSegments := strings.Split("HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp", "\\")
	t.Logf("Creating value with path: %v", pathSegments)

	err = b.SetString(pathSegments, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	b.Close()

	// Reopen and explore
	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	// Comprehensive walk showing full paths and values
	t.Log("\n=== COMPLETE TREE DUMP ===")
	var walkPath func(path string, id types.NodeID, depth int) error
	walkPath = func(path string, id types.NodeID, depth int) error {
		meta, err := h.GetKey(path)
		if err != nil {
			return err
		}

		indent := strings.Repeat("  ", depth)
		t.Logf("%s[%s]", indent, meta.Name)

		// List values at this key
		vals, err := h.ListValues(path)
		if err != nil {
			t.Logf("%s  ListValues ERROR: %v", indent, err)
		} else if len(vals) > 0 {
			for _, v := range vals {
				t.Logf("%s  VALUE: %s (type %d, size %d)", indent, v.Name, v.Type, v.Size)
			}
		}

		// List subkeys
		subkeys, err := h.ListSubkeys(path)
		if err != nil {
			return err
		}

		for _, sk := range subkeys {
			childPath := path
			if childPath == "" {
				childPath = sk.Name
			} else {
				childPath = childPath + "\\" + sk.Name
			}
			if walkErr := walkPath(childPath, 0, depth+1); walkErr != nil {
				t.Logf("%s  ERROR walking %s: %v", indent, childPath, walkErr)
			}
		}

		return nil
	}

	walkErr := walkPath("", 0, 0)
	require.NoError(t, walkErr)

	// Try to find the key using different paths
	t.Log("\n=== FIND ATTEMPTS ===")
	testPaths := []string{
		"",
		"HKEY_LOCAL_MACHINE",
		"HKEY_LOCAL_MACHINE\\SOFTWARE",
		"HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp",
		"SOFTWARE",
		"SOFTWARE\\TestApp",
		"TestApp",
	}

	for _, testPath := range testPaths {
		node, err := h.Find(testPath)
		if err != nil {
			t.Logf("Find(%q) = ERROR: %v", testPath, err)
		} else {
			meta, _ := h.GetKey(testPath)
			t.Logf("Find(%q) = SUCCESS, key name: %s", testPath, meta.Name)

			vals, err := h.ListValues(testPath)
			if err != nil {
				t.Logf("  ListValues ERROR: %v", err)
			} else {
				t.Logf("  ListValues = %d values", len(vals))
				for _, v := range vals {
					t.Logf("    - %s", v.Name)
				}
			}
		}
		_ = node
	}
}

// TestCorrectUsage shows the correct way to use the builder
// that would work with the reader.
func TestCorrectUsage(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "test.hiv")

	b, err := New(out, DefaultOptions())
	require.NoError(t, err)
	defer b.Close()

	// CORRECT: Use path arrays without hive root prefixes
	err = b.SetString([]string{"SOFTWARE", "TestApp"}, "Version", "1.0.0")
	require.NoError(t, err)

	err = b.SetDWORD([]string{"SOFTWARE", "TestApp"}, "Build", 123)
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)
	b.Close()

	// Reopen and verify
	h, err := hive.Open(out)
	require.NoError(t, err)
	defer h.Close()

	// All these should work:
	testPaths := []string{
		"SOFTWARE\\TestApp",
		"HKEY_LOCAL_MACHINE\\SOFTWARE\\TestApp",
		"HKLM\\SOFTWARE\\TestApp",
		"software\\testapp", // case insensitive
	}

	for _, testPath := range testPaths {
		vals, err := h.ListValues(testPath)
		require.NoError(t, err, "ListValues(%q) should work", testPath)
		require.Equal(t, 2, len(vals), "Should have 2 values at %q", testPath)
		t.Logf("✓ ListValues(%q) = %d values", testPath, len(vals))

		// Verify GetString works
		version, err := h.GetString(testPath, "Version")
		require.NoError(t, err)
		require.Equal(t, "1.0.0", version)

		// Verify GetDWORD works
		build, err := h.GetDWORD(testPath, "Build")
		require.NoError(t, err)
		require.Equal(t, uint32(123), build)
	}
}
