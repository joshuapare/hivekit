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

// TestLargeHiveOffsets verifies offset calculations in the large hive
func TestLargeHiveOffsets(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Skipf("Large hive not available: %v", err)
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
	if err := tx.Commit(w, hive.WriteOptions{}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	buf := w.Buf
	t.Logf("Buffer size: %d bytes (0x%x)", len(buf), len(buf))

	numHBINs := (len(buf) - 0x1000) / 0x1000
	t.Logf("Number of HBINs: %d", numHBINs)

	// Read REGF header
	rootOff := binary.LittleEndian.Uint32(buf[0x24:0x28])
	t.Logf("Root offset from REGF header: 0x%08x", rootOff)

	// Verify root NK
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
		t.Fatalf("Expected NK signature at root, got %q", sig)
	}

	t.Logf("✓ Root NK cell verified")

	// Now check the first subkey list
	// The NK cell has subkey list offset at +0x1C (after cell header)
	nkPayload := buf[fileOff+4:]
	subkeyListOff := binary.LittleEndian.Uint32(nkPayload[0x1C:0x20])
	t.Logf("Subkey list offset: 0x%08x", subkeyListOff)

	if subkeyListOff == 0xFFFFFFFF {
		t.Logf("No subkeys")
		return
	}

	// Check subkey list
	subkeyListFileOff := 0x1000 + int(subkeyListOff)
	t.Logf("Subkey list at file offset: 0x%08x", subkeyListFileOff)

	if subkeyListFileOff >= len(buf) {
		t.Fatalf("Subkey list offset beyond buffer: 0x%x, buffer size: 0x%x", subkeyListFileOff, len(buf))
	}

	// Read subkey list cell size
	listCellSize := int32(binary.LittleEndian.Uint32(buf[subkeyListFileOff : subkeyListFileOff+4]))
	t.Logf("Subkey list cell size: %d", listCellSize)

	if listCellSize >= 0 {
		t.Fatalf("Subkey list cell size should be negative (allocated), got %d", listCellSize)
	}

	// Read subkey list signature
	listSig := string(buf[subkeyListFileOff+4 : subkeyListFileOff+6])
	t.Logf("Subkey list signature: %q", listSig)

	if listSig != "lf" && listSig != "lh" && listSig != "ri" && listSig != "li" {
		t.Fatalf("Expected subkey list signature (lf/lh/ri/li), got %q", listSig)
	}

	t.Logf("✓ Subkey list cell verified")

	// Read first child offset from the list
	listPayload := buf[subkeyListFileOff+4:]
	listCount := binary.LittleEndian.Uint16(listPayload[2:4])
	t.Logf("Subkey list count: %d", listCount)

	if listCount > 0 {
		firstChildOff := binary.LittleEndian.Uint32(listPayload[4:8])
		t.Logf("First child offset: 0x%08x", firstChildOff)

		// Verify first child NK
		firstChildFileOff := 0x1000 + int(firstChildOff)
		t.Logf("First child NK at file offset: 0x%08x", firstChildFileOff)

		if firstChildFileOff >= len(buf) {
			t.Fatalf("First child offset beyond buffer: 0x%x, buffer size: 0x%x", firstChildFileOff, len(buf))
		}

		// Read first child cell size
		childCellSize := int32(binary.LittleEndian.Uint32(buf[firstChildFileOff : firstChildFileOff+4]))
		t.Logf("First child NK cell size: %d", childCellSize)

		if childCellSize >= 0 {
			t.Fatalf("First child cell size should be negative (allocated), got %d", childCellSize)
		}

		// Read first child signature
		childSig := string(buf[firstChildFileOff+4 : firstChildFileOff+6])
		t.Logf("First child NK signature: %q", childSig)

		if childSig != "nk" {
			t.Fatalf("Expected NK signature for first child, got %q", childSig)
		}

		t.Logf("✓ First child NK cell verified")
	}
}
