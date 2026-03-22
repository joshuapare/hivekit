package benchmark

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	v2 "github.com/joshuapare/hivekit/hive/merge/v2"
)

// BenchmarkMergeV2E2E mirrors the BenchmarkMergeE2E fixture x patchset matrix
// for the v2 merge engine, enabling direct benchstat comparison between v1 and v2.
//
// Sub-benchmark naming: BenchmarkMergeV2E2E/<fixture>/<patchset>
// This structure is directly compatible with benchstat for v1 vs v2 comparison.
func BenchmarkMergeV2E2E(b *testing.B) {
	ctx := context.Background()

	for _, fix := range benchmarkFixtures() {
		fixName := fix.name

		b.Run(fixName, func(b *testing.B) {
			// Generate this fixture once (cached across sub-benchmarks).
			fixPath := ensureFixture(b, fixName)

			for _, patch := range benchmarkPatches() {
				patchName := patch.name
				buildFn := patch.buildFn

				b.Run(patchName, func(b *testing.B) {
					b.ReportAllocs()

					// Build the plan once outside the iteration loop. The plan
					// is read-only data ([]Op) so it is safe to reuse across
					// iterations without affecting correctness.
					plan := buildFn(fixName)
					ops := plan.Ops

					tmpDir := BenchTempDir(b)

					for i := 0; i < b.N; i++ {
						b.StopTimer()

						// Copy the fixture hive so each iteration starts fresh.
						iterPath := copyHiveFile(b, fixPath, tmpDir,
							fmt.Sprintf("iter-%d", i))

						sizeBefore := HiveSize(iterPath)

						b.StartTimer()

						// Open hive and apply via the v2 pipeline.
						h, err := hive.Open(iterPath)
						if err != nil {
							b.Fatalf("open hive: %v", err)
						}

						result, err := v2.Merge(ctx, h, ops, v2.Options{})
						if err != nil {
							h.Close()
							b.Fatalf("v2.Merge: %v", err)
						}

						h.Close()

						b.StopTimer()

						sizeAfter := HiveSize(iterPath)
						b.ReportMetric(float64(sizeAfter-sizeBefore), "hive-growth-bytes")
						b.ReportMetric(float64(result.PhaseTiming.Parse.Nanoseconds()), "parse-ns")
						b.ReportMetric(float64(result.PhaseTiming.Walk.Nanoseconds()), "walk-ns")
						b.ReportMetric(float64(result.PhaseTiming.Plan.Nanoseconds()), "plan-ns")
						b.ReportMetric(float64(result.PhaseTiming.Write.Nanoseconds()), "write-ns")
						b.ReportMetric(float64(result.PhaseTiming.Flush.Nanoseconds()), "flush-ns")

						// Remove the iteration file to avoid filling the temp dir
						// during high-iteration runs.
						os.Remove(iterPath)

						b.StartTimer()
					}
				})
			}
		})
	}
}
