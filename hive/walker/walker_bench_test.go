package walker

import (
	"testing"
)

// Benchmark_ValidationWalker benchmarks the new validation walker.
func Benchmark_ValidationWalker(b *testing.B) {
	h, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	walker := NewValidationWalker(h)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		cellCount := 0
		err := walker.Walk(func(ref CellRef) error {
			cellCount++
			return nil
		})
		if err != nil {
			b.Fatalf("Walk failed: %v", err)
		}

		walker.Reset()
	}
}

// Benchmark_IndexBuilder benchmarks index building.
func Benchmark_IndexBuilder(b *testing.B) {
	h, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		builder := NewIndexBuilder(h, 10000, 10000)

		_, err := builder.Build()
		if err != nil {
			b.Fatalf("Build failed: %v", err)
		}
	}
}

// Benchmark_CellCounter benchmarks cell counting.
func Benchmark_CellCounter(b *testing.B) {
	h, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	counter := NewCellCounter(h)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, err := counter.Count()
		if err != nil {
			b.Fatalf("Count failed: %v", err)
		}

		counter.Reset()
	}
}

// Benchmark_Bitmap benchmarks bitmap operations.
func Benchmark_Bitmap_Set(b *testing.B) {
	bm := NewBitmap(1000000) // 1MB hive

	b.ResetTimer()

	for i := range b.N {
		offset := uint32(i % 250000) // Cycle through offsets
		bm.Set(offset * 4)
	}
}

func Benchmark_Bitmap_IsSet(b *testing.B) {
	bm := NewBitmap(1000000)

	// Pre-populate some offsets
	for i := range uint32(100000) {
		bm.Set(i * 4)
	}

	b.ResetTimer()

	for i := range b.N {
		offset := uint32(i % 250000)
		_ = bm.IsSet(offset * 4)
	}
}

// Benchmark_ValidationWalker_WithCounting benchmarks walker with counting (simulates real usage).
func Benchmark_ValidationWalker_WithCounting(b *testing.B) {
	h, cleanup := setupTestHive(&testing.T{})
	defer cleanup()

	walker := NewValidationWalker(h)

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		nkCount := 0
		vkCount := 0
		dataCount := 0

		err := walker.Walk(func(ref CellRef) error {
			switch ref.Type {
			case CellTypeNK:
				nkCount++
			case CellTypeVK:
				vkCount++
			case CellTypeData:
				dataCount++
			case CellTypeSK,
				CellTypeLF,
				CellTypeLH,
				CellTypeLI,
				CellTypeRI,
				CellTypeDB,
				CellTypeValueList,
				CellTypeBlocklist:
				// Other cell types - not counted in this benchmark
			}
			return nil
		})
		if err != nil {
			b.Fatalf("Walk failed: %v", err)
		}

		walker.Reset()
	}
}
