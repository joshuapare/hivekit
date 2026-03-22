package benchmark

import "testing"

// BenchmarkMergeV2E2E mirrors the BenchmarkMergeE2E fixture x patchset matrix
// for the v2 merge engine. Every sub-test is skip-marked until the v2 API
// (hive/merge/v2) is implemented in Task 8.
//
// Sub-benchmark naming: BenchmarkMergeV2E2E/<fixture>/<patchset>
// This structure is directly compatible with benchstat for v1 vs v2 comparison.
func BenchmarkMergeV2E2E(b *testing.B) {
	b.Skip("v2 merge engine not yet implemented")

	for _, fix := range benchmarkFixtures() {
		fixName := fix.name

		b.Run(fixName, func(b *testing.B) {
			for _, patch := range benchmarkPatches() {
				patchName := patch.name

				b.Run(patchName, func(b *testing.B) {
					b.Skip("v2 merge engine not yet implemented")
				})
			}
		})
	}
}
