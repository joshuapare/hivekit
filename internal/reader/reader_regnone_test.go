package reader

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestREGNONE_Investigation investigates the REG_NONE compatibility issue.
// Hivex reports some values as REG_NONE with 0 bytes, while we report actual type and data.
func TestREGNONE_Investigation(t *testing.T) {
	hivePath := "../../testdata/rlenvalue_test_hive"
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skipf("Hive not found: %s", hivePath)
	}

	data, err := os.ReadFile(hivePath)
	if err != nil {
		t.Fatalf("Failed to read hive: %v", err)
	}

	r, err := OpenBytes(data, types.OpenOptions{})
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer r.Close()

	root, _ := r.Root()

	// Find ModerateValueParent key using Lookup
	mvp, err := r.Lookup(root, "ModerateValueParent")
	if err != nil {
		t.Fatalf("Failed to find ModerateValueParent: %v", err)
	}

	values, _ := r.Values(mvp)
	t.Logf("Found %d values in ModerateValueParent", len(values))

	for _, valID := range values {
		meta, _ := r.StatValue(valID)
		valueData, readErr := r.ValueBytes(valID, types.ReadOptions{})
		if readErr != nil {
			t.Logf("  %s: ERROR: %v", meta.Name, readErr)
		} else {
			t.Logf("  %s: type=%d, size=%d bytes, data=%x...",
				meta.Name, meta.Type, len(valueData), valueData[:min(8, len(valueData))])
		}
	}

	// Now let's examine the raw VK records to see if there's a flag pattern
	t.Logf("\n--- Raw VK Record Analysis ---")
	rdr := r.(*reader)
	for _, valID := range values {
		offset := uint32(valID)
		abs := int(0x1000) + int(offset)

		// Read VK record
		cellSizeRaw := int32(rdr.buf[abs]) | int32(rdr.buf[abs+1])<<8 |
			int32(rdr.buf[abs+2])<<16 | int32(rdr.buf[abs+3])<<24
		cellSize := -cellSizeRaw

		vk, _ := format.DecodeVK(rdr.buf[abs+4 : abs+4+int(cellSize)])

		t.Logf("  VK at 0x%x:", offset)
		t.Logf("    Type: %d", vk.Type)
		t.Logf("    DataLength: 0x%08x (%d bytes, inline=%v)",
			vk.DataLength, vk.DataLength&0x7fffffff, vk.DataInline())
		t.Logf("    Flags: 0x%04x", vk.Flags)
		t.Logf("    Name: %s", string(vk.NameRaw))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
