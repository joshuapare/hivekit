package reader

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestLargeValue_BootPlan tests reading the BootPlan value from windows-xp-system
// which is ~25KB and currently fails with "value data truncated" error.
//
// This test investigates whether the issue is:
// 1. Cell size mismatch
// 2. Multi-cell data (db records)
// 3. Some other format issue
func TestLargeValue_BootPlan(t *testing.T) {
	hivePath := "../../testdata/suite/windows-xp-system"
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

	// Navigate to BootPlan value
	// Path: \controlset001\services\rdyboost\parameters\BootPlan
	root, _ := r.Root()

	// Find controlset001
	children, _ := r.Subkeys(root)
	var cs001 types.NodeID
	for _, child := range children {
		meta, _ := r.StatKey(child)
		if meta.Name == "ControlSet001" {
			cs001 = child
			break
		}
	}
	if cs001 == 0 {
		t.Fatal("ControlSet001 not found")
	}

	// Find services
	children, _ = r.Subkeys(cs001)
	var services types.NodeID
	for _, child := range children {
		meta, _ := r.StatKey(child)
		if meta.Name == "Services" {
			services = child
			break
		}
	}
	if services == 0 {
		t.Fatal("Services not found")
	}

	// Find rdyboost
	children, _ = r.Subkeys(services)
	var rdyboost types.NodeID
	for _, child := range children {
		meta, _ := r.StatKey(child)
		if meta.Name == "rdyboost" {
			rdyboost = child
			break
		}
	}
	if rdyboost == 0 {
		t.Fatal("rdyboost not found")
	}

	// Find Parameters
	children, _ = r.Subkeys(rdyboost)
	var params types.NodeID
	for _, child := range children {
		meta, _ := r.StatKey(child)
		if meta.Name == "Parameters" {
			params = child
			break
		}
	}
	if params == 0 {
		t.Fatal("Parameters not found")
	}

	// Find BootPlan value
	values, _ := r.Values(params)
	var bootplanID types.ValueID
	for _, valID := range values {
		meta, _ := r.StatValue(valID)
		if meta.Name == "BootPlan" {
			bootplanID = valID
			break
		}
	}
	if bootplanID == 0 {
		t.Fatal("BootPlan value not found")
	}

	t.Logf("Found BootPlan value ID: %d (0x%x)", bootplanID, bootplanID)

	// Get metadata
	meta, err := r.StatValue(bootplanID)
	if err != nil {
		t.Fatalf("StatValue failed: %v", err)
	}
	t.Logf("BootPlan metadata: Type=%d, Size=%d", meta.Type, meta.Size)

	// Now examine the VK record directly
	rdr := r.(*reader)
	offset := uint32(bootplanID)
	abs := int(format.HeaderSize) + int(offset)

	// Read cell size
	cellSizeRaw := int32(rdr.buf[abs]) | int32(rdr.buf[abs+1])<<8 |
		int32(rdr.buf[abs+2])<<16 | int32(rdr.buf[abs+3])<<24
	cellSize := -cellSizeRaw // negative = allocated
	t.Logf("VK cell size: %d bytes", cellSize)

	// Decode VK record
	vk, err := format.DecodeVK(rdr.buf[abs+4 : abs+4+int(cellSize)])
	if err != nil {
		t.Fatalf("DecodeVK failed: %v", err)
	}

	dataLen := int(vk.DataLength & 0x7FFFFFFF)
	t.Logf("VK.DataLength: 0x%08x (%d bytes)", vk.DataLength, dataLen)
	t.Logf("VK.DataOffset: 0x%08x", vk.DataOffset)
	t.Logf("VK.Type: %d", vk.Type)
	t.Logf("VK.Flags: 0x%04x", vk.Flags)
	t.Logf("VK.DataInline: %v", vk.DataInline())

	if !vk.DataInline() {
		// Check the data cell
		dataAbs := int(format.HeaderSize) + int(vk.DataOffset)
		dataCellSizeRaw := int32(rdr.buf[dataAbs]) | int32(rdr.buf[dataAbs+1])<<8 |
			int32(rdr.buf[dataAbs+2])<<16 | int32(rdr.buf[dataAbs+3])<<24
		dataCellSize := -dataCellSizeRaw
		dataPayloadSize := dataCellSize - 4

		t.Logf("\nData cell info:")
		t.Logf("  Data cell offset: 0x%x", dataAbs)
		t.Logf("  Data cell size: %d bytes", dataCellSize)
		t.Logf("  Data payload size: %d bytes (cell size - 4)", dataPayloadSize)

		t.Logf("\nComparison:")
		t.Logf("  VK says data length: %d bytes", dataLen)
		t.Logf("  Cell payload size:   %d bytes", dataPayloadSize)
		t.Logf("  Difference: %d bytes", dataLen-int(dataPayloadSize))

		// Check cell signature
		sig := rdr.buf[dataAbs+4 : dataAbs+6]
		t.Logf("\nData cell signature: %q (0x%02x 0x%02x)", sig, sig[0], sig[1])

		if string(sig) == "db" {
			t.Logf("  *** This is a 'db' record (multi-cell large data) ***")
			t.Logf("  *** Need to implement db record support ***")
		}

		// For db records, it's expected that VK DataLength > cell payload
		// The cell just contains the db record metadata pointing to the actual data blocks
		if dataLen > int(dataPayloadSize) && string(sig) == "db" {
			t.Logf("✓ DB record detected: VK DataLength (%d) > cell payload (%d) as expected",
				dataLen, dataPayloadSize)
		}

		// Decode the db record to see block info
		if string(sig) == "db" {
			dbRec, err := format.DecodeDB(rdr.buf[dataAbs+4 : dataAbs+4+int(dataCellSize)])
			if err != nil {
				t.Logf("DecodeDB error: %v", err)
			} else {
				t.Logf("\nDB Record info:")
				t.Logf("  Number of blocks: %d", dbRec.NumBlocks)
				t.Logf("  Blocklist offset: 0x%x", dbRec.BlocklistOffset)
				t.Logf("  Unknown1: 0x%x", dbRec.Unknown1)
			}
		}
	}

	// Try to read the value
	data, err = r.ValueBytes(bootplanID, types.ReadOptions{})
	if err != nil {
		t.Logf("\nValueBytes error: %v", err)
		t.FailNow()
	} else {
		t.Logf("\n✅ SUCCESS: ValueBytes read %d bytes!", len(data))

		// Verify it matches expected length
		if len(data) != 25544 {
			t.Errorf("Data length mismatch: expected 25544, got %d", len(data))
		}

		// Verify data starts with "MEM0" signature (as seen in hivex output)
		if len(data) >= 4 {
			sig := string(data[0:4])
			if sig != "MEM0" {
				t.Errorf("Data signature mismatch: expected \"MEM0\", got %q", sig)
			} else {
				t.Logf("✓ Data signature verified: \"MEM0\"")
			}
		}
	}
}
