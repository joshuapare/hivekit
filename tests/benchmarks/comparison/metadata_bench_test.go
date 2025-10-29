package comparison

import (
	"testing"
	"time"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkNodeTimestamp compares performance of getting node timestamp
// Measures: hivex_node_timestamp vs Reader.KeyTimestamp().
func BenchmarkNodeTimestamp(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var timestamp time.Time

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				timestamp, err = r.KeyTimestamp(root)
				if err != nil {
					b.Fatalf("KeyTimestamp failed: %v", err)
				}
			}

			benchGoTime = timestamp
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var timestamp int64

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				timestamp = h.NodeTimestamp(root)
			}

			benchHivexInt64 = timestamp
		})
	}
}

// BenchmarkNodeNrChildren compares performance of counting children
// Measures: hivex_node_nr_children vs Reader.KeySubkeyCount().
func BenchmarkNodeNrChildren(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var count int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count, err = r.KeySubkeyCount(root)
				if err != nil {
					b.Fatalf("KeySubkeyCount failed: %v", err)
				}
			}

			benchGoInt = count
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var count int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count = h.NodeNrChildren(root)
			}

			benchHivexInt = count
		})
	}
}

// BenchmarkNodeNrValues compares performance of counting values
// Measures: hivex_node_nr_values vs Reader.KeyValueCount().
func BenchmarkNodeNrValues(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var count int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count, err = r.KeyValueCount(root)
				if err != nil {
					b.Fatalf("KeyValueCount failed: %v", err)
				}
			}

			benchGoInt = count
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var count int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				count = h.NodeNrValues(root)
			}

			benchHivexInt = count
		})
	}
}

// BenchmarkStatKey compares full metadata retrieval
// gohivex-specific - returns all metadata in one call.
func BenchmarkStatKey(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var meta hive.KeyMeta

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				meta, err = r.StatKey(root)
				if err != nil {
					b.Fatalf("StatKey failed: %v", err)
				}
			}

			benchGoKeyMeta = meta
		})

		// Benchmark hivex - multiple calls to get equivalent data
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()

			var (
				name       string
				timestamp  int64
				nrChildren int
				nrValues   int
			)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				name = h.NodeName(root)
				timestamp = h.NodeTimestamp(root)
				nrChildren = h.NodeNrChildren(root)
				nrValues = h.NodeNrValues(root)
			}

			benchHivexString = name
			benchHivexInt64 = timestamp
			benchHivexInt = nrChildren + nrValues
		})
	}
}

// BenchmarkDetailKey benchmarks gohivex DetailKey vs StatKey
// DetailKey returns more detailed metadata than StatKey.
func BenchmarkDetailKey(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark StatKey (lightweight)
		b.Run("StatKey/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var meta hive.KeyMeta

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				meta, err = r.StatKey(root)
				if err != nil {
					b.Fatalf("StatKey failed: %v", err)
				}
			}

			benchGoKeyMeta = meta
		})

		// Benchmark DetailKey (comprehensive)
		b.Run("DetailKey/"+hf.Name, func(b *testing.B) {
			// Open once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var detail hive.KeyDetail

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				detail, err = r.DetailKey(root)
				if err != nil {
					b.Fatalf("DetailKey failed: %v", err)
				}
			}

			benchGoKeyDetail = detail
		})
	}
}

// BenchmarkMetadataOnChildren benchmarks metadata operations on child nodes
// More realistic than just root node.
func BenchmarkMetadataOnChildren(b *testing.B) {
	// Use special hive which has known children
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open once (not benchmarked)
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		children, err := r.Subkeys(root)
		if err != nil {
			b.Fatalf("Subkeys failed: %v", err)
		}

		if len(children) == 0 {
			b.Skip("No children to benchmark")
		}

		var meta hive.KeyMeta

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Get metadata for all children
			for _, child := range children {
				meta, err = r.StatKey(child)
				if err != nil {
					b.Fatalf("StatKey failed: %v", err)
				}
			}
		}

		benchGoKeyMeta = meta
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		children := h.NodeChildren(root)

		if len(children) == 0 {
			b.Skip("No children to benchmark")
		}

		var (
			name  string
			count int
		)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Get metadata for all children
			for _, child := range children {
				name = h.NodeName(child)
				count += h.NodeNrChildren(child)
				count += h.NodeNrValues(child)
			}
		}

		benchHivexString = name
		benchHivexInt = count
	})
}
