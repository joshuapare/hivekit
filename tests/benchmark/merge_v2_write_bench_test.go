package benchmark

import (
	"fmt"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/merge/v2/plan"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/merge/v2/walk"
	"github.com/joshuapare/hivekit/hive/merge/v2/write"
)

func BenchmarkRebuildSubkeyList(b *testing.B) {
	fixPath := ensureFixture(b, "large-wide")
	ops := GenerateCreateSparse(5, 9999)
	tmpDir := BenchTempDir(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		iterPath := copyHiveFile(b, fixPath, tmpDir, fmt.Sprintf("rebuild-%d", i))
		h, err := hive.Open(iterPath)
		if err != nil {
			b.Fatalf("open: %v", err)
		}
		root := trie.Build(ops)
		if err := walk.Annotate(h, root); err != nil {
			h.Close()
			b.Fatalf("walk: %v", err)
		}
		sp, err := plan.Estimate(root)
		if err != nil {
			h.Close()
			b.Fatalf("plan: %v", err)
		}
		dt := dirty.NewTracker(h)
		fa, err := alloc.NewFast(h, dt, nil)
		if err != nil {
			h.Close()
			b.Fatalf("alloc: %v", err)
		}
		if sp.TotalNewBytes > 0 {
			if err := fa.EnableBumpMode(sp.TotalNewBytes); err != nil {
				h.Close()
				b.Fatalf("bump: %v", err)
			}
		}
		b.StartTimer()
		_, _, writeErr := write.Execute(h, root, sp, fa)
		b.StopTimer()
		if writeErr != nil {
			h.Close()
			b.Fatalf("write: %v", writeErr)
		}
		_ = fa.FinalizeBumpMode()
		h.Close()
		os.Remove(iterPath)
	}
}

func BenchmarkProcessValues(b *testing.B) {
	fixPath := ensureFixture(b, "large-wide")
	existingKeys := CollectExistingKeys("large-wide", 50, 8888)
	ops := GenerateDeleteValues(50, existingKeys, 8888)
	tmpDir := BenchTempDir(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		iterPath := copyHiveFile(b, fixPath, tmpDir, fmt.Sprintf("values-%d", i))
		h, err := hive.Open(iterPath)
		if err != nil {
			b.Fatalf("open: %v", err)
		}
		root := trie.Build(ops)
		if err := walk.Annotate(h, root); err != nil {
			h.Close()
			b.Fatalf("walk: %v", err)
		}
		sp, err := plan.Estimate(root)
		if err != nil {
			h.Close()
			b.Fatalf("plan: %v", err)
		}
		dt := dirty.NewTracker(h)
		fa, err := alloc.NewFast(h, dt, nil)
		if err != nil {
			h.Close()
			b.Fatalf("alloc: %v", err)
		}
		if sp.TotalNewBytes > 0 {
			if err := fa.EnableBumpMode(sp.TotalNewBytes); err != nil {
				h.Close()
				b.Fatalf("bump: %v", err)
			}
		}
		b.StartTimer()
		_, _, writeErr := write.Execute(h, root, sp, fa)
		b.StopTimer()
		if writeErr != nil {
			h.Close()
			b.Fatalf("write: %v", writeErr)
		}
		_ = fa.FinalizeBumpMode()
		h.Close()
		os.Remove(iterPath)
	}
}
