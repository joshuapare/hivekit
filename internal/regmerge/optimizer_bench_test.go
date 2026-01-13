package regmerge

import (
	"fmt"
	"strings"
	"testing"
)

// Benchmark_Parsing benchmarks the parsing phase across different file sizes.
func Benchmark_Parsing(b *testing.B) {
	tests := []struct {
		name  string
		files []string
	}{
		{
			name:  "SmallSingleFile",
			files: []string{"base.reg"},
		},
		{
			name:  "MediumMultiFile",
			files: []string{"base.reg", "patch1.reg", "patch2.reg"},
		},
		{
			name:  "LargeDedup",
			files: []string{"duplicates.reg"},
		},
		{
			name:  "DeepDelete",
			files: []string{"deletions.reg"},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Load files once
			var fileData [][]byte
			for _, filename := range tt.files {
				data := loadTestFileB(b, filename)
				fileData = append(fileData, data)
			}

			b.ResetTimer()
			for range b.N {
				_, _, err := ParseAndOptimize(fileData, DefaultOptimizerOptions())
				if err != nil {
					b.Fatalf("ParseAndOptimize failed: %v", err)
				}
			}
		})
	}
}

// Benchmark_OptimizationImpact compares WITH vs WITHOUT optimization.
func Benchmark_OptimizationImpact(b *testing.B) {
	tests := []struct {
		name    string
		files   []string
		enabled bool
	}{
		{
			name:    "Duplicates_WithOptimization",
			files:   []string{"duplicates.reg"},
			enabled: true,
		},
		{
			name:    "Duplicates_WithoutOptimization",
			files:   []string{"duplicates.reg"},
			enabled: false,
		},
		{
			name:    "Deletions_WithOptimization",
			files:   []string{"deletions.reg"},
			enabled: true,
		},
		{
			name:    "Deletions_WithoutOptimization",
			files:   []string{"deletions.reg"},
			enabled: false,
		},
		{
			name:    "MultiFile_WithOptimization",
			files:   []string{"base.reg", "patch1.reg", "patch2.reg"},
			enabled: true,
		},
		{
			name:    "MultiFile_WithoutOptimization",
			files:   []string{"base.reg", "patch1.reg", "patch2.reg"},
			enabled: false,
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Load files once
			var fileData [][]byte
			for _, filename := range tt.files {
				data := loadTestFileB(b, filename)
				fileData = append(fileData, data)
			}

			// Configure optimization
			opts := OptimizerOptions{
				EnableDedup:      tt.enabled,
				EnableDeleteOpt:  tt.enabled,
				EnableOrdering:   tt.enabled,
				EnableSubtreeOpt: tt.enabled,
			}

			b.ResetTimer()
			for range b.N {
				_, _, err := ParseAndOptimize(fileData, opts)
				if err != nil {
					b.Fatalf("ParseAndOptimize failed: %v", err)
				}
			}
		})
	}
}

// Benchmark_FeatureToggles benchmarks different optimizer feature combinations.
func Benchmark_FeatureToggles(b *testing.B) {
	// Use a file that benefits from all optimizations
	fileData := [][]byte{
		loadTestFileB(b, "duplicates.reg"),
		loadTestFileB(b, "deletions.reg"),
	}

	tests := []struct {
		name string
		opts OptimizerOptions
	}{
		{
			name: "AllEnabled",
			opts: OptimizerOptions{
				EnableDedup:      true,
				EnableDeleteOpt:  true,
				EnableOrdering:   true,
				EnableSubtreeOpt: true,
			},
		},
		{
			name: "OnlyDedup",
			opts: OptimizerOptions{
				EnableDedup:      true,
				EnableDeleteOpt:  false,
				EnableOrdering:   false,
				EnableSubtreeOpt: false,
			},
		},
		{
			name: "OnlyDeleteOpt",
			opts: OptimizerOptions{
				EnableDedup:      false,
				EnableDeleteOpt:  true,
				EnableOrdering:   false,
				EnableSubtreeOpt: false,
			},
		},
		{
			name: "OnlyOrdering",
			opts: OptimizerOptions{
				EnableDedup:      false,
				EnableDeleteOpt:  false,
				EnableOrdering:   true,
				EnableSubtreeOpt: false,
			},
		},
		{
			name: "AllDisabled",
			opts: OptimizerOptions{
				EnableDedup:      false,
				EnableDeleteOpt:  false,
				EnableOrdering:   false,
				EnableSubtreeOpt: false,
			},
		},
		{
			name: "DedupAndDelete",
			opts: OptimizerOptions{
				EnableDedup:      true,
				EnableDeleteOpt:  true,
				EnableOrdering:   false,
				EnableSubtreeOpt: false,
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				_, _, err := ParseAndOptimize(fileData, tt.opts)
				if err != nil {
					b.Fatalf("ParseAndOptimize failed: %v", err)
				}
			}
		})
	}
}

// Benchmark_Scale tests optimizer performance at different scales.
func Benchmark_Scale(b *testing.B) {
	tests := []struct {
		name   string
		numOps int
	}{
		{name: "100ops", numOps: 100},
		{name: "1000ops", numOps: 1000},
		{name: "10000ops", numOps: 10000},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Generate synthetic .reg data with many duplicate operations
			regData := generateSyntheticRegData(tt.numOps)
			fileData := [][]byte{[]byte(regData)}

			b.ResetTimer()
			for range b.N {
				_, _, err := ParseAndOptimize(fileData, DefaultOptimizerOptions())
				if err != nil {
					b.Fatalf("ParseAndOptimize failed: %v", err)
				}
			}
		})
	}
}

// Benchmark_OptimizeOnly benchmarks just the optimization phase (no parsing).
func Benchmark_OptimizeOnly(b *testing.B) {
	tests := []struct {
		name  string
		files []string
	}{
		{
			name:  "Duplicates",
			files: []string{"duplicates.reg"},
		},
		{
			name:  "Deletions",
			files: []string{"deletions.reg"},
		},
		{
			name:  "MultiFile",
			files: []string{"base.reg", "patch1.reg", "patch2.reg"},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Parse once, optimize many times
			var fileData [][]byte
			for _, filename := range tt.files {
				data := loadTestFileB(b, filename)
				fileData = append(fileData, data)
			}

			// Pre-parse to get EditOps
			ops, err := ParseFiles(fileData)
			if err != nil {
				b.Fatalf("ParseFiles failed: %v", err)
			}

			b.ResetTimer()
			for range b.N {
				_, _ = Optimize(ops, DefaultOptimizerOptions())
			}
		})
	}
}

// Benchmark_OrderingOnly benchmarks just the ordering phase.
func Benchmark_OrderingOnly(b *testing.B) {
	// Generate operations
	fileData := [][]byte{loadTestFileB(b, "base.reg")}
	ops, err := ParseFiles(fileData)
	if err != nil {
		b.Fatalf("ParseFiles failed: %v", err)
	}

	b.ResetTimer()
	for range b.N {
		_ = orderOps(ops)
	}
}

// Benchmark_PathNormalization benchmarks path normalization.
func Benchmark_PathNormalization(b *testing.B) {
	paths := []string{
		"HKEY_LOCAL_MACHINE\\Software\\Microsoft\\Windows",
		"HKLM\\SOFTWARE\\MICROSOFT\\WINDOWS",
		"Software\\Microsoft\\Windows",
		"software\\microsoft\\windows",
		"SoFtWaRe\\MiCrOsOfT\\WiNdOwS",
	}

	b.ResetTimer()
	for range b.N {
		for _, path := range paths {
			_ = normalizePath(path)
		}
	}
}

// Helper: load test file for benchmarks.
func loadTestFileB(b *testing.B, filename string) []byte {
	b.Helper()
	data := loadTestFile(&testing.T{}, filename)
	return data
}

// Helper: generate synthetic .reg data with specified number of operations.
func generateSyntheticRegData(numOps int) string {
	var sb strings.Builder
	sb.WriteString("Windows Registry Editor Version 5.00\n\n")

	// Generate duplicate SetValue operations (10 unique values repeated)
	numUniqueValues := 10
	for i := range numOps {
		valueNum := (i % numUniqueValues) + 1
		round := i / numUniqueValues

		sb.WriteString("[HKEY_LOCAL_MACHINE\\Software\\BenchTest]\n")
		sb.WriteString(fmt.Sprintf("\"Value%d\"=\"round-%d-v%d\"\n\n", valueNum, round, valueNum))
	}

	return sb.String()
}
