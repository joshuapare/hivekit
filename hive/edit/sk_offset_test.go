package edit

import (
	"encoding/binary"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/stretchr/testify/require"
)

// TestSKOffset_StoredAsRelative verifies that createSKCell stores a relative
// HCELL_INDEX in the NK cell's Security field, not an absolute file offset.
//
// Bug: createSKCell was returning ref + format.HeaderSize (absolute) instead of
// ref (relative). This caused ResolveSecurity to double-add the 0x1000 base,
// reading garbage data instead of the SK cell.
func TestSKOffset_StoredAsRelative(t *testing.T) {
	h, fa, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	ke := NewKeyEditor(h, fa, idx, noopDirtyTracker{}).(*keyEditor)

	// Create a key via v1 — this triggers SK cell creation.
	rootRef := h.RootCellOffset()
	childRef, err := ke.createKey(rootRef, "SKOffsetTest")
	require.NoError(t, err, "createKey")
	require.NotEqual(t, uint32(0), childRef, "child ref should be non-zero")

	// Read the child NK cell and get its security offset.
	childPayload, err := h.ResolveCellPayload(childRef)
	require.NoError(t, err, "resolve child NK")
	childNK, err := hive.ParseNK(childPayload)
	require.NoError(t, err, "parse child NK")

	skRef := childNK.SecurityOffsetRel()
	require.NotEqual(t, format.InvalidOffset, skRef, "SK ref should not be InvalidOffset")

	// The critical test: ResolveSecurity should work because the stored
	// offset is a relative HCELL_INDEX. If createSKCell stored an absolute
	// offset, this will fail with "offset out of range" or read garbage.
	sk, err := childNK.ResolveSecurity(h)
	require.NoError(t, err, "ResolveSecurity should succeed on v1-created key")
	require.Positive(t, sk.ReferenceCount(), "SK refcount should be positive")
}

// TestSKRefcount_IncrementUpdatesCorrectField verifies that incrementSKRefCount
// updates the ReferenceCount field (payload offset 0x0C), not the Blink field
// (payload offset 0x08).
//
// Bug: incrementSKRefCount computed the refcount position as offset + 0x0C
// where offset pointed to the cell header. Since the payload starts 4 bytes
// after the cell header, this actually modified payload[0x08] (Blink) instead
// of payload[0x0C] (ReferenceCount).
func TestSKRefcount_IncrementUpdatesCorrectField(t *testing.T) {
	h, fa, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	ke := NewKeyEditor(h, fa, idx, noopDirtyTracker{}).(*keyEditor)

	// Create two keys — the second reuses the same SK cell, triggering
	// incrementSKRefCount.
	rootRef := h.RootCellOffset()
	ref1, err := ke.createKey(rootRef, "Key1")
	require.NoError(t, err)

	ref2, err := ke.createKey(rootRef, "Key2")
	require.NoError(t, err)

	// Both keys should reference the same SK cell (same security descriptor).
	p1, err := h.ResolveCellPayload(ref1)
	require.NoError(t, err)
	nk1, err := hive.ParseNK(p1)
	require.NoError(t, err)

	p2, err := h.ResolveCellPayload(ref2)
	require.NoError(t, err)
	nk2, err := hive.ParseNK(p2)
	require.NoError(t, err)

	require.Equal(t, nk1.SecurityOffsetRel(), nk2.SecurityOffsetRel(),
		"both keys should share the same SK cell")

	skRef := nk1.SecurityOffsetRel()

	// Read the SK cell payload and check that ReferenceCount == 2.
	// (1 from initial creation + 1 from incrementSKRefCount for Key2)
	skPayload, err := h.ResolveCellPayload(skRef)
	require.NoError(t, err, "resolve SK cell")

	refcount := binary.LittleEndian.Uint32(
		skPayload[format.SKReferenceCountOffset : format.SKReferenceCountOffset+4],
	)
	require.Equal(t, uint32(2), refcount,
		"SK refcount should be 2 (one per key); got %d — if 1, incrementSKRefCount wrote to the wrong field", refcount)

	// Also verify the Blink field was NOT corrupted.
	blink := binary.LittleEndian.Uint32(
		skPayload[format.SKBlinkOffset : format.SKBlinkOffset+4],
	)
	// Blink should be InvalidOffset (single SK cell) or a valid SK ref.
	// If Blink was accidentally incremented from InvalidOffset, it would wrap to 0.
	if blink != format.InvalidOffset && blink != skRef {
		require.NotEqual(t, uint32(0), blink,
			"Blink field was corrupted to 0 — incrementSKRefCount is writing to the wrong offset")
	}
}

// TestSKOffset_MatchesAllocatorRef verifies that the SK offset stored in an NK
// cell matches the allocator's returned CellRef (relative), not CellRef + 0x1000.
func TestSKOffset_MatchesAllocatorRef(t *testing.T) {
	h, fa, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	tracker := &allocTracker{inner: fa}
	ke := NewKeyEditor(h, tracker, idx, noopDirtyTracker{}).(*keyEditor)

	rootRef := h.RootCellOffset()
	childRef, err := ke.createKey(rootRef, "TrackTest")
	require.NoError(t, err)

	// Find the SK allocation.
	var skAllocRef uint32
	foundSK := false
	for _, a := range tracker.allocs {
		if a.class == alloc.ClassSK {
			skAllocRef = a.ref
			foundSK = true
			break
		}
	}
	require.True(t, foundSK, "should have seen an SK allocation")

	// Read the NK cell's stored SK offset.
	childPayload, err := h.ResolveCellPayload(childRef)
	require.NoError(t, err)
	childNK, err := hive.ParseNK(childPayload)
	require.NoError(t, err)
	storedSKRef := childNK.SecurityOffsetRel()

	// The stored offset should equal the allocator's CellRef (relative HCELL_INDEX),
	// NOT CellRef + 0x1000.
	require.Equal(t, skAllocRef, storedSKRef,
		"NK should store the relative CellRef (%#x), not absolute (%#x)",
		skAllocRef, skAllocRef+format.HeaderSize)
}

// allocTracker wraps an allocator to record all allocations.
type allocTracker struct {
	inner  alloc.Allocator
	allocs []allocRecord
}

type allocRecord struct {
	ref   uint32
	size  int32
	class alloc.Class
}

func (a *allocTracker) Alloc(need int32, cls alloc.Class) (uint32, []byte, error) {
	ref, buf, err := a.inner.Alloc(need, cls)
	if err == nil {
		a.allocs = append(a.allocs, allocRecord{ref: ref, size: need, class: cls})
	}
	return ref, buf, err
}

func (a *allocTracker) Free(ref uint32) error           { return a.inner.Free(ref) }
func (a *allocTracker) GrowByPages(n int) error          { return a.inner.GrowByPages(n) }
func (a *allocTracker) TruncatePages(n int) error        { return a.inner.TruncatePages(n) }
func (a *allocTracker) Grow(need int32) error            { return a.inner.Grow(need) }

// noopDirtyTracker satisfies dirty.DirtyTracker without doing anything.
type noopDirtyTracker struct{}

func (noopDirtyTracker) Add(int, int) {}
