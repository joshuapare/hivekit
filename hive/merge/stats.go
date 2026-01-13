package merge

import (
	"context"

	"github.com/joshuapare/hivekit/hive"
)

// StorageStats contains statistics about a hive file's space usage.
type StorageStats struct {
	// FileSize is the total size of the hive file in bytes.
	FileSize int64

	// FreeBytes is the total free space available in the hive (wasted space in HBINs).
	// This includes space that was previously allocated but has been freed.
	FreeBytes int64

	// UsedBytes is the total space currently allocated for active cells.
	// This includes NK (key), VK (value), and data cells.
	UsedBytes int64

	// FreePercent is the percentage of the hive that is free space.
	// Calculated as: (FreeBytes / FileSize) * 100
	FreePercent float64
}

// StatHive returns statistics about a hive file's space usage and fragmentation.
//
// This function opens the hive and creates a temporary merge session to access
// the allocator's efficiency statistics. The allocator tracks free space by
// scanning all HBINs and calculating allocated vs. free space.
//
// Parameters:
//   - hivePath: Absolute path to the hive file
//
// Returns:
//   - StorageStats: Statistics about file size, free space, and usage
//   - error: If hive cannot be opened or session creation fails
//
// Example:
//
//	stats, err := merge.StatHive("/path/to/hive")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("File size: %d bytes\n", stats.FileSize)
//	fmt.Printf("Free space: %d bytes (%.2f%%)\n", stats.FreeBytes, stats.FreePercent)
//	fmt.Printf("Used space: %d bytes\n", stats.UsedBytes)
//
// Note: This operation is relatively fast (microseconds) because it leverages
// the allocator's cached statistics rather than scanning the entire hive.
func StatHive(hivePath string) (StorageStats, error) {
	var stats StorageStats

	// Open the hive
	h, err := hive.Open(hivePath)
	if err != nil {
		return stats, err
	}
	defer h.Close()

	// Get file size
	stats.FileSize = h.Size()

	// Create a temporary session to access the allocator
	// This builds the allocator which scans the hive and tracks free space
	ctx := context.Background()
	sess, err := NewSession(ctx, h, Options{})
	if err != nil {
		return stats, err
	}
	defer sess.Close(ctx)

	// Get efficiency stats from the allocator
	// This provides: TotalCapacity, TotalAllocated, TotalWasted
	effStats := sess.GetEfficiencyStats()

	// Map allocator stats to HiveStats
	// Note: TotalWasted represents free space within HBINs
	stats.FreeBytes = effStats.TotalWasted
	stats.UsedBytes = effStats.TotalAllocated

	// Calculate free percentage based on total file size
	if stats.FileSize > 0 {
		stats.FreePercent = float64(stats.FreeBytes) / float64(stats.FileSize) * 100.0
	}

	return stats, nil
}
