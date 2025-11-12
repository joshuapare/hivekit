//go:build hivex

package reader

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestREGNONE_HivexComparison compares what hivex bindings return vs what we see in raw VK
func TestREGNONE_HivexComparison(t *testing.T) {
	hivePath := "../../testdata/rlenvalue_test_hive"
	if _, err := os.Stat(hivePath); os.IsNotExist(err) {
		t.Skipf("Hive not found: %s", hivePath)
	}

	// Open with hivex bindings
	hx, err := bindings.Open(hivePath, 0)
	if err != nil {
		t.Fatalf("Failed to open with hivex: %v", err)
	}
	defer hx.Close()

	root := hx.Root()
	children := hx.NodeChildren(root)

	var mvpNode bindings.NodeHandle
	for _, child := range children {
		name := hx.NodeName(child)
		if name == "ModerateValueParent" {
			mvpNode = child
			break
		}
	}

	if mvpNode == 0 {
		t.Fatal("ModerateValueParent not found")
	}

	// Get values from hivex
	values := hx.NodeValues(mvpNode)
	t.Logf("Hivex found %d values", len(values))

	// Also open with our reader
	data, _ := os.ReadFile(hivePath)
	r, _ := OpenBytes(data, types.OpenOptions{})
	defer r.Close()

	ourRoot, _ := r.Root()
	ourMVP, _ := r.Lookup(ourRoot, "ModerateValueParent")
	ourValues, _ := r.Values(ourMVP)

	// Compare
	for _, hxVal := range values {
		hxName := hx.ValueKey(hxVal)
		hxType, hxLen, _ := hx.ValueType(hxVal)
		hxData, _, _ := hx.ValueValue(hxVal)

		// Find matching value in our reader
		var ourValID types.ValueID
		for _, ourVal := range ourValues {
			meta, _ := r.StatValue(ourVal)
			if meta.Name == hxName {
				ourValID = ourVal
				break
			}
		}

		if ourValID == 0 {
			t.Logf("Value %s: not found in our reader", hxName)
			continue
		}

		// Get our data
		ourMeta, _ := r.StatValue(ourValID)
		ourData, _ := r.ValueBytes(ourValID, types.ReadOptions{})

		t.Logf("\nValue: %s", hxName)
		t.Logf("  Hivex: type=%d (%s), len=%d, data=%d bytes",
			hxType, hxType.String(), hxLen, len(hxData))
		t.Logf("  Ours:  type=%d (%s), len=%d, data=%d bytes",
			ourMeta.Type, ourMeta.Type.String(), ourMeta.Size, len(ourData))

		// Now examine the raw VK record
		rdr := r.(*reader)
		offset := uint32(ourValID)
		abs := int(format.HeaderSize) + int(offset)

		cellSizeRaw := int32(rdr.buf[abs]) | int32(rdr.buf[abs+1])<<8 |
			int32(rdr.buf[abs+2])<<16 | int32(rdr.buf[abs+3])<<24
		cellSize := -cellSizeRaw

		vk, _ := format.DecodeVK(rdr.buf[abs+4 : abs+4+int(cellSize)])
		t.Logf("  Raw VK: Type=%d, DataLength=0x%08x, Flags=0x%04x",
			vk.Type, vk.DataLength, vk.Flags)

		if hxType != bindings.ValueType(ourMeta.Type) {
			t.Logf("  *** TYPE MISMATCH ***")
		}
		if len(hxData) != len(ourData) {
			t.Logf("  *** DATA LENGTH MISMATCH ***")
		}
	}
}
