package alloc

import (
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// Benchmark_FastAlloc_MediumCells_NoGrowth benchmarks medium cells with pre-allocated space.
func Benchmark_FastAlloc_MediumCells_NoGrowth(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")

	// Pre-allocate enough space for 1 million medium cells
	// 1M cells * 512 bytes = 512MB
	createHiveWithFreeCells(b, hivePath, []int{0x20000000}) // 512MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		size := 256 + (i%256)*2 // 256-512 bytes
		_, _, allocErr := fa.Alloc(int32(size), ClassVK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
	}
}

// Benchmark_FastAlloc_LargeCells_NoGrowth benchmarks large cells with pre-allocated space.
func Benchmark_FastAlloc_LargeCells_NoGrowth(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")

	// Pre-allocate enough space for 1 million large cells
	// 1M cells * 4KB = 4GB (too large, use 1GB)
	createHiveWithFreeCells(b, hivePath, []int{0x40000000}) // 1GB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		size := 1024 + (i % 3072) // 1KB-4KB
		_, _, allocErr := fa.Alloc(int32(size), ClassDB)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
	}
}

// Benchmark_FastAlloc_SmallCells_ProfileReady benchmarks small cells for profiling.
func Benchmark_FastAlloc_SmallCells_ProfileReady(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x10000000}) // 256MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		size := 64 + (i%64)*2 // 64-128 bytes
		_, _, allocErr := fa.Alloc(int32(size), ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
	}
}
