package merge

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/format"
)

// setupBenchHive copies the test hive to a temp directory and opens it.
// Returns the hive and a cleanup function.
func setupBenchHive(b *testing.B) (*hive.Hive, func()) {
	b.Helper()

	tempDir := b.TempDir()
	hivePath := filepath.Join(tempDir, "bench.hive")
	copyTestHive(b, hivePath)

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatalf("Failed to open hive: %v", err)
	}

	return h, func() { h.Close() }
}

// buildWalkApplyPlan creates a plan with numOps SetValue operations
// under distinct keys to exercise the walk-apply engine.
func buildWalkApplyPlan(numOps int, iteration int) *Plan {
	plan := NewPlan()

	for i := range numOps {
		keyPath := []string{fmt.Sprintf("_WalkBench_Iter%d", iteration), fmt.Sprintf("Key%d", i)}
		plan.AddEnsureKey(keyPath)
		plan.AddSetValue(keyPath, "Val", format.REGDWORD, []byte{byte(i), 0, 0, 0})
	}

	return plan
}

// Benchmark_WalkApply_SingleOp benchmarks single-pass walk-apply with 1 op.
func Benchmark_WalkApply_SingleOp(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildWalkApplyPlan(1, i)
		b.StartTimer()

		sess, err := NewSessionForPlan(context.Background(), h, plan, Options{
			IndexMode: IndexModeSinglePass,
		})
		if err != nil {
			b.Fatalf("NewSessionForPlan: %v", err)
		}

		_, err = sess.ApplyPlanDirect(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyPlanDirect: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// Benchmark_WalkApply_SmallPlan benchmarks single-pass walk-apply with 5 ops.
func Benchmark_WalkApply_SmallPlan(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildWalkApplyPlan(5, i)
		b.StartTimer()

		sess, err := NewSessionForPlan(context.Background(), h, plan, Options{
			IndexMode: IndexModeSinglePass,
		})
		if err != nil {
			b.Fatalf("NewSessionForPlan: %v", err)
		}

		_, err = sess.ApplyPlanDirect(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyPlanDirect: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// Benchmark_WalkApply_MediumPlan benchmarks single-pass walk-apply with 25 ops.
func Benchmark_WalkApply_MediumPlan(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildWalkApplyPlan(25, i)
		b.StartTimer()

		sess, err := NewSessionForPlan(context.Background(), h, plan, Options{
			IndexMode: IndexModeSinglePass,
		})
		if err != nil {
			b.Fatalf("NewSessionForPlan: %v", err)
		}

		_, err = sess.ApplyPlanDirect(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyPlanDirect: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// Benchmark_WalkApply_LargePlan benchmarks single-pass walk-apply with 100 ops.
func Benchmark_WalkApply_LargePlan(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildWalkApplyPlan(100, i)
		b.StartTimer()

		sess, err := NewSessionForPlan(context.Background(), h, plan, Options{
			IndexMode: IndexModeSinglePass,
		})
		if err != nil {
			b.Fatalf("NewSessionForPlan: %v", err)
		}

		_, err = sess.ApplyPlanDirect(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyPlanDirect: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// Benchmark_WalkApply_HigherLevel benchmarks the full flow:
// open hive → create session (single-pass) → apply plan → commit.
func Benchmark_WalkApply_HigherLevel(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		tempDir := b.TempDir()
		hivePath := filepath.Join(tempDir, "bench.hive")
		copyTestHive(b, hivePath)
		plan := buildWalkApplyPlan(10, i)
		opts := DefaultOptions()
		opts.IndexMode = IndexModeSinglePass
		b.StartTimer()

		_, err := MergePlan(context.Background(), hivePath, plan, &opts)
		if err != nil {
			b.Fatalf("MergePlan: %v", err)
		}
	}
}

// Benchmark_WalkApply_ExistingKeys benchmarks walk-apply against keys that
// already exist in the hive (pure SetValue, no key creation).
func Benchmark_WalkApply_ExistingKeys(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		_ = i

		// These paths exist in the windows-2003-server-system hive
		plan := NewPlan()
		plan.AddSetValue(
			[]string{"ControlSet001", "Control"},
			"BenchVal", format.REGSZ, []byte("test\x00"),
		)
		plan.AddSetValue(
			[]string{"ControlSet001", "Services"},
			"BenchVal", format.REGSZ, []byte("test\x00"),
		)

		b.StartTimer()

		sess, err := NewSessionForPlan(context.Background(), h, plan, Options{
			IndexMode: IndexModeSinglePass,
		})
		if err != nil {
			b.Fatalf("NewSessionForPlan: %v", err)
		}

		_, err = sess.ApplyPlanDirect(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyPlanDirect: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// Benchmark_normalizePath benchmarks the normalizePath function.
func Benchmark_normalizePath(b *testing.B) {
	paths := [][]string{
		{"Software", "Microsoft", "Windows", "CurrentVersion"},
		{"ControlSet001", "Services", "LanmanServer", "Parameters"},
		{"System"},
		{},
	}

	b.ReportAllocs()

	for range b.N {
		for _, p := range paths {
			_ = normalizePath(p)
		}
	}
}

