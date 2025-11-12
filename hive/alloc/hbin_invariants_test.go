//go:build linux || darwin

package alloc

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// Test_HBIN_Header_Offsets_Correct verifies HBIN offset fields match actual file positions
// This is Test #4 from DEBUG.md: "HBIN_Header_Offsets_Correct".
func Test_HBIN_Header_Offsets_Correct(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Trigger growth to create a new HBIN
	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	// Verify all HBINs have correct offset fields
	data := h.Bytes()
	pos := format.HeaderSize

	for pos < len(data) {
		// Check if we have room for HBIN header
		if pos+format.HBINHeaderSize > len(data) {
			break
		}

		// Verify HBIN signature
		sig := string(data[pos : pos+4])
		if sig != string(format.HBINSignature) {
			break // End of HBINs
		}

		// Read offset field
		offsetField := int(getU32(data, pos+format.HBINFileOffsetField))
		expectedOffset := pos - format.HeaderSize // Relative to 0x1000

		require.Equal(t, expectedOffset, offsetField,
			"HBIN at 0x%X has offset field 0x%X, expected 0x%X",
			pos, offsetField, expectedOffset)

		// Read size and move to next HBIN
		hbinSize := int(getU32(data, pos+format.HBINSizeOffset))
		require.Positive(t, hbinSize, "HBIN size must be positive")
		require.Zero(t, hbinSize%format.HBINAlignment, "HBIN size must be 4KB aligned")

		t.Logf("HBIN at 0x%X: offset field = 0x%X (correct), size = 0x%X",
			pos, offsetField, hbinSize)

		pos += hbinSize
	}
}

// Test_HBIN_PageSize_Alignment verifies all HBIN sizes are 4KB aligned
// This is Test #5 from DEBUG.md: "HBIN_PageSize_Alignment".
func Test_HBIN_PageSize_Alignment(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow with various sizes
	testPages := []int{1, 2, 4} // 4KB, 8KB, 16KB
	for _, pages := range testPages {
		err = fa.GrowByPages(pages)
		require.NoError(t, err)
	}

	// Verify all HBINs are 4KB aligned
	data := h.Bytes()
	pos := format.HeaderSize
	hbinCount := 0

	for pos < len(data) {
		if pos+format.HBINHeaderSize > len(data) {
			break
		}

		sig := string(data[pos : pos+4])
		if sig != string(format.HBINSignature) {
			break
		}

		hbinSize := int(getU32(data, pos+format.HBINSizeOffset))
		require.Zero(t, hbinSize%format.HBINAlignment,
			"HBIN %d at 0x%X has size 0x%X which is not 4KB aligned",
			hbinCount, pos, hbinSize)

		hbinCount++
		pos += hbinSize
	}

	t.Logf("Verified %d HBINs all have 4KB-aligned sizes", hbinCount)
}

// Test_HBIN_SeedFreeCell_SpansToEnd verifies that after growth, the seed free cell spans to HBIN end
// This is Test #6 from DEBUG.md: "HBIN_SeedFreeCell_SpansToEnd".
func Test_HBIN_SeedFreeCell_SpansToEnd(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow to create a new HBIN
	err = fa.GrowByPages(2) // Add 8KB HBIN
	require.NoError(t, err)

	// Find the last HBIN (the one we just created)
	data := h.Bytes()
	var lastHBINStart, lastHBINSize int
	pos := format.HeaderSize

	for pos < len(data) {
		if pos+format.HBINHeaderSize > len(data) {
			break
		}

		sig := string(data[pos : pos+4])
		if sig != string(format.HBINSignature) {
			break
		}

		lastHBINStart = pos
		lastHBINSize = int(getU32(data, pos+format.HBINSizeOffset))
		pos += lastHBINSize
	}

	// Check the first cell in the last HBIN
	firstCellOff := lastHBINStart + format.HBINHeaderSize
	cellSize := int(getI32(data, firstCellOff))

	// Cell should be free (positive size) and span to end of HBIN
	require.Positive(t, cellSize, "First cell in new HBIN should be free (positive size)")

	expectedSize := lastHBINSize - format.HBINHeaderSize
	require.Equal(t, expectedSize, cellSize,
		"First cell in new HBIN should span to HBIN end (expected %d, got %d)",
		expectedSize, cellSize)

	t.Logf("New HBIN at 0x%X has seed free cell spanning to end (size 0x%X)",
		lastHBINStart, cellSize)
}

// Test_HBIN_NoTrailingBytesInPage verifies no data exists past the end of HBIN data
// This is Test #7 from DEBUG.md: "HBIN_NoTrailingBytesInPage".
func Test_HBIN_NoTrailingBytesInPage(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	createHiveWithFreeCells(t, hivePath, []int{128})

	h, err := hive.Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil)
	require.NoError(t, err)

	// Grow
	err = fa.GrowByPages(1) // Add 4KB HBIN
	require.NoError(t, err)

	// Calculate where HBINs should end
	data := h.Bytes()
	pos := format.HeaderSize
	var totalHBINSize int

	for pos < len(data) {
		if pos+format.HBINHeaderSize > len(data) {
			break
		}

		sig := string(data[pos : pos+4])
		if sig != string(format.HBINSignature) {
			break
		}

		hbinSize := int(getU32(data, pos+format.HBINSizeOffset))
		totalHBINSize += hbinSize
		pos += hbinSize
	}

	expectedFileEnd := format.HeaderSize + totalHBINSize
	actualFileEnd := len(data)

	require.Equal(t, expectedFileEnd, actualFileEnd,
		"File should end exactly at HBIN boundary (expected 0x%X, got 0x%X)",
		expectedFileEnd, actualFileEnd)

	// Also verify header's data size field matches
	headerDataSize := int(getU32(data, format.REGFDataSizeOffset))
	require.Equal(t, totalHBINSize, headerDataSize,
		"Header data size field (0x%X) should match total HBIN size (0x%X)",
		headerDataSize, totalHBINSize)

	t.Logf("File ends at 0x%X, exactly where HBINs end (no trailing bytes)",
		actualFileEnd)
}
