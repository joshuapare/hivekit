package hive_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
)

func TestCheckKeyPresence_AllExist(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with some keys
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"Software", "Microsoft"}, "Test", "Value")
	require.NoError(t, err)

	err = b.SetString([]string{"System", "CurrentControlSet"}, "Test", "Value")
	require.NoError(t, err)

	err = b.SetString([]string{"Hardware"}, "Test", "Value")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Check presence of keys that all exist
	result, err := hive.CheckKeyPresence(hivePath, []string{
		"Software\\Microsoft",
		"System\\CurrentControlSet",
		"Hardware",
	})
	require.NoError(t, err)
	require.Empty(t, result.Missing, "all keys should exist")
}

func TestCheckKeyPresence_SomeMissing(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with some keys
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"Software", "Microsoft"}, "Test", "Value")
	require.NoError(t, err)

	err = b.SetString([]string{"System"}, "Test", "Value")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Check presence with some missing
	result, err := hive.CheckKeyPresence(hivePath, []string{
		"Software\\Microsoft",  // exists
		"System",                // exists
		"Hardware",              // missing
		"NonExistent\\Nested",   // missing
	})
	require.NoError(t, err)
	require.Len(t, result.Missing, 2)
	require.Contains(t, result.Missing, "Hardware")
	require.Contains(t, result.Missing, "NonExistent\\Nested")
}

func TestCheckKeyPresence_AllMissing(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Check presence of keys that don't exist
	result, err := hive.CheckKeyPresence(hivePath, []string{
		"Software",
		"System",
		"Hardware",
	})
	require.NoError(t, err)
	require.Len(t, result.Missing, 3)
	require.Equal(t, []string{"Software", "System", "Hardware"}, result.Missing)
}

func TestCheckKeyPresence_EmptyList(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Check with empty list
	result, err := hive.CheckKeyPresence(hivePath, []string{})
	require.NoError(t, err)
	require.Empty(t, result.Missing)
}

func TestCheckKeyPresence_RootKey(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Check root key (should always exist)
	result, err := hive.CheckKeyPresence(hivePath, []string{
		"",      // root (empty path)
		"\\",    // root (backslash)
		"/",     // root (forward slash)
	})
	require.NoError(t, err)
	require.Empty(t, result.Missing, "root key should always exist")
}

func TestCheckKeyPresence_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with keys
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"Software", "Microsoft"}, "Test", "Value")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Check with different casing (should all be found)
	result, err := hive.CheckKeyPresence(hivePath, []string{
		"SOFTWARE\\MICROSOFT",   // uppercase
		"software\\microsoft",   // lowercase
		"SoFtWaRe\\MiCrOsOfT",   // mixed case
	})
	require.NoError(t, err)
	require.Empty(t, result.Missing, "lookups should be case-insensitive")
}

func TestCheckKeyPresence_WithPrefixes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with keys
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"Software"}, "Test", "Value")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Check with various hive root prefixes (should all be stripped and found)
	result, err := hive.CheckKeyPresence(hivePath, []string{
		"Software",                        // no prefix
		"HKEY_LOCAL_MACHINE\\Software",   // full prefix
		"HKLM\\Software",                  // short prefix
	})
	require.NoError(t, err)
	require.Empty(t, result.Missing, "prefixes should be stripped automatically")
}

func TestCheckKeyPresence_InvalidHivePath(t *testing.T) {
	// Try to check presence in non-existent hive
	_, err := hive.CheckKeyPresence("/nonexistent/path/to/hive", []string{"Software"})
	require.Error(t, err, "should error on invalid hive path")
}
