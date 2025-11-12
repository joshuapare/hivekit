package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/edit"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/internal/format"
)

// Benchmark_EnsureKeyPath_Shallow benchmarks creating shallow key paths (1-2 levels).
func Benchmark_EnsureKeyPath_Shallow(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	b.ResetTimer()
	for i := range b.N {
		path := []string{fmt.Sprintf("_BenchKey_%d", i)}
		_, _, err := keyEditor.EnsureKeyPath(rootRef, path)
		if err != nil {
			b.Fatalf("EnsureKeyPath failed: %v", err)
		}
	}
}

// Benchmark_EnsureKeyPath_Deep benchmarks creating deep key paths (5 levels).
func Benchmark_EnsureKeyPath_Deep(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	b.ResetTimer()
	for i := range b.N {
		path := []string{
			fmt.Sprintf("_Bench_%d", i),
			"Level1",
			"Level2",
			"Level3",
			"Level4",
		}
		_, _, err := keyEditor.EnsureKeyPath(rootRef, path)
		if err != nil {
			b.Fatalf("EnsureKeyPath failed: %v", err)
		}
	}
}

// Benchmark_EnsureKeyPath_ExistingPath benchmarks navigating existing paths (no creation).
func Benchmark_EnsureKeyPath_ExistingPath(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create path once
	path := []string{"_ExistingBenchPath", "Sub1", "Sub2"}
	_, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	b.ResetTimer()
	for range b.N {
		_, _, ensureErr := keyEditor.EnsureKeyPath(rootRef, path)
		if ensureErr != nil {
			b.Fatalf("EnsureKeyPath failed: %v", ensureErr)
		}
	}
}

// Benchmark_UpsertValue_Inline benchmarks inline value operations (≤4 bytes).
func Benchmark_UpsertValue_Inline(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key
	path := []string{"_BenchValues"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	data := []byte{0x01, 0x02, 0x03, 0x04}

	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("Value_%d", i)
		upsertErr := valueEditor.UpsertValue(keyRef, name, format.REGDWORD, data)
		if upsertErr != nil {
			b.Fatalf("UpsertValue failed: %v", upsertErr)
		}
	}
}

// Benchmark_UpsertValue_External benchmarks external value operations (1KB).
func Benchmark_UpsertValue_External(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key
	path := []string{"_BenchValues"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	data := bytes.Repeat([]byte{0xAB}, 1024) // 1KB

	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("ExtValue_%d", i)
		upsertErr := valueEditor.UpsertValue(keyRef, name, format.REGBinary, data)
		if upsertErr != nil {
			b.Fatalf("UpsertValue failed: %v", upsertErr)
		}
	}
}

// Benchmark_UpsertValue_BigData benchmarks big-data value operations (20KB).
func Benchmark_UpsertValue_BigData(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key
	path := []string{"_BenchValues"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	data := bytes.Repeat([]byte{0xCD}, 20*1024) // 20KB

	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("BigValue_%d", i)
		upsertErr := valueEditor.UpsertValue(keyRef, name, format.REGBinary, data)
		if upsertErr != nil {
			b.Fatalf("UpsertValue failed: %v", upsertErr)
		}
	}
}

// Benchmark_UpsertValue_Update benchmarks updating existing values.
func Benchmark_UpsertValue_Update(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key and initial value
	path := []string{"_BenchValues"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	initialData := []byte{0x01, 0x02, 0x03, 0x04}
	err = valueEditor.UpsertValue(keyRef, "UpdateValue", format.REGDWORD, initialData)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	b.ResetTimer()
	for i := range b.N {
		data := []byte{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}
		upsertErr := valueEditor.UpsertValue(keyRef, "UpdateValue", format.REGDWORD, data)
		if upsertErr != nil {
			b.Fatalf("UpsertValue failed: %v", upsertErr)
		}
	}
}

// Benchmark_DeleteValue benchmarks value deletion.
func Benchmark_DeleteValue(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create key
	path := []string{"_BenchValues"}
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	// Pre-create values to delete
	data := []byte{0x01, 0x02, 0x03, 0x04}
	for i := range b.N {
		name := fmt.Sprintf("DelValue_%d", i)
		upsertErr := valueEditor.UpsertValue(keyRef, name, format.REGDWORD, data)
		if upsertErr != nil {
			b.Fatalf("Setup failed: %v", upsertErr)
		}
	}

	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("DelValue_%d", i)
		deleteErr := valueEditor.DeleteValue(keyRef, name)
		if deleteErr != nil {
			b.Fatalf("DeleteValue failed: %v", deleteErr)
		}
	}
}

// Benchmark_CompleteWorkflow benchmarks a realistic workflow
// Creates key path → adds multiple values → updates → deletes.
func Benchmark_CompleteWorkflow(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	b.ResetTimer()
	for i := range b.N {
		// Create key path
		path := []string{fmt.Sprintf("_Workflow_%d", i), "App", "Settings"}
		keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
		if err != nil {
			b.Fatalf("EnsureKeyPath failed: %v", err)
		}

		// Add 5 values of different types
		valueEditor.UpsertValue(keyRef, "Version", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		valueEditor.UpsertValue(keyRef, "Path", format.REGSZ, []byte("C:\\Program Files\\App\x00"))
		valueEditor.UpsertValue(keyRef, "Config", format.REGBinary, bytes.Repeat([]byte{0xAB}, 256))
		valueEditor.UpsertValue(keyRef, "Enabled", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		valueEditor.UpsertValue(
			keyRef,
			"InstallDate",
			format.REGQWORD,
			[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		)

		// Update one value
		valueEditor.UpsertValue(keyRef, "Version", format.REGDWORD, []byte{0x02, 0x00, 0x00, 0x00})

		// Delete one value
		valueEditor.DeleteValue(keyRef, "Config")
	}
}

// Benchmark_IndexLookup_Success benchmarks successful index lookups.
func Benchmark_IndexLookup_Success(b *testing.B) {
	h, allocator, idx, dt, cleanup := setupRealHiveBench(b)
	defer cleanup()

	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create keys
	path := []string{"_BenchIndex", "Keys"}
	parentRef, _, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	// Create 100 child keys
	for i := range 100 {
		childPath := append([]string(nil), path...)
		childPath = append(childPath, fmt.Sprintf("Child_%d", i))
		_, _, ensureErr := keyEditor.EnsureKeyPath(rootRef, childPath)
		if ensureErr != nil {
			b.Fatalf("Setup failed: %v", ensureErr)
		}
	}

	b.ResetTimer()
	for i := range b.N {
		name := fmt.Sprintf("child_%d", i%100) // Lookup in round-robin
		_, ok := idx.GetNK(parentRef, name)
		if !ok && i%100 < 100 {
			b.Fatalf("Lookup failed for existing key")
		}
	}
}

// Benchmark_MultiHive_Windows2003 benchmarks against Windows 2003 Server System hive.
func Benchmark_MultiHive_Windows2003(b *testing.B) {
	benchmarkWithHive(b, "../../testdata/suite/windows-2003-server-system")
}

// Benchmark_MultiHive_Windows2012 benchmarks against Windows 2012 System hive.
func Benchmark_MultiHive_Windows2012(b *testing.B) {
	benchmarkWithHive(b, "../../testdata/suite/windows-2012-system")
}

// Benchmark_MultiHive_Windows8 benchmarks against Windows 8 System hive.
func Benchmark_MultiHive_Windows8(b *testing.B) {
	benchmarkWithHive(b, "../../testdata/suite/windows-8-consumer-preview-system")
}

// benchmarkWithHive runs a standard benchmark workflow against a specific hive.
func benchmarkWithHive(b *testing.B, hivePath string) {
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		b.Skipf("Hive not found: %s", hivePath)
	}

	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		h, allocator, idx, dt, cleanup := setupHiveFromPath(b, hivePath)
		b.StartTimer()

		keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
		valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
		rootRef := h.RootCellOffset()

		// Standard workflow
		path := []string{fmt.Sprintf("_Bench_%d", i), "Test"}
		keyRef, _, _ := keyEditor.EnsureKeyPath(rootRef, path)
		valueEditor.UpsertValue(keyRef, "Value1", format.REGDWORD, []byte{0x01, 0x02, 0x03, 0x04})
		valueEditor.UpsertValue(keyRef, "Value2", format.REGSZ, []byte("Test\x00"))

		b.StopTimer()
		cleanup()
	}
}

// setupRealHiveBench sets up a real hive for benchmarking.
func setupRealHiveBench(
	b *testing.B,
) (*hive.Hive, *alloc.FastAllocator, index.Index, *dirty.Tracker, func()) {
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	return setupHiveFromPath(b, testHivePath)
}

// setupHiveFromPath sets up a hive from a specific path.
func setupHiveFromPath(
	b *testing.B,
	hivePath string,
) (*hive.Hive, *alloc.FastAllocator, index.Index, *dirty.Tracker, func()) {
	// Copy to temp directory
	tempHivePath := filepath.Join(b.TempDir(), "bench-hive")
	src, err := os.Open(hivePath)
	if err != nil {
		b.Skipf("Test hive not found: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		b.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		b.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		b.Fatalf("Failed to open hive: %v", err)
	}

	// Create dirty tracker
	dt := dirty.NewTracker(h)

	// Create allocator
	allocator, err := alloc.NewFast(h, dt, nil)
	if err != nil {
		h.Close()
		b.Fatalf("Failed to create allocator: %v", err)
	}

	// Create index
	idx := index.NewStringIndex(10000, 10000)

	cleanup := func() {
		h.Close()
	}

	return h, allocator, idx, dt, cleanup
}
