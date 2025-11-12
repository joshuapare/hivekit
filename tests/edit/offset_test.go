package edit_test

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestMinimalHiveOffsets verifies offset calculations in the minimal hive.
func TestMinimalHiveOffsets(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Read original
	r1, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes original: %v", err)
	}
	defer r1.Close()

	// Rebuild
	ed := edit.NewEditor(r1)
	tx := ed.Begin()

	w := &writer.MemWriter{}
	if commitErr := tx.Commit(w, hive.WriteOptions{}); commitErr != nil {
		t.Fatalf("Commit: %v", commitErr)
	}

	buf := w.Buf
	t.Logf("Buffer size: %d bytes (0x%x)", len(buf), len(buf))

	// Read REGF header
	rootOff := binary.LittleEndian.Uint32(buf[0x24:0x28])
	t.Logf("Root offset from REGF header: 0x%08x", rootOff)

	// First HBIN starts at 0x1000
	// HBIN header is 0x20 bytes
	// So first cell should be at file offset 0x1020
	// And HBIN-relative offset should be 0x20

	if rootOff != 0x20 {
		t.Errorf("Expected root offset 0x20, got 0x%08x", rootOff)
	}

	// Verify we can read at that offset
	fileOff := 0x1000 + int(rootOff)
	t.Logf("Root NK at file offset: 0x%08x", fileOff)

	if fileOff >= len(buf) {
		t.Fatalf("Root offset beyond buffer: 0x%x, buffer size: 0x%x", fileOff, len(buf))
	}

	// Read cell size
	cellSize := int32(binary.LittleEndian.Uint32(buf[fileOff : fileOff+4]))
	t.Logf("Root NK cell size: %d", cellSize)

	if cellSize >= 0 {
		t.Fatalf("Cell size should be negative (allocated), got %d", cellSize)
	}

	// Read NK signature
	sig := string(buf[fileOff+4 : fileOff+6])
	if sig != "nk" {
		t.Fatalf("Expected NK signature, got %q", sig)
	}

	t.Logf("✓ Root NK cell verified")
}
