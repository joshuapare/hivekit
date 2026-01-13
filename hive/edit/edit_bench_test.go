package edit

import (
	"fmt"
	"testing"

	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/internal/format"
)

// Benchmark_DeleteKey_EmptyLeaf benchmarks deleting an empty leaf key.
func Benchmark_DeleteKey_EmptyLeaf(b *testing.B) {
	for range b.N {
		b.StopTimer()
		h, allocator, idx, _, cleanup := setupRealHive(b)

		dt := dirty.NewTracker(h)
		ke := NewKeyEditor(h, allocator, idx, dt)

		// Create a test key
		root := h.RootCellOffset()
		keyRef, _, err := ke.EnsureKeyPath(root, []string{"TestKey"})
		if err != nil {
			b.Fatal(err)
		}

		b.StartTimer()
		if deleteErr := ke.DeleteKey(keyRef, true); deleteErr != nil {
			b.Fatal(deleteErr)
		}
		b.StopTimer()
		cleanup()
	}
}

// Benchmark_DeleteKey_WithValues benchmarks deleting a key with multiple values.
func Benchmark_DeleteKey_WithValues(b *testing.B) {
	for range b.N {
		b.StopTimer()
		h, allocator, idx, _, cleanup := setupRealHive(b)

		dt := dirty.NewTracker(h)
		ke := NewKeyEditor(h, allocator, idx, dt)
		ve := NewValueEditor(h, allocator, idx, dt)

		// Create a test key with values
		root := h.RootCellOffset()
		keyRef, _, err := ke.EnsureKeyPath(root, []string{"TestKey"})
		if err != nil {
			b.Fatal(err)
		}

		// Add multiple values (5 values of varying sizes)
		if upsertErr := ve.UpsertValue(keyRef, "StringValue", format.REGSZ, []byte("test")); upsertErr != nil {
			b.Fatal(upsertErr)
		}
		if upsertErr := ve.UpsertValue(keyRef, "DwordValue", format.REGDWORD, []byte{0x01, 0x02, 0x03, 0x04}); upsertErr != nil {
			b.Fatal(upsertErr)
		}
		if upsertErr := ve.UpsertValue(keyRef, "BinaryValue", format.REGBinary, make([]byte, 100)); upsertErr != nil {
			b.Fatal(upsertErr)
		}
		if upsertErr := ve.UpsertValue(keyRef, "LargeValue", format.REGBinary, make([]byte, 5000)); upsertErr != nil {
			b.Fatal(upsertErr)
		}
		if upsertErr := ve.UpsertValue(keyRef, "DefaultValue", format.REGSZ, []byte("")); upsertErr != nil {
			b.Fatal(upsertErr)
		}

		b.StartTimer()
		if deleteErr := ke.DeleteKey(keyRef, true); deleteErr != nil {
			b.Fatal(deleteErr)
		}
		b.StopTimer()
		cleanup()
	}
}

// Benchmark_DeleteKey_WithSubkeys benchmarks deleting a key with subkeys.
func Benchmark_DeleteKey_WithSubkeys(b *testing.B) {
	for range b.N {
		b.StopTimer()
		h, allocator, idx, _, cleanup := setupRealHive(b)

		dt := dirty.NewTracker(h)
		ke := NewKeyEditor(h, allocator, idx, dt)

		// Create a test key with 5 subkeys
		root := h.RootCellOffset()
		keyRef, _, err := ke.EnsureKeyPath(root, []string{"TestKey"})
		if err != nil {
			b.Fatal(err)
		}

		for range 5 {
			_, _, ensureErr := ke.EnsureKeyPath(keyRef, []string{"Subkey1", "Subkey2", "Subkey3"})
			if ensureErr != nil {
				b.Fatal(ensureErr)
			}
		}

		b.StartTimer()
		if deleteErr := ke.DeleteKey(keyRef, true); deleteErr != nil {
			b.Fatal(deleteErr)
		}
		b.StopTimer()
		cleanup()
	}
}

// Benchmark_DeleteKey_DeepHierarchy benchmarks deleting a deep key hierarchy.
func Benchmark_DeleteKey_DeepHierarchy(b *testing.B) {
	for range b.N {
		b.StopTimer()
		h, allocator, idx, _, cleanup := setupRealHive(b)

		dt := dirty.NewTracker(h)
		ke := NewKeyEditor(h, allocator, idx, dt)
		ve := NewValueEditor(h, allocator, idx, dt)

		// Create a deep hierarchy (10 levels)
		root := h.RootCellOffset()
		path := []string{"L1", "L2", "L3", "L4", "L5", "L6", "L7", "L8", "L9", "L10"}
		keyRef, _, err := ke.EnsureKeyPath(root, path[:1])
		if err != nil {
			b.Fatal(err)
		}

		// Add values to some keys in the hierarchy
		for j := 1; j < len(path); j++ {
			subkeyRef, _, ensureErr := ke.EnsureKeyPath(root, path[:j+1])
			if ensureErr != nil {
				b.Fatal(ensureErr)
			}
			if upsertErr := ve.UpsertValue(subkeyRef, "TestValue", format.REGSZ, []byte("data")); upsertErr != nil {
				b.Fatal(upsertErr)
			}
		}

		b.StartTimer()
		if deleteErr := ke.DeleteKey(keyRef, true); deleteErr != nil {
			b.Fatal(deleteErr)
		}
		b.StopTimer()
		cleanup()
	}
}

// Benchmark_DeleteKey_WithBigData benchmarks deleting a key with big-data (DB) value.
func Benchmark_DeleteKey_WithBigData(b *testing.B) {
	for range b.N {
		b.StopTimer()
		h, allocator, idx, _, cleanup := setupRealHive(b)

		dt := dirty.NewTracker(h)
		ke := NewKeyEditor(h, allocator, idx, dt)
		ve := NewValueEditor(h, allocator, idx, dt)

		// Create a test key with big-data value (>16KB)
		root := h.RootCellOffset()
		keyRef, _, err := ke.EnsureKeyPath(root, []string{"TestKey"})
		if err != nil {
			b.Fatal(err)
		}

		// Add a large value that will trigger DB format (20KB)
		largeData := make([]byte, 20000)
		for j := range largeData {
			largeData[j] = byte(j % 256)
		}
		if upsertErr := ve.UpsertValue(keyRef, "LargeValue", format.REGBinary, largeData); upsertErr != nil {
			b.Fatal(upsertErr)
		}

		b.StartTimer()
		if deleteErr := ke.DeleteKey(keyRef, true); deleteErr != nil {
			b.Fatal(deleteErr)
		}
		b.StopTimer()
		cleanup()
	}
}

// Benchmark_DeleteValue benchmarks deleting individual values.
func Benchmark_DeleteValue(b *testing.B) {
	for range b.N {
		b.StopTimer()
		h, allocator, idx, _, cleanup := setupRealHive(b)

		dt := dirty.NewTracker(h)
		ke := NewKeyEditor(h, allocator, idx, dt)
		ve := NewValueEditor(h, allocator, idx, dt)

		// Create a test key with a value
		root := h.RootCellOffset()
		keyRef, _, err := ke.EnsureKeyPath(root, []string{"TestKey"})
		if err != nil {
			b.Fatal(err)
		}

		if upsertErr := ve.UpsertValue(keyRef, "TestValue", format.REGSZ, []byte("test data")); upsertErr != nil {
			b.Fatal(upsertErr)
		}

		b.StartTimer()
		if deleteErr := ve.DeleteValue(keyRef, "TestValue"); deleteErr != nil {
			b.Fatal(deleteErr)
		}
		b.StopTimer()
		cleanup()
	}
}

// Benchmark_IndexRemoval_NK benchmarks index NK removal performance.
func Benchmark_IndexRemoval_NK(b *testing.B) {
	idx := index.NewStringIndex(10000, 10000)

	// Pre-populate index
	for i := range 1000 {
		idx.AddNK(0x1000, fmt.Sprintf("Key%d", i), uint32(0x2000+i*0x100))
	}

	b.ResetTimer()
	for i := range b.N {
		keyName := fmt.Sprintf("Key%d", i%1000)
		idx.RemoveNK(0x1000, keyName)

		// Re-add to keep index populated
		if i%1000 == 999 {
			b.StopTimer()
			for j := range 1000 {
				idx.AddNK(0x1000, fmt.Sprintf("Key%d", j), uint32(0x2000+j*0x100))
			}
			b.StartTimer()
		}
	}
}

// Benchmark_IndexRemoval_VK benchmarks index VK removal performance.
func Benchmark_IndexRemoval_VK(b *testing.B) {
	idx := index.NewStringIndex(10000, 10000)

	// Pre-populate index
	for i := range 1000 {
		idx.AddVK(0x1000, fmt.Sprintf("Value%d", i), uint32(0x4000+i*0x100))
	}

	b.ResetTimer()
	for i := range b.N {
		valueName := fmt.Sprintf("Value%d", i%1000)
		idx.RemoveVK(0x1000, valueName)

		// Re-add to keep index populated
		if i%1000 == 999 {
			b.StopTimer()
			for j := range 1000 {
				idx.AddVK(0x1000, fmt.Sprintf("Value%d", j), uint32(0x4000+j*0x100))
			}
			b.StartTimer()
		}
	}
}

// Benchmark_DecodeName benchmarks the decodeName function performance.
func Benchmark_DecodeName_ASCII(b *testing.B) {
	nameBytes := []byte("SomeLongRegistryKeyName")

	b.ResetTimer()
	for range b.N {
		_ = decodeName(nameBytes, true)
	}
}

func Benchmark_DecodeName_UTF16LE(b *testing.B) {
	// "SomeLongRegistryKeyName" in UTF-16LE
	nameBytes := []byte{
		'S', 0, 'o', 0, 'm', 0, 'e', 0, 'L', 0, 'o', 0, 'n', 0, 'g', 0,
		'R', 0, 'e', 0, 'g', 0, 'i', 0, 's', 0, 't', 0, 'r', 0, 'y', 0,
		'K', 0, 'e', 0, 'y', 0, 'N', 0, 'a', 0, 'm', 0, 'e', 0,
	}

	b.ResetTimer()
	for range b.N {
		_ = decodeName(nameBytes, false)
	}
}
