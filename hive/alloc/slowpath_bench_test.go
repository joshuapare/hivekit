package alloc

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
)

// Benchmark_SlowPath_PureAlloc measures ONLY allocation time with no Free() in the loop.
// This is the cleanest benchmark for measuring the O(n) slow path behavior.
//
// IMPORTANT: All cell sizes MUST be in the SAME size class!
// With ConfigBalanced: class 32 = 512-767, class 33 = 768-1151
//
// Strategy:
// - Create cells sized 520-760 (all in class 32: 512-767)
// - Request 752-760 bytes (top of class 32)
// - heap[0] will be ~520, request is 752+, forcing slow path scan
func Benchmark_SlowPath_PureAlloc(b *testing.B) {
	for _, cellCount := range []int{100, 500, 1000, 2000, 5000, 10000} {
		b.Run(itoa(cellCount)+"_cells", func(b *testing.B) {
			benchPureAlloc(b, cellCount)
		})
	}
}

func benchPureAlloc(b *testing.B, freeCellCount int) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "pure.hiv")

	createHiveWithFreeCells(b, hivePath, []int{100 * 1024 * 1024})

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Setup: Create fragmented free list with varying cell sizes ALL IN SAME SIZE CLASS
	// ConfigBalanced class 32 = sizes 512-767
	//
	// Strategy to force slow path:
	// - Create cells sized 520-760 (all in class 32)
	// - Most cells are small (520-600), a few are large (700-760)
	// - heap[0] will be ~520 (smallest)
	// - Request 700 - this is larger than heap[0], forcing slow path scan
	//   to find one of the larger cells
	rng := rand.New(rand.NewSource(42))
	allocated := make([]CellRef, 0, freeCellCount*2)

	for range freeCellCount * 2 {
		var size int
		if rng.Float32() < 0.8 {
			// 80% small cells: 520-600 (will be too small for 700 request)
			size = 520 + rng.Intn(80) // 520-599
		} else {
			// 20% large cells: 704-760 (can satisfy 700 request)
			size = 704 + rng.Intn(56) // 704-759
		}
		size = (size + 7) &^ 7 // 8-byte align
		ref, _, allocErr := fa.Alloc(int32(size), ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
		allocated = append(allocated, ref)
	}

	// Free exactly freeCellCount cells to create fragmented free list
	rng.Shuffle(len(allocated), func(i, j int) {
		allocated[i], allocated[j] = allocated[j], allocated[i]
	})

	for i := 0; i < freeCellCount; i++ {
		if err := fa.Free(allocated[i]); err != nil {
			b.Fatal(err)
		}
	}

	// Request 704 - larger than 80% of cells (520-599), but some cells (704-759) can fit
	// heap[0] will be ~520, so slow path must scan to find a 704+ cell
	requestSize := int32(704)

	b.ResetTimer()
	b.ReportAllocs()

	// Timed section: Alloc triggers slow path, then Free to restore state
	// The Free is measured too, but it's constant overhead across all cell counts
	for i := 0; i < b.N; i++ {
		ref, _, allocErr := fa.Alloc(requestSize, ClassNK)
		if allocErr != nil || ref == 0 {
			// Allocation failed - likely ran out of cells that fit
			// Skip this iteration
			continue
		}
		_ = fa.Free(ref) // Free it back to maintain free cell count
	}
}

// Benchmark_SlowPath_FastPathBaseline provides a baseline with allocations that
// ALWAYS use the fast path (heap[0] always fits). Compare against PureAlloc to
// see the overhead of the slow path.
func Benchmark_SlowPath_FastPathBaseline(b *testing.B) {
	for _, cellCount := range []int{100, 500, 1000, 2000, 5000, 10000} {
		b.Run(itoa(cellCount)+"_cells", func(b *testing.B) {
			benchFastPathBaseline(b, cellCount)
		})
	}
}

func benchFastPathBaseline(b *testing.B, freeCellCount int) {
	dir := b.TempDir()
	hivePath := filepath.Join(dir, "fast.hiv")

	createHiveWithFreeCells(b, hivePath, []int{100 * 1024 * 1024})

	h, err := hive.Open(hivePath)
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Setup: Create free list with UNIFORM cell sizes (all exactly 1024 bytes)
	// This means heap[0] will always be 1024 bytes, and requests for 1024 bytes
	// will ALWAYS take the fast path (heap[0] fits)
	allocated := make([]CellRef, 0, freeCellCount*2)

	for range freeCellCount * 2 {
		ref, _, allocErr := fa.Alloc(1024, ClassNK)
		if allocErr != nil {
			b.Fatal(allocErr)
		}
		allocated = append(allocated, ref)
	}

	// Free exactly freeCellCount cells
	for i := 0; i < freeCellCount; i++ {
		if err := fa.Free(allocated[i]); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Timed section: All allocations take fast path (heap[0] is always 1024, request is 1024)
	for i := 0; i < b.N; i++ {
		_, _, _ = fa.Alloc(1024, ClassNK)
	}
}

// Benchmark_SlowPath_DirectScan directly measures the scanning overhead by
// creating a specific heap layout and measuring allocations that must scan.
// This is the most controlled benchmark for isolating the O(n) scan behavior.
func Benchmark_SlowPath_DirectScan(b *testing.B) {
	for _, heapSize := range []int{50, 100, 200, 500, 1000, 2000} {
		b.Run(itoa(heapSize)+"_heap", func(b *testing.B) {
			benchDirectScan(b, heapSize)
		})
	}
}

func benchDirectScan(b *testing.B, heapSize int) {
	// Create a test hive with specific cell layout
	// We want cells that all land in the same size class but have varying sizes

	// Calculate total space needed: heapSize cells * ~800 bytes each = ~640KB per heapSize=1000
	numHBINs := (heapSize*800)/4000 + 5 // Add buffer
	if numHBINs < 2 {
		numHBINs = 2
	}

	// Create cells of varying sizes all in size class for 512-1024 bytes
	cells := make([]int32, heapSize)
	for i := range cells {
		// Create cells with sizes from 520 to 1000, varying
		// Ensure smallest cells come first so heap[0] is small
		size := 520 + (i % 480)
		size = (size + 7) &^ 7 // 8-byte align
		cells[i] = int32(size)
	}

	h, _ := newTestHiveWithLayout(b, numHBINs, cells)
	fa := newFastAllocatorWithRealDirtyTracker(b, h)

	// Pre-generate request sizes that will trigger slow path
	// Request ~950 bytes - this is larger than heap[0] (~520) but fits in the class
	rng := rand.New(rand.NewSource(42))
	requestSizes := make([]int32, b.N)
	for i := range requestSizes {
		size := 920 + rng.Intn(70) // 920-989
		requestSizes[i] = int32((size + 7) &^ 7)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = fa.Alloc(requestSizes[i], ClassNK)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
