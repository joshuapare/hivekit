package alloc

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// setupRealHive copies a real test hive to a temp directory and opens it.
// Returns the hive and a cleanup function.
func setupRealHive(tb testing.TB) (*hive.Hive, func()) {
	tb.Helper()

	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(tb.TempDir(), "bench-test-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		tb.Skipf("Test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		tb.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		tb.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		tb.Fatalf("Failed to open hive: %v", err)
	}

	cleanup := func() {
		h.Close()
	}

	return h, cleanup
}

// setupLargeHive returns a larger test hive for more intensive benchmarks.
func setupLargeHive(tb testing.TB) (*hive.Hive, func()) {
	tb.Helper()

	// Try windows-2012-software which is typically larger
	testHivePath := "../../testdata/suite/windows-2012-software"

	// Copy to temp directory
	tempHivePath := filepath.Join(tb.TempDir(), "bench-large-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		// Fall back to windows-2003-server-system if not available
		tb.Logf("Large hive not found, using standard hive: %v", err)
		return setupRealHive(tb)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		tb.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		tb.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		tb.Fatalf("Failed to open hive: %v", err)
	}

	cleanup := func() {
		h.Close()
	}

	return h, cleanup
}

// BenchmarkGetEfficiencyStats measures full efficiency stats computation.
// This includes HBIN scanning and sorting to find the top 20 worst HBINs.
func BenchmarkGetEfficiencyStats(b *testing.B) {
	h, cleanup := setupRealHive(b)
	defer cleanup()

	// Create allocator (scans HBINs on creation)
	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create allocator: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := fa.GetEfficiencyStats()
		// Prevent compiler from optimizing away the call
		if stats.TotalHBINs == 0 {
			b.Fatal("Expected non-zero HBIN count")
		}
	}
}

// BenchmarkGetEfficiencyStats_LargeHive tests with a larger hive.
func BenchmarkGetEfficiencyStats_LargeHive(b *testing.B) {
	h, cleanup := setupLargeHive(b)
	defer cleanup()

	// Create allocator
	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create allocator: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := fa.GetEfficiencyStats()
		// Prevent compiler from optimizing away the call
		if stats.TotalHBINs == 0 {
			b.Fatal("Expected non-zero HBIN count")
		}
	}
}

// BenchmarkGetEfficiencyStats_Repeated measures repeated calls (for caching baseline).
// After Phase 3 (caching), subsequent calls should be near-instant.
func BenchmarkGetEfficiencyStats_Repeated(b *testing.B) {
	h, cleanup := setupRealHive(b)
	defer cleanup()

	// Create allocator
	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create allocator: %v", err)
	}

	// First call to establish baseline / populate any internal state
	_ = fa.GetEfficiencyStats()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := fa.GetEfficiencyStats()
		if stats.TotalHBINs == 0 {
			b.Fatal("Expected non-zero HBIN count")
		}
	}
}

// BenchmarkScanHBINEfficiency measures the full allocator creation + GetEfficiencyStats.
// This isolates the combined cost of HBIN scanning during allocator creation
// plus the efficiency stats computation.
func BenchmarkScanHBINEfficiency(b *testing.B) {
	h, cleanup := setupRealHive(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Create fresh allocator each time to force re-scan
		fa, err := NewFast(h, nil, nil)
		if err != nil {
			b.Fatalf("Failed to create allocator: %v", err)
		}

		stats := fa.GetEfficiencyStats()
		if stats.TotalHBINs == 0 {
			b.Fatal("Expected non-zero HBIN count")
		}
	}
}

// BenchmarkNewFastAllocator measures allocator creation with HBIN scanning.
func BenchmarkNewFastAllocator(b *testing.B) {
	h, cleanup := setupRealHive(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		fa, err := NewFast(h, nil, nil)
		if err != nil {
			b.Fatalf("Failed to create allocator: %v", err)
		}
		// Verify allocator is valid
		if fa == nil {
			b.Fatal("Expected non-nil allocator")
		}
	}
}
