package integration

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Prevent compiler from optimizing away benchmark results.
//
//nolint:unused // Benchmark sink variables - intentionally write-only
var (
	benchResult    hive.Reader
	benchNodeID    hive.NodeID
	benchValueID   hive.ValueID
	benchKeyMeta   hive.KeyMeta
	benchValueMeta hive.ValueMeta
	benchBytes     []byte
	benchString    string
	benchStrings   []string
	benchUint32    uint32
	errBench       error
	benchNodeIDs   []hive.NodeID
	benchValueIDs  []hive.ValueID
	benchInt       int
)

// Benchmark hive files of different sizes.
var benchmarkHives = []struct {
	name     string
	path     string
	sizeDesc string
}{
	{"small", "../../testdata/suite/windows-xp-system", "~18K keys, ~45K values"},
	{"medium", "../../testdata/suite/windows-2003-server-software", "~66K keys, ~90K values"},
	{"large", "../../testdata/suite/windows-2012-software", "~125K keys, ~204K values"},
}

// BenchmarkOpenHive measures the cost of opening and parsing a hive.
func BenchmarkOpenHive(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			// Load file data once (not part of benchmark)
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			var r hive.Reader

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for range b.N {
				r, err = reader.OpenBytes(data, hive.OpenOptions{})
				if err != nil {
					b.Fatal(err)
				}
				// Immediately close to measure open+close overhead
				r.Close()
			}

			// Store to prevent dead code elimination
			benchResult = r
		})
	}
}

// BenchmarkOpenHive_ZeroCopy measures zero-copy mode performance.
func BenchmarkOpenHive_ZeroCopy(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			var r hive.Reader

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for range b.N {
				r, err = reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
				if err != nil {
					b.Fatal(err)
				}
				r.Close()
			}

			benchResult = r
		})
	}
}

// BenchmarkOpenAndReadRoot measures realistic open + basic operation.
func BenchmarkOpenAndReadRoot(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			var (
				r       hive.Reader
				rootID  hive.NodeID
				keyMeta hive.KeyMeta
			)

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for range b.N {
				r, err = reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
				if err != nil {
					b.Fatal(err)
				}

				// Do something real with the hive
				rootID, err = r.Root()
				if err != nil {
					b.Fatal(err)
				}

				keyMeta, err = r.StatKey(rootID)
				if err != nil {
					b.Fatal(err)
				}

				r.Close()
			}

			// Prevent optimization
			benchResult = r
			benchNodeID = rootID
			benchKeyMeta = keyMeta
		})
	}
}

// BenchmarkReadAllKeys measures the cost of enumerating all keys in the hive.
func BenchmarkReadAllKeys(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			rootID, _ := r.Root()
			keyCount := 0

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				keyCount = 0
				err = r.Walk(rootID, func(nodeID hive.NodeID) error {
					keyCount++
					benchNodeID = nodeID // Prevent optimization
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			benchInt = keyCount
		})
	}
}

// BenchmarkReadAllValues measures the cost of reading all value metadata.
func BenchmarkReadAllValues(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			rootID, _ := r.Root()
			valueCount := 0

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				valueCount = 0
				err = r.Walk(rootID, func(nodeID hive.NodeID) error {
					values, valuesErr := r.Values(nodeID)
					if valuesErr != nil {
						return valuesErr
					}
					for _, valueID := range values {
						valueMeta, statErr := r.StatValue(valueID)
						if statErr != nil {
							return statErr
						}
						valueCount++
						benchValueMeta = valueMeta // Prevent optimization
					}
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			benchInt = valueCount
		})
	}
}

// BenchmarkReadValueData measures the cost of reading actual value data.
func BenchmarkReadValueData(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			rootID, _ := r.Root()
			bytesRead := 0

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				bytesRead = 0
				err = r.Walk(rootID, func(nodeID hive.NodeID) error {
					values, valuesErr := r.Values(nodeID)
					if valuesErr != nil {
						return valuesErr
					}
					for _, valueID := range values {
						valueData, bytesErr := r.ValueBytes(valueID, hive.ReadOptions{})
						if bytesErr == nil {
							bytesRead += len(valueData)
							benchBytes = valueData // Prevent optimization
						}
					}
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			benchInt = bytesRead
		})
	}
}

// BenchmarkPathLookup measures the cost of finding keys by path.
func BenchmarkPathLookup(b *testing.B) {
	paths := []string{
		"\\ControlSet001\\Control\\Session Manager",
		"\\ControlSet001\\Services\\Tcpip\\Parameters",
		"\\Microsoft\\Windows\\CurrentVersion\\Run",
	}

	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			var nodeID hive.NodeID

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				for _, path := range paths {
					nodeID, err = r.Find(path)
					// Don't fail on not found - some hives don't have all paths
					if err == nil {
						benchNodeID = nodeID // Prevent optimization
					}
				}
			}
		})
	}
}

// BenchmarkSubkeyEnumeration measures the cost of listing subkeys.
func BenchmarkSubkeyEnumeration(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			rootID, _ := r.Root()
			var subkeys []hive.NodeID

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				subkeys, err = r.Subkeys(rootID)
				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			benchNodeIDs = subkeys
		})
	}
}

// BenchmarkStatKey measures the cost of reading key metadata.
func BenchmarkStatKey(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			rootID, _ := r.Root()
			var keyMeta hive.KeyMeta

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				keyMeta, err = r.StatKey(rootID)
				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()
			benchKeyMeta = keyMeta
		})
	}
}

// BenchmarkValueStringDecode measures UTF-16LE to UTF-8 conversion cost.
func BenchmarkValueStringDecode(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			// Collect all string value IDs (setup, not benchmarked)
			var stringValues []hive.ValueID
			rootID, _ := r.Root()
			_ = r.Walk(rootID, func(nodeID hive.NodeID) error {
				values, _ := r.Values(nodeID)
				for _, valueID := range values {
					meta, statErr := r.StatValue(valueID)
					if statErr == nil && (meta.Type == hive.REG_SZ || meta.Type == hive.REG_EXPAND_SZ) {
						stringValues = append(stringValues, valueID)
						if len(stringValues) >= 1000 {
							return hive.ErrNotFound // Stop early
						}
					}
				}
				return nil
			})

			if len(stringValues) == 0 {
				b.Skip("No string values found")
			}

			var str string

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				for _, valueID := range stringValues {
					str, err = r.ValueString(valueID, hive.ReadOptions{})
					if err == nil {
						benchString = str // Prevent optimization
					}
				}
			}

			b.StopTimer()
			b.ReportMetric(float64(len(stringValues)), "strings/op")
		})
	}
}

// BenchmarkFullTreeTraversal measures complete hive enumeration.
func BenchmarkFullTreeTraversal(b *testing.B) {
	for _, tc := range benchmarkHives {
		b.Run(tc.name, func(b *testing.B) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				b.Skipf("Hive not found: %s", tc.path)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{ZeroCopy: true})
			if err != nil {
				b.Fatal(err)
			}
			defer r.Close()

			rootID, _ := r.Root()
			keyCount := 0
			valueCount := 0
			bytesRead := 0

			b.ReportAllocs()
			b.ResetTimer()

			for range b.N {
				keyCount = 0
				valueCount = 0
				bytesRead = 0

				err = r.Walk(rootID, func(nodeID hive.NodeID) error {
					keyCount++

					// Read key metadata
					meta, statErr := r.StatKey(nodeID)
					if statErr != nil {
						return statErr
					}
					benchKeyMeta = meta

					// Read all values
					values, _ := r.Values(nodeID)
					for _, valueID := range values {
						valueCount++
						valueMeta, valStatErr := r.StatValue(valueID)
						if valStatErr != nil {
							continue
						}
						benchValueMeta = valueMeta

						valueData, bytesErr := r.ValueBytes(valueID, hive.ReadOptions{})
						if bytesErr == nil {
							bytesRead += len(valueData)
							benchBytes = valueData
						}
					}

					return nil
				})

				if err != nil {
					b.Fatal(err)
				}
			}

			b.StopTimer()

			// Report metrics
			b.ReportMetric(float64(keyCount), "keys/op")
			b.ReportMetric(float64(valueCount), "values/op")
			b.ReportMetric(float64(bytesRead)/1024/1024, "MB_data/op")
		})
	}
}
