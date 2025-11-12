package acceptance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestOpen_BasicOpen tests hivex_open with basic usage
// Compares gohivex reader.Open() vs bindings.Open().
func TestOpen_BasicOpen(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{
			name:     "minimal_hive",
			hivePath: TestHives.Minimal,
		},
		{
			name:     "special_chars_hive",
			hivePath: TestHives.Special,
		},
		{
			name:     "rlenvalue_hive",
			hivePath: TestHives.RLenValue,
		},
		{
			name:     "large_hive",
			hivePath: TestHives.Large,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both implementations should successfully open the hive
			goHive, err := reader.Open(tt.hivePath, hive.OpenOptions{})
			require.NoError(t, err, "gohivex failed to open")
			defer goHive.Close()

			hivexHive, err := bindings.Open(tt.hivePath, 0)
			require.NoError(t, err, "hivex failed to open")
			defer hivexHive.Close()

			// Basic sanity: both should be able to get root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Root nodes should be at same offset
			assertSameNodeID(t, goRoot, hivexRoot, "Root node")
		})
	}
}

// TestOpen_OpenBytes tests opening from memory bytes
// hivex doesn't directly support this, but we can verify gohivex works.
func TestOpen_OpenBytes(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"minimal", TestHives.Minimal},
		{"special", TestHives.Special},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load hive data
			data := loadHiveData(t, tt.hivePath)

			// Open from bytes with gohivex
			goHive := openGoHivexBytes(t, data)
			defer goHive.Close()

			// Open from file with hivex (for comparison)
			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Should produce same root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			assertSameNodeID(t, goRoot, hivexRoot)
		})
	}
}

// TestOpen_ZeroCopy tests gohivex's zero-copy mode
// This is a gohivex-specific feature not in hivex.
func TestOpen_ZeroCopy(t *testing.T) {
	data := loadHiveData(t, TestHives.Minimal)

	// Open with zero-copy
	goHive, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
	require.NoError(t, err)
	defer goHive.Close()

	// Open with hivex for comparison
	hivexHive := openHivex(t, TestHives.Minimal)
	defer hivexHive.Close()

	// Should produce same root
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	assertSameNodeID(t, goRoot, hivexRoot)
}

// TestOpen_FileNotFound tests error handling for missing files.
func TestOpen_FileNotFound(t *testing.T) {
	nonexistent := "/tmp/does-not-exist-12345.hive"

	// Both should fail gracefully
	_, goErr := reader.Open(nonexistent, hive.OpenOptions{})
	require.Error(t, goErr, "gohivex should error on missing file")

	_, hivexErr := bindings.Open(nonexistent, 0)
	require.Error(t, hivexErr, "hivex should error on missing file")
}

// TestOpen_InvalidHive tests error handling for corrupt/invalid hive files.
func TestOpen_InvalidHive(t *testing.T) {
	// Create temporary invalid hive file
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.hive")

	// Write garbage data
	err := os.WriteFile(invalidPath, []byte("not a valid hive file"), 0644)
	require.NoError(t, err)

	// Both should fail gracefully
	_, goErr := reader.Open(invalidPath, hive.OpenOptions{})
	require.Error(t, goErr, "gohivex should error on invalid hive")

	_, hivexErr := bindings.Open(invalidPath, 0)
	require.Error(t, hivexErr, "hivex should error on invalid hive")
}

// TestClose_BasicClose tests hivex_close
// Verifies both implementations properly release resources.
func TestClose_BasicClose(t *testing.T) {
	// Open with gohivex
	goHive, err := reader.Open(TestHives.Minimal, hive.OpenOptions{})
	require.NoError(t, err)

	// Close should succeed
	err = goHive.Close()
	require.NoError(t, err, "gohivex close failed")

	// Open with hivex
	hivexHive, err := bindings.Open(TestHives.Minimal, 0)
	require.NoError(t, err)

	// Close should succeed
	err = hivexHive.Close()
	assert.NoError(t, err, "hivex close failed")
}

// TestClose_DoubleClose tests closing twice
// Both implementations should handle this gracefully.
func TestClose_DoubleClose(t *testing.T) {
	// Test gohivex
	goHive, err := reader.Open(TestHives.Minimal, hive.OpenOptions{})
	require.NoError(t, err)

	err = goHive.Close()
	require.NoError(t, err)

	// Second close should be safe (no-op or graceful error)
	err = goHive.Close()
	// gohivex may or may not error - just ensure no panic
	_ = err

	// Test hivex
	hivexHive, err := bindings.Open(TestHives.Minimal, 0)
	require.NoError(t, err)

	err = hivexHive.Close()
	require.NoError(t, err)

	// Second close
	err = hivexHive.Close()
	// Hivex returns 0 (success) on double close per their implementation
	_ = err
}

// TestLastModified tests hivex_last_modified
// Returns the last modification timestamp from the REGF header.
func TestLastModified(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"minimal", TestHives.Minimal},
		{"special", TestHives.Special},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Open with gohivex
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			// Open with hivex
			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get timestamps
			goInfo := goHive.Info()
			goTimestamp := goInfo.LastWrite.Unix()

			hivexTimestamp := hivexHive.LastModified()

			// Both should report same timestamp
			// Note: goTimestamp is time.Time.Unix(), hivexTimestamp is raw FILETIME
			// We need to convert for comparison
			// For now, just verify both are non-zero and in reasonable range
			assert.NotZero(t, goTimestamp, "gohivex timestamp should not be zero")
			assert.NotZero(t, hivexTimestamp, "hivex timestamp should not be zero")

			// TODO: Add proper conversion and comparison when timestamp format is confirmed
		})
	}
}
