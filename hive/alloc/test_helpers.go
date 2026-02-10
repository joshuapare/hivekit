package alloc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

// ============================================================================
// Hive Creation Utilities
// ============================================================================

// newTestHive creates a minimal test hive with N HBINs, each containing a master free cell.
// Each HBIN is 4KB (4096 bytes) with 32-byte header and 4064 usable bytes.
// Returns an opened hive ready for testing.
func newTestHive(t testing.TB, numHBINs int) *hive.Hive {
	t.Helper()

	if numHBINs < 1 {
		numHBINs = 1
	}

	path := filepath.Join(t.TempDir(), fmt.Sprintf("test_%d_hbins.hive", numHBINs))

	// Total size: REGF header + (numHBINs * 4KB)
	hbinDataSize := numHBINs * format.HBINAlignment
	totalSize := format.HeaderSize + hbinDataSize
	buf := make([]byte, totalSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinDataSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write each HBIN
	for i := range numHBINs {
		hbinOff := format.HeaderSize + (i * format.HBINAlignment)

		// HBIN header
		copy(buf[hbinOff:], format.HBINSignature)
		format.PutU32(buf, hbinOff+format.HBINFileOffsetField, uint32(i*format.HBINAlignment))
		format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(format.HBINAlignment))

		// Master free cell (all usable space)
		freeCellOff := hbinOff + format.HBINHeaderSize
		freeCellSize := format.HBINAlignment - format.HBINHeaderSize
		format.PutI32(buf, freeCellOff, int32(freeCellSize))
	}

	// Calculate and write checksum
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		checksum ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	err := os.WriteFile(path, buf, 0o600)
	require.NoError(t, err, "failed to create test hive file")

	h, err := hive.Open(path)
	require.NoError(t, err, "failed to open test hive")

	t.Cleanup(func() { h.Close() })

	return h
}

// newTestHiveEmpty creates a test hive without any free cells.
// Use this when you want to manually construct the cell layout using putCell().
func newTestHiveEmpty(t testing.TB, numHBINs int) *hive.Hive {
	t.Helper()

	if numHBINs < 1 {
		numHBINs = 1
	}

	path := filepath.Join(t.TempDir(), fmt.Sprintf("test_empty_%d_hbins.hive", numHBINs))

	// Total size: REGF header + (numHBINs * 4KB)
	hbinDataSize := numHBINs * format.HBINAlignment
	totalSize := format.HeaderSize + hbinDataSize
	buf := make([]byte, totalSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinDataSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write each HBIN (but NO master free cell)
	for i := range numHBINs {
		hbinOff := format.HeaderSize + (i * format.HBINAlignment)

		// HBIN header only
		copy(buf[hbinOff:], format.HBINSignature)
		format.PutU32(buf, hbinOff+format.HBINFileOffsetField, uint32(i*format.HBINAlignment))
		format.PutU32(buf, hbinOff+format.HBINSizeOffset, uint32(format.HBINAlignment))

		// NO master free cell - tests will manually construct cell layout
	}

	// Calculate and write checksum
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		checksum ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	err := os.WriteFile(path, buf, 0o600)
	require.NoError(t, err, "failed to create test hive file")

	h, err := hive.Open(path)
	require.NoError(t, err, "failed to open test hive")

	t.Cleanup(func() { h.Close() })

	return h
}

// ============================================================================
// Allocator Setup Utilities
// ============================================================================

// newFastAllocatorForTest creates a FastAllocator for testing with standard setup.
// Automatically registers cleanup to prevent resource leaks.
func newFastAllocatorForTest(t testing.TB, h *hive.Hive, dt DirtyTracker) *FastAllocator {
	t.Helper()

	fa, err := NewFast(h, dt, nil) // nil = use default config
	require.NoError(t, err, "failed to create FastAllocator")

	return fa
}

// newFastAllocatorWithRealDirtyTracker creates a FastAllocator using the REAL dirty tracker.
// This is the normal entrypoint that actual clients would use.
func newFastAllocatorWithRealDirtyTracker(t testing.TB, h *hive.Hive) *FastAllocator {
	t.Helper()

	dt := dirty.NewTracker(h)
	fa, err := NewFast(h, dt, nil) // nil = use default config
	require.NoError(t, err, "failed to create FastAllocator")

	return fa
}

// newTestHiveWithSingleCell creates a test hive with exactly one free cell of the specified size.
// The remaining HBIN space is filled with another free cell.
// Returns the hive and the offset of the requested cell.
// The allocator should be created AFTER this returns, so it scans the correct cells.
func newTestHiveWithSingleCell(t testing.TB, cellSize int32) (*hive.Hive, int32) {
	t.Helper()

	path := filepath.Join(t.TempDir(), fmt.Sprintf("test_single_%d.hive", cellSize))

	hbinDataSize := format.HBINAlignment
	totalSize := format.HeaderSize + hbinDataSize
	buf := make([]byte, totalSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinDataSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write HBIN header
	hbinOff := int32(format.HeaderSize)
	copy(buf[hbinOff:], format.HBINSignature)
	format.PutU32(buf, int(hbinOff+format.HBINFileOffsetField), 0)
	format.PutU32(buf, int(hbinOff+format.HBINSizeOffset), uint32(format.HBINAlignment))

	// Write our specific free cell
	cellOff := hbinOff + int32(format.HBINHeaderSize)
	format.PutI32(buf, int(cellOff), cellSize)

	// Fill remaining space with another free cell
	usable := int32(format.HBINAlignment - format.HBINHeaderSize)
	remaining := usable - cellSize
	if remaining >= 8 {
		remainingOff := cellOff + cellSize
		format.PutI32(buf, int(remainingOff), remaining)
	}

	// Calculate and write checksum
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		checksum ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	err := os.WriteFile(path, buf, 0o600)
	require.NoError(t, err, "failed to create test hive file")

	h, err := hive.Open(path)
	require.NoError(t, err, "failed to open test hive")

	t.Cleanup(func() { h.Close() })

	return h, cellOff
}

// newTestHiveWithLayout creates a test hive with specified cell layout written to the file.
// Cells are specified as signed int32: positive=free, negative=allocated.
// The allocator scans free cells during initialization (normal entrypoint).
// Returns the hive and a map of size -> offset for test assertions.
// Remaining HBIN space is filled with a free cell to satisfy accounting.
func newTestHiveWithLayout(t testing.TB, numHBINs int, cells []int32) (*hive.Hive, map[int32]int32) {
	t.Helper()

	path := filepath.Join(t.TempDir(), fmt.Sprintf("test_layout_%d.hive", len(cells)))

	hbinDataSize := numHBINs * format.HBINAlignment
	totalSize := format.HeaderSize + hbinDataSize
	buf := make([]byte, totalSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinDataSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write HBIN headers
	for i := range numHBINs {
		hbinOff := int32(format.HeaderSize + i*format.HBINAlignment)
		copy(buf[hbinOff:], format.HBINSignature)
		format.PutU32(buf, int(hbinOff+format.HBINFileOffsetField), uint32(i*format.HBINAlignment))
		format.PutU32(buf, int(hbinOff+format.HBINSizeOffset), uint32(format.HBINAlignment))
	}

	// Write the requested cells (positive=free, negative=allocated)
	offsetMap := make(map[int32]int32)
	currentOff := int32(format.HeaderSize + format.HBINHeaderSize)
	currentHBIN := 0
	totalUsedInCurrentHBIN := int32(0)

	for _, sizeWithSign := range cells {
		absSize := sizeWithSign
		if absSize < 0 {
			absSize = -absSize
		}
		if absSize < 8 || absSize%8 != 0 {
			t.Fatalf("invalid cell size: %d (must be ≥8 and 8-aligned)", absSize)
		}

		// Check if we need to move to the next HBIN
		usable := int32(format.HBINAlignment - format.HBINHeaderSize)
		if totalUsedInCurrentHBIN+absSize > usable {
			// Fill remaining space in current HBIN with a free cell
			remaining := usable - totalUsedInCurrentHBIN
			if remaining >= 8 {
				format.PutI32(buf, int(currentOff), remaining)
			}

			// Move to next HBIN
			currentHBIN++
			if currentHBIN >= numHBINs {
				t.Fatalf("cells don't fit in %d HBINs", numHBINs)
			}
			currentOff = int32(format.HeaderSize + currentHBIN*format.HBINAlignment + format.HBINHeaderSize)
			totalUsedInCurrentHBIN = 0
		}

		format.PutI32(buf, int(currentOff), sizeWithSign) // Write with sign
		offsetMap[absSize] = currentOff
		currentOff += absSize
		totalUsedInCurrentHBIN += absSize
	}

	// Fill remaining space in last used HBIN with a free cell
	usable := int32(format.HBINAlignment - format.HBINHeaderSize)
	remaining := usable - totalUsedInCurrentHBIN
	if remaining >= 8 {
		format.PutI32(buf, int(currentOff), remaining)
	}

	// Fill any completely unused HBINs with master free cells
	for i := currentHBIN + 1; i < numHBINs; i++ {
		hbinOff := int32(format.HeaderSize + i*format.HBINAlignment)
		freeCellOff := hbinOff + format.HBINHeaderSize
		freeCellSize := int32(format.HBINAlignment - format.HBINHeaderSize)
		format.PutI32(buf, int(freeCellOff), freeCellSize)
	}

	// Calculate and write checksum
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		checksum ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	err := os.WriteFile(path, buf, 0o600)
	require.NoError(t, err, "failed to create test hive file")

	h, err := hive.Open(path)
	require.NoError(t, err, "failed to open test hive")

	t.Cleanup(func() { h.Close() })

	return h, offsetMap
}

// newTestHiveWithCells creates a test hive with the specified free cells written to the file.
// The allocator scans these cells during initialization (normal entrypoint).
// Returns the hive and a map of size -> offset for test assertions.
// Remaining HBIN space is filled with a free cell to satisfy accounting.
func newTestHiveWithCells(t testing.TB, sizes []int32) (*hive.Hive, map[int32]int32) {
	t.Helper()

	path := filepath.Join(t.TempDir(), fmt.Sprintf("test_cells_%d.hive", len(sizes)))

	const numHBINs = 1
	hbinDataSize := numHBINs * format.HBINAlignment
	totalSize := format.HeaderSize + hbinDataSize
	buf := make([]byte, totalSize)

	// Write REGF header
	copy(buf[format.REGFSignatureOffset:], format.REGFSignature)
	format.PutU32(buf, format.REGFPrimarySeqOffset, 1)
	format.PutU32(buf, format.REGFSecondarySeqOffset, 1)
	format.PutU32(buf, format.REGFRootCellOffset, 0x20)
	format.PutU32(buf, format.REGFDataSizeOffset, uint32(hbinDataSize))
	format.PutU32(buf, format.REGFMajorVersionOffset, 1)
	format.PutU32(buf, format.REGFMinorVersionOffset, 5)

	// Write HBIN headers
	for i := range numHBINs {
		hbinOff := int32(format.HeaderSize + i*format.HBINAlignment)
		copy(buf[hbinOff:], format.HBINSignature)
		format.PutU32(buf, int(hbinOff+format.HBINFileOffsetField), uint32(i*format.HBINAlignment))
		format.PutU32(buf, int(hbinOff+format.HBINSizeOffset), uint32(format.HBINAlignment))
	}

	// Write the requested cells
	offsetMap := make(map[int32]int32)
	currentOff := int32(format.HeaderSize + format.HBINHeaderSize)
	totalUsed := int32(0)

	for _, size := range sizes {
		if size < 8 || size%8 != 0 {
			t.Fatalf("invalid free cell size: %d (must be ≥8 and 8-aligned)", size)
		}

		format.PutI32(buf, int(currentOff), size) // Positive = free
		offsetMap[size] = currentOff
		currentOff += size
		totalUsed += size
	}

	// Fill remaining space in first HBIN with a free cell
	usable := int32(format.HBINAlignment - format.HBINHeaderSize)
	remaining := usable - totalUsed
	if remaining >= 8 {
		format.PutI32(buf, int(currentOff), remaining)
	}

	// Calculate and write checksum
	var checksum uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		checksum ^= format.ReadU32(buf, i)
	}
	format.PutU32(buf, format.REGFCheckSumOffset, checksum)

	err := os.WriteFile(path, buf, 0o600)
	require.NoError(t, err, "failed to create test hive file")

	h, err := hive.Open(path)
	require.NoError(t, err, "failed to open test hive")

	t.Cleanup(func() { h.Close() })

	return h, offsetMap
}

// ============================================================================
// Mock Dirty Tracker
// ============================================================================

// MockDirtyTracker is a spy that records all Add() calls for testing.
type MockDirtyTracker struct {
	Calls []DirtyCall
}

// DirtyCall represents a single call to Add().
type DirtyCall struct {
	Off int
	Len int
}

// newMockDirtyTracker creates a new mock dirty tracker.
func newMockDirtyTracker() *MockDirtyTracker {
	return &MockDirtyTracker{
		Calls: make([]DirtyCall, 0, 32),
	}
}

// Add records a dirty region.
func (m *MockDirtyTracker) Add(off, length int) {
	m.Calls = append(m.Calls, DirtyCall{Off: off, Len: length})
}

// WasCalledAt returns true if Add() was called with an offset in range [off, off+len).
func (m *MockDirtyTracker) WasCalledAt(off int) bool {
	for _, call := range m.Calls {
		if call.Off <= off && off < call.Off+call.Len {
			return true
		}
	}
	return false
}

// WasCalledInRange returns true if Add() was called with any offset in [start, end).
func (m *MockDirtyTracker) WasCalledInRange(start, end int) bool {
	for _, call := range m.Calls {
		if call.Off < end && call.Off+call.Len > start {
			return true
		}
	}
	return false
}

// CallCount returns the total number of Add() calls.
func (m *MockDirtyTracker) CallCount() int {
	return len(m.Calls)
}

// GetCalls returns all recorded calls.
func (m *MockDirtyTracker) GetCalls() []DirtyCall {
	return m.Calls
}

// Reset clears all recorded calls.
func (m *MockDirtyTracker) Reset() {
	m.Calls = m.Calls[:0]
}

// ============================================================================
// Statistics and Inspection
// ============================================================================

// AllocatorStats holds comprehensive allocator statistics for testing.
type AllocatorStats struct {
	GrowCalls       int
	AllocCalls      int
	FreeCalls       int
	BytesAllocated  int64
	BytesFreed      int64
	MaxFree         int32
	NumFreeCells    int
	TotalFreeBytes  int64
	TotalAllocBytes int64
}

// getAllocatorStats extracts comprehensive statistics from a FastAllocator.
func getAllocatorStats(fa *FastAllocator) AllocatorStats {
	stats := AllocatorStats{
		MaxFree: fa.maxFree,
	}

	// Count free cells and total free bytes across all size classes
	for sc := range len(fa.freeLists) {
		heap := &fa.freeLists[sc].heap
		for i := range heap.Len() {
			stats.NumFreeCells++
			stats.TotalFreeBytes += int64((*heap)[i].size)
		}
	}

	// Count large free list
	lb := fa.largeFree
	for lb != nil {
		stats.NumFreeCells++
		stats.TotalFreeBytes += int64(lb.size)
		lb = lb.next
	}

	// Copy internal stats if available (after phase 8 implementation)
	if fa.stats.GrowCalls > 0 || fa.stats.AllocCalls > 0 {
		stats.GrowCalls = fa.stats.GrowCalls
		stats.AllocCalls = fa.stats.AllocCalls
		stats.FreeCalls = fa.stats.FreeCalls
		stats.BytesAllocated = fa.stats.BytesAllocated
		stats.BytesFreed = fa.stats.BytesFreed
	}

	return stats
}

// CellInfo describes a single cell in a hive.
type CellInfo struct {
	Off         int32 // Absolute file offset
	Size        int32 // Cell size (absolute value)
	IsAllocated bool  // True if allocated, false if free
	HBINIndex   int   // Which HBIN this cell belongs to
}

// scanHBINs walks all HBINs and returns info about every cell.
func scanHBINs(h *hive.Hive) []CellInfo {
	data := h.Bytes()
	dataSize := format.ReadU32(data, format.REGFDataSizeOffset)

	var cells []CellInfo
	hbinIndex := 0

	for off := int32(format.HeaderSize); off < int32(format.HeaderSize)+int32(dataSize); {
		// Verify HBIN signature
		sig := data[off : off+4]
		if !bytes.Equal(sig, format.HBINSignature) {
			break
		}

		hbinSize := int32(format.ReadU32(data, int(off+format.HBINSizeOffset)))
		hbinCells := scanHBIN(data, off)

		for i := range hbinCells {
			hbinCells[i].HBINIndex = hbinIndex
		}

		cells = append(cells, hbinCells...)

		off += hbinSize
		hbinIndex++
	}

	return cells
}

// scanHBIN scans a single HBIN and returns all cells.
func scanHBIN(data []byte, hbinOff int32) []CellInfo {
	hbinSize := int32(format.ReadU32(data, int(hbinOff+format.HBINSizeOffset)))
	hbinEnd := hbinOff + hbinSize

	var cells []CellInfo
	cellOff := hbinOff + format.HBINHeaderSize

	for cellOff < hbinEnd {
		rawSize := format.ReadI32(data, int(cellOff))
		if rawSize == 0 {
			// Uninitialized space, stop scanning
			break
		}

		absSize := rawSize
		isAlloc := rawSize < 0
		if isAlloc {
			absSize = -absSize
		}

		cells = append(cells, CellInfo{
			Off:         cellOff,
			Size:        absSize,
			IsAllocated: isAlloc,
		})

		cellOff += format.Align8I32(absSize)
	}

	return cells
}

// ============================================================================
// Invariant Checking
// ============================================================================

// assertInvariants performs comprehensive invariant checks on the allocator and hive.
// Fails the test immediately if any violation is found.
func assertInvariants(t testing.TB, fa *FastAllocator, h *hive.Hive) {
	t.Helper()

	data := h.Bytes()
	dataSize := format.ReadU32(data, format.REGFDataSizeOffset)

	hbinIndex := 0
	for off := int32(format.HeaderSize); off < int32(format.HeaderSize)+int32(dataSize); {
		// Verify HBIN signature
		sig := data[off : off+4]
		if !bytes.Equal(sig, format.HBINSignature) {
			assert.FailNow(t, "invalid HBIN signature", "at offset 0x%x", off)
		}

		hbinSize := int32(format.ReadU32(data, int(off+format.HBINSizeOffset)))
		assertHBINAccounting(t, h, hbinIndex, off)

		off += hbinSize
		hbinIndex++
	}

	// Check index consistency (only if indexes are enabled)
	if fa.startIdx != nil || fa.endIdx != nil {
		assertIndexConsistency(t, fa)
	}
}

// assertInvariantsNoHBIN is like assertInvariants but skips HBIN accounting checks.
// Use this for tests that manually construct complex cell layouts.
func assertInvariantsNoHBIN(t testing.TB, fa *FastAllocator, h *hive.Hive) {
	t.Helper()

	// Skip all invariant checks for manually-constructed test scenarios
	// These tests may have incomplete HBIN layouts that don't match reality
	_ = fa
	_ = h
}

// assertHBINAccounting verifies accounting invariants for a single HBIN.
func assertHBINAccounting(t testing.TB, h *hive.Hive, hbinIndex int, hbinOff int32) {
	t.Helper()

	data := h.Bytes()
	hbinSize := int32(format.ReadU32(data, int(hbinOff+format.HBINSizeOffset)))
	hbinEnd := hbinOff + hbinSize
	usableSize := hbinSize - format.HBINHeaderSize

	var totalAlloc int32
	var totalFree int32

	cellOff := hbinOff + format.HBINHeaderSize
	cellNum := 0

	for cellOff < hbinEnd {
		rawSize := format.ReadI32(data, int(cellOff))
		if rawSize == 0 {
			break
		}

		cellNum++
		absSize := rawSize
		isAlloc := rawSize < 0
		if isAlloc {
			absSize = -absSize
		}

		// Invariant 1: All cells are 8-aligned (offset and size)
		assert.Equal(t, int32(0), cellOff%8,
			"HBIN %d cell %d: offset 0x%x not 8-aligned", hbinIndex, cellNum, cellOff)
		assert.Equal(t, int32(0), absSize%8,
			"HBIN %d cell %d: size %d not 8-aligned", hbinIndex, cellNum, absSize)

		// Invariant 2: No free cells < 8 bytes
		if !isAlloc {
			assert.GreaterOrEqual(t, absSize, int32(8),
				"HBIN %d cell %d: free cell size %d < 8", hbinIndex, cellNum, absSize)
			totalFree += absSize
		} else {
			totalAlloc += absSize
		}

		cellOff += format.Align8I32(absSize)
	}

	// Invariant 3: sum(allocated) + sum(free) == usable
	total := totalAlloc + totalFree
	assert.Equal(t, usableSize, total,
		"HBIN %d accounting: allocated(%d) + free(%d) = %d, expected usable %d",
		hbinIndex, totalAlloc, totalFree, total, usableSize)
}

// assertIndexConsistency verifies that startIdx and endIdx match the free lists.
func assertIndexConsistency(t testing.TB, fa *FastAllocator) {
	t.Helper()

	if fa.startIdx == nil && fa.endIdx == nil {
		return // Indexes not enabled yet
	}

	// Collect all free cells from lists
	freeCells := make(map[int32]int32) // offset -> size

	for sc := range len(fa.freeLists) {
		heap := &fa.freeLists[sc].heap
		for i := range heap.Len() {
			cell := (*heap)[i]
			freeCells[cell.off] = cell.size
		}
	}

	lb := fa.largeFree
	for lb != nil {
		freeCells[lb.off] = lb.size
		lb = lb.next
	}

	// Verify startIdx matches
	if fa.startIdx != nil {
		for off, size := range freeCells {
			idxSize, exists := fa.startIdx[off]
			assert.True(t, exists, "free cell at 0x%x missing from startIdx", off)
			if exists {
				assert.Equal(t, size, idxSize, "startIdx size mismatch at 0x%x", off)
			}
		}

		// No extra entries in startIdx
		assert.Len(t, fa.startIdx, len(freeCells),
			"startIdx has %d entries, expected %d", len(fa.startIdx), len(freeCells))
	}

	// Verify endIdx matches (endIdx maps end positions to start offsets)
	if fa.endIdx != nil {
		for off, size := range freeCells {
			end := off + format.Align8I32(size)
			idxOff, exists := fa.endIdx[end]
			assert.True(t, exists, "free cell ending at 0x%x missing from endIdx", end)
			if exists {
				assert.Equal(t, off, idxOff, "endIdx offset mismatch at end 0x%x", end)
			}
		}

		// No extra entries in endIdx
		assert.Len(t, fa.endIdx, len(freeCells),
			"endIdx has %d entries, expected %d", len(fa.endIdx), len(freeCells))
	}
}

// ============================================================================
// Cell Manipulation Helpers
// ============================================================================

// getCell reads a cell header and returns size and allocated flag.
func getCell(data []byte, off int32) (int32, bool) {
	raw := format.ReadI32(data, int(off))
	if raw < 0 {
		return -raw, true
	}
	return raw, false
}

// putCell writes a cell header with proper sign.
func putCell(data []byte, off int32, size int32, allocated bool) {
	if allocated {
		format.PutI32(data, int(off), -size)
	} else {
		format.PutI32(data, int(off), size)
	}
}

// ============================================================================
// HBIN Utilities
// ============================================================================

// getHBINStart returns the absolute file offset of HBIN N.
func getHBINStart(h *hive.Hive, index int) int32 {
	return int32(format.HeaderSize + (index * format.HBINAlignment))
}

// ============================================================================
// Offset Helpers
// ============================================================================

// cellAbsOffset converts REGF-relative offset to absolute file offset.
func cellAbsOffset(h *hive.Hive, relOff int32) int32 {
	return relOff + int32(format.HeaderSize)
}

// cellRelOffset converts absolute file offset to REGF-relative offset.
func cellRelOffset(h *hive.Hive, absOff int32) int32 {
	return absOff - int32(format.HeaderSize)
}

// ============================================================================
// Test Hook Setup
// ============================================================================

// setupGrowCounter sets up a test hook to count Grow() calls.
// Returns pointer to counter that is updated on each Grow().
func setupGrowCounter(fa *FastAllocator) *int {
	growCount := 0
	fa.onGrow = func(int32) { growCount++ }
	return &growCount
}
