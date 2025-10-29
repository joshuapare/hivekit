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

// TestDebugLargeHiveOffsets inspects the offsets in the rebuilt large hive
func TestDebugLargeHiveOffsets(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Skipf("Large hive not available: %v", err)
	}

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

	// Inspect the rebuilt hive binary structure
	buf := w.Buf

	// Count HBINs
	numHBINs := (len(buf) - 0x1000) / 0x1000
	t.Logf("Total buffer size: %d bytes (0x%x)", len(buf), len(buf))
	t.Logf("Number of HBINs: %d", numHBINs)
	t.Logf("Expected cell buffer overhead from HBIN headers: %d bytes (0x%x)", numHBINs*0x20, numHBINs*0x20)

	// Read REGF header
	if len(buf) < 0x1000+0x20 {
		t.Fatalf("Buffer too small: %d bytes", len(buf))
	}

	// Root offset from header
	rootOff := binary.LittleEndian.Uint32(buf[0x24:0x28])
	t.Logf("Root offset from header: 0x%08x", rootOff)

	// Calculate file offset of root NK cell
	rootFileOff := 0x1000 + int(rootOff)
	t.Logf("Root NK at file offset: 0x%08x", rootFileOff)

	if rootFileOff+0x100 > len(buf) {
		t.Fatalf("Root offset beyond buffer: 0x%x, buffer size: 0x%x", rootFileOff, len(buf))
	}

	// Read root NK cell
	cellSize := int32(binary.LittleEndian.Uint32(buf[rootFileOff : rootFileOff+4]))
	t.Logf("Root NK cell size: %d", cellSize)

	// NK payload starts after cell header
	nkPayload := buf[rootFileOff+4:]

	// Verify NK signature
	sig := string(nkPayload[0:2])
	if sig != "nk" {
		t.Fatalf("Invalid NK signature: %q", sig)
	}

	// Read fields from NK structure
	flags := binary.LittleEndian.Uint16(nkPayload[0x02:0x04])
	subkeyCount := binary.LittleEndian.Uint32(nkPayload[0x14:0x18])
	subkeyListOff := binary.LittleEndian.Uint32(nkPayload[0x1C:0x20])
	valueCount := binary.LittleEndian.Uint32(nkPayload[0x24:0x28])
	valueListOff := binary.LittleEndian.Uint32(nkPayload[0x28:0x2C])

	t.Logf("NK fields:")
	t.Logf("  Flags: 0x%04x", flags)
	t.Logf("  Subkey count: %d", subkeyCount)
	t.Logf("  Subkey list offset: 0x%08x", subkeyListOff)
	t.Logf("  Value count: %d", valueCount)
	t.Logf("  Value list offset: 0x%08x", valueListOff)

	if subkeyCount > 0 && subkeyListOff != 0xFFFFFFFF {
		// Calculate file offset of subkey list
		subkeyListFileOff := 0x1000 + int(subkeyListOff)
		t.Logf("Subkey list at file offset: 0x%08x", subkeyListFileOff)

		if subkeyListFileOff >= len(buf) {
			t.Fatalf("Subkey list offset beyond buffer: 0x%x, buffer size: 0x%x", subkeyListFileOff, len(buf))
		}

		// Read subkey list cell
		listCellSize := int32(binary.LittleEndian.Uint32(buf[subkeyListFileOff : subkeyListFileOff+4]))
		t.Logf("Subkey list cell size: %d", listCellSize)

		if listCellSize == 0 {
			t.Fatalf("ERROR: Subkey list cell has ZERO size! This is the bug.")
		}

		if listCellSize > 0 {
			t.Fatalf("ERROR: Subkey list cell has positive size %d (should be negative for allocated cell)", listCellSize)
		}

		listPayload := buf[subkeyListFileOff+4:]
		listSig := string(listPayload[0:2])
		listCount := binary.LittleEndian.Uint16(listPayload[2:4])

		t.Logf("Subkey list:")
		t.Logf("  Signature: %q", listSig)
		t.Logf("  Count: %d", listCount)
	}
}
