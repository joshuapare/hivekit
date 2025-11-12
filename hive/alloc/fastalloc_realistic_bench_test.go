package alloc

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
)

// Benchmark_FastAlloc_Realistic_MixedWorkload benchmarks with real-world size distribution
// Based on analysis of Windows 2012 System hive (9MB, 157K cells):
//   - 99.1% small (≤256B)
//   - 0.9% medium (≤1KB)
//   - 0.1% large (≤4KB)
//
// Size percentiles: p50=40B, p75=80B, p90=104B, p95=120B, p99=248B.
func Benchmark_FastAlloc_Realistic_MixedWorkload(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")

	// Pre-allocate 15MB (typical large system hive)
	createHiveWithFreeCells(b, hivePath, []int{0xF00000}) // 15MB free

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
		var size int
		r := rng.Float32()

		switch {
		case r < 0.991:
			// 99.1% small cells (≤256B)
			// Use realistic distribution: p50=40, p75=80, p90=104, p95=120, p99=248
			p := rng.Float32()
			switch {
			case p < 0.50:
				size = 32 + rng.Intn(16) // 32-48B (around p50=40)
			case p < 0.75:
				size = 48 + rng.Intn(40) // 48-88B (up to p75=80)
			case p < 0.90:
				size = 88 + rng.Intn(24) // 88-112B (up to p90=104)
			case p < 0.95:
				size = 112 + rng.Intn(16) // 112-128B (up to p95=120)
			default:
				size = 128 + rng.Intn(128) // 128-256B (up to p99=248)
			}
		case r < 0.991+0.009:
			// 0.9% medium cells (256B-1KB)
			size = 256 + rng.Intn(768) // 256-1024B
		default:
			// 0.1% large cells (1KB-4KB)
			size = 1024 + rng.Intn(3072) // 1KB-4KB
		}

		_, _, allocErr := fa.Alloc(int32(size), ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
	}
}

// Benchmark_FastAlloc_Realistic_SmallOnly benchmarks pure small cell workload (99%+ of real usage).
func Benchmark_FastAlloc_Realistic_SmallOnly(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")

	// Pre-allocate 10MB
	createHiveWithFreeCells(b, hivePath, []int{0xA00000}) // 10MB free

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

	rng := rand.New(rand.NewSource(42))

	for b.Loop() {
		// Realistic small cell distribution matching p50=40, p75=80, p90=104
		var size int
		p := rng.Float32()
		switch {
		case p < 0.50:
			size = 32 + rng.Intn(16) // 32-48B (p50=40)
		case p < 0.75:
			size = 48 + rng.Intn(40) // 48-88B (p75=80)
		case p < 0.90:
			size = 88 + rng.Intn(24) // 88-112B (p90=104)
		default:
			size = 112 + rng.Intn(144) // 112-256B (p95-p99)
		}

		_, _, allocErr := fa.Alloc(int32(size), ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
	}
}

// Benchmark_FastAlloc_Realistic_WithFree simulates real-world alloc/free patterns
// This matches the steady-state usage where keys/values are created and deleted.
func Benchmark_FastAlloc_Realistic_WithFree(b *testing.B) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "bench.hiv")

	// Pre-allocate 10MB
	createHiveWithFreeCells(b, hivePath, []int{0xA00000}) // 10MB free

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Warm up: allocate some cells to steady state (5000 cells ~= 500KB)
	allocated := make([]CellRef, 0, 10000)
	rng := rand.New(rand.NewSource(42))

	for range 5000 {
		size := 32 + rng.Intn(200) // Mostly small cells
		ref, _, _ := fa.Alloc(int32(size), ClassNK)
		allocated = append(allocated, ref)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		// 70% allocate, 30% free (net growth, like real hives that grow over time)
		if rng.Float32() < 0.7 || len(allocated) < 1000 {
			// Allocate with realistic size distribution
			var size int
			p := rng.Float32()
			switch {
			case p < 0.50:
				size = 32 + rng.Intn(16)
			case p < 0.75:
				size = 48 + rng.Intn(40)
			case p < 0.90:
				size = 88 + rng.Intn(24)
			case p < 0.99:
				size = 112 + rng.Intn(144)
			default:
				// Rare medium/large allocation
				size = 256 + rng.Intn(768)
			}

			ref, _, allocErr := fa.Alloc(int32(size), ClassNK)
			if allocErr != nil {
				b.Fatal(allocErr)
			}
			allocated = append(allocated, ref)
		} else if len(allocated) > 0 {
			// Free a random cell
			idx := rng.Intn(len(allocated))
			freeErr := fa.Free(allocated[idx])
			if freeErr != nil {
				b.Fatal(freeErr)
			}
			// Remove from slice
			allocated[idx] = allocated[len(allocated)-1]
			allocated = allocated[:len(allocated)-1]
		}
	}
}

// Benchmark_FastAlloc_Realistic_TypicalHive simulates building a 5MB hive from scratch
// This represents creating ~100K cells with realistic size distribution.
func Benchmark_FastAlloc_Realistic_TypicalHive(b *testing.B) {
	// Run once per iteration to measure full hive creation
	b.ReportAllocs()

	for b.Loop() {
		dir := b.TempDir()
		hivePath := filepath.Join(dir, "bench.hiv")

		// Start with 10MB to avoid growth during benchmark
		createHiveWithFreeCells(b, hivePath, []int{0xA00000})

		h, err := hive.Open(hivePath)
		if err != nil {
			b.Fatal(err)
		}

		fa, err := NewFast(h, nil, nil)
		if err != nil {
			h.Close()
			b.Fatal(err)
		}

		rng := rand.New(rand.NewSource(42))

		// Create ~100K cells (typical for 5MB hive)
		for range 100000 {
			var size int
			p := rng.Float32()
			switch {
			case p < 0.991:
				// Small cells
				switch {
				case p < 0.50:
					size = 32 + rng.Intn(16)
				case p < 0.75:
					size = 48 + rng.Intn(40)
				case p < 0.90:
					size = 88 + rng.Intn(24)
				default:
					size = 112 + rng.Intn(144)
				}
			case p < 0.991+0.009:
				size = 256 + rng.Intn(768)
			default:
				size = 1024 + rng.Intn(3072)
			}

			_, _, allocErr := fa.Alloc(int32(size), ClassNK)
			if allocErr != nil {
				h.Close()
				b.Fatal(allocErr)
			}
		}

		h.Close()
	}
}
