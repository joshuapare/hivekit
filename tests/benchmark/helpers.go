package benchmark

import (
	"os"
	"runtime"
	"testing"
)

// BenchTempDir creates a temporary directory for benchmark use and registers
// cleanup to run after the benchmark completes. This is the benchmark equivalent
// of t.TempDir() for use in *testing.B contexts.
func BenchTempDir(b *testing.B) string {
	b.Helper()
	dir, err := os.MkdirTemp("", "hivekit-bench-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	b.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// HiveSize returns the file size in bytes for the hive at the given path.
// Returns 0 if the file does not exist or cannot be stat'd.
func HiveSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// ReportMemStats reports memory allocation metrics as custom benchmark metrics.
func ReportMemStats(b *testing.B) {
	b.Helper()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	b.ReportMetric(float64(m.TotalAlloc), "total-bytes")
	b.ReportMetric(float64(m.NumGC), "gc-cycles")
}

// NOTE on testing.TB compatibility:
//
// The testutil.SetupTestHive and SetupTestHiveWithAllocator helpers in
// internal/testutil/setup.go accept *testing.T, not testing.TB. This means
// they cannot be used directly from *testing.B benchmark functions.
//
// For benchmark use, either:
//   - Change the testutil helpers to accept testing.TB (broader change)
//   - Use the builder API directly (as done in this package's generators)
//   - Create wrapper functions that adapt testing.B to the required interface
//
// The generators in this package use the builder API directly, which avoids
// this compatibility issue entirely.
