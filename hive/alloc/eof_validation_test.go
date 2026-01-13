//go:build linux || darwin

package alloc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_EOF_NoTrailingBytes verifies intentionally corrupted hive with trailing bytes is detected
// This is Test #19 from DEBUG.md: "EOF_NoTrailingBytes".
func Test_EOF_NoTrailingBytes(t *testing.T) {
	dir := t.TempDir()
	cleanPath := filepath.Join(dir, "clean.hiv")
	corruptPath := filepath.Join(dir, "corrupt.hiv")

	// Create a clean hive
	createMinimalHive(t, cleanPath, 4096)

	// Read it
	data, err := os.ReadFile(cleanPath)
	require.NoError(t, err)

	// Create corrupted version with trailing bytes
	corruptData := make([]byte, len(data)+64)
	copy(corruptData, data)
	// Add garbage at the end
	for i := len(data); i < len(corruptData); i++ {
		corruptData[i] = 0xFF
	}

	err = os.WriteFile(corruptPath, corruptData, 0644)
	require.NoError(t, err)

	// Try to open corrupted hive
	h, err := hive.Open(corruptPath)
	if err != nil {
		t.Logf("Corrupted hive rejected during open: %v", err)
		return
	}
	defer h.Close()

	// If it opened, verify file size vs header
	hiveData := h.Bytes()
	headerDataSize := int(getU32(hiveData, format.REGFDataSizeOffset))
	expectedFileSize := format.HeaderSize + headerDataSize
	actualFileSize := len(hiveData)

	// The hive implementation truncates trailing slack on Open()
	// So this should match after opening
	if actualFileSize == expectedFileSize {
		t.Logf("Hive truncated trailing bytes on open (expected: 0x%X, actual: 0x%X)",
			expectedFileSize, actualFileSize)
	} else {
		t.Errorf("File has trailing bytes: expected size 0x%X, actual 0x%X (diff: %d bytes)",
			expectedFileSize, actualFileSize, actualFileSize-expectedFileSize)
	}
}

// Test_Header_Checksum_Recomputed verifies checksum is correct after modifications
// This is Test #1 from DEBUG.md: "Header_Checksum_Recomputed".
func Test_Header_Checksum_Recomputed(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Modify hive (grow it)
	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	// Mark header dirty so it gets flushed
	dt.Add(0, format.HeaderSize)

	// Get current header
	data := h.Bytes()

	// Calculate checksum manually (XOR of first 508 dwords, excluding checksum field itself)
	// Checksum is at offset 0x1FC
	var checksum uint32
	for i := 0; i < 0x1FC; i += 4 {
		checksum ^= getU32(data, i)
	}

	storedChecksum := getU32(data, 0x1FC)

	t.Logf("Calculated checksum: 0x%08X", checksum)
	t.Logf("Stored checksum:     0x%08X", storedChecksum)

	// TDD: This test MUST fail until checksum recomputation is implemented
	// After modifications (like Grow), the header checksum MUST be updated
	require.Equal(t, checksum, storedChecksum,
		"CHECKSUM NOT RECOMPUTED: After Grow(), header checksum must be recalculated\n"+
			"Expected: 0x%08X\nActual:   0x%08X\n"+
			"TODO: Implement checksum recomputation in Grow() or when header is marked dirty",
		checksum, storedChecksum)
}

// Test_Header_Blocks_Truncation verifies header updates after hypothetical compaction
// This is Test #3 from DEBUG.md: "Header_Blocks_Truncation".
func Test_Header_Blocks_Truncation(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createMinimalHive(t, hivePath, 4096)

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow to 3 pages
	err = fa.GrowByPages(1) // Add 4KB HBIN (now 2 pages total)
	require.NoError(t, err)
	err = fa.GrowByPages(1) // Add 4KB HBIN (now 3 pages total)
	require.NoError(t, err)

	data := h.Bytes()
	beforeSize := int(getU32(data, format.REGFDataSizeOffset))
	beforeFileSize := len(data)

	t.Logf("After growth: header data size = 0x%X, file size = 0x%X", beforeSize, beforeFileSize)

	// Verify current invariant holds
	require.Equal(t, format.HeaderSize+beforeSize, beforeFileSize,
		"File size should match header + data size")

	// Now test truncation
	// Truncate by 1 page (removing the last HBIN)
	err = fa.TruncatePages(1)
	require.NoError(t, err, "TruncatePages(1) should succeed")

	// Verify header data size decreased
	data = h.Bytes()
	afterTruncate := int(getU32(data, format.REGFDataSizeOffset))
	expectedAfterTruncate := beforeSize - 4096

	require.Equal(t, expectedAfterTruncate, afterTruncate,
		"After truncating 1 page, data size should decrease by 4096 bytes")

	// Verify file size matches header + data size
	afterFileSize := len(data)
	require.Equal(t, format.HeaderSize+afterTruncate, afterFileSize,
		"File size should equal header (0x1000) + data size after truncation")

	t.Logf("Truncation successful:")
	t.Logf("   Before: data size = 0x%X, file size = 0x%X", beforeSize, beforeFileSize)
	t.Logf("   After:  data size = 0x%X, file size = 0x%X", afterTruncate, afterFileSize)
	t.Logf("   Removed: 0x%X bytes (1 page)", beforeSize-afterTruncate)
}
