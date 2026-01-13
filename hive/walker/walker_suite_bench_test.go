package walker

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// Suite hives for comprehensive testing.
var suiteHives = []struct {
	name string
	path string
}{
	{"windows-2003-server-system", "../../testdata/suite/windows-2003-server-system"},
	{"windows-2003-server-software", "../../testdata/suite/windows-2003-server-software"},
	{"windows-2012-system", "../../testdata/suite/windows-2012-system"},
	{"windows-2012-software", "../../testdata/suite/windows-2012-software"},
	{"windows-8-system", "../../testdata/suite/windows-8-consumer-preview-system"},
	{"windows-8-software", "../../testdata/suite/windows-8-consumer-preview-software"},
}

// openHive opens a hive file directly without copying (read-only benchmark).
func openHive(path string) (*hive.Hive, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("hive not found: %s", path)
	}

	return hive.Open(path)
}

// Benchmark_Suite_NewWalker_2003System benchmarks new walker on Windows 2003 System.
func Benchmark_Suite_NewWalker_2003System(b *testing.B) {
	benchmarkNewWalker(b, suiteHives[0].path)
}

// Benchmark_Suite_NewWalker_2003Software benchmarks new walker on Windows 2003 Software.
func Benchmark_Suite_NewWalker_2003Software(b *testing.B) {
	benchmarkNewWalker(b, suiteHives[1].path)
}

// Benchmark_Suite_NewWalker_2012System benchmarks new walker on Windows 2012 System.
func Benchmark_Suite_NewWalker_2012System(b *testing.B) {
	benchmarkNewWalker(b, suiteHives[2].path)
}

// Benchmark_Suite_NewWalker_2012Software benchmarks new walker on Windows 2012 Software.
func Benchmark_Suite_NewWalker_2012Software(b *testing.B) {
	benchmarkNewWalker(b, suiteHives[3].path)
}

// Benchmark_Suite_NewWalker_Win8System benchmarks new walker on Windows 8 System.
func Benchmark_Suite_NewWalker_Win8System(b *testing.B) {
	benchmarkNewWalker(b, suiteHives[4].path)
}

// Benchmark_Suite_NewWalker_Win8Software benchmarks new walker on Windows 8 Software.
func Benchmark_Suite_NewWalker_Win8Software(b *testing.B) {
	benchmarkNewWalker(b, suiteHives[5].path)
}

// benchmarkNewWalker runs the new ValidationWalker.
func benchmarkNewWalker(b *testing.B, hivePath string) {
	h, err := openHive(hivePath)
	if err != nil {
		b.Skipf("Skipping: %v", err)
		return
	}
	defer h.Close()

	walker := NewValidationWalker(h)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		cellCount := 0
		err := walker.Walk(func(ref CellRef) error {
			cellCount++
			return nil
		})
		if err != nil {
			b.Fatalf("Walk failed: %v", err)
		}

		walker.Reset()
	}
}

// Test_SuiteComparison is not a benchmark, but a test that runs the walker
// and prints a performance table for documentation purposes.
func Test_SuiteComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive suite comparison in short mode")
	}

	t.Log("\n" + strings.Repeat("=", 80))
	t.Log("WALKER PERFORMANCE ON TEST SUITE")
	t.Log(strings.Repeat("=", 80))
	t.Logf("\n%-45s %12s %12s %12s\n",
		"Hive", "Time", "Memory", "Allocs")
	t.Log(strings.Repeat("-", 80))

	for _, suite := range suiteHives {
		// Get file size
		info, err := os.Stat(suite.path)
		if err != nil {
			t.Logf("%-45s  SKIP: %v\n", suite.name, err)
			continue
		}

		// Open hive
		h, err := openHive(suite.path)
		if err != nil {
			t.Logf("%-45s  SKIP: %v\n", suite.name, err)
			continue
		}

		// Count cells using new walker (for reference)
		walker := NewValidationWalker(h)
		cellCount := 0
		_ = walker.Walk(func(ref CellRef) error {
			cellCount++
			return nil
		})

		// Run walker and measure
		result := testing.Benchmark(func(b *testing.B) {
			benchmarkNewWalker(b, suite.path)
		})

		h.Close()

		// Print row
		t.Logf("%-45s %10.2fms %10.1fKB %10d\n",
			fmt.Sprintf("%s (%dM, %dk cells)", suite.name, info.Size()/(1024*1024), cellCount/1000),
			float64(result.NsPerOp())/1e6,
			float64(result.AllocedBytesPerOp())/1024,
			result.AllocsPerOp(),
		)
	}

	t.Log(strings.Repeat("=", 80) + "\n")
}
