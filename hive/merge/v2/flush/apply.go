package flush

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
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

// categorize returns the UpdateCategory for an InPlaceUpdate.
// The category is set at creation time by the write phase.
func categorize(u *write.InPlaceUpdate) UpdateCategory {
	return UpdateCategory(u.Category)
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
//   - CheckSum is fully recomputed over the 508-byte header region (O(127 XORs))
func Apply(h *hive.Hive, updates []write.InPlaceUpdate, fa *alloc.FastAllocator, dt dirty.DirtyTracker) error {
	if h == nil {
		return fmt.Errorf("flush: nil hive passed to Apply")
	}
	if fa == nil {
		return fmt.Errorf("flush: nil allocator passed to Apply")
	}
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
			if u.Offset < 0 {
				return fmt.Errorf("flush: negative offset 0x%X", u.Offset)
			}
			end := off + len(u.Data)
			if end > len(data) || end < off { // overflow or bounds check
				return fmt.Errorf("flush: update at offset 0x%X len %d exceeds hive size %d",
					u.Offset, len(u.Data), len(data))
			}
			copy(data[off:end], u.Data)
			if dt != nil {
				dt.Add(off, len(u.Data))
			}
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
	if err := finalizeHeader(h, data, dt); err != nil {
		return fmt.Errorf("flush: finalize header: %w", err)
	}

	return nil
}

// finalizeHeader updates the base block header fields and recomputes the
// checksum using the delta XOR method (O(1) instead of O(127)).
func finalizeHeader(h *hive.Hive, data []byte, dt dirty.DirtyTracker) error {
	// Update header fields before checksum recompute.
	oldSeq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)
	newSeq1 := oldSeq1 + 1
	newLen := uint32(len(data) - format.HeaderSize)
	nowFT := format.TimeToFiletime(time.Now())

	format.PutU32(data, format.REGFPrimarySeqOffset, newSeq1)       // Sequence1 (write-in-progress)
	format.PutU64(data, format.REGFTimeStampOffset, nowFT)           // TimeStamp
	format.PutU32(data, format.REGFDataSizeOffset, newLen)           // Length
	format.PutU32(data, format.REGFSecondarySeqOffset, newSeq1)      // Sequence2 = Sequence1 (write-complete)

	// Recompute checksum over the updated header. Full recompute (127 XORs)
	// is used instead of delta XOR because the stored checksum may have been
	// remapped (0→1 or 0xFFFFFFFF→0xFFFFFFFE), making the delta base
	// ambiguous. 127 XORs is negligible cost.
	cs := ComputeFullChecksum(data[:508])

	format.PutU32(data, format.REGFCheckSumOffset, cs)

	// Mark the entire header page as dirty so it is flushed to disk.
	if dt != nil {
		dt.Add(0, format.HeaderSize)
	}

	return nil
}
