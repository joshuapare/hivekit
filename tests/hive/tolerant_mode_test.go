package hive_test

import (
	"encoding/binary"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func TestTolerantModeAllowsTruncatedValue(t *testing.T) {
	buf, _ := synthHive()
	// Corrupt the DWORD payload to simulate truncation (length says 4, data smaller).
	// Locate data cell at offset 0x180 and shrink it by 2 bytes.
	dataCell := regfHeaderSize + 0x180
	// overwrite cell header with smaller payload (only two bytes instead of four)
	truncated := int32(-0x06)
	binary.LittleEndian.PutUint32(buf[dataCell:], uint32(truncated))
	buf[dataCell+4] = 0x78
	buf[dataCell+5] = 0x56
	for i := 6; i < 0x10; i++ {
		buf[dataCell+4+i] = 0
	}

	strict, err := reader.OpenBytes(buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes strict: %v", err)
	}
	defer strict.Close()
	root, _ := strict.Root()
	vals, _ := strict.Values(root)
	if len(vals) == 0 {
		t.Fatalf("expected values present")
	}
	if _, readErr := strict.ValueBytes(vals[0], hive.ReadOptions{}); readErr == nil {
		t.Fatalf("expected ValueBytes to fail without tolerant mode")
	}

	tolerant, err := reader.OpenBytes(buf, hive.OpenOptions{Tolerant: true})
	if err != nil {
		t.Fatalf("OpenBytes tolerant: %v", err)
	}
	defer tolerant.Close()
	root, _ = tolerant.Root()
	vals, _ = tolerant.Values(root)
	if len(vals) == 0 {
		t.Fatalf("expected values even in tolerant mode")
	}
	if _, dwordErr := tolerant.ValueDWORD(vals[0]); dwordErr == nil {
		t.Fatalf("expected ValueDWORD to fail due to short data")
	}
}
