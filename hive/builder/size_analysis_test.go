package builder

import (
	"fmt"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
)

// TestAnalyzeHiveSize builds a test hive and compares it to the original
//
// Set HIVE_LOG_ALLOC=1 to enable detailed allocation logging:
//
//	HIVE_LOG_ALLOC=1 go test -v -run TestAnalyzeHiveSize ./hive/builder 2>&1 | head -200
func TestAnalyzeHiveSize(t *testing.T) {
	// Build from .reg file
	opts := DefaultOptions()
	opts.Strategy = StrategyInPlace

	builtPath := "/tmp/analyze-built.hive"
	origPath := "../../testdata/suite/windows-2003-server-system"
	regPath := "../../testdata/suite/windows-2003-server-system.reg"

	// Remove existing file to start fresh
	os.Remove(builtPath)

	err := BuildFromRegFile(builtPath, regPath, opts)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	builtInfo, _ := os.Stat(builtPath)
	origInfo, _ := os.Stat(origPath)

	fmt.Fprintf(os.Stderr, "\n=== SIZE ANALYSIS ===\n")
	fmt.Fprintf(os.Stderr, "Original (Windows):  %10d bytes\n", origInfo.Size())
	fmt.Fprintf(os.Stderr, "Built (InPlace):     %10d bytes\n", builtInfo.Size())
	fmt.Fprintf(os.Stderr, "Difference:          %10d bytes (%.1f%% larger)\n",
		builtInfo.Size()-origInfo.Size(),
		float64(builtInfo.Size()-origInfo.Size())/float64(origInfo.Size())*100)

	// Calculate allocation density
	totalGrows := builtInfo.Size() / 4096
	fmt.Fprintf(os.Stderr, "Total 4KB HBINs:     %10d blocks\n", totalGrows)
	fmt.Fprintf(os.Stderr, "=====================\n\n")

	t.Logf("Built hive saved to: %s", builtPath)
}

// TestHiveEfficiency analyzes the efficiency and fragmentation of a built hive.
func TestHiveEfficiency(t *testing.T) {
	// First build the hive
	opts := DefaultOptions()
	opts.Strategy = StrategyInPlace

	builtPath := "/tmp/analyze-built.hive"
	regPath := "../../testdata/suite/windows-2003-server-system.reg"

	// Remove existing file to start fresh
	os.Remove(builtPath)

	err := BuildFromRegFile(builtPath, regPath, opts)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Open the built hive for analysis
	h, err := hive.Open(builtPath)
	if err != nil {
		t.Fatalf("Failed to open built hive: %v", err)
	}
	defer h.Close()

	// Create allocator to analyze the hive
	config := alloc.DefaultConfig
	allocator, err := alloc.NewFast(h, nil, &config)
	if err != nil {
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Print comprehensive efficiency statistics
	allocator.PrintEfficiencyStats()

	// Also get stats programmatically for assertions
	stats := allocator.GetEfficiencyStats()

	fmt.Fprintf(os.Stderr, "\n=== SUMMARY FOR DEFRAG PLANNING ===\n")
	fmt.Fprintf(os.Stderr, "Total HBINs:        %d\n", stats.TotalHBINs)
	fmt.Fprintf(
		os.Stderr,
		"Well-packed (≥95%%): %d HBINs (%.1f%%)\n",
		stats.PerfectHBINs+stats.ExcellentHBINs+stats.VeryGoodHBINs+stats.GoodHBINs,
		100.0*float64(
			stats.PerfectHBINs+stats.ExcellentHBINs+stats.VeryGoodHBINs+stats.GoodHBINs,
		)/float64(
			stats.TotalHBINs,
		),
	)
	fmt.Fprintf(os.Stderr, "Poorly-packed (<95%%): %d HBINs (%.1f%%)\n",
		stats.SuboptimalHBINs,
		100.0*float64(stats.SuboptimalHBINs)/float64(stats.TotalHBINs))
	fmt.Fprintf(os.Stderr, "Very poor (<80%%):  %d HBINs (%.1f%%)\n",
		stats.PoorHBINs,
		100.0*float64(stats.PoorHBINs)/float64(stats.TotalHBINs))

	wastedMB := float64(stats.TotalWasted) / (1024 * 1024)
	fmt.Fprintf(os.Stderr, "\nWasted space:       %.2f MB\n", wastedMB)
	fmt.Fprintf(os.Stderr, "Potential savings:  %.2f MB (if we could pack to 98%% efficiency)\n",
		wastedMB*0.5) // Rough estimate

	fmt.Fprintf(os.Stderr, "===================================\n\n")
}

// TestWindowsHiveEfficiency analyzes the efficiency of a Windows-generated hive.
func TestWindowsHiveEfficiency(t *testing.T) {
	origPath := "../../testdata/suite/windows-2003-server-system"

	// Open the Windows-generated hive for analysis
	h, err := hive.Open(origPath)
	if err != nil {
		t.Fatalf("Failed to open Windows hive: %v", err)
	}
	defer h.Close()

	// Create allocator to analyze the hive
	config := alloc.DefaultConfig
	allocator, err := alloc.NewFast(h, nil, &config)
	if err != nil {
		t.Fatalf("Failed to create allocator: %v", err)
	}

	fmt.Fprintf(os.Stderr, "\n=== WINDOWS-GENERATED HIVE ===\n")
	allocator.PrintEfficiencyStats()

	// Get stats for comparison
	stats := allocator.GetEfficiencyStats()

	fmt.Fprintf(os.Stderr, "\n=== COMPARISON BASELINE ===\n")
	fmt.Fprintf(os.Stderr, "Total HBINs:        %d\n", stats.TotalHBINs)
	fmt.Fprintf(os.Stderr, "Avg alloc/HBIN:     %.0f bytes\n", stats.AverageAllocPerHBIN)
	fmt.Fprintf(os.Stderr, "Overall efficiency: %.1f%%\n", stats.OverallEfficiency)
	fmt.Fprintf(os.Stderr, "============================\n\n")
}
