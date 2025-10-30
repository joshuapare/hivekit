package comparison

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Suite benchmarks to compare
var suiteBenchmarks = []struct {
	name      string
	hivePath  string
	deltaFile string
}{
	// win2003-system
	{
		name:      "win2003-system/small",
		hivePath:  "../../../testdata/suite/windows-2003-server-system",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2003-system-small.reg",
	},
	{
		name:      "win2003-system/medium",
		hivePath:  "../../../testdata/suite/windows-2003-server-system",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2003-system-medium.reg",
	},
	// win2012-system
	{
		name:      "win2012-system/small",
		hivePath:  "../../../testdata/suite/windows-2012-system",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-system-small.reg",
	},
	{
		name:      "win2012-system/medium",
		hivePath:  "../../../testdata/suite/windows-2012-system",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-system-medium.reg",
	},
	{
		name:      "win2012-system/large",
		hivePath:  "../../../testdata/suite/windows-2012-system",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-system-large.reg",
	},
	// win2012-software
	{
		name:      "win2012-software/small",
		hivePath:  "../../../testdata/suite/windows-2012-software",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-software-small.reg",
	},
	{
		name:      "win2012-software/medium",
		hivePath:  "../../../testdata/suite/windows-2012-software",
		deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-software-medium.reg",
	},
}

// TestComparison_SuiteBenchmarks compares gohivex and hivexregedit outputs
func TestComparison_SuiteBenchmarks(t *testing.T) {
	// Check if hivexregedit is available
	if _, err := os.Stat(hivexregeditPath); os.IsNotExist(err) {
		t.Skipf("hivexregedit not found at %s", hivexregeditPath)
	}

	for _, tc := range suiteBenchmarks {
		t.Run(tc.name, func(t *testing.T) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				t.Skipf("Hive not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.deltaFile); os.IsNotExist(err) {
				t.Skipf("Delta not found: %s", tc.deltaFile)
			}

			// Create temp files for outputs
			gohivexOut, err := os.CreateTemp("", "gohivex-*.hive")
			if err != nil {
				t.Fatalf("CreateTemp failed: %v", err)
			}
			gohivexPath := gohivexOut.Name()
			gohivexOut.Close()
			defer os.Remove(gohivexPath)

			hivexregeditOut, err := os.CreateTemp("", "hivexregedit-*.hive")
			if err != nil {
				t.Fatalf("CreateTemp failed: %v", err)
			}
			hivexregeditPath := hivexregeditOut.Name()
			hivexregeditOut.Close()
			defer os.Remove(hivexregeditPath)

			// Copy input hive for gohivex
			if err := copyFile(tc.hivePath, gohivexPath); err != nil {
				t.Fatalf("Failed to copy hive: %v", err)
			}

			// Run gohivex merge
			t.Log("Running gohivex merge...")
			startGohivex := time.Now()
			_, err = hive.MergeRegFile(gohivexPath, tc.deltaFile, nil)
			gohivexDuration := time.Since(startGohivex)
			if err != nil {
				t.Fatalf("gohivex merge failed: %v", err)
			}
			t.Logf("  gohivex:       %v", gohivexDuration)

			// Run hivexregedit merge
			t.Log("Running hivexregedit merge...")
			startHivexregedit := time.Now()
			err = runHivexregeditMerge(tc.hivePath, tc.deltaFile, hivexregeditPath)
			hivexregeditDuration := time.Since(startHivexregedit)
			if err != nil {
				t.Fatalf("hivexregedit merge failed: %v", err)
			}
			t.Logf("  hivexregedit:  %v", hivexregeditDuration)

			// Calculate speedup
			speedup := float64(hivexregeditDuration) / float64(gohivexDuration)
			t.Logf("  Speedup:       %.2fx", speedup)

			// Compare outputs
			t.Log("Comparing outputs...")
			compareOutputs(t, gohivexPath, hivexregeditPath)
		})
	}
}

// BenchmarkComparison_Gohivex benchmarks gohivex merge operations
func BenchmarkComparison_Gohivex(b *testing.B) {
	for _, tc := range suiteBenchmarks {
		b.Run(tc.name, func(b *testing.B) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.deltaFile); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", tc.deltaFile)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Create temp copy
				tmpFile, err := os.CreateTemp("", "bench-gohivex-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				if err := copyFile(tc.hivePath, tmpPath); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("Failed to copy hive: %v", err)
				}

				b.StartTimer()
				_, err = hive.MergeRegFile(tmpPath, tc.deltaFile, nil)
				b.StopTimer()

				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("Merge failed: %v", err)
				}
				os.Remove(tmpPath)
			}
		})
	}
}

// BenchmarkComparison_Hivexregedit benchmarks hivexregedit merge operations
func BenchmarkComparison_Hivexregedit(b *testing.B) {
	// Check if hivexregedit is available
	if _, err := os.Stat(hivexregeditPath); os.IsNotExist(err) {
		b.Skipf("hivexregedit not found at %s", hivexregeditPath)
	}

	for _, tc := range suiteBenchmarks {
		b.Run(tc.name, func(b *testing.B) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.deltaFile); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", tc.deltaFile)
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Create temp copy
				tmpFile, err := os.CreateTemp("", "bench-hivexregedit-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				b.StartTimer()
				err = runHivexregeditMerge(tc.hivePath, tc.deltaFile, tmpPath)
				b.StopTimer()

				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("Merge failed: %v", err)
				}
				os.Remove(tmpPath)
			}
		})
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}
	if err := os.WriteFile(dst, input, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}
