package walker

import (
	"context"
	"testing"

	"github.com/joshuapare/hivekit/hive/index"
)

// BenchmarkCurrentFlow benchmarks the current VK indexing flow with string allocation.
func BenchmarkCurrentFlow(b *testing.B) {
	// Simulate the current flow: decode ASCII bytes to string, then hash
	data := []byte("SomeTypicalValueName")

	b.ReportAllocs()

	for b.Loop() {
		// Current flow: allocate string, then call AddVKLower
		nameLower := decodeASCIILower(data)
		_ = nameLower
	}
}

// BenchmarkOptimizedFlow benchmarks the optimized VK indexing flow without string allocation.
func BenchmarkOptimizedFlow(b *testing.B) {
	// Simulate the optimized flow: hash bytes directly
	data := []byte("SomeTypicalValueName")

	b.ReportAllocs()

	for b.Loop() {
		// Optimized flow: hash bytes directly, no string allocation
		hash := index.Fnv32LowerBytes(data)
		_ = hash
	}
}

// BenchmarkDecodeASCIILower_Short benchmarks decoding short ASCII names.
func BenchmarkDecodeASCIILower_Short(b *testing.B) {
	data := []byte("Name")

	b.ReportAllocs()

	for b.Loop() {
		_ = decodeASCIILower(data)
	}
}

// BenchmarkDecodeASCIILower_Medium benchmarks decoding medium ASCII names.
func BenchmarkDecodeASCIILower_Medium(b *testing.B) {
	data := []byte("SomeTypicalValueName")

	b.ReportAllocs()

	for b.Loop() {
		_ = decodeASCIILower(data)
	}
}

// BenchmarkDecodeASCIILower_Long benchmarks decoding long ASCII names.
func BenchmarkDecodeASCIILower_Long(b *testing.B) {
	data := []byte("SomeVeryLongTypicalValueNameThatMightAppearInRegistry")

	b.ReportAllocs()

	for b.Loop() {
		_ = decodeASCIILower(data)
	}
}

// BenchmarkDecodeASCIILower_AlreadyLower benchmarks the fast path for already-lowercase names.
func BenchmarkDecodeASCIILower_AlreadyLower(b *testing.B) {
	data := []byte("alreadylowercase")

	b.ReportAllocs()

	for b.Loop() {
		_ = decodeASCIILower(data)
	}
}

// BenchmarkFnv32LowerBytes_Short benchmarks hashing short names.
func BenchmarkFnv32LowerBytes_Short(b *testing.B) {
	data := []byte("Name")

	b.ReportAllocs()

	for b.Loop() {
		_ = index.Fnv32LowerBytes(data)
	}
}

// BenchmarkFnv32LowerBytes_Medium benchmarks hashing medium names.
func BenchmarkFnv32LowerBytes_Medium(b *testing.B) {
	data := []byte("SomeTypicalValueName")

	b.ReportAllocs()

	for b.Loop() {
		_ = index.Fnv32LowerBytes(data)
	}
}

// BenchmarkFnv32LowerBytes_Long benchmarks hashing long names.
func BenchmarkFnv32LowerBytes_Long(b *testing.B) {
	data := []byte("SomeVeryLongTypicalValueNameThatMightAppearInRegistry")

	b.ReportAllocs()

	for b.Loop() {
		_ = index.Fnv32LowerBytes(data)
	}
}

// BenchmarkFullIndexBuild benchmarks the full index build process on a real hive.
func BenchmarkFullIndexBuild(b *testing.B) {
	if len(suiteHives) == 0 {
		b.Skip("No suite hives available")
	}

	// Use the largest hive for meaningful benchmark
	hivePath := suiteHives[3].path // windows-2012-software
	h, err := openHive(hivePath)
	if err != nil {
		b.Skipf("Skipping: %v", err)
		return
	}
	defer h.Close()

	b.ReportAllocs()

	for b.Loop() {
		builder := NewIndexBuilder(h, 0, 0)
		_, err := builder.Build(context.Background())
		if err != nil {
			b.Fatalf("Build failed: %v", err)
		}
	}
}

// BenchmarkAddVKHash_VsAddVKLower compares hash-based vs string-based VK addition.
func BenchmarkAddVKHash_VsAddVKLower(b *testing.B) {
	b.Run("AddVKHash", func(b *testing.B) {
		idx := index.NewNumericIndex(b.N, b.N)
		nameBytes := []byte("SomeValueName")
		hash := index.Fnv32LowerBytes(nameBytes)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			idx.AddVKHash(uint32(i), hash, nameBytes, uint32(i*100))
		}
	})

	b.Run("AddVKLower", func(b *testing.B) {
		idx := index.NewNumericIndex(b.N, b.N)
		nameLower := "somevaluename"

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			idx.AddVKLower(uint32(i), nameLower, uint32(i*100))
		}
	})
}

// BenchmarkAddNKHash_VsAddNKLower compares hash-based vs string-based NK addition.
func BenchmarkAddNKHash_VsAddNKLower(b *testing.B) {
	b.Run("AddNKHash", func(b *testing.B) {
		idx := index.NewNumericIndex(b.N, b.N)
		nameBytes := []byte("SubKeyName")
		hash := index.Fnv32LowerBytes(nameBytes)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			idx.AddNKHash(uint32(i), hash, nameBytes, uint32(i*100))
		}
	})

	b.Run("AddNKLower", func(b *testing.B) {
		idx := index.NewNumericIndex(b.N, b.N)
		nameLower := "subkeyname"

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			idx.AddNKLower(uint32(i), nameLower, uint32(i*100))
		}
	})
}

// BenchmarkIndexCapacityEstimation benchmarks the capacity estimation function.
func BenchmarkIndexCapacityEstimation(b *testing.B) {
	sizes := []int64{
		1024 * 1024,      // 1 MB
		10 * 1024 * 1024, // 10 MB
		50 * 1024 * 1024, // 50 MB
	}

	for _, size := range sizes {
		b.Run("", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = estimateIndexCapacity(size)
			}
		})
	}
}

// BenchmarkMemoryProfile runs index build with memory profiling annotations.
func BenchmarkMemoryProfile(b *testing.B) {
	if len(suiteHives) == 0 {
		b.Skip("No suite hives available")
	}

	hivePath := suiteHives[3].path // windows-2012-software
	h, err := openHive(hivePath)
	if err != nil {
		b.Skipf("Skipping: %v", err)
		return
	}
	defer h.Close()

	b.ReportAllocs()

	for b.Loop() {
		builder := NewIndexBuilder(h, 0, 0)
		idx, err := builder.Build(context.Background())
		if err != nil {
			b.Fatalf("Build failed: %v", err)
		}

		// Record stats
		stats := idx.Stats()
		b.ReportMetric(float64(stats.NKCount), "NKs")
		b.ReportMetric(float64(stats.VKCount), "VKs")
	}
}
