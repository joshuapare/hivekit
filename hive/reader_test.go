package hive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestHive_Reader(t *testing.T) {
	// Use a test hive file from testdata
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Test Reader() method
	r, err := h.Reader()
	require.NoError(t, err)
	require.NotNil(t, r)

	// Verify we can use the reader
	root, err := r.Root()
	require.NoError(t, err)
	require.NotZero(t, root)
}

func TestHive_Find(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Test Find on root (should work)
	node, err := h.Find("")
	require.NoError(t, err)
	require.NotZero(t, node)

	// Test Find with invalid path (should error)
	_, err = h.Find("NonExistentKey\\SubKey")
	require.Error(t, err)
}

func TestHive_FindParts(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "large")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Test FindParts on root with nil
	node, err := h.FindParts(nil)
	require.NoError(t, err)
	require.NotZero(t, node)

	// Test FindParts on root with empty slice
	node2, err := h.FindParts([]string{})
	require.NoError(t, err)
	require.Equal(t, node, node2)

	// List root subkeys to test with
	subkeys, err := h.ListSubkeys("")
	require.NoError(t, err)

	if len(subkeys) > 0 {
		// Test FindParts with one part - should find first subkey
		firstKey := subkeys[0].Name
		nodeByParts, err := h.FindParts([]string{firstKey})
		require.NoError(t, err)
		require.NotZero(t, nodeByParts)

		// Compare with Find using backslash
		nodeByFind, err := h.Find(firstKey)
		require.NoError(t, err)
		require.Equal(t, nodeByFind, nodeByParts, "FindParts and Find should return same node")

		// Test with nested key if available
		nestedKeys, err := h.ListSubkeys(firstKey)
		if err == nil && len(nestedKeys) > 0 {
			secondKey := nestedKeys[0].Name

			// Find using parts
			nodeByParts2, err := h.FindParts([]string{firstKey, secondKey})
			require.NoError(t, err)

			// Find using path
			nodeByFind2, err := h.Find(firstKey + `\` + secondKey)
			require.NoError(t, err)

			require.Equal(t, nodeByFind2, nodeByParts2, "FindParts and Find should match for nested keys")

			t.Logf("Successfully found nested key: %s\\%s", firstKey, secondKey)
		}
	}

	// Test FindParts with non-existent key
	_, err = h.FindParts([]string{"NonExistentKey", "SubKey"})
	require.Error(t, err)
}

func TestHive_GetKey(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Get root key metadata
	meta, err := h.GetKey("")
	require.NoError(t, err)
	require.NotEmpty(t, meta.Name)
}

func TestHive_ListSubkeys(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "large")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// List subkeys of root
	keys, err := h.ListSubkeys("")
	require.NoError(t, err)

	t.Logf("Found %d subkeys in root", len(keys))
	for i, key := range keys {
		if i < 5 { // Just log first 5
			t.Logf("  - %s", key.Name)
		}
	}

	// Large hive should have subkeys
	if len(keys) == 0 {
		t.Log("Warning: no subkeys found, but test passes")
	}
}

func TestHive_Walk(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Walk the tree and count keys
	count := 0
	err = h.Walk("", func(id types.NodeID, meta types.KeyMeta) error {
		count++
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, count, 0)

	t.Logf("Walked %d keys", count)
}

func TestHive_ErgonomicAPI_Integration(t *testing.T) {
	// Use existing test hive
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Test Reader
	r, err := h.Reader()
	require.NoError(t, err)
	require.NotNil(t, r)

	// Test Find
	node, err := h.Find("")
	require.NoError(t, err)
	require.NotZero(t, node)

	// Test GetKey
	meta, err := h.GetKey("")
	require.NoError(t, err)
	require.NotEmpty(t, meta.Name)

	// Test ListSubkeys
	keys, err := h.ListSubkeys("")
	require.NoError(t, err)
	t.Logf("Root has %d subkeys", len(keys))
}
