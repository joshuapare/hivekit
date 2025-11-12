package comparison

import (
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkNodeValues compares performance of enumerating values
// Measures: hivex_node_values vs Reader.Values().
func BenchmarkNodeValues(b *testing.B) {
	// Use special hive which has nodes with values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and navigate to child once (not benchmarked)
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

		var values []hive.ValueID

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			values, err = r.Values(child)
			if err != nil {
				b.Fatalf("Values failed: %v", err)
			}
		}

		benchGoValueIDs = values
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and navigate to child once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")

		var values []bindings.ValueHandle

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			values = h.NodeValues(child)
		}

		benchHivexValues = values
	})
}

// BenchmarkValueKey compares performance of getting value name
// Measures: hivex_value_key vs Reader.ValueName().
func BenchmarkValueKey(b *testing.B) {
	// Use special hive which has nodes with values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
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
			b.Fatalf("Values failed")
		}

		value := values[0]
		var name string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			name, err = r.ValueName(value)
			if err != nil {
				b.Fatalf("ValueName failed: %v", err)
			}
		}

		benchGoString = name
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)

		if len(values) == 0 {
			b.Fatalf("No values")
		}

		value := values[0]
		var name string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			name = h.ValueKey(value)
		}

		benchHivexString = name
	})
}

// BenchmarkValueType compares performance of getting value type
// Measures: hivex_value_type vs Reader.ValueType().
func BenchmarkValueType(b *testing.B) {
	// Use special hive which has nodes with values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
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
			b.Fatalf("Values failed")
		}

		value := values[0]
		var vtype hive.RegType

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			vtype, err = r.ValueType(value)
			if err != nil {
				b.Fatalf("ValueType failed: %v", err)
			}
		}

		benchGoInt = int(vtype)
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)

		if len(values) == 0 {
			b.Fatalf("No values")
		}

		value := values[0]
		var vtype bindings.ValueType
		var size int

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			vtype, size, err = h.ValueType(value)
			if err != nil {
				b.Fatalf("ValueType failed: %v", err)
			}
		}

		benchHivexInt = int(vtype) + size
	})
}

// BenchmarkValueValue compares performance of reading raw value bytes
// Measures: hivex_value_value vs Reader.ValueBytes().
func BenchmarkValueValue(b *testing.B) {
	// Use special hive which has nodes with values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
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
			b.Fatalf("Values failed")
		}

		value := values[0]
		var bytes []byte

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			bytes, err = r.ValueBytes(value, hive.ReadOptions{})
			if err != nil {
				b.Fatalf("ValueBytes failed: %v", err)
			}
		}

		benchGoBytes = bytes
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)

		if len(values) == 0 {
			b.Fatalf("No values")
		}

		value := values[0]
		var bytes []byte

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			bytes, _, err = h.ValueValue(value)
			if err != nil {
				b.Fatalf("ValueValue failed: %v", err)
			}
		}

		benchHivexBytes = bytes
	})
}

// BenchmarkNodeGetValue compares performance of looking up value by name
// Measures: hivex_node_get_value vs Reader.GetValue().
func BenchmarkNodeGetValue(b *testing.B) {
	// Use special hive which has nodes with values
	hf := BenchmarkHives[1] // special

	testCases := []struct {
		childName string
		valueName string
	}{
		{"abcd_äöüß", "abcd_äöüß"},
		{"zero\x00key", "zero\x00val"},
		{"weird™", "symbols $£₤₧€"},
	}

	for _, tc := range testCases {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name+"/"+tc.valueName, func(b *testing.B) {
			// Open and navigate to child once (not benchmarked)
			r, err := reader.Open(hf.Path, hive.OpenOptions{})
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer r.Close()

			root, err := r.Root()
			if err != nil {
				b.Fatalf("Root failed: %v", err)
			}

			child, err := r.Lookup(root, tc.childName)
			if err != nil {
				b.Fatalf("Lookup failed: %v", err)
			}

			var value hive.ValueID

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				value, err = r.GetValue(child, tc.valueName)
				if err != nil {
					b.Fatalf("GetValue failed: %v", err)
				}
			}

			benchGoValueID = value
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name+"/"+tc.valueName, func(b *testing.B) {
			// Open and navigate to child once (not benchmarked)
			h, err := bindings.Open(hf.Path, 0)
			if err != nil {
				b.Fatalf("Open failed: %v", err)
			}
			defer h.Close()

			root := h.Root()
			child := h.NodeGetChild(root, tc.childName)

			var value bindings.ValueHandle

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				value = h.NodeGetValue(child, tc.valueName)
			}

			benchHivexValue = value
		})
	}
}

// BenchmarkStatValue compares full value metadata retrieval
// gohivex-specific - returns all metadata in one call.
func BenchmarkStatValue(b *testing.B) {
	// Use special hive which has nodes with values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
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
			b.Fatalf("Values failed")
		}

		value := values[0]
		var meta hive.ValueMeta

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			meta, err = r.StatValue(value)
			if err != nil {
				b.Fatalf("StatValue failed: %v", err)
			}
		}

		benchGoValueMeta = meta
	})

	// Benchmark hivex - multiple calls to get equivalent data
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and get a value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		values := h.NodeValues(child)

		if len(values) == 0 {
			b.Fatalf("No values")
		}

		value := values[0]
		var (
			name  string
			vtype bindings.ValueType
			size  int
		)

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			name = h.ValueKey(value)
			vtype, size, err = h.ValueType(value)
			if err != nil {
				b.Fatalf("ValueType failed: %v", err)
			}
		}

		benchHivexString = name
		benchHivexInt = int(vtype) + size
	})
}

// BenchmarkAllValuesWithMetadata benchmarks getting all values and their metadata
// More realistic workload than single value operations.
func BenchmarkAllValuesWithMetadata(b *testing.B) {
	// Use rlenvalue hive which has more values
	hf := BenchmarkHives[1] // special, but could use a hive with more values

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and navigate to child once (not benchmarked)
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

		var meta hive.ValueMeta

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			values, valuesErr := r.Values(child)
			if valuesErr != nil {
				b.Fatalf("Values failed: %v", valuesErr)
			}

			for _, val := range values {
				var statErr error
				meta, statErr = r.StatValue(val)
				if statErr != nil {
					b.Fatalf("StatValue failed: %v", statErr)
				}
			}
		}

		benchGoValueMeta = meta
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and navigate to child once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")

		var name string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			values := h.NodeValues(child)

			for _, val := range values {
				name = h.ValueKey(val)
				_, _, typeErr := h.ValueType(val)
				if typeErr != nil {
					b.Fatalf("ValueType failed: %v", typeErr)
				}
			}
		}

		benchHivexString = name
	})
}
