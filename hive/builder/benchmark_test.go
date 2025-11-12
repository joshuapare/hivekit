package builder

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkBuildFromRegFile benchmarks building hives from .reg files.
// These benchmarks serve as performance regression tests - if performance
// degrades significantly, the CI will catch it.
//
// Baseline performance (Tier 4A with deferred mode):
//   - Small hives (~8K keys):   ~0.3-0.5s
//   - Medium hives (~35K keys):  ~3-7s
//   - Large hives (~150K keys):  ~45-52s (when run in parallel with other tests)
//   - Large hives (~150K keys):  ~1.3-1.5s (when run standalone)

func BenchmarkBuildFromRegFile_Small(b *testing.B) {
	benchmarkBuildFromRegFile(b, "windows-2003-server-system", 8055, 21270)
}

func BenchmarkBuildFromRegFile_Medium(b *testing.B) {
	benchmarkBuildFromRegFile(b, "windows-xp-2-software", 35858, 90612)
}

func BenchmarkBuildFromRegFile_Large(b *testing.B) {
	benchmarkBuildFromRegFile(b, "windows-8-consumer-preview-software", 151914, 238228)
}

func benchmarkBuildFromRegFile(b *testing.B, name string, expectedKeys, expectedValues int) {
	regPath := filepath.Join("../../testdata/suite", name+".reg")

	// Skip if file doesn't exist
	if _, err := os.Stat(regPath); os.IsNotExist(err) {
		b.Skipf("Test .reg file not found: %s", regPath)
	}

	b.ResetTimer()

	for range b.N {
		// Create temp directory for each iteration
		dir := b.TempDir()
		hivePath := filepath.Join(dir, name+".hive")

		// Build the hive
		err := BuildFromRegFile(hivePath, regPath, nil)
		if err != nil {
			b.Fatalf("BuildFromRegFile failed: %v", err)
		}

		// Verify the file was created
		info, err := os.Stat(hivePath)
		if err != nil {
			b.Fatalf("Output hive file should exist: %v", err)
		}
		if info.Size() == 0 {
			b.Fatal("Hive file should not be empty")
		}
	}

	// Report keys/values per operation
	b.ReportMetric(float64(expectedKeys), "keys/op")
	b.ReportMetric(float64(expectedValues), "values/op")
}

// Note: Use benchmarks above to measure performance.
// Run with: go test -bench=BenchmarkBuildFromRegFile -benchtime=1x
// For profiling: go test -bench=. -cpuprofile=cpu.prof -memprofile=mem.prof
