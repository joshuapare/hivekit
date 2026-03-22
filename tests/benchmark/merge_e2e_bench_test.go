package benchmark

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
)

// fixtureCache provides per-fixture lazy generation so only the fixtures
// actually referenced by the benchmark matrix are built. Each fixture is
// generated at most once per test binary invocation.
var fixtureCache = struct {
	mu    sync.Mutex
	dir   string
	paths map[string]string // fixture name -> file path
}{
	paths: make(map[string]string),
}

// fixtureGenerators maps fixture names to their generator functions.
var fixtureGenerators = map[string]func(string) error{
	"small-flat":      GenerateSmallFlat,
	"small-deep":      GenerateSmallDeep,
	"medium-mixed":    GenerateMediumMixed,
	"large-wide":      GenerateLargeWide,
	"large-realistic": GenerateLargeRealistic,
}

// ensureFixture generates a single fixture if it hasn't been built yet and
// returns its file path. Thread-safe.
func ensureFixture(b *testing.B, name string) string {
	b.Helper()

	fixtureCache.mu.Lock()
	defer fixtureCache.mu.Unlock()

	// Return cached path if already generated.
	if p, ok := fixtureCache.paths[name]; ok {
		return p
	}

	// Create shared fixture directory on first call.
	if fixtureCache.dir == "" {
		dir, err := os.MkdirTemp("", "hivekit-e2e-fixtures-*")
		if err != nil {
			b.Fatalf("create fixture dir: %v", err)
		}
		fixtureCache.dir = dir
	}

	gen, ok := fixtureGenerators[name]
	if !ok {
		b.Fatalf("unknown fixture name: %s", name)
	}

	path := filepath.Join(fixtureCache.dir, name+".hive")
	if err := gen(path); err != nil {
		b.Fatalf("generate fixture %s: %v", name, err)
	}

	fixtureCache.paths[name] = path
	return path
}

// copyHiveFile copies a fixture hive to a temporary path so each benchmark
// iteration operates on a fresh file. Uses os.ReadFile/os.WriteFile for speed.
func copyHiveFile(b *testing.B, src, dstDir, name string) string {
	b.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		b.Fatalf("read fixture %s: %v", name, err)
	}
	dst := filepath.Join(dstDir, name+".hive")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		b.Fatalf("write copy %s: %v", name, err)
	}
	return dst
}

// fixtureEntry maps a human-readable name to its generator.
type fixtureEntry struct {
	name string
}

// benchmarkFixtures returns the default fixture matrix. Skips small-deep and
// large-realistic to keep the default run manageable; they can be added later
// or run selectively via -bench filtering.
func benchmarkFixtures() []fixtureEntry {
	return []fixtureEntry{
		{"small-flat"},
		{"medium-mixed"},
		{"large-wide"},
	}
}

// patchEntry describes how to build a merge.Plan for a given fixture.
type patchEntry struct {
	name    string
	buildFn func(fixtureName string) *merge.Plan
}

// benchmarkPatches returns the default patch-set matrix.
func benchmarkPatches() []patchEntry {
	return []patchEntry{
		{
			name: "create-sparse-100",
			buildFn: func(_ string) *merge.Plan {
				ops := GenerateCreateSparse(100, 1001)
				return &merge.Plan{Ops: ops}
			},
		},
		{
			name: "create-dense-500",
			buildFn: func(_ string) *merge.Plan {
				ops := GenerateCreateDense(500, 1002)
				return &merge.Plan{Ops: ops}
			},
		},
		{
			name: "delete-values-500",
			buildFn: func(fixtureName string) *merge.Plan {
				keys := CollectExistingKeys(fixtureName, 500, 1003)
				ops := GenerateDeleteValues(500, keys, 1003)
				return &merge.Plan{Ops: ops}
			},
		},
		{
			name: "delete-keys-leaf-200",
			buildFn: func(fixtureName string) *merge.Plan {
				keys := CollectExistingKeys(fixtureName, 200, 1004)
				ops := GenerateDeleteKeysLeaf(200, keys, 1004)
				return &merge.Plan{Ops: ops}
			},
		},
		{
			name: "mixed-realistic-1000",
			buildFn: func(fixtureName string) *merge.Plan {
				keys := CollectExistingKeys(fixtureName, 1000, 1005)
				ops := GenerateMixedRealistic(1000, keys, 1005)
				return &merge.Plan{Ops: ops}
			},
		},
	}
}

// BenchmarkMergeE2E runs the full fixture x patchset benchmark matrix.
//
// Sub-benchmark naming: BenchmarkMergeE2E/<fixture>/<patchset>
// This structure is directly compatible with benchstat for comparison.
//
// Fixtures are generated lazily and cached for the lifetime of the test binary.
// Each benchmark iteration copies the fixture hive to a temp file so the merge
// operates on a pristine hive every time.
func BenchmarkMergeE2E(b *testing.B) {
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

					tmpDir := BenchTempDir(b)

					for i := 0; i < b.N; i++ {
						b.StopTimer()

						// Copy the fixture hive so each iteration starts fresh.
						iterPath := copyHiveFile(b, fixPath, tmpDir,
							fmt.Sprintf("iter-%d", i))

						sizeBefore := HiveSize(iterPath)

						b.StartTimer()

						// Open hive, create session, apply, close.
						h, err := hive.Open(iterPath)
						if err != nil {
							b.Fatalf("open hive: %v", err)
						}

						session, err := merge.NewSessionForPlan(ctx, h, plan, merge.DefaultOptions())
						if err != nil {
							h.Close()
							b.Fatalf("create session: %v", err)
						}

						if _, err := session.ApplyWithTx(ctx, plan); err != nil {
							session.Close(ctx)
							h.Close()
							b.Fatalf("apply plan: %v", err)
						}

						session.Close(ctx)
						h.Close()

						b.StopTimer()

						sizeAfter := HiveSize(iterPath)
						b.ReportMetric(float64(sizeAfter-sizeBefore), "hive-growth-bytes")

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
