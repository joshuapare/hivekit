package comparison

import (
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkValueDword compares performance of decoding DWORD values
// Measures: hivex_value_dword vs Reader.ValueDWORD().
func BenchmarkValueDword(b *testing.B) {
	// Use special hive which has DWORD values
	hf := BenchmarkHives[1] // special

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a DWORD value once (not benchmarked)
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

		value, err := r.GetValue(child, "abcd_äöüß")
		if err != nil {
			b.Fatalf("GetValue failed: %v", err)
		}

		var dword uint32

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			dword, err = r.ValueDWORD(value)
			if err != nil {
				b.Fatalf("ValueDWORD failed: %v", err)
			}
		}

		benchGoUint32 = dword
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and get a DWORD value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		value := h.NodeGetValue(child, "abcd_äöüß")

		var dword int32

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			dword, err = h.ValueDword(value)
			if err != nil {
				b.Fatalf("ValueDword failed: %v", err)
			}
		}

		benchHivexDword = dword
	})
}

// BenchmarkValueString compares performance of decoding string values
// Measures: hivex_value_string vs Reader.ValueString().
func BenchmarkValueString(b *testing.B) {
	// Use large hive which has REG_SZ values (compatible with both gohivex and hivex)
	hf := BenchmarkHives[2] // large

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a string value once (not benchmarked)
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		child, err := r.Lookup(root, "A")
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}

		// Find a REG_SZ value
		value, err := r.GetValue(child, "A")
		if err != nil {
			b.Fatalf("GetValue failed: %v", err)
		}

		var str string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			str, err = r.ValueString(value, hive.ReadOptions{})
			if err != nil {
				b.Fatalf("ValueString failed: %v", err)
			}
		}

		benchGoString = str
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		// Open and get a string value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "A")
		value := h.NodeGetValue(child, "A")

		var str string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			str, err = h.ValueString(value)
			if err != nil {
				b.Fatalf("ValueString failed: %v", err)
			}
		}

		benchHivexString = str
	})
}

// BenchmarkValueQword compares performance of decoding QWORD values
// Measures: hivex_value_qword vs Reader.ValueQWORD()
// Note: Only benchmarks gohivex - no existing hives with REG_QWORD are compatible with hivex C library.
func BenchmarkValueQword(b *testing.B) {
	// Use typed_values hive which has QWORD values
	hf := BenchmarkHives[3] // typed_values

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a QWORD value once (not benchmarked)
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		child, err := r.Lookup(root, "TypedValues")
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}

		// Find a REG_QWORD value
		value, err := r.GetValue(child, "QwordValue")
		if err != nil {
			b.Fatalf("GetValue failed: %v", err)
		}

		var qword uint64

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			qword, err = r.ValueQWORD(value)
			if err != nil {
				b.Fatalf("ValueQWORD failed: %v", err)
			}
		}

		benchGoUint64 = qword
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		b.Skip("typed_values hive is not compatible with hivex C library")

		// Open and get a QWORD value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "TypedValues")
		value := h.NodeGetValue(child, "QwordValue")

		var qword int64

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			qword, err = h.ValueQword(value)
			if err != nil {
				b.Fatalf("ValueQword failed: %v", err)
			}
		}

		benchHivexQword = qword
	})
}

// BenchmarkValueMultipleStrings compares performance of decoding multi-string values
// Measures: hivex_value_multiple_strings vs Reader.ValueStrings()
// Note: Only benchmarks gohivex - no existing hives with REG_MULTI_SZ are compatible with hivex C library.
func BenchmarkValueMultipleStrings(b *testing.B) {
	// Use typed_values hive which has multi-string values
	hf := BenchmarkHives[3] // typed_values

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		// Open and get a multi-string value once (not benchmarked)
		r, err := reader.Open(hf.Path, hive.OpenOptions{})
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer r.Close()

		root, err := r.Root()
		if err != nil {
			b.Fatalf("Root failed: %v", err)
		}

		child, err := r.Lookup(root, "TypedValues")
		if err != nil {
			b.Fatalf("Lookup failed: %v", err)
		}

		// Find a REG_MULTI_SZ value
		value, err := r.GetValue(child, "MultiStringValue")
		if err != nil {
			b.Fatalf("GetValue failed: %v", err)
		}

		var strs []string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			strs, err = r.ValueStrings(value, hive.ReadOptions{})
			if err != nil {
				b.Fatalf("ValueStrings failed: %v", err)
			}
		}

		benchGoStrings = strs
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		b.Skip("typed_values hive is not compatible with hivex C library")

		// Open and get a multi-string value once (not benchmarked)
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "TypedValues")
		value := h.NodeGetValue(child, "MultiStringValue")

		var strs []string

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			strs, err = h.ValueMultipleStrings(value)
			if err != nil {
				b.Fatalf("ValueMultipleStrings failed: %v", err)
			}
		}

		benchHivexStrings = strs
	})
}

// BenchmarkValueDecodeVsRawBytes compares typed decode vs raw bytes
// Shows overhead of type-specific decoding.
func BenchmarkValueDecodeVsRawBytes(b *testing.B) {
	// Use special hive which has DWORD values
	hf := BenchmarkHives[1] // special

	// Benchmark raw bytes (gohivex)
	b.Run("raw_bytes/gohivex/"+hf.Name, func(b *testing.B) {
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

		value, err := r.GetValue(child, "abcd_äöüß")
		if err != nil {
			b.Fatalf("GetValue failed: %v", err)
		}

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

	// Benchmark typed decode (gohivex)
	b.Run("typed_decode/gohivex/"+hf.Name, func(b *testing.B) {
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

		value, err := r.GetValue(child, "abcd_äöüß")
		if err != nil {
			b.Fatalf("GetValue failed: %v", err)
		}

		var dword uint32

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			dword, err = r.ValueDWORD(value)
			if err != nil {
				b.Fatalf("ValueDWORD failed: %v", err)
			}
		}

		benchGoUint32 = dword
	})

	// Benchmark raw bytes (hivex)
	b.Run("raw_bytes/hivex/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		value := h.NodeGetValue(child, "abcd_äöüß")

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

	// Benchmark typed decode (hivex)
	b.Run("typed_decode/hivex/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		child := h.NodeGetChild(root, "abcd_äöüß")
		value := h.NodeGetValue(child, "abcd_äöüß")

		var dword int32

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			dword, err = h.ValueDword(value)
			if err != nil {
				b.Fatalf("ValueDword failed: %v", err)
			}
		}

		benchHivexDword = dword
	})
}

// BenchmarkAllDwordValues benchmarks reading all DWORD values from a node
// More realistic than single value.
func BenchmarkAllDwordValues(b *testing.B) {
	// Use special hive
	hf := BenchmarkHives[1] // special

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

		// Get all children with DWORD values
		children, err := r.Subkeys(root)
		if err != nil {
			b.Fatalf("Subkeys failed: %v", err)
		}

		var dword uint32

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			for _, child := range children {
				values, valuesErr := r.Values(child)
				if valuesErr != nil {
					b.Fatalf("Values failed: %v", valuesErr)
				}

				for _, val := range values {
					vtype, typeErr := r.ValueType(val)
					if typeErr != nil {
						b.Fatalf("ValueType failed: %v", typeErr)
					}

					if vtype == hive.REG_DWORD {
						var dwordErr error
						dword, dwordErr = r.ValueDWORD(val)
						if dwordErr != nil {
							b.Fatalf("ValueDWORD failed: %v", dwordErr)
						}
					}
				}
			}
		}

		benchGoUint32 = dword
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		h, err := bindings.Open(hf.Path, 0)
		if err != nil {
			b.Fatalf("Open failed: %v", err)
		}
		defer h.Close()

		root := h.Root()
		children := h.NodeChildren(root)

		var dword int32

		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			for _, child := range children {
				values := h.NodeValues(child)

				for _, val := range values {
					vtype, _, typeErr := h.ValueType(val)
					if typeErr != nil {
						b.Fatalf("ValueType failed: %v", typeErr)
					}

					if vtype == bindings.REG_DWORD {
						dword, err = h.ValueDword(val)
						if err != nil {
							b.Fatalf("ValueDword failed: %v", err)
						}
					}
				}
			}
		}

		benchHivexDword = dword
	})
}
