package merge

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// setupBenchSession creates a test session for benchmarking.
// Returns the session and a cleanup function.
func setupBenchSession(tb testing.TB) (*Session, func()) {
	tb.Helper()

	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Copy to temp directory
	tempHivePath := filepath.Join(tb.TempDir(), "bench-session-hive")
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

	// Create session with default options
	session, err := NewSession(context.Background(), h, DefaultOptions())
	if err != nil {
		h.Close()
		tb.Fatalf("Failed to create session: %v", err)
	}

	cleanup := func() {
		session.Close(context.Background())
		h.Close()
	}

	return session, cleanup
}

// setupLargeBenchSession creates a session using a larger test hive.
func setupLargeBenchSession(tb testing.TB) (*Session, func()) {
	tb.Helper()

	testHivePath := "../../testdata/suite/windows-2012-software"

	// Copy to temp directory
	tempHivePath := filepath.Join(tb.TempDir(), "bench-large-session-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		tb.Logf("Large hive not found, using standard hive: %v", err)
		return setupBenchSession(tb)
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

	// Create session with default options
	session, err := NewSession(context.Background(), h, DefaultOptions())
	if err != nil {
		h.Close()
		tb.Fatalf("Failed to create session: %v", err)
	}

	cleanup := func() {
		session.Close(context.Background())
		h.Close()
	}

	return session, cleanup
}

// BenchmarkGetStorageStats measures storage stats retrieval through Session.
// This calls GetEfficiencyStats internally, which includes sorting.
func BenchmarkGetStorageStats(b *testing.B) {
	session, cleanup := setupBenchSession(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := session.GetStorageStats()
		// Prevent compiler from optimizing away the call
		if stats.FileSize == 0 {
			b.Fatal("Expected non-zero file size")
		}
	}
}

// BenchmarkGetStorageStats_LargeHive tests with a larger hive.
func BenchmarkGetStorageStats_LargeHive(b *testing.B) {
	session, cleanup := setupLargeBenchSession(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := session.GetStorageStats()
		if stats.FileSize == 0 {
			b.Fatal("Expected non-zero file size")
		}
	}
}

// BenchmarkGetEfficiencyStatsThroughSession measures the Session's GetEfficiencyStats.
// This provides the full efficiency stats including the top 20 worst HBINs.
func BenchmarkGetEfficiencyStatsThroughSession(b *testing.B) {
	session, cleanup := setupBenchSession(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := session.GetEfficiencyStats()
		if stats.TotalHBINs == 0 {
			b.Fatal("Expected non-zero HBIN count")
		}
	}
}

// BenchmarkGetHiveStats measures full hive stats (storage + efficiency).
func BenchmarkGetHiveStats(b *testing.B) {
	session, cleanup := setupBenchSession(b)
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := session.GetHiveStats()
		if stats.Storage.FileSize == 0 {
			b.Fatal("Expected non-zero file size")
		}
	}
}

// BenchmarkSessionCreation measures how long it takes to create a full session.
// This includes index building and allocator creation.
func BenchmarkSessionCreation(b *testing.B) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	// Ensure the hive exists before starting
	if _, err := os.Stat(testHivePath); err != nil {
		b.Skipf("Test hive not found: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Copy hive to temp directory
		tempHivePath := filepath.Join(b.TempDir(), "bench-creation-hive")
		src, err := os.Open(testHivePath)
		if err != nil {
			b.Fatalf("Failed to open source: %v", err)
		}
		dst, err := os.Create(tempHivePath)
		if err != nil {
			src.Close()
			b.Fatalf("Failed to create temp: %v", err)
		}
		if _, copyErr := io.Copy(dst, src); copyErr != nil {
			src.Close()
			dst.Close()
			b.Fatalf("Failed to copy: %v", copyErr)
		}
		src.Close()
		dst.Close()

		b.StartTimer()

		// Open hive
		h, err := hive.Open(tempHivePath)
		if err != nil {
			b.Fatalf("Failed to open hive: %v", err)
		}

		// Create session
		session, err := NewSession(context.Background(), h, DefaultOptions())
		if err != nil {
			h.Close()
			b.Fatalf("Failed to create session: %v", err)
		}

		b.StopTimer()
		session.Close(context.Background())
		h.Close()
		b.StartTimer()
	}
}
