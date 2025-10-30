package comparison

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Benchmark hives specifically for merge operations
var mergeHives = []struct {
	Name     string
	Path     string
	SizeDesc string
}{
	{
		Name:     "minimal",
		Path:     "../../../testdata/minimal",
		SizeDesc: "~8KB minimal hive",
	},
	{
		Name:     "large",
		Path:     "../../../testdata/large",
		SizeDesc: "~436KB test hive with 12 keys",
	},
	{
		Name:     "win2003-system",
		Path:     "../../../testdata/suite/windows-2003-server-system",
		SizeDesc: "~2MB real Windows 2003 Server System hive",
	},
}

// Delta files for different operation scales
var deltaFiles = map[string][]string{
	"small": {
		"../../../testdata/merge_benchmarks/deltas/small/add.reg",
		"../../../testdata/merge_benchmarks/deltas/small/modify.reg",
		"../../../testdata/merge_benchmarks/deltas/small/delete.reg",
		"../../../testdata/merge_benchmarks/deltas/small/mixed.reg",
	},
	"medium": {
		"../../../testdata/merge_benchmarks/deltas/medium/add.reg",
		"../../../testdata/merge_benchmarks/deltas/medium/modify.reg",
		"../../../testdata/merge_benchmarks/deltas/medium/delete.reg",
		"../../../testdata/merge_benchmarks/deltas/medium/mixed.reg",
	},
	"large": {
		"../../../testdata/merge_benchmarks/deltas/large/add.reg",
		"../../../testdata/merge_benchmarks/deltas/large/modify.reg",
		"../../../testdata/merge_benchmarks/deltas/large/delete.reg",
		"../../../testdata/merge_benchmarks/deltas/large/mixed.reg",
	},
}

// ============================================================================
// Single Merge Benchmarks
// ============================================================================

// BenchmarkMergeSingleDelta benchmarks merging a single .reg file into a hive
// Tests various combinations of hive sizes and delta sizes
func BenchmarkMergeSingleDelta(b *testing.B) {
	// Test with large hive (most realistic for performance testing)
	testHive := mergeHives[1] // large hive

	for deltaSize, deltas := range deltaFiles {
		for _, deltaPath := range deltas {
			deltaName := filepath.Base(deltaPath)
			deltaName = deltaName[:len(deltaName)-4] // Remove .reg extension

			b.Run(fmt.Sprintf("gohivex/%s/%s_%s", testHive.Name, deltaSize, deltaName), func(b *testing.B) {
				// Check if files exist
				if _, err := os.Stat(testHive.Path); os.IsNotExist(err) {
					b.Skipf("Hive not found: %s", testHive.Path)
				}
				if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
					b.Skipf("Delta not found: %s", deltaPath)
				}

				// Get delta file size for throughput calculation
				deltaInfo, err := os.Stat(deltaPath)
				if err != nil {
					b.Fatalf("Failed to stat delta: %v", err)
				}

				b.SetBytes(deltaInfo.Size())
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					b.StopTimer()

					// Create temp copy of hive
					tmpFile, err := os.CreateTemp("", "bench-merge-*.hive")
					if err != nil {
						b.Fatalf("CreateTemp failed: %v", err)
					}
					tmpPath := tmpFile.Name()
					tmpFile.Close()

					// Copy hive
					hiveData, err := os.ReadFile(testHive.Path)
					if err != nil {
						os.Remove(tmpPath)
						b.Fatalf("ReadFile failed: %v", err)
					}
					if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
						os.Remove(tmpPath)
						b.Fatalf("WriteFile failed: %v", err)
					}

					b.StartTimer()

					// Perform the merge
					_, err = hive.MergeRegFile(tmpPath, deltaPath, nil)

					b.StopTimer()

					if err != nil {
						b.Logf("Merge failed: %v", err)
					}

					// Cleanup
					os.Remove(tmpPath)
				}
			})
		}
	}
}

// ============================================================================
// Sequential Merge Benchmarks
// ============================================================================

// BenchmarkMergeSequential benchmarks applying multiple .reg files sequentially
// Simulates real-world patch application scenarios
func BenchmarkMergeSequential(b *testing.B) {
	testHive := mergeHives[1] // large hive

	sequentialCounts := []int{1, 5, 10, 20}

	for _, count := range sequentialCounts {
		b.Run(fmt.Sprintf("gohivex/%s/%d_deltas", testHive.Name, count), func(b *testing.B) {
			// Check if hive exists
			if _, err := os.Stat(testHive.Path); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", testHive.Path)
			}

			// Build list of sequential delta files
			deltaBasePath := "../../../testdata/merge_benchmarks/deltas/sequential"
			var deltaPaths []string
			for i := 1; i <= count; i++ {
				deltaPath := filepath.Join(deltaBasePath, fmt.Sprintf("delta_%02d.reg", i))
				if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
					b.Skipf("Delta not found: %s", deltaPath)
				}
				deltaPaths = append(deltaPaths, deltaPath)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Create temp copy of hive
				tmpFile, err := os.CreateTemp("", "bench-seq-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				// Copy hive
				hiveData, err := os.ReadFile(testHive.Path)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()

				// Apply all deltas sequentially
				for _, deltaPath := range deltaPaths {
					_, err := hive.MergeRegFile(tmpPath, deltaPath, nil)
					if err != nil {
						b.Fatalf("Merge failed on %s: %v", deltaPath, err)
					}
				}

				b.StopTimer()

				// Cleanup
				os.Remove(tmpPath)
			}
		})
	}
}

// ============================================================================
// Full Hive Merge Benchmarks
// ============================================================================

// BenchmarkMergeFullHive benchmarks merging complete .reg exports
// Tests performance with large, production-sized registry files
func BenchmarkMergeFullHive(b *testing.B) {
	testCases := []struct {
		name     string
		hivePath string
		regPath  string
	}{
		{
			name:     "gohivex/win2003-system",
			hivePath: "../../../testdata/suite/windows-2003-server-system",
			regPath:  "../../../testdata/suite/windows-2003-server-system.reg",
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
				b.Skipf("Reg file not found: %s", tc.regPath)
			}

			// Get reg file size for throughput calculation
			regInfo, err := os.Stat(tc.regPath)
			if err != nil {
				b.Fatalf("Failed to stat reg file: %v", err)
			}

			b.SetBytes(regInfo.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Create temp copy of hive
				tmpFile, err := os.CreateTemp("", "bench-full-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				// Copy hive (use empty minimal hive as base)
				minimalPath := "../../../testdata/minimal"
				hiveData, err := os.ReadFile(minimalPath)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()

				// Merge the full .reg file
				_, err = hive.MergeRegFile(tmpPath, tc.regPath, nil)

				b.StopTimer()

				if err != nil {
					b.Fatalf("Merge failed: %v", err)
				}

				// Cleanup
				os.Remove(tmpPath)
			}
		})
	}
}

// ============================================================================
// Throughput Benchmarks
// ============================================================================

// BenchmarkMergeThroughput measures operations per second and MB/s
func BenchmarkMergeThroughput(b *testing.B) {
	testHive := mergeHives[1] // large hive
	deltaPath := deltaFiles["medium"][3] // medium mixed delta

	// Check if files exist
	if _, err := os.Stat(testHive.Path); os.IsNotExist(err) {
		b.Skipf("Hive not found: %s", testHive.Path)
	}
	if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
		b.Skipf("Delta not found: %s", deltaPath)
	}

	// Get hive size for throughput calculation
	hiveInfo, err := os.Stat(testHive.Path)
	if err != nil {
		b.Fatalf("Failed to stat hive: %v", err)
	}

	b.SetBytes(hiveInfo.Size())
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Create temp copy of hive
		tmpFile, err := os.CreateTemp("", "bench-throughput-*.hive")
		if err != nil {
			b.Fatalf("CreateTemp failed: %v", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		// Copy hive
		hiveData, err := os.ReadFile(testHive.Path)
		if err != nil {
			os.Remove(tmpPath)
			b.Fatalf("ReadFile failed: %v", err)
		}
		if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
			os.Remove(tmpPath)
			b.Fatalf("WriteFile failed: %v", err)
		}

		b.StartTimer()

		// Perform merge
		_, err = hive.MergeRegFile(tmpPath, deltaPath, nil)

		b.StopTimer()

		if err != nil {
			b.Fatalf("Merge failed: %v", err)
		}

		// Cleanup
		os.Remove(tmpPath)
	}
}

// ============================================================================
// Operation-Specific Benchmarks
// ============================================================================

// BenchmarkMergeAddOnly benchmarks pure add operations
func BenchmarkMergeAddOnly(b *testing.B) {
	sizes := []string{"small", "medium", "large"}
	testHive := mergeHives[1] // large hive

	for _, size := range sizes {
		deltaPath := deltaFiles[size][0] // add.reg

		b.Run(fmt.Sprintf("gohivex/%s/%s", testHive.Name, size), func(b *testing.B) {
			if _, err := os.Stat(testHive.Path); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", testHive.Path)
			}
			if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", deltaPath)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				tmpFile, err := os.CreateTemp("", "bench-add-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				hiveData, err := os.ReadFile(testHive.Path)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()
				_, err = hive.MergeRegFile(tmpPath, deltaPath, nil)
				b.StopTimer()

				if err != nil {
					b.Fatalf("Merge failed: %v", err)
				}

				os.Remove(tmpPath)
			}
		})
	}
}

// BenchmarkMergeModifyOnly benchmarks pure modify operations
func BenchmarkMergeModifyOnly(b *testing.B) {
	sizes := []string{"small", "medium", "large"}
	testHive := mergeHives[1] // large hive

	for _, size := range sizes {
		deltaPath := deltaFiles[size][1] // modify.reg

		b.Run(fmt.Sprintf("gohivex/%s/%s", testHive.Name, size), func(b *testing.B) {
			if _, err := os.Stat(testHive.Path); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", testHive.Path)
			}
			if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", deltaPath)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				tmpFile, err := os.CreateTemp("", "bench-mod-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				hiveData, err := os.ReadFile(testHive.Path)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()
				_, err = hive.MergeRegFile(tmpPath, deltaPath, nil)
				b.StopTimer()

				if err != nil {
					b.Fatalf("Merge failed: %v", err)
				}

				os.Remove(tmpPath)
			}
		})
	}
}

// BenchmarkMergeDeleteOnly benchmarks pure delete operations
func BenchmarkMergeDeleteOnly(b *testing.B) {
	sizes := []string{"small", "medium", "large"}
	testHive := mergeHives[1] // large hive

	for _, size := range sizes {
		deltaPath := deltaFiles[size][2] // delete.reg

		b.Run(fmt.Sprintf("gohivex/%s/%s", testHive.Name, size), func(b *testing.B) {
			if _, err := os.Stat(testHive.Path); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", testHive.Path)
			}
			if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", deltaPath)
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				tmpFile, err := os.CreateTemp("", "bench-del-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				hiveData, err := os.ReadFile(testHive.Path)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()
				_, err = hive.MergeRegFile(tmpPath, deltaPath, nil)
				b.StopTimer()

				if err != nil {
					b.Fatalf("Merge failed: %v", err)
				}

				os.Remove(tmpPath)
			}
		})
	}
}

// ============================================================================
// Production Suite Hive Benchmarks (Skip in -short mode)
// ============================================================================

// BenchmarkMergeSuiteHives_System benchmarks merging deltas into production-sized system hives
// These are large (2-9MB) real Windows system hives
func BenchmarkMergeSuiteHives_System(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping production hive benchmarks in short mode")
	}

	testCases := []struct {
		name      string
		hivePath  string
		deltaFile string
		desc      string
	}{
		{
			name:      "win2003-system/small",
			hivePath:  "../../../testdata/suite/windows-2003-server-system",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2003-system-small.reg",
			desc:      "2MB hive, 10 ops",
		},
		{
			name:      "win2003-system/medium",
			hivePath:  "../../../testdata/suite/windows-2003-server-system",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2003-system-medium.reg",
			desc:      "2MB hive, 50+ ops",
		},
		{
			name:      "win2012-system/small",
			hivePath:  "../../../testdata/suite/windows-2012-system",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-system-small.reg",
			desc:      "9MB hive, 10 ops",
		},
		{
			name:      "win2012-system/medium",
			hivePath:  "../../../testdata/suite/windows-2012-system",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-system-medium.reg",
			desc:      "9MB hive, 100+ ops",
		},
		{
			name:      "win2012-system/large",
			hivePath:  "../../../testdata/suite/windows-2012-system",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-system-large.reg",
			desc:      "9MB hive, 500+ ops",
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.deltaFile); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", tc.deltaFile)
			}

			// Get hive size for reporting
			hiveInfo, err := os.Stat(tc.hivePath)
			if err != nil {
				b.Fatalf("Failed to stat hive: %v", err)
			}

			b.SetBytes(hiveInfo.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Create temp copy of hive
				tmpFile, err := os.CreateTemp("", "bench-suite-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				// Copy hive
				hiveData, err := os.ReadFile(tc.hivePath)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()

				// Perform merge
				_, err = hive.MergeRegFile(tmpPath, tc.deltaFile, nil)

				b.StopTimer()

				if err != nil {
					b.Fatalf("Merge failed: %v", err)
				}

				// Cleanup
				os.Remove(tmpPath)
			}
		})
	}
}

// BenchmarkMergeSuiteHives_Software benchmarks merging deltas into production software hives
// These are very large (15-34MB) real Windows software hives
func BenchmarkMergeSuiteHives_Software(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping production software hive benchmarks in short mode")
	}

	testCases := []struct {
		name      string
		hivePath  string
		deltaFile string
		desc      string
	}{
		{
			name:      "win2012-software/small",
			hivePath:  "../../../testdata/suite/windows-2012-software",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-software-small.reg",
			desc:      "34MB hive, 20 ops",
		},
		{
			name:      "win2012-software/medium",
			hivePath:  "../../../testdata/suite/windows-2012-software",
			deltaFile: "../../../testdata/merge_benchmarks/deltas/suite/win2012-software-medium.reg",
			desc:      "34MB hive, 100+ ops",
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				b.Skipf("Hive not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.deltaFile); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", tc.deltaFile)
			}

			// Get hive size for reporting
			hiveInfo, err := os.Stat(tc.hivePath)
			if err != nil {
				b.Fatalf("Failed to stat hive: %v", err)
			}

			b.SetBytes(hiveInfo.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Create temp copy of hive
				tmpFile, err := os.CreateTemp("", "bench-suite-software-*.hive")
				if err != nil {
					b.Fatalf("CreateTemp failed: %v", err)
				}
				tmpPath := tmpFile.Name()
				tmpFile.Close()

				// Copy hive
				hiveData, err := os.ReadFile(tc.hivePath)
				if err != nil {
					os.Remove(tmpPath)
					b.Fatalf("ReadFile failed: %v", err)
				}
				if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
					os.Remove(tmpPath)
					b.Fatalf("WriteFile failed: %v", err)
				}

				b.StartTimer()

				// Perform merge
				_, err = hive.MergeRegFile(tmpPath, tc.deltaFile, nil)

				b.StopTimer()

				if err != nil {
					b.Fatalf("Merge failed: %v", err)
				}

				// Cleanup
				os.Remove(tmpPath)
			}
		})
	}
}

// BenchmarkMergeSuiteSequential benchmarks applying multiple patches sequentially to production hives
// Simulates real-world Windows Update or patch management scenarios
func BenchmarkMergeSuiteSequential(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping production sequential benchmarks in short mode")
	}

	testHive := "../../../testdata/suite/windows-2012-system"
	patchCount := 5

	b.Run(fmt.Sprintf("win2012-system/%d_patches", patchCount), func(b *testing.B) {
		// Check if hive exists
		if _, err := os.Stat(testHive); os.IsNotExist(err) {
			b.Skipf("Hive not found: %s", testHive)
		}

		// Build list of sequential patches
		deltaBasePath := "../../../testdata/merge_benchmarks/deltas/suite/sequential"
		var deltaPaths []string
		for i := 1; i <= patchCount; i++ {
			deltaPath := filepath.Join(deltaBasePath, fmt.Sprintf("patch_%02d.reg", i))
			if _, err := os.Stat(deltaPath); os.IsNotExist(err) {
				b.Skipf("Delta not found: %s", deltaPath)
			}
			deltaPaths = append(deltaPaths, deltaPath)
		}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			b.StopTimer()

			// Create temp copy of hive
			tmpFile, err := os.CreateTemp("", "bench-suite-seq-*.hive")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			hiveData, err := os.ReadFile(testHive)
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", err)
			}
			if err := os.WriteFile(tmpPath, hiveData, 0644); err != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", err)
			}

			b.StartTimer()

			// Apply all patches sequentially
			for _, deltaPath := range deltaPaths {
				_, err := hive.MergeRegFile(tmpPath, deltaPath, nil)
				if err != nil {
					b.Fatalf("Merge failed on %s: %v", deltaPath, err)
				}
			}

			b.StopTimer()

			// Cleanup
			os.Remove(tmpPath)
		}
	})
}
