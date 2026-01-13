//go:build linux || darwin

package edit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// Test_DataCell_CorruptionAfterGrow reproduces the critical bug where
// writing data after Grow() invalidates buffer pointers, causing
// test data to overwrite cell size fields.
//
// Bug manifestation:
// - Large value (30KB) triggers DB format + Grow()
// - Size field at allocated cell contains test data instead of valid size
// - hivexsh reports: "the block at 0xXXX size NNNNN extends beyond current page"
//
// This test is expected to FAIL until the bug is fixed.
func Test_DataCell_CorruptionAfterGrow(t *testing.T) {
	// Use a real hive (setupRealHive copies from testdata)
	h, allocator, idx, tempHivePath, cleanup := setupRealHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	keyEditor := NewKeyEditor(h, allocator, idx, dt)
	valueEditor := NewValueEditor(h, allocator, idx, dt)

	// Create a test key
	rootRef := h.RootCellOffset()
	keyRef, _, err := keyEditor.EnsureKeyPath(rootRef, []string{"_CorruptionTest"})
	require.NoError(t, err)

	// Write a large value (30KB) - this should trigger Grow() internally
	largeData := make([]byte, 30*1024) // 30KB
	for i := range largeData {
		largeData[i] = byte(i % 256) // Pattern: 00 01 02 03 ... FF 00 01 ...
	}

	err = valueEditor.UpsertValue(keyRef, "LargeTestValue", format.REGBinary, largeData)
	require.NoError(t, err)

	// CRITICAL CHECK: Verify no corruption in memory
	// The bug causes test data pattern to overwrite cell size fields
	memData := h.Bytes()

	// Scan for cells that were allocated in newly grown HBINs
	// Look for cells with size fields that contain the test data pattern
	corruptionFound := false
	var corruptionDetails string

	// Walk through file looking for our data pattern in size fields
	for offset := 0x1000; offset < len(memData)-4; offset += 8 {
		if offset+4 > len(memData) {
			break
		}

		// Read what should be a cell size field
		potentialSize := int32(format.ReadU32(memData, offset))

		// Check if this "size" actually contains our test data pattern
		// Pattern bytes: 0x00 0x01 0x02 0x03 = 0x03020100 as little-endian uint32
		// Or: 0x48 0xA2 0x69 0x6E (which we saw in debugging)
		if potentialSize == 0x03020100 || potentialSize == 0x6E69A248 {
			corruptionFound = true
			corruptionDetails = "Found test data pattern in cell size field at offset 0x%X: 0x%08X"
			t.Errorf(corruptionDetails, offset, uint32(potentialSize))

			// Log surrounding bytes for debugging
			if offset >= 16 && offset+32 < len(memData) {
				t.Logf("Context at 0x%X:", offset-16)
				for i := -16; i < 32; i += 16 {
					line := ""
					var lineSb78 strings.Builder
					for j := 0; j < 16 && offset+i+j < len(memData); j++ {
						lineSb78.WriteString(" %02X")
					}
					line += lineSb78.String()
					t.Logf(line, memData[offset+i:offset+i+16])
				}
			}
		}
	}

	if corruptionFound {
		t.Errorf("Cell size field corruption detected! Test data overwrote size fields.")
	}

	h.Close()

	// Validate with hivexsh to ensure the hive is parseable
	if !hivexval.IsHivexshAvailable() {
		t.Log("hivexsh not available, skipping external validation")
		return
	}

	v := hivexval.Must(hivexval.New(tempHivePath, &hivexval.Options{UseHivexsh: true}))
	defer v.Close()

	v.AssertHivexshValid(t)
	t.Log("hivexsh successfully parsed the hive after large data write!")
	t.Log("   This confirms no cell corruption occurred.")
}
