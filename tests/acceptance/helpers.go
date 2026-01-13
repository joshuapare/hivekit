package acceptance

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// openGoHivex opens a hive file using the gohivex implementation.
func openGoHivex(t *testing.T, path string) hive.Reader {
	t.Helper()

	r, err := reader.Open(path, hive.OpenOptions{})
	require.NoError(t, err, "Failed to open hive with gohivex: %s", path)
	return r
}

// openGoHivexBytes opens a hive from bytes using the gohivex implementation.
func openGoHivexBytes(t *testing.T, data []byte) hive.Reader {
	t.Helper()

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	require.NoError(t, err, "Failed to open hive bytes with gohivex")
	return r
}

// openHivex opens a hive file using the original hivex library via bindings.
func openHivex(t *testing.T, path string) *bindings.Hive {
	t.Helper()

	h, err := bindings.Open(path, 0)
	require.NoError(t, err, "Failed to open hive with hivex: %s", path)
	return h
}

// loadHiveData loads a hive file into memory.
func loadHiveData(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read hive file: %s", path)
	return data
}

// assertSameNodeID compares a gohivex NodeID with a hivex NodeHandle
// They should represent the same underlying node, but use different offset conventions:
// - gohivex uses HBIN-relative offsets (what's in REGF header)
// - hivex uses absolute file offsets (HBIN-relative + format.HeaderSize).
func assertSameNodeID(t *testing.T, goNode hive.NodeID, hivexNode bindings.NodeHandle, msgAndArgs ...interface{}) {
	t.Helper()

	// Convert gohivex HBIN-relative offset to absolute file offset
	// The first HBIN starts at offset format.HeaderSize (4096) in the file
	goAbsolute := uint64(goNode) + format.HeaderSize

	if goAbsolute != uint64(hivexNode) {
		msg := "Node IDs don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf(
			"%s:\n  gohivex HBIN-relative: %d (0x%x)\n  gohivex absolute:      %d (0x%x)\n  hivex absolute:        %d (0x%x)",
			msg,
			goNode,
			goNode,
			goAbsolute,
			goAbsolute,
			hivexNode,
			hivexNode,
		)
	}
}

// assertSameValueID compares a gohivex ValueID with a hivex ValueHandle
// Same offset convention difference as NodeID (HBIN-relative vs absolute).
func assertSameValueID(t *testing.T, goVal hive.ValueID, hivexVal bindings.ValueHandle, msgAndArgs ...interface{}) {
	t.Helper()

	// Convert gohivex HBIN-relative offset to absolute file offset
	goAbsolute := uint64(goVal) + format.HeaderSize

	if goAbsolute != uint64(hivexVal) {
		msg := "Value IDs don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf(
			"%s:\n  gohivex HBIN-relative: %d (0x%x)\n  gohivex absolute:      %d (0x%x)\n  hivex absolute:        %d (0x%x)",
			msg,
			goVal,
			goVal,
			goAbsolute,
			goAbsolute,
			hivexVal,
			hivexVal,
		)
	}
}

// assertNodeListsEqual compares lists of nodes from both implementations.
func assertNodeListsEqual(
	t *testing.T,
	goNodes []hive.NodeID,
	hivexNodes []bindings.NodeHandle,
	msgAndArgs ...interface{},
) {
	t.Helper()

	if len(goNodes) != len(hivexNodes) {
		msg := "Node list lengths don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf("%s: gohivex=%d nodes, hivex=%d nodes", msg, len(goNodes), len(hivexNodes))
		return
	}

	for i := range goNodes {
		assertSameNodeID(t, goNodes[i], hivexNodes[i], "Node at index %d", i)
	}
}

// assertValueListsEqual compares lists of values from both implementations.
func assertValueListsEqual(
	t *testing.T,
	goVals []hive.ValueID,
	hivexVals []bindings.ValueHandle,
	msgAndArgs ...interface{},
) {
	t.Helper()

	if len(goVals) != len(hivexVals) {
		msg := "Value list lengths don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf("%s: gohivex=%d values, hivex=%d values", msg, len(goVals), len(hivexVals))
		return
	}

	for i := range goVals {
		assertSameValueID(t, goVals[i], hivexVals[i], "Value at index %d", i)
	}
}

// assertStringsEqual compares strings from both implementations
// Handles any encoding differences that might occur.
func assertStringsEqual(t *testing.T, goStr, hivexStr string, msgAndArgs ...interface{}) {
	t.Helper()

	if goStr != hivexStr {
		msg := "Strings don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf("%s:\n  gohivex: %q\n  hivex:   %q", msg, goStr, hivexStr)
	}
}

// assertBytesEqual compares byte slices from both implementations.
func assertBytesEqual(t *testing.T, goBytes, hivexBytes []byte, msgAndArgs ...interface{}) {
	t.Helper()

	if len(goBytes) != len(hivexBytes) {
		msg := "Byte slice lengths don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf("%s: gohivex=%d bytes, hivex=%d bytes", msg, len(goBytes), len(hivexBytes))
		return
	}

	for i := range goBytes {
		if goBytes[i] != hivexBytes[i] {
			msg := "Bytes differ"
			if len(msgAndArgs) > 0 {
				format, ok := msgAndArgs[0].(string)
				if !ok {
					t.Errorf("invalid format string type: %T", msgAndArgs[0])
					return
				}
				msg = fmt.Sprintf(format, msgAndArgs[1:]...)
			}
			t.Errorf("%s at index %d: gohivex=0x%02x, hivex=0x%02x", msg, i, goBytes[i], hivexBytes[i])
			if i >= 10 {
				t.Error("(stopping after 10 differences)")
				return
			}
		}
	}
}

// assertIntEqual compares int values from both implementations.
func assertIntEqual(t *testing.T, goVal, hivexVal int, msgAndArgs ...interface{}) {
	t.Helper()

	if goVal != hivexVal {
		msg := "Int values don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf("%s: gohivex=%d, hivex=%d", msg, goVal, hivexVal)
	}
}

// assertRegTypeEqual compares registry value types.
func assertRegTypeEqual(t *testing.T, goType hive.RegType, hivexType bindings.ValueType, msgAndArgs ...interface{}) {
	t.Helper()

	if int32(goType) != int32(hivexType) {
		msg := "Registry types don't match"
		if len(msgAndArgs) > 0 {
			format, ok := msgAndArgs[0].(string)
			if !ok {
				t.Errorf("invalid format string type: %T", msgAndArgs[0])
				return
			}
			msg = fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		t.Errorf("%s: gohivex=%s(%d), hivex=%s(%d)", msg, goType, goType, hivexType, hivexType)
	}
}
