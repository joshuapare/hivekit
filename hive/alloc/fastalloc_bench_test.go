package alloc

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// Benchmark_FastAlloc_SmallCells benchmarks FastAllocator with small cells.
func Benchmark_FastAlloc_SmallCells(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x10000}) // 64KB free

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

// Benchmark_FastAlloc_MediumCells benchmarks FastAllocator with medium cells.
func Benchmark_FastAlloc_MediumCells(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x100000}) // 1MB free

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

// Benchmark_FastAlloc_LargeCells benchmarks FastAllocator with large cells.
func Benchmark_FastAlloc_LargeCells(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x1000000}) // 16MB free

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

// Benchmark_FastAlloc_Free benchmarks FastAllocator freeing.
func Benchmark_FastAlloc_Free(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x100000}) // 1MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Pre-allocate cells to free
	refs := make([]CellRef, b.N)
	for i := range b.N {
		ref, _, allocErr := fa.Alloc(128, ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
		refs[i] = ref
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		freeErr := fa.Free(refs[i])
		if freeErr != nil {
			b.Fatal(freeErr)
		}
	}
}

// Benchmark_FastAlloc_AllocFree_SteadyState benchmarks realistic mixed workload.
func Benchmark_FastAlloc_AllocFree_SteadyState(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x100000}) // 1MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Warm up: allocate some cells to steady state (500 cells)
	allocated := make([]CellRef, 0, 1000)
	for range 500 {
		ref, _, _ := fa.Alloc(128, ClassNK)
		allocated = append(allocated, ref)
	}

	b.ReportAllocs()

	rng := rand.New(rand.NewSource(42))

	for b.Loop() {
		// Maintain steady state: if too many allocated, free more often
		shouldAlloc := len(allocated) < 500 || (len(allocated) < 700 && rng.Float32() < 0.5)

		if !shouldAlloc {
			// Free
			if len(allocated) > 0 {
				idx := rng.Intn(len(allocated))
				freeErr := fa.Free(allocated[idx])
				if freeErr != nil {
					b.Fatal(freeErr)
				}
				// Remove from slice
				allocated[idx] = allocated[len(allocated)-1]
				allocated = allocated[:len(allocated)-1]
			}
		} else {
			// Allocate
			size := 64 + rng.Intn(512) // 64-576 bytes
			ref, _, allocErr := fa.Alloc(int32(size), ClassNK)
			if allocErr != nil {
				b.Fatal(allocErr)
			}
			allocated = append(allocated, ref)
		}
	}
}

// Benchmark_FastAlloc_PowerLaw benchmarks with power-law size distribution.
func Benchmark_FastAlloc_PowerLaw(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x1000000}) // 16MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	rng := rand.New(rand.NewSource(42))

	for b.Loop() {
		// Power-law distribution: 90% small, 9% medium, 1% large
		var size int
		r := rng.Float32()
		switch {
		case r < 0.9:
			size = 64 + rng.Intn(192) // 64-256 bytes
		case r < 0.99:
			size = 256 + rng.Intn(768) // 256-1024 bytes
		default:
			size = 1024 + rng.Intn(3072) // 1024-4096 bytes
		}

		_, _, allocErr := fa.Alloc(int32(size), ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
	}
}

// Benchmark_FastAlloc_Coalesce benchmarks coalescing performance.
func Benchmark_FastAlloc_Coalesce(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")
	createHiveWithFreeCells(b, hivePath, []int{0x100000}) // 1MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Allocate many small cells
	refs := make([]CellRef, 0, 1000)
	for range 1000 {
		ref, _, _ := fa.Alloc(128, ClassNK)
		refs = append(refs, ref)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		// Free a cell (triggers coalescing)
		idx := i % len(refs)
		_ = fa.Free(refs[idx])

		// Re-allocate to keep steady state
		ref, _, _ := fa.Alloc(128, ClassNK)
		refs[idx] = ref
	}
}
