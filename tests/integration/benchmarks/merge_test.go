package integration

import (
	"bytes"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/ast"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Prevent compiler optimization.
var (
	benchHiveResult []byte
	benchASTResult  *ast.Tree
)

// BenchmarkFullRebuild_1KeyChange benchmarks the current full rebuild approach
// when changing just 1 key in a large hive.
func BenchmarkFullRebuild_1KeyChange(b *testing.B) {
	// Use large hive
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		// Open hive
		r, openErr := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
		if openErr != nil {
			b.Fatal(openErr)
		}

		// Create transaction and modify 1 key
		ed := edit.NewEditor(r)
		tx := ed.Begin()
		tx.CreateKey("Software\\TestKey", hive.CreateKeyOptions{CreateParents: false})
		tx.SetValue("Software\\TestKey", "TestValue", hive.REG_SZ, []byte("test"))

		// Commit (full rebuild)
		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		benchHiveResult = buf.Bytes()
		r.Close()
	}
}

// BenchmarkFullRebuild_10KeyChanges benchmarks full rebuild with 10 key changes.
func BenchmarkFullRebuild_10KeyChanges(b *testing.B) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		r, openErr := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := edit.NewEditor(r)
		tx := ed.Begin()

		// Modify 10 keys
		for j := range 10 {
			path := "Software\\TestKey" + string(rune('0'+j))
			tx.CreateKey(path, hive.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "TestValue", hive.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		benchHiveResult = buf.Bytes()
		r.Close()
	}
}

// BenchmarkFullRebuild_100KeyChanges benchmarks full rebuild with 100 key changes.
func BenchmarkFullRebuild_100KeyChanges(b *testing.B) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		r, openErr := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := edit.NewEditor(r)
		tx := ed.Begin()

		// Modify 100 keys
		for j := range 100 {
			path := "Software\\TestKey" + string(rune('0'+j%10)) + string(rune('0'+j/10))
			tx.CreateKey(path, hive.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "Value", hive.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		buf := &bytes.Buffer{}
		if commitErr := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); commitErr != nil {
			b.Fatal(commitErr)
		}

		benchHiveResult = buf.Bytes()
		r.Close()
	}
}

// BenchmarkASTBuild_1KeyChange benchmarks building AST for 1 key change.
// This measures the incremental AST building overhead.
func BenchmarkASTBuild_1KeyChange(b *testing.B) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		r, openErr := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := edit.NewEditor(r)
		tx := ed.Begin()
		tx.CreateKey("Software\\TestKey", hive.CreateKeyOptions{CreateParents: false})
		tx.SetValue("Software\\TestKey", "TestValue", hive.REG_SZ, []byte("test"))

		// Build AST (incremental)
		tree, buildErr := ast.BuildIncremental(r, tx.(ast.TransactionChanges), getBaseBuffer(r))
		if buildErr != nil {
			b.Fatal(buildErr)
		}

		benchASTResult = tree
		r.Close()
	}
}

// BenchmarkASTBuild_10KeyChanges benchmarks building AST for 10 key changes.
func BenchmarkASTBuild_10KeyChanges(b *testing.B) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		r, openErr := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := edit.NewEditor(r)
		tx := ed.Begin()

		for j := range 10 {
			path := "Software\\TestKey" + string(rune('0'+j))
			tx.CreateKey(path, hive.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "TestValue", hive.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		tree, buildErr := ast.BuildIncremental(r, tx.(ast.TransactionChanges), getBaseBuffer(r))
		if buildErr != nil {
			b.Fatal(buildErr)
		}

		benchASTResult = tree
		r.Close()
	}
}

// BenchmarkASTBuild_100KeyChanges benchmarks building AST for 100 key changes.
func BenchmarkASTBuild_100KeyChanges(b *testing.B) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for range b.N {
		r, openErr := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
		if openErr != nil {
			b.Fatal(openErr)
		}

		ed := edit.NewEditor(r)
		tx := ed.Begin()

		for j := range 100 {
			path := "Software\\TestKey" + string(rune('0'+j%10)) + string(rune('0'+j/10))
			tx.CreateKey(path, hive.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "Value", hive.REG_DWORD, []byte{0x01, 0x00, 0x00, 0x00})
		}

		tree, buildErr := ast.BuildIncremental(r, tx.(ast.TransactionChanges), getBaseBuffer(r))
		if buildErr != nil {
			b.Fatal(buildErr)
		}

		benchASTResult = tree
		r.Close()
	}
}

// BenchmarkSequentialMerges benchmarks merging 100 small deltas sequentially.
// This simulates the use case of merging many .reg files.
func BenchmarkSequentialMerges(b *testing.B) {
	baseData, err := os.ReadFile("../../testdata/large")
	if err != nil {
		b.Skip("Large hive not available")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		currentData := make([]byte, len(baseData))
		copy(currentData, baseData)

		// Merge 100 small deltas
		for j := range 100 {
			r, openErr := reader.OpenBytes(currentData, hive.OpenOptions{ZeroCopy: true})
			if openErr != nil {
				b.Fatal(openErr)
			}

			ed := edit.NewEditor(r)
			tx := ed.Begin()

			// Each delta modifies 1 key
			path := "Software\\Delta" + string(rune('0'+j%10)) + string(rune('0'+j/10))
			tx.CreateKey(path, hive.CreateKeyOptions{CreateParents: false})
			tx.SetValue(path, "Value", hive.REG_DWORD, []byte{byte(j), 0x00, 0x00, 0x00})

			buf := &bytes.Buffer{}
			if commitErr := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); commitErr != nil {
				b.Fatal(commitErr)
			}

			currentData = buf.Bytes()
			r.Close()
		}

		benchHiveResult = currentData
	}
}

// bufWriter implements hive.Writer.
type bufWriter struct {
	buf *bytes.Buffer
}

func (w *bufWriter) WriteHive(data []byte) error {
	_, err := w.buf.Write(data)
	return err
}

// getBaseBuffer extracts base buffer from reader for zero-copy.
type baseBufferReader interface {
	BaseBuffer() []byte
}

func getBaseBuffer(r hive.Reader) []byte {
	if bb, ok := r.(baseBufferReader); ok {
		return bb.BaseBuffer()
	}
	return nil
}
