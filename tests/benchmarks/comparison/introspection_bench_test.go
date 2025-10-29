package comparison

import (
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkLastModified compares hive-level timestamp access
// Measures: reader.Info().LastWrite vs ops.LastModified().
func BenchmarkLastModified(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			var timestamp int64

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				info := r.Info()
				timestamp = info.LastWrite.Unix()
			}

			benchGoInt64 = timestamp
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			var timestamp int64

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				timestamp = h.LastModified()
			}

			benchHivexInt64 = timestamp
		})
	}
}

// BenchmarkNodeNameLen compares name length queries
// NOTE: gohivex doesn't have KeyNameLen(), so we compare:
// - gohivex: StatKey() to get full metadata (includes name length implicitly)
// - hivex: NodeNameLen() to get just the length
// This shows the overhead difference between getting full metadata vs just length.
func BenchmarkNodeNameLen(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex (StatKey for full metadata)
		b.Run("gohivex_statkey/"+hf.Name, func(b *testing.B) {
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
			if err != nil || len(children) == 0 {
				b.Skip("No children to benchmark")
			}

			node := children[0]
			var nameLen int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				meta, err := r.StatKey(node)
				if err != nil {
					b.Fatalf("StatKey failed: %v", err)
				}
				nameLen = len(meta.Name)
			}

			benchGoInt = nameLen
		})

		// Benchmark hivex (NodeNameLen for just length)
		b.Run("hivex_namelen/"+hf.Name, func(b *testing.B) {
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

			node := children[0]
			var nameLen uint64

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				nameLen = h.NodeNameLen(node)
			}

			benchCount = nameLen
		})
	}
}

// BenchmarkValueKeyLen compares value name length queries
// NOTE: Similar to NodeNameLen, gohivex doesn't have ValueNameLen()
// We compare StatValue() vs ValueKeyLen().
func BenchmarkValueKeyLen(b *testing.B) {
	// Use special hive which has values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex (StatValue for full metadata)
	b.Run("gohivex_statvalue/"+hf.Name, func(b *testing.B) {
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		child, err := r.Lookup(root, "abcd_äöüß")
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}

		values, err := r.Values(child)
		if err != nil || len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		value := values[0]
		var nameLen int

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			meta, err := r.StatValue(value)
			if err != nil {
				b.Fatalf("StatValue failed: %v", err)
			}
			nameLen = len(meta.Name)
		}

		benchGoInt = nameLen
	})

	// Benchmark hivex (ValueKeyLen for just length)
	b.Run("hivex_keylen/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)
		if len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		value := values[0]
		var nameLen uint64

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			nameLen = h.ValueKeyLen(value)
		}

		benchCount = nameLen
	})
}

// BenchmarkNodeStructLength compares NK struct size queries.
func BenchmarkNodeStructLength(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			var structLen int

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				structLen, _ = r.NodeStructSize(root)
			}

			benchGoInt = structLen
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()
			var structLen uint64

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				structLen = h.NodeStructLength(root)
			}

			benchCount = structLen
		})
	}
}

// BenchmarkValueStructLength compares VK struct size queries.
func BenchmarkValueStructLength(b *testing.B) {
	// Use special hive which has values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, _ := r.Root()
		child, err := r.GetChild(root, "abcd_äöüß")
		if err != nil {
			b.Skipf("GetChild failed: %v", err)
		}

		values, err := r.Values(child)
		if err != nil || len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		value := values[0]
		var structLen int

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			structLen, _ = r.ValueStructSize(value)
		}

		benchGoInt = structLen
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)
		if len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		value := values[0]
		var structLen uint64

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			structLen = h.ValueStructLength(value)
		}

		benchCount = structLen
	})
}

// BenchmarkValueDataCellOffset compares data cell offset queries.
func BenchmarkValueDataCellOffset(b *testing.B) {
	// Use special hive which has values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, _ := r.Root()
		child, err := r.GetChild(root, "abcd_äöüß")
		if err != nil {
			b.Skipf("GetChild failed: %v", err)
		}

		values, err := r.Values(child)
		if err != nil || len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		value := values[0]
		var offset uint32
		var length int

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			offset, length, _ = r.ValueDataCellOffset(value)
		}

		benchGoInt = int(offset) + length
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)
		if len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		value := values[0]
		var offset, length uint64

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			offset, length = h.ValueDataCellOffset(value)
		}

		benchCount = offset + length
	})
}

// BenchmarkIntrospectionRecursive benchmarks introspection across entire hive
// This provides realistic performance for forensics use cases.
func BenchmarkIntrospectionRecursive(b *testing.B) {
	// Use medium hive for realistic but not too slow benchmark
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, _ := r.Root()

		var nodeCount, valueCount int

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			nodeCount = 0
			valueCount = 0

			var walkNode func(node hive.NodeID, depth int)
			walkNode = func(node hive.NodeID, depth int) {
				if depth > 100 {
					return
				}

				nodeCount++

				// Get node introspection data
				_, _ = r.KeyNameLen(node)
				_, _ = r.NodeStructSize(node)

				// Get value introspection data
				values, _ := r.Values(node)
				for _, val := range values {
					valueCount++
					_, _ = r.ValueNameLen(val)
					_, _ = r.ValueStructSize(val)
					_, _, _ = r.ValueDataCellOffset(val)
				}

				// Recurse to children
				children, _ := r.Subkeys(node)
				for _, child := range children {
					walkNode(child, depth+1)
				}
			}

			walkNode(root, 0)
		}

		benchGoInt = nodeCount
		benchHivexInt = valueCount
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()

		var nodeCount, valueCount int

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			nodeCount = 0
			valueCount = 0

			var walkNode func(node bindings.NodeHandle, depth int)
			walkNode = func(node bindings.NodeHandle, depth int) {
				if depth > 100 {
					return
				}

				nodeCount++

				// Get node introspection data
				_ = h.NodeNameLen(node)
				_ = h.NodeStructLength(node)

				// Get value introspection data
				values := h.NodeValues(node)
				for _, val := range values {
					valueCount++
					_ = h.ValueKeyLen(val)
					_ = h.ValueStructLength(val)
					_, _ = h.ValueDataCellOffset(val)
				}

				// Recurse to children
				children := h.NodeChildren(node)
				for _, child := range children {
					walkNode(child, depth+1)
				}
			}

			walkNode(root, 0)
		}

		benchGoInt = nodeCount
		benchHivexInt = valueCount
	})
}

// BenchmarkIntrospectionOverhead compares introspection vs regular metadata
// Shows the performance difference between:
// - gohivex: Getting full metadata (StatKey, StatValue)
// - hivex: Getting just introspection info (name length, struct size, etc.)
func BenchmarkIntrospectionOverhead(b *testing.B) {
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex full metadata approach
	b.Run("gohivex_full_metadata/"+hf.Name, func(b *testing.B) {
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		child, err := r.Lookup(root, "abcd_äöüß")
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}

		values, err := r.Values(child)
		if err != nil || len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		var nameLen, size int

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Get key metadata (includes name, timestamp, counts, etc.)
			keyMeta, _ := r.StatKey(child)
			nameLen = len(keyMeta.Name)

			// Get value metadata (includes name, type, size, etc.)
			valMeta, _ := r.StatValue(values[0])
			size = valMeta.Size
		}

		benchGoInt = nameLen + size
	})

	// Benchmark hivex introspection-only approach
	b.Run("hivex_introspection_only/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)
		if len(values) == 0 {
			b.Skip("No values to benchmark")
		}

		var nameLen, structLen uint64

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Get just introspection info (no decoding)
			nameLen = h.NodeNameLen(child)
			structLen = h.ValueStructLength(values[0])
		}

		benchCount = nameLen + structLen
	})
}
