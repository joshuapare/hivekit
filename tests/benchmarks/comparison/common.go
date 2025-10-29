// Package comparison provides benchmark utilities for comparing gohivex and hivex performance.
package comparison

import (
	"time"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// BenchmarkHives defines test hive files used across benchmarks
// Includes small, medium, and large hives for performance comparison.
var BenchmarkHives = []struct {
	Name     string // Short name for benchmark output
	Path     string // Path to hive file
	SizeDesc string // Human-readable size description
}{
	{
		Name:     "small",
		Path:     "../../../testdata/minimal",
		SizeDesc: "~8KB, 0 keys, 0 values",
	},
	{
		Name:     "medium",
		Path:     "../../../testdata/special",
		SizeDesc: "~8KB, 3 keys, 3 values",
	},
	{
		Name:     "large",
		Path:     "../../../testdata/large",
		SizeDesc: "~446KB, many keys/values",
	},
	{
		Name:     "typed_values",
		Path:     "../../../testdata/typed_values",
		SizeDesc: "~8KB, test hive with REG_SZ, REG_QWORD, REG_MULTI_SZ values",
	},
}

// Prevent compiler optimizations from eliminating benchmark code
// These variables are written to by benchmarks to ensure operations aren't optimized away.
var (
	// Reader results.
	benchGoReader    hive.Reader
	benchHivexReader *bindings.Hive

	// Node results.
	benchGoNodeID    hive.NodeID
	benchHivexNode   bindings.NodeHandle
	benchGoNodeIDs   []hive.NodeID
	benchHivexNodes  []bindings.NodeHandle
	benchGoKeyMeta   hive.KeyMeta
	benchGoKeyDetail hive.KeyDetail

	// Value results.
	benchGoValueID   hive.ValueID
	benchHivexValue  bindings.ValueHandle
	benchGoValueIDs  []hive.ValueID
	benchHivexValues []bindings.ValueHandle
	benchGoValueMeta hive.ValueMeta

	// Data results.
	benchGoBytes      []byte
	benchHivexBytes   []byte
	benchGoString     string
	benchHivexString  string
	benchGoStrings    []string
	benchHivexStrings []string
	benchGoInt32      int32
	benchHivexInt32   int32
	benchGoInt64      int64
	benchHivexInt64   int64
	benchGoUint32     uint32
	benchHivexDword   int32
	benchGoUint64     uint64
	benchHivexQword   int64
	benchGoTime       time.Time

	// Error results.
	benchErr error

	// Counters.
	benchInt      int
	benchGoInt    int
	benchHivexInt int
	benchCount    uint64
)
