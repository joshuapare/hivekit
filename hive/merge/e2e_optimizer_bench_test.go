package merge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/internal/regmerge"
)

// Benchmark_E2E tests the full merge pipeline: parse → optimize → merge execution
// This measures the TRUE performance impact including hive I/O, not just parsing

func Benchmark_E2E_SmallClean(b *testing.B) {
	regData := generateSmallCleanReg()
	benchmarkE2EScenario(b, "SmallClean", regData)
}

func Benchmark_E2E_Duplicates(b *testing.B) {
	regData := loadRegmergeTestFile(b, "duplicates.reg")
	benchmarkE2EScenario(b, "Duplicates", string(regData))
}

func Benchmark_E2E_DeleteShadowing(b *testing.B) {
	regData := loadRegmergeTestFile(b, "deletions.reg")
	benchmarkE2EScenario(b, "DeleteShadowing", string(regData))
}

func Benchmark_E2E_MultiFile(b *testing.B) {
	base := string(loadRegmergeTestFile(b, "base.reg"))
	patch1 := string(loadRegmergeTestFile(b, "patch1.reg"))
	patch2 := string(loadRegmergeTestFile(b, "patch2.reg"))
	runBenchmarkE2EMultiFile(b, "MultiFile", []string{base, patch1, patch2})
}

func Benchmark_E2E_MixedCase(b *testing.B) {
	regData := loadRegmergeTestFile(b, "mixed_case.reg")
	benchmarkE2EScenario(b, "MixedCase", string(regData))
}

func Benchmark_E2E_RealWorld(b *testing.B) {
	// Combine multiple realistic scenarios
	regData := generateRealWorldReg()
	benchmarkE2EScenario(b, "RealWorld", regData)
}

// benchmarkE2EScenario runs a single-file E2E benchmark with/without optimization.
func benchmarkE2EScenario(b *testing.B, name string, regData string) {
	b.Run(name+"/WithOptimization", func(b *testing.B) {
		benchmarkE2ESingleFile(b, regData, true)
	})

	b.Run(name+"/WithoutOptimization", func(b *testing.B) {
		benchmarkE2ESingleFile(b, regData, false)
	})
}

// runBenchmarkE2EMultiFile runs a multi-file E2E benchmark with/without optimization.
func runBenchmarkE2EMultiFile(b *testing.B, name string, regTexts []string) {
	b.Run(name+"/WithOptimization", func(b *testing.B) {
		benchmarkE2EMultiFile(b, regTexts, true)
	})

	b.Run(name+"/WithoutOptimization", func(b *testing.B) {
		benchmarkE2EMultiFile(b, regTexts, false)
	})
}

// benchmarkE2ESingleFile tests the full pipeline for a single file.
func benchmarkE2ESingleFile(b *testing.B, regData string, optimize bool) {
	baseHive := getBaseHive(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		// Full E2E pipeline
		var plan *Plan
		var err error

		if optimize {
			// Use optimizer
			var stats regmerge.Stats
			plan, stats, err = PlanFromRegTexts([]string{regData})
			if err != nil {
				b.Fatalf("PlanFromRegTexts failed: %v", err)
			}
			b.ReportMetric(float64(stats.InputOps), "input_ops")
			b.ReportMetric(float64(stats.OutputOps), "output_ops")
			if stats.InputOps > 0 {
				b.ReportMetric(stats.ReductionPercent(), "reduction_%")
			}
		} else {
			// Skip optimizer - parse directly
			plan, err = PlanFromRegText(regData)
			if err != nil {
				b.Fatalf("PlanFromRegText failed: %v", err)
			}
		}

		// Execute merge (same backend for both)
		_, err = MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatalf("MergePlan failed: %v", err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// benchmarkE2EMultiFile tests the full pipeline for multiple files.
func benchmarkE2EMultiFile(b *testing.B, regTexts []string, optimize bool) {
	baseHive := getBaseHive(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		// Full E2E pipeline
		var plan *Plan
		var err error

		if optimize {
			// Use optimizer for multi-file (cross-file optimization)
			var stats regmerge.Stats
			plan, stats, err = PlanFromRegTexts(regTexts)
			if err != nil {
				b.Fatalf("PlanFromRegTexts failed: %v", err)
			}
			b.ReportMetric(float64(stats.InputOps), "input_ops")
			b.ReportMetric(float64(stats.OutputOps), "output_ops")
			if stats.InputOps > 0 {
				b.ReportMetric(stats.ReductionPercent(), "reduction_%")
			}
		} else {
			// Without optimizer - parse each file separately and combine
			plan = NewPlan()
			for _, regText := range regTexts {
				filePlan, parseErr := PlanFromRegText(regText)
				if parseErr != nil {
					b.Fatalf("PlanFromRegText failed: %v", parseErr)
				}
				// Append operations from this file's plan (Plan.Ops is public)
				plan.Ops = append(plan.Ops, filePlan.Ops...)
			}
		}

		// Execute merge (same backend for both)
		_, err = MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatalf("MergePlan failed: %v", err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// Helper functions

// getBaseHive returns path to the base test hive (already uncompressed in testdata/suite).
func getBaseHive(b *testing.B) string {
	b.Helper()

	// Use windows-2003-server-system (same as comparison_bench_test.go)
	// The uncompressed version is already available in testdata/suite
	// From hive/merge/ we need to go up two levels to repo root
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2003-server-system")

	// Verify it exists
	if _, err := os.Stat(hivePath); err != nil {
		b.Fatalf("Base hive not found at %s: %v", hivePath, err)
	}

	return hivePath
}

// Note: createTempCopy() is already defined in comparison_bench_test.go

// Test data generators

func generateSmallCleanReg() string {
	// 10 operations, no redundancy (baseline - optimizer should have minimal impact)
	return `Windows Registry Editor Version 5.00

[HKEY_LOCAL_MACHINE\Software\BenchTest]
"Value1"="data1"
"Value2"="data2"
"Value3"="data3"
"Value4"=dword:00000004
"Value5"=dword:00000005

[HKEY_LOCAL_MACHINE\Software\BenchTest\SubKey]
"SubValue1"="subdata1"
"SubValue2"="subdata2"
"SubValue3"="subdata3"
`
}

func generateRealWorldReg() string {
	// Realistic scenario: mix of updates, duplicates, deletes
	return `Windows Registry Editor Version 5.00

; Initial setup
[HKEY_LOCAL_MACHINE\Software\MyApp]
"Version"="1.0.0"
"InstallPath"="C:\\Program Files\\MyApp"

[HKEY_LOCAL_MACHINE\Software\MyApp\Config]
"Setting1"="value1"
"Setting2"=dword:00000001

; Updates (will deduplicate)
[HKEY_LOCAL_MACHINE\Software\MyApp]
"Version"="1.0.1"

[HKEY_LOCAL_MACHINE\Software\MyApp]
"Version"="1.0.2"

; More config
[HKEY_LOCAL_MACHINE\Software\MyApp\Config]
"Setting3"="value3"
"Setting1"="value1-updated"

; Delete old settings
[HKEY_LOCAL_MACHINE\Software\MyApp\OldSettings]
"Legacy1"="old"
"Legacy2"="old"

[-HKEY_LOCAL_MACHINE\Software\MyApp\OldSettings]

; New features
[HKEY_LOCAL_MACHINE\Software\MyApp\Features]
"Feature1"=dword:00000001
"Feature2"=dword:00000001

[HKEY_LOCAL_MACHINE\Software\MyApp\Features]
"Feature3"=dword:00000001
`
}

// loadRegmergeTestFile loads a test file from internal/regmerge/testdata.
func loadRegmergeTestFile(b *testing.B, filename string) []byte {
	b.Helper()

	// From hive/merge/ we need to go to internal/regmerge/testdata
	path := filepath.Join("..", "..", "internal", "regmerge", "testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("Failed to load test file %s: %v", filename, err)
	}
	return data
}
