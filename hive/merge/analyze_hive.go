package merge

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/walker"
)

// HiveStats contains comprehensive statistics about a hive's structure and storage.
//
// This struct combines:
// - Key/value counts from tree traversal
// - Storage metrics (file size, free/used space)
// - Allocator efficiency statistics
type HiveStats struct {
	// TotalKeys is the count of all NK (node key) cells in the hive.
	TotalKeys int

	// TotalValues is the count of all VK (value key) cells in the hive.
	TotalValues int

	// Storage contains file size, free space, and usage metrics.
	// Note: Only populated when using GetHiveStats() from a Session.
	Storage StorageStats

	// Efficiency contains detailed allocator statistics including:
	// - TotalWasted: Total wasted space in bytes
	// - OverallEfficiency: Percentage of space actually used (0-100)
	// - Per-HBIN efficiency distribution
	// - LeastEfficientHBINs: Worst performing HBINs for analysis
	// Note: Only populated when using GetHiveStats() from a Session.
	Efficiency alloc.EfficiencyStats
}

// AnalyzeHive returns exact statistics about a hive's structure.
func AnalyzeHive(h *hive.Hive) (*HiveStats, error) {
	stats := &HiveStats{}

	// Use ValidationWalker to count all cells
	vw := walker.NewValidationWalker(h)

	err := vw.Walk(func(ref walker.CellRef) error {
		// Count NK cells (keys)
		if ref.Type == walker.CellTypeNK {
			stats.TotalKeys++
		}
		// Count VK cells (values)
		if ref.Type == walker.CellTypeVK {
			stats.TotalValues++
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk hive: %w", err)
	}

	return stats, nil
}
