package merge

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// copyTestHive copies the test hive to a temporary location for benchmarking.
func copyTestHive(b *testing.B, destPath string) {
	b.Helper()
	testHivePath := "../../testdata/suite/windows-2003-server-system"

	src, err := os.Open(testHivePath)
	if err != nil {
		b.Skipf("Test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		b.Fatalf("Failed to create temp hive: %v", err)
	}
	defer dst.Close()

	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		b.Fatalf("Failed to copy hive: %v", copyErr)
	}
}

// Benchmark_IndexMode_SingleOp benchmarks a single operation with different index modes.
// This is where single-pass mode should show the biggest advantage.
func Benchmark_IndexMode_SingleOp(b *testing.B) {
	b.Run("FullIndex", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 1, IndexModeFull)
	})
	b.Run("SinglePass", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 1, IndexModeSinglePass)
	})
}

// Benchmark_IndexMode_SmallPlan benchmarks a small plan (5 ops) with different index modes.
func Benchmark_IndexMode_SmallPlan(b *testing.B) {
	b.Run("FullIndex", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 5, IndexModeFull)
	})
	b.Run("SinglePass", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 5, IndexModeSinglePass)
	})
}

// Benchmark_IndexMode_MediumPlan benchmarks a medium plan (25 ops) with different index modes.
func Benchmark_IndexMode_MediumPlan(b *testing.B) {
	b.Run("FullIndex", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 25, IndexModeFull)
	})
	b.Run("SinglePass", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 25, IndexModeSinglePass)
	})
}

// Benchmark_IndexMode_LargePlan benchmarks a larger plan (100 ops) with different index modes.
// This is around the crossover point where full index may become advantageous.
func Benchmark_IndexMode_LargePlan(b *testing.B) {
	b.Run("FullIndex", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 100, IndexModeFull)
	})
	b.Run("SinglePass", func(b *testing.B) {
		benchmarkMergePlanWithMode(b, 100, IndexModeSinglePass)
	})
}

// Benchmark_IndexMode_Scaling benchmarks how performance scales with plan size.
func Benchmark_IndexMode_Scaling(b *testing.B) {
	sizes := []int{1, 2, 5, 10, 25, 50, 100}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("FullIndex_%dOps", size), func(b *testing.B) {
			benchmarkMergePlanWithMode(b, size, IndexModeFull)
		})
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("SinglePass_%dOps", size), func(b *testing.B) {
			benchmarkMergePlanWithMode(b, size, IndexModeSinglePass)
		})
	}
}

// benchmarkMergePlanWithMode runs the benchmark with a specific index mode.
func benchmarkMergePlanWithMode(b *testing.B, numOps int, mode IndexMode) {
	b.Helper()
	b.ReportAllocs()

	// Pre-build the plan template (operations will be customized per iteration)
	planTemplate := buildPlanTemplate(numOps)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Create a fresh hive copy for each iteration
		tempDir := b.TempDir()
		hivePath := filepath.Join(tempDir, "bench.hive")
		copyTestHive(b, hivePath)

		// Customize the plan for this iteration to avoid conflicts
		plan := customizePlan(planTemplate, i)

		opts := DefaultOptions()
		opts.IndexMode = mode

		b.StartTimer()

		_, err := MergePlan(context.Background(), hivePath, plan, &opts)
		if err != nil {
			b.Fatalf("MergePlan failed: %v", err)
		}
	}
}

// buildPlanTemplate creates a plan with the specified number of operations.
func buildPlanTemplate(numOps int) *Plan {
	plan := NewPlan()

	for i := 0; i < numOps; i++ {
		keyPath := []string{"_BenchTest", fmt.Sprintf("Key%d", i)}
		plan.AddEnsureKey(keyPath)
		plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(i), 0, 0, 0})
	}

	return plan
}

// customizePlan creates a unique plan for each iteration to avoid key conflicts.
func customizePlan(template *Plan, iteration int) *Plan {
	plan := NewPlan()

	for _, op := range template.Ops {
		// Create a new path with iteration prefix
		newPath := make([]string, len(op.KeyPath))
		copy(newPath, op.KeyPath)
		if len(newPath) > 0 {
			newPath[0] = fmt.Sprintf("_BenchTest_Iter%d", iteration)
		}

		switch op.Type {
		case OpEnsureKey:
			plan.AddEnsureKey(newPath)
		case OpSetValue:
			plan.AddSetValue(newPath, op.ValueName, op.ValueType, op.Data)
		case OpDeleteValue:
			plan.AddDeleteValue(newPath, op.ValueName)
		case OpDeleteKey:
			plan.AddDeleteKey(newPath)
		}
	}

	return plan
}

// Benchmark_IndexMode_DeepPath benchmarks operations on deep key paths.
func Benchmark_IndexMode_DeepPath(b *testing.B) {
	b.Run("FullIndex", func(b *testing.B) {
		benchmarkDeepPathWithMode(b, IndexModeFull)
	})
	b.Run("SinglePass", func(b *testing.B) {
		benchmarkDeepPathWithMode(b, IndexModeSinglePass)
	})
}

func benchmarkDeepPathWithMode(b *testing.B, mode IndexMode) {
	b.Helper()
	b.ReportAllocs()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		tempDir := b.TempDir()
		hivePath := filepath.Join(tempDir, "bench.hive")
		copyTestHive(b, hivePath)

		// Create a deep path: 10 levels deep
		plan := NewPlan()
		path := []string{fmt.Sprintf("_DeepTest_Iter%d", i)}
		for j := 0; j < 10; j++ {
			path = append(path, fmt.Sprintf("Level%d", j))
		}
		plan.AddEnsureKey(path)
		plan.AddSetValue(path, "DeepValue", format.REGSZ, []byte("deep\x00"))

		opts := DefaultOptions()
		opts.IndexMode = mode

		b.StartTimer()

		_, err := MergePlan(context.Background(), hivePath, plan, &opts)
		if err != nil {
			b.Fatalf("MergePlan failed: %v", err)
		}
	}
}

// Benchmark_IndexMode_MultipleKeys benchmarks creating multiple sibling keys.
func Benchmark_IndexMode_MultipleKeys(b *testing.B) {
	b.Run("FullIndex_10Keys", func(b *testing.B) {
		benchmarkMultipleKeysWithMode(b, 10, IndexModeFull)
	})
	b.Run("SinglePass_10Keys", func(b *testing.B) {
		benchmarkMultipleKeysWithMode(b, 10, IndexModeSinglePass)
	})
	b.Run("FullIndex_50Keys", func(b *testing.B) {
		benchmarkMultipleKeysWithMode(b, 50, IndexModeFull)
	})
	b.Run("SinglePass_50Keys", func(b *testing.B) {
		benchmarkMultipleKeysWithMode(b, 50, IndexModeSinglePass)
	})
}

func benchmarkMultipleKeysWithMode(b *testing.B, numKeys int, mode IndexMode) {
	b.Helper()
	b.ReportAllocs()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		tempDir := b.TempDir()
		hivePath := filepath.Join(tempDir, "bench.hive")
		copyTestHive(b, hivePath)

		plan := NewPlan()
		basePath := []string{fmt.Sprintf("_MultiKey_Iter%d", i)}
		plan.AddEnsureKey(basePath)

		for j := 0; j < numKeys; j++ {
			keyPath := append(basePath, fmt.Sprintf("Child%d", j))
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Val", format.REGDWORD, []byte{byte(j), 0, 0, 0})
		}

		opts := DefaultOptions()
		opts.IndexMode = mode

		b.StartTimer()

		_, err := MergePlan(context.Background(), hivePath, plan, &opts)
		if err != nil {
			b.Fatalf("MergePlan failed: %v", err)
		}
	}
}
