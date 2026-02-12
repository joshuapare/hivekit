package alloc

import (
	"testing"
)

// BenchmarkBumpAllocator_Init measures initialization time.
// This is the key metric - BumpAllocator should be O(1) vs FastAllocator's O(n log n).
func BenchmarkBumpAllocator_Init(b *testing.B) {
	// Create hive with some data to make the comparison meaningful
	h := newTestHive(b, 10) // 10 HBINs = 40KB

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		ba, err := NewBump(h, nil)
		if err != nil {
			b.Fatal(err)
		}
		ba.Close()
	}
}

// BenchmarkFastAllocator_Init measures FastAllocator initialization for comparison.
func BenchmarkFastAllocator_Init(b *testing.B) {
	h := newTestHive(b, 10) // 10 HBINs = 40KB

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		fa, err := NewFast(h, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		fa.Close()
	}
}

// BenchmarkBumpAllocator_Alloc measures allocation throughput.
func BenchmarkBumpAllocator_Alloc(b *testing.B) {
	h := newTestHive(b, 100) // 100 HBINs = 400KB

	ba, err := NewBump(h, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer ba.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		size := int32(64 + (i%64)*2) // 64-128 bytes
		_, _, err := ba.Alloc(size, ClassNK)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFastAllocator_Alloc measures FastAllocator allocation for comparison.
func BenchmarkFastAllocator_Alloc(b *testing.B) {
	h := newTestHive(b, 100) // 100 HBINs = 400KB

	fa, err := NewFast(h, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer fa.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		size := int32(64 + (i%64)*2) // 64-128 bytes
		_, _, err := fa.Alloc(size, ClassNK)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBumpAllocator_AllocSequential measures sequential allocation.
func BenchmarkBumpAllocator_AllocSequential(b *testing.B) {
	h := newTestHive(b, 500) // 500 HBINs = 2MB

	ba, err := NewBump(h, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer ba.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for range b.N {
		_, _, err := ba.Alloc(64, ClassVK)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBumpAllocator_1000Cells measures 1000 sequential allocations.
func BenchmarkBumpAllocator_1000Cells(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		h := newTestHive(b, 50)

		ba, err := NewBump(h, nil)
		if err != nil {
			b.Fatal(err)
		}

		for range 1000 {
			_, _, err := ba.Alloc(64, ClassNK)
			if err != nil {
				b.Fatal(err)
			}
		}

		ba.Close()
	}
}

// BenchmarkFastAllocator_1000Cells measures 1000 sequential allocations for comparison.
func BenchmarkFastAllocator_1000Cells(b *testing.B) {
	b.ReportAllocs()

	for range b.N {
		h := newTestHive(b, 50)

		fa, err := NewFast(h, nil, nil)
		if err != nil {
			b.Fatal(err)
		}

		for range 1000 {
			_, _, err := fa.Alloc(64, ClassNK)
			if err != nil {
				b.Fatal(err)
			}
		}

		fa.Close()
	}
}

// BenchmarkBumpAllocator_VariedSizes measures allocation with varied sizes.
func BenchmarkBumpAllocator_VariedSizes(b *testing.B) {
	sizes := []int32{32, 64, 128, 256, 512, 1024}

	h := newTestHive(b, 200)

	ba, err := NewBump(h, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer ba.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		size := sizes[i%len(sizes)]
		_, _, err := ba.Alloc(size, ClassNK)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBumpAllocator_Free measures free operation.
// Should be very fast since it just flips a sign bit.
func BenchmarkBumpAllocator_Free(b *testing.B) {
	h := newTestHive(b, 200)

	ba, err := NewBump(h, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer ba.Close()

	// Pre-allocate cells to free
	refs := make([]CellRef, b.N)
	for i := range b.N {
		ref, _, err := ba.Alloc(64, ClassNK)
		if err != nil {
			b.Fatal(err)
		}
		refs[i] = ref
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := range b.N {
		err := ba.Free(refs[i])
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAllocator_InitLargeHive compares init time on larger hives.
func BenchmarkAllocator_InitLargeHive(b *testing.B) {
	b.Run("Bump_100HBIN", func(b *testing.B) {
		h := newTestHive(b, 100) // 100 HBINs = 400KB
		b.ResetTimer()
		b.ReportAllocs()
		for range b.N {
			ba, _ := NewBump(h, nil)
			ba.Close()
		}
	})

	b.Run("Fast_100HBIN", func(b *testing.B) {
		h := newTestHive(b, 100)
		b.ResetTimer()
		b.ReportAllocs()
		for range b.N {
			fa, _ := NewFast(h, nil, nil)
			fa.Close()
		}
	})
}
