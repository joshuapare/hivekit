package index

import (
	"testing"
	"unique"
)

// Microbenchmark: Measure JUST map insertion performance with pre-allocated data.
func Benchmark_MapInsertions(b *testing.B) {
	// Pre-generate keys to eliminate key generation cost
	const numKeys = 63000

	// Generate data once
	keys := make([]string, numKeys)
	for i := range numKeys {
		keys[i] = "key" + string(rune(i%1000)) // Limited cardinality to simulate real hives
	}

	b.Run("StringMap", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			m := make(map[string]uint32, numKeys)
			for j, k := range keys {
				m[k] = uint32(j)
			}
		}
	})

	b.Run("UniqueHandleMap", func(b *testing.B) {
		// Pre-intern all strings (simulates warm cache in long-running service)
		internedKeys := make([]unique.Handle[string], numKeys)
		for i, k := range keys {
			internedKeys[i] = unique.Make(k)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			m := make(map[unique.Handle[string]]uint32, numKeys)
			for j, k := range internedKeys {
				m[k] = uint32(j) // Just pointer copy, no interning!
			}
		}
	})

	b.Run("UniqueHandleMap_ColdCache", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			m := make(map[unique.Handle[string]]uint32, numKeys)
			for j, k := range keys {
				m[unique.Make(k)] = uint32(j) // Intern on every insert
			}
		}
	})
}

// Benchmark: Does unique.Make() benefit from warm cache?
func Benchmark_UniqueWarmCache(b *testing.B) {
	keys := []string{"System", "Software", "Services", "CurrentControlSet"}

	b.Run("FirstRun_ColdCache", func(b *testing.B) {
		// Note: unique cache persists across runs, so only first run is truly cold
		b.ReportAllocs()
		for range b.N {
			for _, k := range keys {
				_ = unique.Make(k)
			}
		}
	})

	// Pre-warm the cache
	for _, k := range keys {
		unique.Make(k)
	}

	b.Run("SubsequentRun_WarmCache", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for _, k := range keys {
				_ = unique.Make(k) // Should be fast - just lookup
			}
		}
	})
}

// Benchmark: Full pipeline breakdown.
func Benchmark_BuildPipeline(b *testing.B) {
	data := loadHiveData(b, testHives[0])

	b.Run("Step1_LoadHive", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = loadHiveData(b, testHives[0])
		}
	})

	b.Run("Step2_IndexBuild_StringIndex", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			idx := NewStringIndex(len(data.nks), len(data.vks))
			for _, nk := range data.nks {
				idx.AddNK(nk[0], data.names[nk[2]], nk[1])
			}
			for _, vk := range data.vks {
				idx.AddVK(vk[0], data.names[vk[2]], vk[1])
			}
		}
	})

	b.Run("Step2_IndexBuild_UniqueV2", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			idx := NewUniqueIndexV2(len(data.nks), len(data.vks))
			for _, nk := range data.nks {
				idx.AddNK(nk[0], data.names[nk[2]], nk[1])
			}
			for _, vk := range data.vks {
				idx.AddVK(vk[0], data.names[vk[2]], vk[1])
			}
		}
	})

	b.Run("Step3_FullScan_StringIndex", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			scanData := loadHiveData(b, testHives[0])
			idx := NewStringIndex(len(scanData.nks), len(scanData.vks))
			for _, nk := range scanData.nks {
				idx.AddNK(nk[0], scanData.names[nk[2]], nk[1])
			}
			for _, vk := range scanData.vks {
				idx.AddVK(vk[0], scanData.names[vk[2]], vk[1])
			}
		}
	})
}

// Benchmark: Long-running service simulation (1000 hives).
func Benchmark_LongRunningService(b *testing.B) {
	data := loadHiveData(b, testHives[0])

	b.Run("StringIndex_1000Hives", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for range 1000 {
				idx := NewStringIndex(len(data.nks), len(data.vks))
				for _, nk := range data.nks {
					idx.AddNK(nk[0], data.names[nk[2]], nk[1])
				}
			}
		}
	})

	b.Run("UniqueV2_1000Hives", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			for range 1000 {
				idx := NewUniqueIndexV2(len(data.nks), len(data.vks))
				for _, nk := range data.nks {
					idx.AddNK(nk[0], data.names[nk[2]], nk[1])
				}
			}
		}
	})
}
