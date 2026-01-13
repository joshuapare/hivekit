//go:build hivex

package reader

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestDB_BlockInspection inspects the db record blocks for BootPlan
func TestDB_BlockInspection(t *testing.T) {
	data, err := os.ReadFile("../../testdata/suite/windows-xp-system")
	if err != nil {
		t.Skip(err)
	}

	r, err := OpenBytes(data, types.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Navigate to BootPlan
	root, _ := r.Root()
	node := root
	for _, name := range []string{"ControlSet001", "Services", "RdyBoost", "Parameters"} {
		node, _ = r.Lookup(node, name)
	}

	values, _ := r.Values(node)
	var bootplanID types.ValueID
	for _, val := range values {
		meta, _ := r.StatValue(val)
		if meta.Name == "BootPlan" {
			bootplanID = val
			break
		}
	}

	// Cast to internal reader to access internals
	rdr := r.(*reader)

	// Get the VK record
	vkCell, _ := rdr.cell(uint32(bootplanID))
	vk, _ := format.DecodeVK(vkCell.Data)

	t.Logf("VK DataOffset: 0x%x, DataLength: 0x%x (inline=%v)", vk.DataOffset, vk.DataLength, vk.DataInline())

	// Get the db record
	dbCell, _ := rdr.cell(vk.DataOffset)
	t.Logf(
		"DB Cell: offset=0x%x, size=%d, data[:16]=%x",
		vk.DataOffset,
		dbCell.Size,
		dbCell.Data[:min(16, len(dbCell.Data))],
	)

	db, _ := format.DecodeDB(dbCell.Data)
	t.Logf("DB Record: NumBlocks=%d, BlocklistOffset=0x%x", db.NumBlocks, db.BlocklistOffset)

	// Get the blocklist
	blocklistCell, _ := rdr.cell(db.BlocklistOffset)
	t.Logf(
		"Blocklist Cell: size=%d, first 32 bytes=%x",
		blocklistCell.Size,
		blocklistCell.Data[:min(32, len(blocklistCell.Data))],
	)

	// Read block offsets
	t.Logf("\n=== BLOCK OFFSETS AND SIZES ===")
	bytesRead := 0
	for i := uint16(0); i < db.NumBlocks; i++ {
		offset := int(i) * 4
		blockOffset := binary.LittleEndian.Uint32(blocklistCell.Data[offset : offset+4])

		blockCell, err := rdr.cell(blockOffset)
		if err != nil {
			t.Logf("Block %d: offset=0x%x ERROR: %v", i, blockOffset, err)
			continue
		}

		t.Logf("Block %d: offset=0x%x, cell.Size=%d, len(Data)=%d, bytesRead so far=%d",
			i, blockOffset, blockCell.Size, len(blockCell.Data), bytesRead)

		if i < 5 || i == db.NumBlocks-1 {
			t.Logf("  First 16 bytes: %x", blockCell.Data[:min(16, len(blockCell.Data))])
			if len(blockCell.Data) > 16 {
				t.Logf("  Last 16 bytes:  %x", blockCell.Data[max(0, len(blockCell.Data)-16):])
			}
		}

		// Check boundary near where the error occurs (block 4 at ~16KB)
		if bytesRead <= 16344 && bytesRead+len(blockCell.Data) > 16344 {
			t.Logf("  *** This block contains byte 16344! ***")
			localOffset := 16344 - bytesRead
			t.Logf("  Byte 16344 is at local offset %d in this block", localOffset)
			if localOffset > 16 && localOffset+16 < len(blockCell.Data) {
				t.Logf("  Around byte 16344: %x", blockCell.Data[localOffset-16:localOffset+16])
			}
		}

		bytesRead += len(blockCell.Data)
	}

	t.Logf("\nTotal bytes from blocks: %d (expected: 25544)", bytesRead)
}
