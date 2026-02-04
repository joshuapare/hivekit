package merge

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
)

// Benchmark_MultiHive_FullIndex opens the same hive N times, builds a full index,
// applies a plan, and closes. Measures cross-hive amortization of infrastructure
// (index maps, bitmaps, decoded strings).
func Benchmark_MultiHive_FullIndex(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildWalkApplyPlan(5, i)
		opts := DefaultOptions()
		opts.IndexMode = IndexModeFull
		b.StartTimer()

		sess, err := NewSession(context.Background(), h, opts)
		if err != nil {
			b.Fatalf("NewSession: %v", err)
		}

		_, err = sess.ApplyWithTx(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyWithTx: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// Benchmark_MultiHive_SinglePass opens the same hive N times, applies a plan
// via single-pass walk-apply, and closes. Measures cross-hive amortization.
func Benchmark_MultiHive_SinglePass(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildWalkApplyPlan(5, i)
		opts := DefaultOptions()
		opts.IndexMode = IndexModeSinglePass
		b.StartTimer()

		sess, err := NewSessionForPlan(context.Background(), h, plan, opts)
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

// Benchmark_MultiHive_IndexBuildOnly isolates the index build cost across
// multiple hive sessions. Each iteration opens a hive and builds the full index.
func Benchmark_MultiHive_IndexBuildOnly(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		b.StopTimer()
		tempDir := b.TempDir()
		hivePath := filepath.Join(tempDir, "bench.hive")
		copyTestHive(b, hivePath)

		h, err := hive.Open(hivePath)
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		b.StartTimer()

		builder := walker.NewIndexBuilder(h, 0, 0)
		idx, err := builder.Build(context.Background())
		if err != nil {
			b.Fatalf("Build: %v", err)
		}
		_ = idx

		b.StopTimer()
		h.Close()
		b.StartTimer()
	}
}

// Benchmark_MultiHive_RepeatedApply simulates the patcher-service pattern:
// open hive, apply small plan, close, repeat. Measures the amortization of
// pooled resources across sequential sessions.
func Benchmark_MultiHive_RepeatedApply(b *testing.B) {
	b.ReportAllocs()

	plan := NewPlan()
	plan.AddSetValue(
		[]string{"ControlSet001", "Control"},
		"BenchVal", format.REGSZ, []byte("test\x00"),
	)
	plan.AddSetValue(
		[]string{"ControlSet001", "Services"},
		"BenchVal", format.REGSZ, []byte("test\x00"),
	)

	for range b.N {
		b.StopTimer()
		tempDir := b.TempDir()
		hivePath := filepath.Join(tempDir, "bench.hive")
		copyTestHive(b, hivePath)
		opts := DefaultOptions()
		opts.IndexMode = IndexModeSinglePass
		b.StartTimer()

		_, err := MergePlan(context.Background(), hivePath, plan, &opts)
		if err != nil {
			b.Fatalf("MergePlan: %v", err)
		}
	}
}

// Benchmark_MultiHive_ServiceMerge simulates adding services under
// ControlSet001\Services (150+ existing siblings). Each key insertion
// triggers subkeys.Read() which decodes ALL sibling names.
// This is the hot path for decode cache improvement.
func Benchmark_MultiHive_ServiceMerge(b *testing.B) {
	b.ReportAllocs()

	for i := range b.N {
		b.StopTimer()
		h, cleanup := setupBenchHive(b)
		plan := buildServicePlan(5, i)
		opts := DefaultOptions()
		opts.IndexMode = IndexModeFull
		b.StartTimer()

		sess, err := NewSession(context.Background(), h, opts)
		if err != nil {
			b.Fatalf("NewSession: %v", err)
		}

		_, err = sess.ApplyWithTx(context.Background(), plan)
		if err != nil {
			b.Fatalf("ApplyWithTx: %v", err)
		}

		b.StopTimer()
		sess.Close(context.Background())
		cleanup()
		b.StartTimer()
	}
}

// buildServicePlan creates a plan that adds service entries under
// ControlSet001\Services, exercising the decode-heavy subkeys.Read() path.
func buildServicePlan(numOps int, iteration int) *Plan {
	plan := NewPlan()
	for i := range numOps {
		keyPath := []string{
			"ControlSet001", "Services",
			fmt.Sprintf("_BenchSvc%d_%d", iteration, i),
		}
		plan.AddEnsureKey(keyPath)
		plan.AddSetValue(keyPath, "Start", format.REGDWORD, []byte{2, 0, 0, 0})
	}
	return plan
}
