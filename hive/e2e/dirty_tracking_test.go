package e2e

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/edit"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_DirtyTracking_SimpleFlush tests basic dirty tracking and flushing.
func Test_DirtyTracking_SimpleFlush(t *testing.T) {
	// Setup test hive
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	if _, err := os.Stat(testHivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", testHivePath)
	}

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "dirty-test-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Fatalf("Failed to open source hive: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		dst.Close()
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create dirty tracker
	dt := dirty.NewTracker(h)

	// Create allocator
	allocator, err := alloc.NewFast(h, dt, nil)
	if err != nil {
		t.Fatalf("Failed to create allocator: %v", err)
	}

	// Create index
	idx := index.NewStringIndex(10000, 10000)

	// Create editors
	keyEditor := edit.NewKeyEditor(h, allocator, idx, dt)
	valueEditor := edit.NewValueEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	t.Log("=== STEP 1: Create a simple key ===")
	path := []string{"_DirtyTest"}
	keyRef, created, err := keyEditor.EnsureKeyPath(rootRef, path)
	if err != nil {
		t.Fatalf("EnsureKeyPath failed: %v", err)
	}
	t.Logf("Key created: ref=0x%X, created=%v", keyRef, created)

	t.Log("=== STEP 2: Add a simple value ===")
	data := []byte{0x01, 0x02, 0x03, 0x04}
	err = valueEditor.UpsertValue(keyRef, "TestValue", format.REGDWORD, data)
	if err != nil {
		t.Fatalf("UpsertValue failed: %v", err)
	}
	t.Log("Value created successfully")

	t.Log("=== STEP 3: Check dirty ranges ===")
	// Access debug ranges
	ranges := dt.DebugRanges()
	t.Logf("Total raw dirty ranges: %d", len(ranges))
	for i, r := range ranges {
		t.Logf("  Raw range %d: offset=0x%X, length=%d (end=0x%X)", i, r.Off, r.Len, r.Off+r.Len)
	}

	// Check coalesced ranges
	coalesced := dt.DebugCoalescedRanges()
	t.Logf("Total coalesced ranges: %d", len(coalesced))
	for i, r := range coalesced {
		t.Logf("  Coalesced range %d: offset=0x%X, length=%d (end=0x%X)", i, r.Off, r.Len, r.Off+r.Len)

		// Check if range is page-aligned
		pageSize := int64(4096)
		if r.Off%pageSize != 0 {
			t.Logf("    WARNING: offset not page-aligned (0x%X %% %d = %d)", r.Off, pageSize, r.Off%pageSize)
		}
		if r.Len%pageSize != 0 {
			t.Logf("    WARNING: length not page-aligned (%d %% %d = %d)", r.Len, pageSize, r.Len%pageSize)
		}

		// Check if range is within file bounds
		fileSize := h.Size()
		dataLen := int64(len(h.Bytes()))
		t.Logf("    File size: 0x%X, Data length: 0x%X", fileSize, dataLen)
		if r.Off >= dataLen {
			t.Errorf("    ERROR: offset 0x%X is beyond data length 0x%X", r.Off, dataLen)
		}
		if r.Off+r.Len > dataLen {
			t.Errorf("    ERROR: range end 0x%X is beyond data length 0x%X", r.Off+r.Len, dataLen)
		}
	}

	t.Log("=== STEP 4: Attempt to flush ===")
	err = dt.FlushDataOnly()
	if err != nil {
		t.Errorf("FlushDataOnly failed: %v", err)

		// Try to understand the error better
		t.Log("=== DEBUGGING FLUSH FAILURE ===")
		t.Logf("File path: %s", tempHivePath)
		t.Logf("File size: 0x%X (%d bytes)", h.Size(), h.Size())
		t.Logf("Hive bytes length: %d", len(h.Bytes()))

		// Check file info
		fi, statErr := os.Stat(tempHivePath)
		if statErr != nil {
			t.Logf("Failed to stat file: %v", statErr)
		} else {
			t.Logf("OS file size: %d bytes", fi.Size())
			t.Logf("File mode: %v", fi.Mode())
		}

		return
	}

	t.Log("Flush succeeded!")
}

// Test_DirtyTracking_ManualRange tests flushing a manually-added dirty range.
func Test_DirtyTracking_ManualRange(t *testing.T) {
	// Setup test hive
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	if _, err := os.Stat(testHivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", testHivePath)
	}

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "manual-dirty-test-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Fatalf("Failed to open source hive: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		dst.Close()
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create dirty tracker
	dt := dirty.NewTracker(h)

	t.Log("=== STEP 1: Manually modify a cell in memory ===")
	// Get root cell
	rootRef := h.RootCellOffset()
	t.Logf("Root cell ref: 0x%X", rootRef)

	// Read the root NK cell
	data := h.Bytes()
	rootCellOffset := 4096 + int(rootRef) // HeaderSize + ref

	// Read current cell size
	cellSize := int32(format.ReadU32(data, rootCellOffset))
	if cellSize < 0 {
		cellSize = -cellSize
	}
	t.Logf("Root cell size: %d bytes", cellSize)

	// Modify a timestamp in the root NK (at offset 4 in NK structure)
	// This is safe because we're just changing a timestamp
	timestampOffset := rootCellOffset + 4 + 4 // cell size + signature
	originalTimestamp := format.ReadU64(data, timestampOffset)
	t.Logf("Original timestamp: 0x%X", originalTimestamp)

	// Change timestamp
	newTimestamp := originalTimestamp + 1
	format.PutU64(data, timestampOffset, newTimestamp)
	t.Logf("New timestamp: 0x%X", newTimestamp)

	t.Log("=== STEP 2: Mark cell as dirty ===")
	dt.Add(rootCellOffset, int(cellSize))

	ranges := dt.DebugRanges()
	t.Logf("Dirty ranges after manual add: %d", len(ranges))
	for i, r := range ranges {
		t.Logf("  Range %d: offset=0x%X, length=%d", i, r.Off, r.Len)
	}

	t.Log("=== STEP 3: Check coalesced ranges before flush ===")
	coalesced := dt.DebugCoalescedRanges()
	t.Logf("Coalesced ranges: %d", len(coalesced))
	for i, r := range coalesced {
		t.Logf("  Coalesced %d: offset=0x%X, length=%d (end=0x%X)", i, r.Off, r.Len, r.Off+r.Len)
	}

	// Check data slice
	data2 := h.Bytes()
	t.Logf("Data slice address: %p, length: %d (0x%X)", &data2[0], len(data2), len(data2))

	t.Log("=== STEP 4: Attempt flush ===")
	err = dt.FlushDataOnly()
	if err != nil {
		t.Errorf("FlushDataOnly failed: %v", err)

		// More debugging
		t.Log("=== Investigating the failure ===")
		for _, r := range coalesced {
			start := int(r.Off)
			end := int(r.Off + r.Len)
			t.Logf("Would flush slice [%d:%d] from data (len=%d)", start, end, len(data2))
			if end > len(data2) {
				t.Logf("  ERROR: end %d > data length %d", end, len(data2))
			}
			if start >= len(data2) {
				t.Logf("  ERROR: start %d >= data length %d", start, len(data2))
			}
			// Check alignment of the slice
			sliceAddr := uintptr(unsafe.Pointer(&data2[start]))
			t.Logf("  Slice address: 0x%X, page-aligned: %v", sliceAddr, sliceAddr%4096 == 0)
		}
		return
	}

	t.Log("FlushDataOnly succeeded!")

	t.Log("=== STEP 5: Close and reopen to verify persistence ===")
	h.Close()

	h2, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to reopen hive: %v", err)
	}
	defer h2.Close()

	// Verify the change persisted
	data3 := h2.Bytes()
	persistedTimestamp := format.ReadU64(data3, timestampOffset)
	t.Logf("Persisted timestamp: 0x%X", persistedTimestamp)

	if persistedTimestamp != newTimestamp {
		t.Errorf("Timestamp not persisted: expected 0x%X, got 0x%X", newTimestamp, persistedTimestamp)
	} else {
		t.Log("Timestamp persisted correctly!")
	}
}

// Test_DirtyTracking_EmptyFlush tests flushing with no dirty ranges.
func Test_DirtyTracking_EmptyFlush(t *testing.T) {
	// Setup test hive
	testHivePath := "../../testdata/suite/windows-2003-server-system"
	if _, err := os.Stat(testHivePath); os.IsNotExist(err) {
		t.Skipf("Test hive not found: %s", testHivePath)
	}

	// Copy to temp directory
	tempHivePath := filepath.Join(t.TempDir(), "empty-dirty-test-hive")
	src, err := os.Open(testHivePath)
	if err != nil {
		t.Fatalf("Failed to open source hive: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}
	if _, copyErr := io.Copy(dst, src); copyErr != nil {
		dst.Close()
		t.Fatalf("Failed to copy hive: %v", copyErr)
	}
	dst.Close()

	// Open hive
	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}
	defer h.Close()

	// Create dirty tracker
	dt := dirty.NewTracker(h)

	t.Log("=== Testing flush with no dirty ranges ===")
	ranges := dt.DebugRanges()
	t.Logf("Dirty ranges: %d", len(ranges))

	err = dt.FlushDataOnly()
	if err != nil {
		t.Errorf("Empty FlushDataOnly failed: %v", err)
		return
	}

	t.Log("Empty FlushDataOnly succeeded!")
}
