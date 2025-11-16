package merge_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/hive/merge"
)

func TestStatHive_EmptyHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create an empty hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Get stats
	stats, err := merge.StatHive(hivePath)
	require.NoError(t, err)

	// Empty hive should have some free space
	require.Greater(t, stats.FileSize, int64(0))
	require.Greater(t, stats.FreeBytes, int64(0))
	require.GreaterOrEqual(t, stats.UsedBytes, int64(0))
	require.Greater(t, stats.FreePercent, 0.0, "empty hive should have some free space")
	require.LessOrEqual(t, stats.FreePercent, 100.0, "free percent should be <= 100%")

	t.Logf("Empty hive stats: FileSize=%d, Free=%d (%.2f%%), Used=%d",
		stats.FileSize, stats.FreeBytes, stats.FreePercent, stats.UsedBytes)
}

func TestStatHive_WithData(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive with some data
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Add multiple keys and values
	for i := 0; i < 100; i++ {
		err = b.SetString([]string{"Software", "Test", "Key" + string(rune(i))}, "Value", "Data")
		require.NoError(t, err)
	}

	err = b.Commit()
	require.NoError(t, err)

	// Get stats
	stats, err := merge.StatHive(hivePath)
	require.NoError(t, err)

	// Should have reasonable stats
	require.Greater(t, stats.FileSize, int64(0))
	require.GreaterOrEqual(t, stats.FreeBytes, int64(0))
	require.Greater(t, stats.UsedBytes, int64(0))
	require.GreaterOrEqual(t, stats.FreePercent, 0.0)
	require.LessOrEqual(t, stats.FreePercent, 100.0)

	// Used bytes should be less than file size
	require.Less(t, stats.UsedBytes, stats.FileSize)

	t.Logf("Populated hive stats: FileSize=%d, Free=%d (%.2f%%), Used=%d",
		stats.FileSize, stats.FreeBytes, stats.FreePercent, stats.UsedBytes)
}

func TestStatHive_LargeHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a larger hive with more data
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	// Add many keys with larger values
	largeData := make([]byte, 1000)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	for i := 0; i < 1000; i++ {
		path := []string{"Software", "App" + string(rune(i/100)), "Key" + string(rune(i%100))}
		err = b.SetBinary(path, "Data", largeData)
		require.NoError(t, err)
	}

	err = b.Commit()
	require.NoError(t, err)

	// Get stats
	stats, err := merge.StatHive(hivePath)
	require.NoError(t, err)

	// Should have meaningful stats
	require.Greater(t, stats.FileSize, int64(100000), "large hive should be >100KB")
	require.Greater(t, stats.UsedBytes, int64(50000), "should have significant used space")
	require.GreaterOrEqual(t, stats.FreePercent, 0.0)
	require.LessOrEqual(t, stats.FreePercent, 100.0)

	// Free + Used should be less than or equal to file size
	// (file size includes header and other overhead)
	require.LessOrEqual(t, stats.FreeBytes+stats.UsedBytes, stats.FileSize)

	t.Logf("Large hive stats: FileSize=%d, Free=%d (%.2f%%), Used=%d",
		stats.FileSize, stats.FreeBytes, stats.FreePercent, stats.UsedBytes)
}

func TestStatHive_InvalidPath(t *testing.T) {
	// Try to stat non-existent hive
	_, err := merge.StatHive("/nonexistent/path/to/hive")
	require.Error(t, err, "should error on invalid hive path")
}

func TestStatHive_ConsistentStats(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.SetString([]string{"Test"}, "Value", "Data")
	require.NoError(t, err)

	err = b.Commit()
	require.NoError(t, err)

	// Get stats multiple times
	stats1, err := merge.StatHive(hivePath)
	require.NoError(t, err)

	stats2, err := merge.StatHive(hivePath)
	require.NoError(t, err)

	// Stats should be identical for same hive
	require.Equal(t, stats1.FileSize, stats2.FileSize)
	require.Equal(t, stats1.FreeBytes, stats2.FreeBytes)
	require.Equal(t, stats1.UsedBytes, stats2.UsedBytes)
	require.Equal(t, stats1.FreePercent, stats2.FreePercent)
}

func TestStatHive_PercentageCalculation(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hive")

	// Create a hive
	b, err := builder.New(hivePath, nil)
	require.NoError(t, err)
	defer b.Close()

	err = b.Commit()
	require.NoError(t, err)

	// Get stats
	stats, err := merge.StatHive(hivePath)
	require.NoError(t, err)

	// Verify percentage calculation
	expectedPercent := float64(stats.FreeBytes) / float64(stats.FileSize) * 100.0
	require.InDelta(t, expectedPercent, stats.FreePercent, 0.01, "percentage should be correctly calculated")
}
