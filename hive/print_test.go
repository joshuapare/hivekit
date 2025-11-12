package hive

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive/printer"
	"github.com/stretchr/testify/require"
)

func TestHive_PrintKey(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.ShowValues = true

	err = h.PrintKey(&buf, "", opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("PrintKey output:\n%s", output)

	// Check basic structure
	require.Greater(t, len(output), 0)
	require.Contains(t, output, "[")
}

func TestHive_PrintKey_JSON(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.Format = printer.FormatJSON
	opts.ShowValues = true

	err = h.PrintKey(&buf, "", opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("PrintKey JSON output:\n%s", output)

	// Verify valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
}

func TestHive_PrintValue(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Get a value to test with
	values, err := h.ListValues("")
	require.NoError(t, err)

	if len(values) == 0 {
		t.Skip("No values to test")
	}

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.ShowValueTypes = true

	err = h.PrintValue(&buf, "", values[0].Name, opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("PrintValue output:\n%s", output)

	require.Greater(t, len(output), 0)
}

func TestHive_PrintTree(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.ShowValues = true
	opts.MaxDepth = 2

	err = h.PrintTree(&buf, "", opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("PrintTree output (truncated):\n%s", output[:min(len(output), 500)])

	require.Greater(t, len(output), 0)
}

func TestHive_PrintTree_RegFormat(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.Format = printer.FormatReg
	opts.ShowValues = true
	opts.MaxDepth = 2

	err = h.PrintTree(&buf, "", opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf(".reg output (truncated):\n%s", output[:min(len(output), 500)])

	// Check .reg format
	require.Contains(t, output, "Windows Registry Editor Version 5.00")

	// Should have multiple keys
	keyCount := strings.Count(output, "[")
	require.Greater(t, keyCount, 0)
	t.Logf("Found %d keys in .reg output", keyCount)
}

func TestHive_PrintTree_JSONFormat(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "minimal")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.Format = printer.FormatJSON
	opts.ShowValues = true
	opts.MaxDepth = 2

	err = h.PrintTree(&buf, "", opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("JSON tree output (truncated):\n%s", output[:min(len(output), 500)])

	// Verify valid JSON
	var result map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
}

func TestHive_PrintKey_WithPath(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "large")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Get first subkey
	keys, err := h.ListSubkeys("")
	require.NoError(t, err)

	if len(keys) == 0 {
		t.Skip("No subkeys to test")
	}

	var buf bytes.Buffer
	opts := printer.DefaultOptions()
	opts.ShowValues = true

	err = h.PrintKey(&buf, keys[0].Name, opts)
	require.NoError(t, err)

	output := buf.String()
	t.Logf("PrintKey with path output:\n%s", output)

	require.Greater(t, len(output), 0)
	require.Contains(t, output, keys[0].Name)
}

func TestHive_Print_Integration(t *testing.T) {
	hivePath := filepath.Join("..", "testdata", "large")
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skip("test hive not found")
	}

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	t.Run("text format", func(t *testing.T) {
		var buf bytes.Buffer
		opts := printer.DefaultOptions()
		opts.Format = printer.FormatText
		opts.ShowValues = true
		opts.MaxDepth = 1

		err = h.PrintTree(&buf, "", opts)
		require.NoError(t, err)
		require.Greater(t, buf.Len(), 0)
	})

	t.Run("json format", func(t *testing.T) {
		var buf bytes.Buffer
		opts := printer.DefaultOptions()
		opts.Format = printer.FormatJSON
		opts.ShowValues = true
		opts.MaxDepth = 1

		err = h.PrintTree(&buf, "", opts)
		require.NoError(t, err)

		// Verify valid JSON
		var result map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &result)
		require.NoError(t, err)
	})

	t.Run("reg format", func(t *testing.T) {
		var buf bytes.Buffer
		opts := printer.DefaultOptions()
		opts.Format = printer.FormatReg
		opts.ShowValues = true
		opts.MaxDepth = 1

		err = h.PrintTree(&buf, "", opts)
		require.NoError(t, err)

		output := buf.String()
		require.Contains(t, output, "Windows Registry Editor Version 5.00")
	})
}
