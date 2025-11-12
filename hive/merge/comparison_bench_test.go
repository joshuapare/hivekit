package merge

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	oldedit "github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

// These benchmarks compare old merge (full rebuild) vs new merge (mmap + dirty tracking)
// using the exact same test scenarios for an apples-to-apples comparison.

var (
	// Prevent compiler optimization.
	benchResult []byte
)

// =============================================================================
// SCENARIO 1: Single Key Change (Minimal merge)
// =============================================================================

func BenchmarkOldMerge_1KeyChange(b *testing.B) {
	// Setup: Create a copy of test hive for each iteration
	baseHive := getTestHivePath(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		// Create temp copy for this iteration
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		// Read entire hive into memory
		data, err := os.ReadFile(tempHive)
		if err != nil {
			b.Fatal(err)
		}

		// Open with old reader/editor
		r, openErr := reader.OpenBytes(data, types.OpenOptions{})
		if openErr != nil {
			b.Fatal(openErr)
		}

		// Create transaction and modify 1 key
		ed := oldedit.NewEditor(r)
		tx := ed.Begin()
		tx.CreateKey("Software\\BenchKey", types.CreateKeyOptions{CreateParents: false})
		tx.SetValue("Software\\BenchKey", "TestValue", types.REG_SZ, []byte("test\x00"))

		// Commit (full rebuild to buffer)
		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, types.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		// Write back to disk
		if writeErr := os.WriteFile(tempHive, buf.Bytes(), 0644); writeErr != nil {
			b.Fatal(writeErr)
		}

		benchResult = buf.Bytes()
		r.Close()

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_1KeyChange_InPlace(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "BenchKey"})
		plan.AddSetValue([]string{"Software", "BenchKey"}, "TestValue", format.REGSZ, []byte("test\x00"))

		opts := DefaultOptions()
		opts.Strategy = StrategyInPlace
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_1KeyChange_Append(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "BenchKey"})
		plan.AddSetValue([]string{"Software", "BenchKey"}, "TestValue", format.REGSZ, []byte("test\x00"))

		opts := DefaultOptions()
		opts.Strategy = StrategyAppend
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_1KeyChange_Hybrid(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "BenchKey"})
		plan.AddSetValue([]string{"Software", "BenchKey"}, "TestValue", format.REGSZ, []byte("test\x00"))

		// Uses default (Hybrid) strategy
		_, err := MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// =============================================================================
// SCENARIO 2: 10 Key Changes (Small merge)
// =============================================================================

func BenchmarkOldMerge_10KeyChanges(b *testing.B) {
	baseHive := getTestHivePath(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		data, err := os.ReadFile(tempHive)
		if err != nil {
			b.Fatal(err)
		}

		r, openErr := reader.OpenBytes(data, types.OpenOptions{})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := oldedit.NewEditor(r)
		tx := ed.Begin()

		// Modify 10 keys
		for j := range 10 {
			path := fmt.Sprintf("Software\\BenchKey%d", j)
			tx.CreateKey(path, types.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "Value", types.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, types.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		if writeErr := os.WriteFile(tempHive, buf.Bytes(), 0644); writeErr != nil {
			b.Fatal(writeErr)
		}

		benchResult = buf.Bytes()
		r.Close()

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_10KeyChanges_InPlace(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		for j := range 10 {
			keyPath := []string{"Software", fmt.Sprintf("BenchKey%d", j)}
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		opts := DefaultOptions()
		opts.Strategy = StrategyInPlace
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_10KeyChanges_Append(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		for j := range 10 {
			keyPath := []string{"Software", fmt.Sprintf("BenchKey%d", j)}
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		opts := DefaultOptions()
		opts.Strategy = StrategyAppend
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_10KeyChanges_Hybrid(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		for j := range 10 {
			keyPath := []string{"Software", fmt.Sprintf("BenchKey%d", j)}
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		// Uses default (Hybrid) strategy
		_, err := MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// =============================================================================
// SCENARIO 3: 100 Key Changes (Medium merge)
// =============================================================================

func BenchmarkOldMerge_100KeyChanges(b *testing.B) {
	baseHive := getTestHivePath(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		data, err := os.ReadFile(tempHive)
		if err != nil {
			b.Fatal(err)
		}

		r, openErr := reader.OpenBytes(data, types.OpenOptions{})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := oldedit.NewEditor(r)
		tx := ed.Begin()

		// Modify 100 keys
		for j := range 100 {
			path := fmt.Sprintf("Software\\BenchKey%d", j)
			tx.CreateKey(path, types.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "Value", types.REG_DWORD, []byte{byte(j), 0x00, 0x00, 0x00})
		}

		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, types.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		if writeErr := os.WriteFile(tempHive, buf.Bytes(), 0644); writeErr != nil {
			b.Fatal(writeErr)
		}

		benchResult = buf.Bytes()
		r.Close()

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_100KeyChanges_InPlace(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		for j := range 100 {
			keyPath := []string{"Software", fmt.Sprintf("BenchKey%d", j)}
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(j), 0x00, 0x00, 0x00})
		}

		opts := DefaultOptions()
		opts.Strategy = StrategyInPlace
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_100KeyChanges_Append(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		for j := range 100 {
			keyPath := []string{"Software", fmt.Sprintf("BenchKey%d", j)}
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(j), 0x00, 0x00, 0x00})
		}

		opts := DefaultOptions()
		opts.Strategy = StrategyAppend
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_100KeyChanges_Hybrid(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		for j := range 100 {
			keyPath := []string{"Software", fmt.Sprintf("BenchKey%d", j)}
			plan.AddEnsureKey(keyPath)
			plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(j), 0x00, 0x00, 0x00})
		}

		// Uses default (Hybrid) strategy
		_, err := MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// =============================================================================
// SCENARIO 4: Sequential Merges (100 small merges in sequence)
// =============================================================================

func BenchmarkOldMerge_SequentialMerges(b *testing.B) {
	baseHive := getTestHivePath(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		currentData, err := os.ReadFile(tempHive)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		// Merge 100 small deltas sequentially
		for j := range 100 {
			r, openErr := reader.OpenBytes(currentData, types.OpenOptions{})
			if openErr != nil {
				b.Fatal(openErr)
			}

			ed := oldedit.NewEditor(r)
			tx := ed.Begin()

			// Each delta modifies 1 key
			path := fmt.Sprintf("Software\\Delta%d", j)
			tx.CreateKey(path, types.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "Value", types.REG_DWORD, []byte{byte(j), 0x00, 0x00, 0x00})

			buf := &bytes.Buffer{}
			if commitErr := tx.Commit(&bufWriter{buf}, types.WriteOptions{}); commitErr != nil {
				b.Fatal(commitErr)
			}

			currentData = buf.Bytes()
			r.Close()
		}

		// Write final result
		if writeErr := os.WriteFile(tempHive, currentData, 0644); writeErr != nil {
			b.Fatal(writeErr)
		}

		benchResult = currentData

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_SequentialMerges_InPlace(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		// Merge 100 small deltas sequentially
		// With new merge, we can keep the hive open and reuse the session!
		opts := DefaultOptions()
		opts.Strategy = StrategyInPlace
		err := WithSession(tempHive, &opts, func(s *Session) error {
			for j := range 100 {
				plan := NewPlan()
				keyPath := []string{"Software", fmt.Sprintf("Delta%d", j)}
				plan.AddEnsureKey(keyPath)
				plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(j), 0x00, 0x00, 0x00})

				if _, err := s.ApplyWithTx(plan); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_SequentialMerges_Append(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		// Merge 100 small deltas sequentially
		// With new merge, we can keep the hive open and reuse the session!
		opts := DefaultOptions()
		opts.Strategy = StrategyAppend
		err := WithSession(tempHive, &opts, func(s *Session) error {
			for j := range 100 {
				plan := NewPlan()
				keyPath := []string{"Software", fmt.Sprintf("Delta%d", j)}
				plan.AddEnsureKey(keyPath)
				plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(j), 0x00, 0x00, 0x00})

				if _, err := s.ApplyWithTx(plan); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_SequentialMerges_Hybrid(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		// Merge 100 small deltas sequentially
		// With new merge, we can keep the hive open and reuse the session!
		// Uses default (Hybrid) strategy
		err := WithSession(tempHive, nil, func(s *Session) error {
			for j := range 100 {
				plan := NewPlan()
				keyPath := []string{"Software", fmt.Sprintf("Delta%d", j)}
				plan.AddEnsureKey(keyPath)
				plan.AddSetValue(keyPath, "Value", format.REGDWORD, []byte{byte(j), 0x00, 0x00, 0x00})

				if _, err := s.ApplyWithTx(plan); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// =============================================================================
// SCENARIO 5: Large Values (20KB big-data format)
// =============================================================================

func BenchmarkOldMerge_LargeValue(b *testing.B) {
	baseHive := getTestHivePath(b)
	largeData := bytes.Repeat([]byte{0xAB}, 20*1024) // 20KB

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		data, err := os.ReadFile(tempHive)
		if err != nil {
			b.Fatal(err)
		}

		r, openErr := reader.OpenBytes(data, types.OpenOptions{})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := oldedit.NewEditor(r)
		tx := ed.Begin()

		tx.CreateKey("Software\\LargeBench", types.CreateKeyOptions{CreateParents: false})
		tx.SetValue("Software\\LargeBench", "BigData", types.REG_BINARY, largeData)

		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, types.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		if writeErr := os.WriteFile(tempHive, buf.Bytes(), 0644); writeErr != nil {
			b.Fatal(writeErr)
		}

		benchResult = buf.Bytes()
		r.Close()

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_LargeValue_InPlace(b *testing.B) {
	baseHive := getTestHivePath(b)
	largeData := bytes.Repeat([]byte{0xAB}, 20*1024) // 20KB
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "LargeBench"})
		plan.AddSetValue([]string{"Software", "LargeBench"}, "BigData", format.REGBinary, largeData)

		opts := DefaultOptions()
		opts.Strategy = StrategyInPlace
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_LargeValue_Append(b *testing.B) {
	baseHive := getTestHivePath(b)
	largeData := bytes.Repeat([]byte{0xAB}, 20*1024) // 20KB
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "LargeBench"})
		plan.AddSetValue([]string{"Software", "LargeBench"}, "BigData", format.REGBinary, largeData)

		opts := DefaultOptions()
		opts.Strategy = StrategyAppend
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_LargeValue_Hybrid(b *testing.B) {
	baseHive := getTestHivePath(b)
	largeData := bytes.Repeat([]byte{0xAB}, 20*1024) // 20KB
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()
		plan.AddEnsureKey([]string{"Software", "LargeBench"})
		plan.AddSetValue([]string{"Software", "LargeBench"}, "BigData", format.REGBinary, largeData)

		// Uses default (Hybrid) strategy
		_, err := MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// =============================================================================
// SCENARIO 6: Deep Hierarchy (10 nested keys)
// =============================================================================

func BenchmarkOldMerge_DeepHierarchy(b *testing.B) {
	baseHive := getTestHivePath(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		data, err := os.ReadFile(tempHive)
		if err != nil {
			b.Fatal(err)
		}

		r, openErr := reader.OpenBytes(data, types.OpenOptions{})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := oldedit.NewEditor(r)
		tx := ed.Begin()

		// Create 10-level deep hierarchy
		path := "Software"
		for j := range 10 {
			path += fmt.Sprintf("\\Level%d", j)
			tx.CreateKey(path, types.CreateKeyOptions{CreateParents: false})
		}
		tx.SetValue(path, "DeepValue", types.REG_SZ, []byte("test\x00"))

		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, types.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		if writeErr := os.WriteFile(tempHive, buf.Bytes(), 0644); writeErr != nil {
			b.Fatal(writeErr)
		}

		benchResult = buf.Bytes()
		r.Close()

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_DeepHierarchy_InPlace(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()

		// Create 10-level deep hierarchy
		keyPath := []string{"Software"}
		for j := range 10 {
			keyPath = append(keyPath, fmt.Sprintf("Level%d", j))
		}
		plan.AddEnsureKey(keyPath)
		plan.AddSetValue(keyPath, "DeepValue", format.REGSZ, []byte("test\x00"))

		opts := DefaultOptions()
		opts.Strategy = StrategyInPlace
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_DeepHierarchy_Append(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()

		// Create 10-level deep hierarchy
		keyPath := []string{"Software"}
		for j := range 10 {
			keyPath = append(keyPath, fmt.Sprintf("Level%d", j))
		}
		plan.AddEnsureKey(keyPath)
		plan.AddSetValue(keyPath, "DeepValue", format.REGSZ, []byte("test\x00"))

		opts := DefaultOptions()
		opts.Strategy = StrategyAppend
		_, err := MergePlan(tempHive, plan, &opts)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

func BenchmarkNewMerge_DeepHierarchy_Hybrid(b *testing.B) {
	baseHive := getTestHivePath(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		tempHive := createTempCopy(b, baseHive)
		b.StartTimer()

		plan := NewPlan()

		// Create 10-level deep hierarchy
		keyPath := []string{"Software"}
		for j := range 10 {
			keyPath = append(keyPath, fmt.Sprintf("Level%d", j))
		}
		plan.AddEnsureKey(keyPath)
		plan.AddSetValue(keyPath, "DeepValue", format.REGSZ, []byte("test\x00"))

		// Uses default (Hybrid) strategy
		_, err := MergePlan(tempHive, plan, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		os.Remove(tempHive)
	}
}

// =============================================================================
// Helper functions
// =============================================================================

type bufWriter struct {
	buf *bytes.Buffer
}

func (w *bufWriter) WriteHive(data []byte) error {
	_, err := w.buf.Write(data)
	return err
}

func getTestHivePath(b *testing.B) string {
	const name = "windows-2003-server-system"
	path := filepath.Join("../../testdata/suite", name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		b.Skipf("Test hive not found: %s", path)
	}
	return path
}

func createTempCopy(b *testing.B, srcPath string) string {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		b.Fatalf("Failed to read source hive: %v", err)
	}

	tempFile, createErr := os.CreateTemp("", "bench-*.hive")
	if createErr != nil {
		b.Fatalf("Failed to create temp file: %v", createErr)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	if writeErr := os.WriteFile(tempPath, data, 0644); writeErr != nil {
		b.Fatalf("Failed to write temp hive: %v", writeErr)
	}

	return tempPath
}
