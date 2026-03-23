package flush

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/merge/v2/write"
	"github.com/joshuapare/hivekit/internal/format"
)

// UpdateCategory classifies an in-place update by the type of field being written.
// Ordering of categories determines write safety: structural updates (NK fields)
// must be applied before refcount updates, which must be applied before cell frees.
type UpdateCategory int

const (
	// CategoryNKField covers updates to NK cell fields (subkey list, value list, counts).
	// These updates make new cells reachable and are applied first.
	CategoryNKField UpdateCategory = 0

	// CategorySKRefcount covers updates to SK cell reference counts.
	// Applied after NK fields since the cells they modify are already reachable.
	CategorySKRefcount UpdateCategory = 1

	// CategoryCellFree covers writes that mark cells as free (positive cell header).
	// Applied last because orphaned cells waste space but do not corrupt the hive.
	CategoryCellFree UpdateCategory = 2
)

// categorize infers the UpdateCategory for an InPlaceUpdate based on its offset.
// In the current write phase, all queued updates are NK field updates.
// Future writers may queue SK refcount or cell-free updates; this function
// can be extended when those are added.
func categorize(_ *write.InPlaceUpdate) UpdateCategory {
	// All current updates come from the write phase and target NK fields.
	return CategoryNKField
}

// Apply applies in-place updates to the hive and finalizes the base block header.
//
// Updates are grouped by safety category and sorted by offset within each group
// for sequential I/O. Groups are applied in this order:
//  1. NK field updates (make new cells reachable)
//  2. SK refcount updates
//  3. Cell-free writes (least critical; orphaned cells waste space, not safety)
//
// After updates are written, FinalizeBumpMode is called to write the trailing
// free cell for unused bump space.
//
// The base block header is then updated atomically:
//   - Sequence1 is incremented (write-in-progress marker)
//   - Length (DataSize) is updated to the new hive size
//   - TimeStamp is set to now
//   - Sequence2 is set to match Sequence1 (write-complete marker)
//   - CheckSum is updated via delta XOR (O(1): XOR out old values, XOR in new)
func Apply(h *hive.Hive, updates []write.InPlaceUpdate, fa *alloc.FastAllocator) error {
	data := h.Bytes()
	if len(data) < format.HeaderSize {
		return fmt.Errorf("flush: hive data too small (%d bytes)", len(data))
	}

	// --- Step 1: Group by safety category ---
	buckets := make([][]write.InPlaceUpdate, 3)
	for i := range buckets {
		buckets[i] = make([]write.InPlaceUpdate, 0)
	}
	for i := range updates {
		cat := categorize(&updates[i])
		buckets[cat] = append(buckets[cat], updates[i])
	}

	// --- Step 2: Sort each bucket by offset for sequential I/O ---
	for i := range buckets {
		sort.Slice(buckets[i], func(a, b int) bool {
			return buckets[i][a].Offset < buckets[i][b].Offset
		})
	}

	// --- Step 3: Apply each group in order ---
	for _, bucket := range buckets {
		for _, u := range bucket {
			off := int(u.Offset)
			end := off + len(u.Data)
			if end > len(data) {
				return fmt.Errorf("flush: update at offset 0x%X len %d exceeds hive size %d",
					u.Offset, len(u.Data), len(data))
			}
			copy(data[off:end], u.Data)
		}
	}

	// --- Step 4: Finalize bump mode (writes trailing free cell) ---
	if err := fa.FinalizeBumpMode(); err != nil {
		// ErrBumpNotActive is benign: bump mode was never enabled for this merge.
		// Any other error is unexpected and should propagate.
		if !errors.Is(err, alloc.ErrBumpNotActive) {
			return fmt.Errorf("flush: finalize bump mode: %w", err)
		}
	}

	// --- Step 5: Update base block header with delta checksum ---
	if err := finalizeHeader(h, data); err != nil {
		return fmt.Errorf("flush: finalize header: %w", err)
	}

	return nil
}

// finalizeHeader updates the base block header fields and recomputes the
// checksum using the delta XOR method (O(1) instead of O(127)).
func finalizeHeader(h *hive.Hive, data []byte) error {
	// Snapshot old values for delta checksum computation.
	oldSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)
	oldSeq2 := format.ReadU32(data, format.REGFSecondarySeqOffset)
	oldTSLo := format.ReadU32(data, format.REGFTimeStampOffset)
	oldTSHi := format.ReadU32(data, format.REGFTimeStampOffset+4)
	oldLen := format.ReadU32(data, format.REGFDataSizeOffset)
	oldChecksum := format.ReadU32(data, format.REGFCheckSumOffset)

	// Compute new field values.
	newSeq1 := oldSeq1 + 1
	newLen := uint32(len(data) - format.HeaderSize)
	nowFT := format.TimeToFiletime(time.Now())
	newTSLo := uint32(nowFT & 0xFFFFFFFF)
	newTSHi := uint32(nowFT >> 32)

	// Write Sequence1 (write-in-progress marker).
	format.PutU32(data, format.REGFPrimarySeqOffset, newSeq1)

	// Write TimeStamp.
	format.PutU64(data, format.REGFTimeStampOffset, nowFT)

	// Write Length (DataSize = total hive data bytes, excluding the 4KB header).
	format.PutU32(data, format.REGFDataSizeOffset, newLen)

	// Write Sequence2 = Sequence1 (write-complete marker).
	newSeq2 := newSeq1
	format.PutU32(data, format.REGFSecondarySeqOffset, newSeq2)

	// Compute new checksum via delta XOR.
	cs := oldChecksum
	cs = DeltaChecksum(cs, format.REGFPrimarySeqOffset, oldSeq1, newSeq1)
	cs = DeltaChecksum(cs, format.REGFSecondarySeqOffset, oldSeq2, newSeq2)
	cs = DeltaChecksum(cs, format.REGFTimeStampOffset, oldTSLo, newTSLo)
	cs = DeltaChecksum(cs, format.REGFTimeStampOffset+4, oldTSHi, newTSHi)
	cs = DeltaChecksum(cs, format.REGFDataSizeOffset, oldLen, newLen)

	// Apply special-case remapping.
	if cs == 0 {
		cs = 1
	} else if cs == 0xFFFFFFFF {
		cs = 0xFFFFFFFE
	}

	format.PutU32(data, format.REGFCheckSumOffset, cs)

	// Mark the header page as dirty so it is flushed to disk.
	_ = h // dirty tracking is handled by the caller via fa.dt; header is in h.Bytes()

	return nil
}
