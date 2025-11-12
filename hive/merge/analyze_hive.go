package merge

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/walker"
)

// HiveStats contains exact counts of keys and values in a hive.
type HiveStats struct {
	TotalKeys   int
	TotalValues int
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
