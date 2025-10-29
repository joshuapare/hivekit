package comparison

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkOpen compares performance of opening hive files
// Measures: hivex_open vs reader.Open().
func BenchmarkOpen(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			var r hive.Reader
			var err error

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r, err = reader.Open(hf.Path, hive.OpenOptions{})
				if err != nil {
					b.Fatalf("Open failed: %v", err)
				}
				r.Close()
			}

			benchGoReader = r
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			var h *bindings.Hive
			var err error

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				h, err = bindings.Open(hf.Path, 0)
				if err != nil {
					b.Fatalf("Open failed: %v", err)
				}
				h.Close()
			}

			benchHivexReader = h
		})
	}
}

// BenchmarkOpenBytes compares performance of opening from memory
// gohivex-specific feature - hivex doesn't support this directly.
func BenchmarkOpenBytes_Gohivex(b *testing.B) {
	for _, hf := range BenchmarkHives {
		b.Run(hf.Name, func(b *testing.B) {
			// Load data once (not benchmarked)
			data, err := os.ReadFile(hf.Path)
			if err != nil {
				b.Skipf("File not found: %s", hf.Path)
			}

			var r hive.Reader

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r, err = reader.OpenBytes(data, hive.OpenOptions{})
				if err != nil {
					b.Fatalf("OpenBytes failed: %v", err)
				}
				r.Close()
			}

			benchGoReader = r
		})
	}
}

// BenchmarkOpenBytes_ZeroCopy compares zero-copy vs normal mode
// gohivex-specific optimization.
func BenchmarkOpenBytes_ZeroCopy(b *testing.B) {
	for _, hf := range BenchmarkHives {
		data, err := os.ReadFile(hf.Path)
		if err != nil {
			b.Skipf("File not found: %s", hf.Path)
		}

		// Normal mode
		b.Run("normal/"+hf.Name, func(b *testing.B) {
			var r hive.Reader

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r, err = reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: false})
				if err != nil {
					b.Fatalf("OpenBytes failed: %v", err)
				}
				r.Close()
			}

			benchGoReader = r
		})

		// Zero-copy mode
		b.Run("zerocopy/"+hf.Name, func(b *testing.B) {
			var r hive.Reader

			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r, err = reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
				if err != nil {
					b.Fatalf("OpenBytes failed: %v", err)
				}
				r.Close()
			}

			benchGoReader = r
		})
	}
}

// BenchmarkOpenAndGetRoot measures realistic open + first operation
// More realistic than just open/close.
func BenchmarkOpenAndGetRoot(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			var (
				r    hive.Reader
				root hive.NodeID
				err  error
			)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r, err = reader.Open(hf.Path, hive.OpenOptions{})
				if err != nil {
					b.Fatalf("Open failed: %v", err)
				}

				root, err = r.Root()
				if err != nil {
					b.Fatalf("Root failed: %v", err)
				}

				r.Close()
			}

			benchGoReader = r
			benchGoNodeID = root
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			var (
				h    *bindings.Hive
				root bindings.NodeHandle
				err  error
			)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				h, err = bindings.Open(hf.Path, 0)
				if err != nil {
					b.Fatalf("Open failed: %v", err)
				}

				root = h.Root()

				h.Close()
			}

			benchHivexReader = h
			benchHivexNode = root
		})
	}
}

// BenchmarkOpenAndReadRootMetadata measures open + basic metadata read.
func BenchmarkOpenAndReadRootMetadata(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			var (
				r    hive.Reader
				root hive.NodeID
				meta hive.KeyMeta
				err  error
			)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				r, err = reader.Open(hf.Path, hive.OpenOptions{})
				if err != nil {
					b.Fatalf("Open failed: %v", err)
				}

				root, err = r.Root()
				if err != nil {
					b.Fatalf("Root failed: %v", err)
				}

				meta, err = r.StatKey(root)
				if err != nil {
					b.Fatalf("StatKey failed: %v", err)
				}

				r.Close()
			}

			benchGoReader = r
			benchGoNodeID = root
			benchGoKeyMeta = meta
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			var (
				h    *bindings.Hive
				root bindings.NodeHandle
				name string
				err  error
			)

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				h, err = bindings.Open(hf.Path, 0)
				if err != nil {
					b.Fatalf("Open failed: %v", err)
				}

				root = h.Root()
				name = h.NodeName(root)
				_ = h.NodeNrChildren(root)
				_ = h.NodeNrValues(root)

				h.Close()
			}

			benchHivexReader = h
			benchHivexNode = root
			benchHivexString = name
		})
	}
}

// BenchmarkClose measures just close performance
// Rarely used alone, but good for understanding overhead.
func BenchmarkClose(b *testing.B) {
	for _, hf := range BenchmarkHives {
		// Benchmark gohivex
		b.Run("gohivex/"+hf.Name, func(b *testing.B) {
			// Pre-open readers (not benchmarked)
			readers := make([]hive.Reader, b.N)
			for i := 0; i < b.N; i++ {
				r, err := reader.Open(hf.Path, hive.OpenOptions{})
				if err != nil {
					b.Fatalf("Pre-open failed: %v", err)
				}
				readers[i] = r
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				readers[i].Close()
			}
		})

		// Benchmark hivex
		b.Run("hivex/"+hf.Name, func(b *testing.B) {
			// Pre-open hives (not benchmarked)
			hives := make([]*bindings.Hive, b.N)
			for i := 0; i < b.N; i++ {
				h, err := bindings.Open(hf.Path, 0)
				if err != nil {
					b.Fatalf("Pre-open failed: %v", err)
				}
				hives[i] = h
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				hives[i].Close()
			}
		})
	}
}
