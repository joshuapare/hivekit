package hive

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestBumpDataSize verifies that BumpDataSize correctly updates the REGF data size field.
func TestBumpDataSize(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Read initial data size (should be 0x1000 = 4096 from writeMinimalHive)
	initialSize := format.ReadU32(h.Bytes(), format.REGFDataSizeOffset)
	require.Equal(t, uint32(0x1000), initialSize)

	// Bump by 0x1000
	h.BumpDataSize(0x1000)

	// Verify it was updated
	newSize := format.ReadU32(h.Bytes(), format.REGFDataSizeOffset)
	require.Equal(t, uint32(0x2000), newSize)

	// Bump again by 0x500
	h.BumpDataSize(0x500)

	finalSize := format.ReadU32(h.Bytes(), format.REGFDataSizeOffset)
	require.Equal(t, uint32(0x2500), finalSize)
}

// TestBumpDataSize_NilHive verifies that BumpDataSize handles nil hive gracefully.
func TestBumpDataSize_NilHive(_ *testing.T) {
	var h *Hive
	// Should not panic
	h.BumpDataSize(100)
}

// TestTouchNowAndBumpSeq verifies that TouchNowAndBumpSeq updates sequences and timestamp.
func TestTouchNowAndBumpSeq(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Read initial sequences (should be 1 from writeMinimalHive)
	initialSeq1 := format.ReadU32(h.Bytes(), format.REGFPrimarySeqOffset)
	initialSeq2 := format.ReadU32(h.Bytes(), format.REGFSecondarySeqOffset)
	require.Equal(t, uint32(1), initialSeq1)
	require.Equal(t, uint32(1), initialSeq2)

	// Read initial timestamp
	initialTimestamp := format.ReadU64(h.Bytes(), format.REGFTimeStampOffset)

	// Small delay to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Touch
	beforeTouch := time.Now()
	h.TouchNowAndBumpSeq()
	afterTouch := time.Now()

	// Verify sequences incremented
	newSeq1 := format.ReadU32(h.Bytes(), format.REGFPrimarySeqOffset)
	newSeq2 := format.ReadU32(h.Bytes(), format.REGFSecondarySeqOffset)
	require.Equal(t, uint32(2), newSeq1)
	require.Equal(t, uint32(2), newSeq2)

	// Verify timestamp updated and is reasonable
	newTimestamp := format.ReadU64(h.Bytes(), format.REGFTimeStampOffset)
	require.NotEqual(t, initialTimestamp, newTimestamp)

	// Convert back to time.Time and verify it's between before/after
	touchTime := format.FiletimeToTime(newTimestamp)
	require.True(t, touchTime.After(beforeTouch.Add(-1*time.Second)), "timestamp should be after beforeTouch")
	require.True(t, touchTime.Before(afterTouch.Add(1*time.Second)), "timestamp should be before afterTouch")

	// Touch again
	h.TouchNowAndBumpSeq()
	finalSeq1 := format.ReadU32(h.Bytes(), format.REGFPrimarySeqOffset)
	finalSeq2 := format.ReadU32(h.Bytes(), format.REGFSecondarySeqOffset)
	require.Equal(t, uint32(3), finalSeq1)
	require.Equal(t, uint32(3), finalSeq2)
}

// TestTouchNowAndBumpSeq_NilHive verifies that TouchNowAndBumpSeq handles nil hive gracefully.
func TestTouchNowAndBumpSeq_NilHive(_ *testing.T) {
	var h *Hive
	// Should not panic
	h.TouchNowAndBumpSeq()
}

// TestAppend_GrowsFile verifies that Append correctly grows the file and remaps memory.
func TestAppend_GrowsFile(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Initial size is 0x2000 (8192 bytes)
	require.Equal(t, int64(0x2000), h.Size())
	require.Len(t, h.Bytes(), 0x2000)

	// Append 0x1000 bytes
	err = h.Append(0x1000)
	require.NoError(t, err)

	// Verify size grew
	require.Equal(t, int64(0x3000), h.Size())
	require.Len(t, h.Bytes(), 0x3000)

	// Verify original data is intact
	magic := string(h.Bytes()[0:4])
	require.Equal(t, "regf", magic)

	rootOffset := format.ReadU32(h.Bytes(), format.REGFRootCellOffset)
	require.Equal(t, uint32(0x20), rootOffset)

	// Verify new bytes are zero-initialized
	for i := 0x2000; i < 0x3000; i++ {
		require.Equal(t, byte(0), h.Bytes()[i], "byte at offset %d should be zero", i)
	}

	// Verify file on disk also grew
	stat, err := os.Stat(hivePath)
	require.NoError(t, err)
	require.Equal(t, int64(0x3000), stat.Size())
}

// TestAppend_MultipleGrows verifies that multiple Append calls work correctly.
func TestAppend_MultipleGrows(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	initialSize := h.Size()

	// Grow multiple times
	for range 5 {
		err = h.Append(0x500)
		require.NoError(t, err)
	}

	// Verify total growth
	expectedSize := initialSize + (0x500 * 5)
	require.Equal(t, expectedSize, h.Size())
	require.Len(t, h.Bytes(), int(expectedSize))

	// Verify original data still intact
	magic := string(h.Bytes()[0:4])
	require.Equal(t, "regf", magic)
}

// TestAppend_ZeroBytes verifies that Append(0) is a no-op.
func TestAppend_ZeroBytes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	initialSize := h.Size()

	err = h.Append(0)
	require.NoError(t, err)

	require.Equal(t, initialSize, h.Size())
}

// TestAppend_NegativeBytes verifies that Append with negative value is a no-op.
func TestAppend_NegativeBytes(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	initialSize := h.Size()

	err = h.Append(-100)
	require.NoError(t, err)

	require.Equal(t, initialSize, h.Size())
}

// TestAppend_NilHive verifies that Append on nil hive returns error.
func TestAppend_NilHive(t *testing.T) {
	var h *Hive
	err := h.Append(100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "nil")
}

// TestAppend_ClosedHive verifies that Append on closed hive returns error.
func TestAppend_ClosedHive(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	h.Close()

	err = h.Append(100)
	require.Error(t, err)
}

// TestAppend_LargeGrowth verifies that large growth works correctly.
func TestAppend_LargeGrowth(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Grow by 1 MB
	err = h.Append(1024 * 1024)
	require.NoError(t, err)

	require.Equal(t, int64(0x2000+1024*1024), h.Size())

	// Verify data integrity
	magic := string(h.Bytes()[0:4])
	require.Equal(t, "regf", magic)
}

// TestAppend_PreservesDataIntegrity writes known patterns and verifies they survive growth.
func TestAppend_PreservesDataIntegrity(t *testing.T) {
	dir := t.TempDir()
	hivePath := filepath.Join(dir, "test.hiv")
	writeMinimalHive(t, hivePath)

	h, err := Open(hivePath)
	require.NoError(t, err)
	defer h.Close()

	// Write some pattern in the HBIN area (after 0x1000)
	// This simulates having actual data in the hive
	pattern := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	testOffset := 0x1500
	copy(h.Bytes()[testOffset:], pattern)

	// Grow the file
	err = h.Append(0x1000)
	require.NoError(t, err)

	// Verify pattern is still there
	for i, b := range pattern {
		require.Equal(t, b, h.Bytes()[testOffset+i], "pattern byte %d should match", i)
	}
}
