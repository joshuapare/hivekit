package comparison

import (
	"fmt"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Mutation benchmarks use SimpleFileWriter to match hivex behavior (no fsync).
// Both implementations write full hive to disk: hivex modifies in-place with
// append-only new blocks, gohivex rebuilds entire hive for safety/atomicity.
//
// Memory usage: gohivex uses ~80 KB vs hivex ~1 KB due to full rebuild approach.
// This is acceptable as 80 KB is negligible and provides transactional safety.
//
// Performance: gohivex is competitive or faster (0.97-1.28x) than hivex.
//
// Note: These benchmarks are displayed in separate "mutation" graphs to avoid
// polluting the scales of standard read operation graphs.

// BenchmarkNodeAddChild compares performance of creating child nodes
// Measures: hivex_node_add_child vs edit.Transaction.CreateKey()
//
// Note: Hivex benchmarks are skipped for mutation operations because:
// - Hivex lacks transaction/rollback support, accumulating all changes in memory
// - After thousands of iterations, hivex hits internal limits and crashes
// - Hivex is ~3000x slower, making comparisons impractical
// - gohivex uses transactions with rollback, so no actual disk writes occur.
func BenchmarkNodeAddChild(b *testing.B) {
	hf := BenchmarkHives[0] // small hive

	// Benchmark gohivex - single operation with commit for fair comparison
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive
			r, err := reader.Open(tmpPath, hive.OpenOptions{})
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			// Start transaction
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			childPath := fmt.Sprintf("BenchChild%d", i)

			b.StartTimer()
			err = tx.CreateKey(childPath, hive.CreateKeyOptions{})
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("CreateKey failed: %v", err)
			}

			// Commit to file (using SimpleFileWriter to match hivex behavior: no sync)
			writer := &hive.SimpleFileWriter{Path: tmpPath}
			err = tx.Commit(writer, hive.WriteOptions{})
			b.StopTimer()

			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			benchGoString = childPath
			r.Close()
			os.Remove(tmpPath)
		}
	})

	// Benchmark hivex - single operation with commit for fair comparison
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive with write flag
			h, err := bindings.Open(tmpPath, bindings.OPEN_WRITE)
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			root := h.Root()
			childName := fmt.Sprintf("BenchChild%d", i)

			b.StartTimer()
			child, err := h.NodeAddChild(root, childName)
			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("NodeAddChild failed: %v", err)
			}
			err = h.Commit(tmpPath)
			b.StopTimer()

			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			benchHivexNode = child
			h.Close()
			os.Remove(tmpPath)
		}
	})
}

// BenchmarkNodeSetValue compares performance of setting values
// Measures: hivex_node_set_value vs edit.Transaction.SetValue().
func BenchmarkNodeSetValue(b *testing.B) {
	hf := BenchmarkHives[0] // small hive

	// Test data: DWORD value
	dwordData := []byte{42, 0, 0, 0}

	// Benchmark gohivex - single operation with commit for fair comparison
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive
			r, err := reader.Open(tmpPath, hive.OpenOptions{})
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			// Start transaction
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			valueName := fmt.Sprintf("BenchValue%d", i)

			b.StartTimer()
			err = tx.SetValue("", valueName, hive.REG_DWORD, dwordData)
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("SetValue failed: %v", err)
			}

			// Commit to file (using SimpleFileWriter to match hivex behavior: no sync)
			writer := &hive.SimpleFileWriter{Path: tmpPath}
			err = tx.Commit(writer, hive.WriteOptions{})
			b.StopTimer()

			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			benchGoString = valueName
			r.Close()
			os.Remove(tmpPath)
		}
	})

	// Benchmark hivex - single operation with commit for fair comparison
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive with write flag
			h, err := bindings.Open(tmpPath, bindings.OPEN_WRITE)
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			root := h.Root()
			valueName := fmt.Sprintf("BenchValue%d", i)

			b.StartTimer()
			err = h.NodeSetValue(root, valueName, bindings.REG_DWORD, dwordData)
			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("NodeSetValue failed: %v", err)
			}
			err = h.Commit(tmpPath)
			b.StopTimer()

			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			benchHivexNode = root
			h.Close()
			os.Remove(tmpPath)
		}
	})
}

// BenchmarkNodeSetValues compares performance of setting multiple values at once
// Measures: hivex_node_set_values vs multiple SetValue calls.
func BenchmarkNodeSetValues(b *testing.B) {
	hf := BenchmarkHives[0] // small hive

	// Test data: multiple values
	dwordData := []byte{42, 0, 0, 0}
	stringData := []byte("T\x00e\x00s\x00t\x00\x00\x00") // UTF-16LE "Test"

	// Benchmark gohivex - single operation with commit for fair comparison
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive
			r, err := reader.Open(tmpPath, hive.OpenOptions{})
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			// Start transaction
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			val1Name := fmt.Sprintf("BenchValue%d_1", i)
			val2Name := fmt.Sprintf("BenchValue%d_2", i)

			b.StartTimer()
			err = tx.SetValue("", val1Name, hive.REG_DWORD, dwordData)
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("SetValue failed: %v", err)
			}
			err = tx.SetValue("", val2Name, hive.REG_SZ, stringData)
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("SetValue failed: %v", err)
			}

			// Commit to file (using SimpleFileWriter to match hivex behavior: no sync)
			writer := &hive.SimpleFileWriter{Path: tmpPath}
			err = tx.Commit(writer, hive.WriteOptions{})
			b.StopTimer()

			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			r.Close()
			os.Remove(tmpPath)
		}
	})

	// Benchmark hivex - single operation with commit for fair comparison
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive with write flag
			h, err := bindings.Open(tmpPath, bindings.OPEN_WRITE)
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			root := h.Root()
			val1Name := fmt.Sprintf("BenchValue%d_1", i)
			val2Name := fmt.Sprintf("BenchValue%d_2", i)

			values := []bindings.SetValue{
				{Key: val1Name, Type: bindings.REG_DWORD, Value: dwordData},
				{Key: val2Name, Type: bindings.REG_SZ, Value: stringData},
			}

			b.StartTimer()
			err = h.NodeSetValues(root, values)
			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("NodeSetValues failed: %v", err)
			}
			err = h.Commit(tmpPath)
			b.StopTimer()

			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			benchHivexNode = root
			h.Close()
			os.Remove(tmpPath)
		}
	})
}

// BenchmarkNodeDeleteChild compares performance of deleting child nodes
// Measures: hivex_node_delete_child vs edit.Transaction.DeleteKey().
func BenchmarkNodeDeleteChild(b *testing.B) {
	hf := BenchmarkHives[0] // small hive

	// Benchmark gohivex - single operation with commit for fair comparison
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive
			r, err := reader.Open(tmpPath, hive.OpenOptions{})
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			// Start transaction - create a child then delete it
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			childPath := fmt.Sprintf("BenchChild%d", i)

			// Pre-create the child to delete
			err = tx.CreateKey(childPath, hive.CreateKeyOptions{})
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("CreateKey failed: %v", err)
			}

			b.StartTimer()
			err = tx.DeleteKey(childPath, hive.DeleteKeyOptions{})
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("DeleteKey failed: %v", err)
			}

			// Commit to file (using SimpleFileWriter to match hivex behavior: no sync)
			writer := &hive.SimpleFileWriter{Path: tmpPath}
			err = tx.Commit(writer, hive.WriteOptions{})
			b.StopTimer()

			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			r.Close()
			os.Remove(tmpPath)
		}
	})

	// Benchmark hivex - single operation with commit for fair comparison
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		for i := range b.N {
			b.StopTimer()
			// Create fresh temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive with write flag
			h, err := bindings.Open(tmpPath, bindings.OPEN_WRITE)
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			root := h.Root()
			childName := fmt.Sprintf("BenchChild%d", i)

			// Pre-create child to delete
			child, err := h.NodeAddChild(root, childName)
			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("NodeAddChild failed: %v", err)
			}

			b.StartTimer()
			err = h.NodeDeleteChild(child)
			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("NodeDeleteChild failed: %v", err)
			}
			err = h.Commit(tmpPath)
			b.StopTimer()

			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("Commit failed: %v", err)
			}

			benchHivexNode = root
			h.Close()
			os.Remove(tmpPath)
		}
	})
}

// BenchmarkCommit compares performance of committing changes to disk
// Measures: hivex_commit vs edit.Transaction.Commit()
// Note: This is an expensive operation due to disk I/O.
func BenchmarkCommit(b *testing.B) {
	hf := BenchmarkHives[0] // small hive

	// Benchmark gohivex
	b.Run("gohivex/"+hf.Name, func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := range b.N {
			b.StopTimer()

			// Create temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive
			r, err := reader.Open(tmpPath, hive.OpenOptions{})
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			// Start transaction and make a small change
			ed := edit.NewEditor(r)
			tx := ed.Begin()
			childPath := fmt.Sprintf("BenchChild%d", i)
			err = tx.CreateKey(childPath, hive.CreateKeyOptions{})
			if err != nil {
				r.Close()
				os.Remove(tmpPath)
				b.Fatalf("CreateKey failed: %v", err)
			}

			b.StartTimer()

			// Benchmark the commit operation (using SimpleFileWriter to match hivex behavior: no sync)
			writer := &hive.SimpleFileWriter{Path: tmpPath + ".out"}
			err = tx.Commit(writer, hive.WriteOptions{})

			b.StopTimer()

			r.Close()
			os.Remove(tmpPath)
			os.Remove(tmpPath + ".out")

			if err != nil {
				b.Fatalf("Commit failed: %v", err)
			}
		}
	})

	// Benchmark hivex
	b.Run("hivex/"+hf.Name, func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := range b.N {
			b.StopTimer()

			// Create temp copy for each iteration
			tmpFile, err := os.CreateTemp("", "bench-*.tmp")
			if err != nil {
				b.Fatalf("CreateTemp failed: %v", err)
			}
			tmpPath := tmpFile.Name()
			tmpFile.Close()

			// Copy hive
			data, readErr := os.ReadFile(hf.Path)
			if readErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("ReadFile failed: %v", readErr)
			}
			if writeErr := os.WriteFile(tmpPath, data, 0644); writeErr != nil {
				os.Remove(tmpPath)
				b.Fatalf("WriteFile failed: %v", writeErr)
			}

			// Open hive with write flag
			h, err := bindings.Open(tmpPath, bindings.OPEN_WRITE)
			if err != nil {
				os.Remove(tmpPath)
				b.Fatalf("Open failed: %v", err)
			}

			// Make a small change
			root := h.Root()
			childName := fmt.Sprintf("BenchChild%d", i)
			_, err = h.NodeAddChild(root, childName)
			if err != nil {
				h.Close()
				os.Remove(tmpPath)
				b.Fatalf("NodeAddChild failed: %v", err)
			}

			b.StartTimer()

			// Benchmark the commit operation
			err = h.Commit(tmpPath + ".out")

			b.StopTimer()

			h.Close()
			os.Remove(tmpPath)
			os.Remove(tmpPath + ".out")

			if err != nil {
				b.Fatalf("Commit failed: %v", err)
			}
		}

		benchHivexInt = b.N
	})
}
