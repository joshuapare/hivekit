package write

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/builder"
	"github.com/joshuapare/hivekit/internal/format"
)

// TestQueueCellFree_RI verifies that queueCellFree detects RI cells and frees
// both the leaf cells referenced by the RI header AND the header itself.
// Before the fix, only the RI header was freed, leaking leaf cells.
func TestQueueCellFree_RI(t *testing.T) {
	// Create a minimal hive using the builder.
	tmpDir := t.TempDir()
	hivePath := filepath.Join(tmpDir, "ri-test.hive")
	b, err := builder.New(hivePath, nil)
	if err != nil {
		t.Fatalf("builder.New: %v", err)
	}
	if err := b.Commit(); err != nil {
		t.Fatalf("builder.Commit: %v", err)
	}
	b.Close()

	// Reopen to get a *hive.Hive with writable bytes.
	h, err := hive.Open(hivePath)
	if err != nil {
		t.Fatalf("hive.Open: %v", err)
	}
	defer h.Close()

	data := h.Bytes()

	// We'll place synthetic cells within the hive data.
	// The hive has HeaderSize (4096) + one HBIN (4096) = 8192 bytes.
	// The HBIN header is 0x20 (32) bytes, then cells start.
	// We'll grow the file to have enough space and place our cells there.

	// We need to extend the hive to make room for our synthetic cells.
	// Append a new 4KB HBIN that we'll use for test cells.
	if err := h.Append(4096); err != nil {
		t.Fatalf("Append: %v", err)
	}
	data = h.Bytes() // re-read after append

	// Place cells in the second HBIN area (starting at offset 8192).
	// We need to write a minimal HBIN header first.
	hbin2Start := 8192
	copy(data[hbin2Start:], []byte("hbin"))
	binary.LittleEndian.PutUint32(data[hbin2Start+4:], uint32(hbin2Start-int(format.HeaderSize))) // offset from data start
	binary.LittleEndian.PutUint32(data[hbin2Start+8:], 4096)                                      // HBIN size

	// Cell positions (absolute file offsets)
	leaf0Abs := hbin2Start + 0x20 // after HBIN header
	leaf1Abs := leaf0Abs + 32
	riAbs := leaf1Abs + 32

	// Relative offsets (subtract HiveDataBase to get cell-relative offset)
	leaf0Rel := uint32(leaf0Abs - int(format.HeaderSize))
	leaf1Rel := uint32(leaf1Abs - int(format.HeaderSize))
	riRel := uint32(riAbs - int(format.HeaderSize))

	// Helper to write a negative int32 as the cell size header (allocated cell).
	putCellSize := func(absOff int, size int32) {
		binary.LittleEndian.PutUint32(data[absOff:], uint32(size))
	}

	// Write leaf cell 0: size = -32 (allocated), payload starts with "lh"
	putCellSize(leaf0Abs, -32)
	data[leaf0Abs+4] = 'l'
	data[leaf0Abs+5] = 'h'

	// Write leaf cell 1: size = -32 (allocated), payload starts with "lh"
	putCellSize(leaf1Abs, -32)
	data[leaf1Abs+4] = 'l'
	data[leaf1Abs+5] = 'h'

	// Write RI header cell: size = -20 (allocated)
	// Payload: "ri" (2 bytes) + count (uint16 = 2) + leaf0Rel (uint32) + leaf1Rel (uint32) = 12 bytes
	// Total cell = 4 (header) + 12 (payload) = 16, round to 20 for alignment
	putCellSize(riAbs, -20)
	data[riAbs+4] = 'r'
	data[riAbs+5] = 'i'
	binary.LittleEndian.PutUint16(data[riAbs+6:], 2) // count = 2
	binary.LittleEndian.PutUint32(data[riAbs+8:], leaf0Rel)
	binary.LittleEndian.PutUint32(data[riAbs+12:], leaf1Rel)

	// Flush modified data to disk so hive sees it.
	f, err := os.OpenFile(hivePath, os.O_WRONLY, 0)
	if err == nil {
		f.Write(data)
		f.Close()
	}

	// Create an executor with the test hive.
	ex := &executor{
		h:       h,
		updates: make([]InPlaceUpdate, 0, 8),
	}

	// Call queueCellFree on the RI cell.
	ex.queueCellFree(riRel)

	// We expect 3 free updates: leaf0, leaf1, and the RI header itself.
	// Before the fix, only 1 update (RI header) was generated.
	freedOffsets := make(map[int32]bool)
	for _, u := range ex.updates {
		if u.Category == categoryCellFree {
			freedOffsets[u.Offset] = true
		}
	}

	wantOffsets := []int32{
		int32(leaf0Abs),
		int32(leaf1Abs),
		int32(riAbs),
	}

	for _, want := range wantOffsets {
		if !freedOffsets[want] {
			t.Errorf("expected free update at offset %d, not found in %d updates", want, len(ex.updates))
		}
	}

	if len(freedOffsets) != 3 {
		t.Errorf("expected 3 cell-free updates, got %d", len(freedOffsets))
		for _, u := range ex.updates {
			t.Logf("  update: offset=%d category=%d", u.Offset, u.Category)
		}
	}
}
