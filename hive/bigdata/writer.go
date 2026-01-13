package bigdata

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// maxUint16 is the maximum value for a uint16 (2^16 - 1).
	maxUint16 = 65535
)

// Writer handles storing large value data using the DB (big-data) format.
type Writer struct {
	h         *hive.Hive
	allocator alloc.Allocator
	dt        dirty.DirtyTracker
}

// NewWriter creates a new big-data writer.
func NewWriter(h *hive.Hive, allocator alloc.Allocator, dt dirty.DirtyTracker) *Writer {
	return &Writer{
		h:         h,
		allocator: allocator,
		dt:        dt,
	}
}

// Store writes large data using the DB format with chunking.
// Returns the DB header cell reference.
//
// CRITICAL: This function uses the Reserve-then-write pattern to avoid stale slice issues.
// On macOS, Append() remaps the entire file to a new virtual address, invalidating any
// previously returned []byte slices from Alloc(). To prevent this, we:
// 1. Pre-calculate total space needed for ALL cells
// 2. Reserve space with a single Grow() call
// 3. Allocate and write all cells WITHOUT any further Grow() calls
//
// Process:
// 1. Calculate total space needed (DB header + blocklist + all data blocks)
// 2. Reserve space upfront to avoid remapping during writes
// 3. Allocate and write data cells
// 4. Allocate and write blocklist cell
// 5. Allocate and write DB header cell
// 6. Return DB header cell reference.
func (w *Writer) Store(data []byte) (uint32, error) {
	if len(data) == 0 {
		return 0, ErrEmptyData
	}

	// Calculate number of chunks needed
	numBlocks := (len(data) + MaxBlockSize - 1) / MaxBlockSize
	if numBlocks > maxUint16 {
		return 0, fmt.Errorf("data too large: %d blocks (max %d)", numBlocks, maxUint16)
	}

	// CRITICAL: Pre-calculate total space needed to avoid multiple grow calls
	// which would invalidate previously returned payload slices.
	totalNeed := w.calculateTotalNeed(data, numBlocks)

	// Reserve space upfront with a single GrowByPages() call
	// After this, all subsequent Alloc() calls will NOT trigger remapping
	// Round up to nearest page (4KB)
	numPages := (int(totalNeed) + 4095) / 4096
	if err := w.allocator.GrowByPages(numPages); err != nil {
		return 0, fmt.Errorf("failed to reserve space: %w", err)
	}

	// Step 1: Allocate and write data cells
	// No more Grow() calls will happen, so payload slices remain valid
	blockRefs := make([]uint32, numBlocks)
	for i := range numBlocks {
		start := i * MaxBlockSize
		end := start + MaxBlockSize
		if end > len(data) {
			end = len(data)
		}

		blockData := data[start:end]
		ref, err := w.writeDataBlock(blockData)
		if err != nil {
			return 0, fmt.Errorf("failed to write data block %d: %w", i, err)
		}

		blockRefs[i] = ref
	}

	// Step 2: Allocate and write blocklist cell
	blocklistRef, err := w.writeBlocklist(blockRefs)
	if err != nil {
		return 0, fmt.Errorf("failed to write blocklist: %w", err)
	}

	// Step 3: Allocate and write DB header cell
	dbHeaderRef, err := w.writeDBHeader(uint16(numBlocks), blocklistRef)
	if err != nil {
		return 0, fmt.Errorf("failed to write DB header: %w", err)
	}

	return dbHeaderRef, nil
}

// calculateTotalNeed computes the total space needed for all cells.
// This includes proper alignment for each cell.
func (w *Writer) calculateTotalNeed(data []byte, numBlocks int) int32 {
	var total int32

	// DB header: signature (2) + count (2) + blocklist ref (4)
	dbPayload := DBHeaderSize
	total += int32(align8(format.CellHeaderSize + dbPayload))

	// Blocklist: array of uint32 block references
	blocklistPayload := numBlocks * format.DWORDSize
	total += int32(align8(format.CellHeaderSize + blocklistPayload))

	// Data blocks: each chunk with its own cell header
	remaining := len(data)
	for remaining > 0 {
		chunk := remaining
		if chunk > MaxBlockSize {
			chunk = MaxBlockSize
		}
		total += int32(align8(format.CellHeaderSize + chunk))
		remaining -= chunk
	}

	return total
}

// align8 aligns size to 8-byte boundary (required for registry cells).
func align8(size int) int {
	return (size + format.CellAlignmentMask) &^ format.CellAlignmentMask
}

// writeDataBlock allocates a cell and writes a data block.
func (w *Writer) writeDataBlock(data []byte) (uint32, error) {
	if len(data) > MaxBlockSize {
		return 0, ErrBlockTooBig
	}

	// CRITICAL: Add header size
	payloadSize := len(data)
	totalSize := int32(payloadSize + format.CellHeaderSize)

	// Allocate cell for data block
	ref, buf, err := w.allocator.Alloc(totalSize, alloc.ClassRD)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate data block: %w", err)
	}

	// Verify buffer size
	if len(buf) < payloadSize {
		return 0, fmt.Errorf("allocator returned buffer of size %d, need %d", len(buf), payloadSize)
	}

	// Copy data to buffer
	copy(buf, data)

	// Mark data block as dirty so it's flushed to disk
	w.markCellDirty(ref)

	return ref, nil
}

// writeBlocklist allocates a cell and writes the blocklist (array of HCELL_INDEX).
func (w *Writer) writeBlocklist(blockRefs []uint32) (uint32, error) {
	// CRITICAL: Add header size
	payloadSize := len(blockRefs) * format.DWORDSize
	totalSize := int32(payloadSize + format.CellHeaderSize)

	// Allocate cell for blocklist
	ref, buf, err := w.allocator.Alloc(totalSize, alloc.ClassBL)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate blocklist: %w", err)
	}

	// Verify buffer size
	if len(buf) < payloadSize {
		return 0, fmt.Errorf("allocator returned buffer of size %d, need %d", len(buf), payloadSize)
	}

	// Write blocklist
	err = WriteBlocklist(buf, blockRefs)
	if err != nil {
		return 0, err
	}

	// Mark blocklist as dirty so it's flushed to disk
	w.markCellDirty(ref)

	return ref, nil
}

// writeDBHeader allocates a cell and writes the DB header.
func (w *Writer) writeDBHeader(count uint16, blocklistRef uint32) (uint32, error) {
	// CRITICAL: Add header size
	payloadSize := DBHeaderSize
	totalSize := int32(payloadSize + format.CellHeaderSize)

	// Allocate cell for DB header
	ref, buf, err := w.allocator.Alloc(totalSize, alloc.ClassDB)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate DB header: %w", err)
	}

	// Verify buffer size
	if len(buf) < payloadSize {
		return 0, fmt.Errorf("allocator returned buffer of size %d, need %d", len(buf), payloadSize)
	}

	// Write DB header
	err = WriteDBHeader(buf, count, blocklistRef)
	if err != nil {
		return 0, err
	}

	// Mark DB header as dirty so it's flushed to disk
	w.markCellDirty(ref)

	return ref, nil
}

// markCellDirty marks a cell as dirty in the dirty tracker.
func (w *Writer) markCellDirty(ref uint32) {
	data := w.h.Bytes()
	offset := format.HeaderSize + int(ref)

	// Read cell size (including header)
	cellSize := int32(format.ReadU32(data, offset))
	if cellSize < 0 {
		cellSize = -cellSize
	}

	// Mark the entire cell (including header) as dirty
	w.dt.Add(offset, int(cellSize))
}
