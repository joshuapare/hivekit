package edit

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_LargeValue_Boundaries tests large value handling at critical boundaries.
func Test_LargeValue_Boundaries(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"Exactly 16344 bytes (threshold)", MaxExternalValueBytes},
		{"16345 bytes (triggers DB)", MaxExternalValueBytes + 1},
		{"20KB (well into DB)", 20 * 1024},
		{"50KB (large DB)", 50 * 1024},
		{"100KB (very large DB)", 100 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, allocator, idx, _, cleanup := setupRealHive(t)
			defer cleanup()

			dt := dirty.NewTracker(h)
			keyEditor := NewKeyEditor(h, allocator, idx, dt)
			valueEditor := NewValueEditor(h, allocator, idx, dt)

			rootRef := h.RootCellOffset()
			keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_LargeValTest"})
			if err != nil {
				t.Fatalf("EnsureKeyPath failed: %v", err)
			}

			// Create test data with a pattern so we can verify integrity
			testData := make([]byte, tt.size)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Store the value
			err = valueEditor.UpsertValue(keyRef, "LargeValue", format.REGBinary, testData)
			if err != nil {
				t.Fatalf("UpsertValue failed for %d bytes: %v", tt.size, err)
			}

			// Verify it's in the index
			vkRef, ok := idx.GetVK(keyRef, "largevalue")
			if !ok {
				t.Fatal("Value not found in index after insert")
			}
			if vkRef == 0 {
				t.Fatal("Got zero VK reference")
			}

			t.Logf("Successfully stored %d bytes at VK ref 0x%X", tt.size, vkRef)
		})
	}
}

// Test_LargeValue_UpdateAndDelete tests updating and deleting large values.
func Test_LargeValue_UpdateAndDelete(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)

	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_UpdateDeleteTest"})
	if err != nil {
		t.Fatalf("EnsureKeyPath failed: %v", err)
	}

	// Create initial large value (20KB)
	initialData := bytes.Repeat([]byte{0xAA}, 20*1024)
	err = valueEditor.UpsertValue(keyRef, "TestValue", format.REGBinary, initialData)
	if err != nil {
		t.Fatalf("Initial UpsertValue failed: %v", err)
	}

	// Update to larger value (50KB)
	updatedData := bytes.Repeat([]byte{0xBB}, 50*1024)
	err = valueEditor.UpsertValue(keyRef, "TestValue", format.REGBinary, updatedData)
	if err != nil {
		t.Fatalf("Update UpsertValue failed: %v", err)
	}

	// Verify still in index
	vkRef, ok := idx.GetVK(keyRef, "testvalue")
	if !ok {
		t.Fatal("Value not found in index after update")
	}
	if vkRef == 0 {
		t.Fatal("Got zero VK reference after update")
	}

	// Delete the value
	err = valueEditor.DeleteValue(keyRef, "TestValue")
	if err != nil {
		t.Fatalf("DeleteValue failed: %v", err)
	}

	// Verify removed from index
	_, ok = idx.GetVK(keyRef, "testvalue")
	if ok {
		t.Error("Value still in index after deletion")
	}

	t.Log("Successfully updated and deleted large value")
}

// Test_LargeValue_DeleteKeyWithLargeValues tests deleting a key that has multiple large values.
func Test_LargeValue_DeleteKeyWithLargeValues(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)

	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_DeleteKeyLargeVals"})
	if err != nil {
		t.Fatalf("EnsureKeyPath failed: %v", err)
	}

	// Create multiple large values
	for i := range 3 {
		data := bytes.Repeat([]byte{byte(i)}, 25*1024)
		err = valueEditor.UpsertValue(keyRef, "LargeValue"+string(rune('A'+i)), format.REGBinary, data)
		if err != nil {
			t.Fatalf("UpsertValue %d failed: %v", i, err)
		}
	}

	// Delete the entire key (recursive)
	err = keyEditor.DeleteKey(keyRef, true)
	if err != nil {
		t.Fatalf("DeleteKey failed: %v", err)
	}

	// Verify key removed from index
	_, ok := idx.GetNK(rootRef, "_deletekeylargevals")
	if ok {
		t.Error("Key still in index after deletion")
	}

	// Verify all values removed from index
	for i := range 3 {
		_, valOk := idx.GetVK(keyRef, "largevalue"+string(rune('a'+i)))
		if valOk {
			t.Errorf("Value %d still in index after key deletion", i)
		}
	}

	t.Log("Successfully deleted key with multiple large values")
}

// Test_LargeValue_MultipleKeys tests large values across multiple keys.
func Test_LargeValue_MultipleKeys(t *testing.T) {
	h, allocator, idx, _, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)

	rootRef := h.RootCellOffset()

	// Create 5 keys each with a large value
	for i := range 5 {
		keyName := "_MultiKey" + string(rune('A'+i))
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{keyName})
		if err != nil {
			t.Fatalf("EnsureKeyPath for key %d failed: %v", i, err)
		}

		// Different size for each (15KB, 20KB, 25KB, 30KB, 35KB)
		size := (15 + i*5) * 1024
		data := make([]byte, size)
		for j := range data {
			data[j] = byte(j % 256)
		}

		err = valueEditor.UpsertValue(keyRef, "Data", format.REGBinary, data)
		if err != nil {
			t.Fatalf("UpsertValue for key %d failed: %v", i, err)
		}
	}

	// Verify all are in index
	for i := range 5 {
		keyName := "_multikey" + string(rune('a'+i))
		keyRef, ok := idx.GetNK(rootRef, keyName)
		if !ok {
			t.Errorf("Key %d not found in index", i)
			continue
		}

		_, ok = idx.GetVK(keyRef, "data")
		if !ok {
			t.Errorf("Value for key %d not found in index", i)
		}
	}

	t.Log("Successfully stored large values across multiple keys")
}

// Benchmark_LargeValue_Store benchmarks storing large values at different sizes.
func Benchmark_LargeValue_Store(b *testing.B) {
	sizes := []int{
		MaxExternalValueBytes,     // 16344 bytes (threshold)
		MaxExternalValueBytes + 1, // 16345 bytes (first DB)
		20 * 1024,                 // 20KB
		50 * 1024,                 // 50KB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			// Prepare test data once
			testData := make([]byte, size)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Setup once
			h, allocator, idx, _, cleanup := setupRealHive(b)
			defer cleanup()
			dt := dirty.NewTracker(h)
			keyEditor := NewKeyEditor(h, allocator, idx, dt)
			valueEditor := NewValueEditor(h, allocator, idx, dt)
			rootRef := h.RootCellOffset()
			keyRef, _, _ := keyEditor.EnsureKeyPath(rootRef, []string{"_BenchKey"})

			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				valueName := fmt.Sprintf("Value%d", i)
				err := valueEditor.UpsertValue(keyRef, valueName, format.REGBinary, testData)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Benchmark_LargeValue_Delete benchmarks deleting values with large data.
func Benchmark_LargeValue_Delete(b *testing.B) {
	sizes := []int{
		MaxExternalValueBytes + 1, // 16345 bytes (DB)
		20 * 1024,                 // 20KB
		50 * 1024,                 // 50KB
	}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			testData := make([]byte, size)
			for i := range testData {
				testData[i] = byte(i % 256)
			}

			// Setup once and pre-populate with values to delete
			h, allocator, idx, _, cleanup := setupRealHive(b)
			defer cleanup()
			dt := dirty.NewTracker(h)
			keyEditor := NewKeyEditor(h, allocator, idx, dt)
			valueEditor := NewValueEditor(h, allocator, idx, dt)
			rootRef := h.RootCellOffset()
			keyRef, _, _ := keyEditor.EnsureKeyPath(rootRef, []string{"_BenchKey"})

			// Pre-create all values
			for i := range b.N {
				valueName := fmt.Sprintf("Value%d", i)
				_ = valueEditor.UpsertValue(keyRef, valueName, format.REGBinary, testData)
			}

			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				valueName := fmt.Sprintf("Value%d", i)
				err := valueEditor.DeleteValue(keyRef, valueName)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// formatSize returns a human-readable size string for benchmark names.
func formatSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	}
	kb := bytes / 1024
	return fmt.Sprintf("%dKB", kb)
}
