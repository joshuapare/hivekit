package subkeys

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/format"
)

const (
	// LFThreshold is the maximum number of entries to use LF lists.
	// Above this threshold, use LH (hash leaf) lists.
	LFThreshold = 12

	// RIThreshold is the threshold for using RI (indirect) lists.
	// Above this threshold, entries are split into chunks.
	RIThreshold = 1024

	// bitsPerByte is the number of bits in a byte.
	bitsPerByte = 8

	// bitsPerUint16 is the bit position for the 3rd byte (16 bits).
	bitsPerUint16 = 16

	// bitsPerUint24 is the bit position for the 4th byte (24 bits).
	bitsPerUint24 = 24
)

// Write writes a subkey list to the hive and returns the cell reference.
// The function automatically selects between LF (≤12 entries) and LH (>12 entries).
//
// For LF/LH lists, entries are sorted by NameLower for consistent ordering.
// For lists with >1024 entries, this function may create an RI (indirect) list.
func Write(h *hive.Hive, allocator alloc.Allocator, entries []Entry) (uint32, error) {
	if len(entries) == 0 {
		return format.InvalidOffset, nil // No subkeys
	}

	// Sort entries by lowercase name for consistent ordering
	sortedEntries := make([]Entry, len(entries))
	copy(sortedEntries, entries)
	if !slices.IsSortedFunc(sortedEntries, func(a, b Entry) int {
		return cmp.Compare(a.NameLower, b.NameLower)
	}) {
		slices.SortFunc(sortedEntries, func(a, b Entry) int {
			return cmp.Compare(a.NameLower, b.NameLower)
		})
	}

	// For very large lists (>RIThreshold), use RI (indirect) lists
	// Split into chunks of 512 entries each
	if len(sortedEntries) > RIThreshold {
		return writeRIList(h, allocator, sortedEntries)
	}

	// For small to medium lists, write a single LF or LH list
	if len(sortedEntries) <= LFThreshold {
		return writeLFList(h, allocator, sortedEntries)
	}

	return writeLHList(h, allocator, sortedEntries)
}

// WritePresorted writes a subkey list that is already sorted by NameLower.
// This skips the copy and sort that Write() performs, avoiding O(n) allocation
// and O(n log n) sorting overhead for callers that guarantee sorted input.
//
// Callers MUST ensure entries are sorted by NameLower ascending.
// Used by FlushDeferredSubkeys (sorts before calling) and insertImmediateChild
// (List.Insert maintains sorted order).
func WritePresorted(h *hive.Hive, allocator alloc.Allocator, entries []Entry) (uint32, error) {
	if len(entries) == 0 {
		return format.InvalidOffset, nil
	}

	if len(entries) > RIThreshold {
		return writeRIList(h, allocator, entries)
	}

	if len(entries) <= LFThreshold {
		return writeLFList(h, allocator, entries)
	}

	return writeLHList(h, allocator, entries)
}

// WriteRaw writes a subkey list from RawEntry slice (NKRef + Hash only).
// This is much faster than Write/WritePresorted because it doesn't need names.
//
// IMPORTANT: Entries must already be in sorted order (by name, ascending).
// Use this for delete-only operations where original order is preserved.
//
// The function writes LH format for all lists (LH supports any count).
func WriteRaw(h *hive.Hive, allocator alloc.Allocator, entries []RawEntry) (uint32, error) {
	if len(entries) == 0 {
		return format.InvalidOffset, nil
	}

	if len(entries) > RIThreshold {
		return writeRIListRaw(h, allocator, entries)
	}

	if len(entries) <= LFThreshold {
		return writeRawHashLeafList(allocator, entries, 'f', alloc.ClassLF, "LF")
	}

	return writeRawHashLeafList(allocator, entries, 'h', alloc.ClassLH, "LH")
}

// writeRawHashLeafList writes an LF or LH list from RawEntry slice.
func writeRawHashLeafList(
	allocator alloc.Allocator,
	entries []RawEntry,
	sigByte byte,
	class alloc.Class,
	listType string,
) (uint32, error) {
	count := uint16(len(entries))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.QWORDSize)
	size := payloadSize + format.CellHeaderSize

	ref, buf, err := allocator.Alloc(size, class)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate %s list: %w", listType, err)
	}

	// Write signature
	buf[0] = 'l'
	buf[1] = sigByte

	// Write count
	buf[2] = byte(count)
	buf[3] = byte(count >> bitsPerByte)

	// Write entries
	for i, entry := range entries {
		offset := format.ListHeaderSize + i*format.QWORDSize

		// Write NK offset (4 bytes)
		buf[offset] = byte(entry.NKRef)
		buf[offset+1] = byte(entry.NKRef >> bitsPerByte)
		buf[offset+2] = byte(entry.NKRef >> bitsPerUint16)
		buf[offset+3] = byte(entry.NKRef >> bitsPerUint24)

		// Write hash (4 bytes)
		buf[offset+4] = byte(entry.Hash)
		buf[offset+5] = byte(entry.Hash >> bitsPerByte)
		buf[offset+6] = byte(entry.Hash >> bitsPerUint16)
		buf[offset+7] = byte(entry.Hash >> bitsPerUint24)
	}

	return ref, nil
}

// writeRIListRaw writes an RI (indirect) list from RawEntry slice.
func writeRIListRaw(h *hive.Hive, allocator alloc.Allocator, entries []RawEntry) (uint32, error) {
	const chunkSize = 1024
	numChunks := (len(entries) + chunkSize - 1) / chunkSize

	subListRefs := make([]uint32, 0, numChunks)
	for i := 0; i < len(entries); i += chunkSize {
		end := min(i+chunkSize, len(entries))
		chunk := entries[i:end]
		ref, err := writeRawHashLeafList(allocator, chunk, 'h', alloc.ClassLH, "LH")
		if err != nil {
			return 0, fmt.Errorf("failed to write RI chunk %d: %w", i/chunkSize, err)
		}
		subListRefs = append(subListRefs, ref)
	}

	// Write RI list header
	count := uint16(len(subListRefs))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.DWORDSize)
	size := payloadSize + format.CellHeaderSize

	ref, buf, err := allocator.Alloc(size, alloc.ClassRI)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate RI list: %w", err)
	}

	buf[0] = 'r'
	buf[1] = 'i'
	buf[2] = byte(count)
	buf[3] = byte(count >> bitsPerByte)

	for i, subListRef := range subListRefs {
		offset := format.ListHeaderSize + i*format.DWORDSize
		buf[offset] = byte(subListRef)
		buf[offset+1] = byte(subListRef >> bitsPerByte)
		buf[offset+2] = byte(subListRef >> bitsPerUint16)
		buf[offset+3] = byte(subListRef >> bitsPerUint24)
	}

	return ref, nil
}

// writeHashLeafList is a common helper for writing LF and LH lists.
// Both formats are identical: signature (2) + count (2) + [offset (4) + hash (4)] * count.
func writeHashLeafList(
	allocator alloc.Allocator,
	entries []Entry,
	sigByte byte,
	class alloc.Class,
	listType string,
) (uint32, error) {
	count := uint16(len(entries))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.QWORDSize)
	// Allocator expects total cell size (including header)
	size := payloadSize + format.CellHeaderSize

	// Allocate cell
	ref, buf, err := allocator.Alloc(size, class)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate %s list: %w", listType, err)
	}

	// Write signature (first byte is always 'l', second byte varies)
	buf[0] = 'l'
	buf[1] = sigByte

	// Write count
	buf[2] = byte(count)
	buf[3] = byte(count >> bitsPerByte)

	// Write entries
	for i, entry := range entries {
		offset := format.ListHeaderSize + i*format.QWORDSize

		// Write NK offset (4 bytes)
		buf[offset] = byte(entry.NKRef)
		buf[offset+1] = byte(entry.NKRef >> bitsPerByte)
		buf[offset+2] = byte(entry.NKRef >> bitsPerUint16)
		buf[offset+3] = byte(entry.NKRef >> bitsPerUint24)

		// Write hash (4 bytes)
		// Use cached hash if available, otherwise compute it
		hash := entry.Hash
		if hash == 0 {
			// Fallback: compute hash if not cached (e.g., test-created entries)
			hash = Hash(entry.NameLower)
		}
		buf[offset+4] = byte(hash)
		buf[offset+5] = byte(hash >> bitsPerByte)
		buf[offset+6] = byte(hash >> bitsPerUint16)
		buf[offset+7] = byte(hash >> bitsPerUint24)
	}

	return ref, nil
}

// writeLFList writes an LF (fast leaf) list.
func writeLFList(_ *hive.Hive, allocator alloc.Allocator, entries []Entry) (uint32, error) {
	return writeHashLeafList(allocator, entries, 'f', alloc.ClassLF, "LF")
}

// writeLHList writes an LH (hash leaf) list.
// LH is identical to LF but uses a different signature.
func writeLHList(_ *hive.Hive, allocator alloc.Allocator, entries []Entry) (uint32, error) {
	return writeHashLeafList(allocator, entries, 'h', alloc.ClassLH, "LH")
}

// writeRIList writes an RI (indirect) list which references multiple sub-lists.
func writeRIList(h *hive.Hive, allocator alloc.Allocator, entries []Entry) (uint32, error) {
	// Split entries into chunks of 1024 to match typical Windows hive chunk sizes.
	// This produces ~8200-byte cells that can reuse space from freed original cells.
	const chunkSize = 1024
	numChunks := (len(entries) + chunkSize - 1) / chunkSize

	// Write each chunk as an LH list
	subListRefs := make([]uint32, 0, numChunks)
	for i := 0; i < len(entries); i += chunkSize {
		end := min(i+chunkSize, len(entries))

		chunk := entries[i:end]
		ref, err := writeLHList(h, allocator, chunk)
		if err != nil {
			return 0, fmt.Errorf("failed to write RI chunk %d: %w", i/chunkSize, err)
		}

		subListRefs = append(subListRefs, ref)
	}

	// Write RI list: signature (2) + count (2) + [offset (4)] * count
	count := uint16(len(subListRefs))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.DWORDSize)
	// Allocator expects total cell size (including header)
	size := payloadSize + format.CellHeaderSize

	ref, buf, err := allocator.Alloc(size, alloc.ClassRI)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate RI list: %w", err)
	}

	// Verify buffer size
	if len(buf) < int(payloadSize) {
		return 0, fmt.Errorf(
			"allocator returned buffer of size %d, requested %d",
			len(buf),
			payloadSize,
		)
	}

	// Write signature
	buf[0] = 'r'
	buf[1] = 'i'

	// Write count
	buf[2] = byte(count)
	buf[3] = byte(count >> bitsPerByte)

	// Write sub-list references
	for i, subListRef := range subListRefs {
		offset := format.ListHeaderSize + i*format.DWORDSize
		buf[offset] = byte(subListRef)
		buf[offset+1] = byte(subListRef >> bitsPerByte)
		buf[offset+2] = byte(subListRef >> bitsPerUint16)
		buf[offset+3] = byte(subListRef >> bitsPerUint24)
	}

	return ref, nil
}

// Insert adds an entry to the list in sorted order.
// Returns a new list with the entry inserted.
func (l *List) Insert(entry Entry) *List {
	if l == nil {
		return &List{Entries: []Entry{entry}}
	}

	// Check for duplicate
	for _, e := range l.Entries {
		if e.NameLower == entry.NameLower {
			// Replace existing entry
			newEntries := make([]Entry, len(l.Entries))
			copy(newEntries, l.Entries)
			for i := range newEntries {
				if newEntries[i].NameLower == entry.NameLower {
					newEntries[i] = entry
					break
				}
			}
			return &List{Entries: newEntries}
		}
	}

	// Insert in sorted order
	newEntries := make([]Entry, 0, len(l.Entries)+1)
	inserted := false
	for _, e := range l.Entries {
		if !inserted && entry.NameLower < e.NameLower {
			newEntries = append(newEntries, entry)
			inserted = true
		}
		newEntries = append(newEntries, e)
	}
	if !inserted {
		newEntries = append(newEntries, entry)
	}

	return &List{Entries: newEntries}
}

// Remove removes an entry from the list by name.
// Returns a new list with the entry removed.
func (l *List) Remove(nameLower string) *List {
	if l == nil {
		return nil
	}

	newEntries := make([]Entry, 0, len(l.Entries))
	for _, e := range l.Entries {
		if e.NameLower != nameLower {
			newEntries = append(newEntries, e)
		}
	}

	return &List{Entries: newEntries}
}

// Find searches for an entry by name (case-insensitive).
func (l *List) Find(nameLower string) (Entry, bool) {
	if l == nil {
		return Entry{}, false
	}

	// Binary search since entries are sorted
	idx, found := slices.BinarySearchFunc(l.Entries, nameLower, func(e Entry, target string) int {
		return cmp.Compare(e.NameLower, target)
	})

	if found {
		return l.Entries[idx], true
	}

	return Entry{}, false
}

// RefHashEntry is a lightweight entry containing only the NKRef and Hash.
// Used by RemoveByRef to avoid parsing NK cells.
type RefHashEntry struct {
	NKRef uint32
	Hash  uint32
}

// RemoveByRef removes an entry from a subkey list by NKRef without parsing NK cells.
// This is O(n) in list size but avoids the O(n) NK cell parsing that Read() does.
// Returns the new list reference, the new count, and any error.
// If the list becomes empty, returns format.InvalidOffset.
func RemoveByRef(h *hive.Hive, allocator alloc.Allocator, listRef uint32, targetRef uint32) (uint32, int, error) {
	if listRef == 0 || listRef == format.InvalidOffset {
		return format.InvalidOffset, 0, nil
	}

	// Read the list cell payload
	payload, err := h.ResolveCellPayload(listRef)
	if err != nil {
		return 0, 0, fmt.Errorf("resolve list cell: %w", err)
	}

	if len(payload) < format.ListHeaderSize {
		return 0, 0, fmt.Errorf("list payload too small")
	}

	// Check for RI (indirect) list
	if payload[0] == 'r' && payload[1] == 'i' {
		return removeByRefRI(h, allocator, payload, targetRef)
	}

	// Handle direct lists (LF, LH, LI)
	return removeByRefDirect(h, allocator, payload, targetRef)
}

// removeByRefDirect handles LF, LH, and LI lists.
func removeByRefDirect(_ *hive.Hive, allocator alloc.Allocator, payload []byte, targetRef uint32) (uint32, int, error) {
	sig0, sig1 := payload[0], payload[1]
	count := int(payload[2]) | int(payload[3])<<8

	var entries []RefHashEntry
	var isLI bool

	switch {
	case sig0 == 'l' && (sig1 == 'f' || sig1 == 'h'):
		// LF or LH: 8 bytes per entry (offset + hash)
		entries = make([]RefHashEntry, 0, count)
		for i := 0; i < count; i++ {
			off := format.ListHeaderSize + i*format.QWORDSize
			if off+8 > len(payload) {
				break
			}
			nkRef := uint32(payload[off]) | uint32(payload[off+1])<<8 | uint32(payload[off+2])<<16 | uint32(payload[off+3])<<24
			hash := uint32(payload[off+4]) | uint32(payload[off+5])<<8 | uint32(payload[off+6])<<16 | uint32(payload[off+7])<<24
			if nkRef != targetRef {
				entries = append(entries, RefHashEntry{NKRef: nkRef, Hash: hash})
			}
		}
	case sig0 == 'l' && sig1 == 'i':
		// LI: 4 bytes per entry (offset only, no hash)
		isLI = true
		entries = make([]RefHashEntry, 0, count)
		for i := 0; i < count; i++ {
			off := format.ListHeaderSize + i*format.DWORDSize
			if off+4 > len(payload) {
				break
			}
			nkRef := uint32(payload[off]) | uint32(payload[off+1])<<8 | uint32(payload[off+2])<<16 | uint32(payload[off+3])<<24
			if nkRef != targetRef {
				entries = append(entries, RefHashEntry{NKRef: nkRef, Hash: 0})
			}
		}
	default:
		return 0, 0, fmt.Errorf("unknown list signature: %c%c", sig0, sig1)
	}

	if len(entries) == 0 {
		return format.InvalidOffset, 0, nil
	}

	// Write the filtered list
	if isLI {
		return writeRefHashListLI(allocator, entries)
	}
	// Use LF for small lists, LH for larger
	if len(entries) <= LFThreshold {
		return writeRefHashListLFLH(allocator, entries, 'f', alloc.ClassLF)
	}
	return writeRefHashListLFLH(allocator, entries, 'h', alloc.ClassLH)
}

// removeByRefRI handles RI (indirect) lists.
func removeByRefRI(h *hive.Hive, allocator alloc.Allocator, payload []byte, targetRef uint32) (uint32, int, error) {
	count := int(payload[2]) | int(payload[3])<<8

	// Collect all entries from all sub-lists, filtering out targetRef
	var allEntries []RefHashEntry

	for i := 0; i < count; i++ {
		off := format.ListHeaderSize + i*format.DWORDSize
		if off+4 > len(payload) {
			break
		}
		subListRef := uint32(payload[off]) | uint32(payload[off+1])<<8 | uint32(payload[off+2])<<16 | uint32(payload[off+3])<<24

		subPayload, err := h.ResolveCellPayload(subListRef)
		if err != nil {
			continue
		}

		// Sub-lists are always LH
		subCount := int(subPayload[2]) | int(subPayload[3])<<8
		for j := 0; j < subCount; j++ {
			entryOff := format.ListHeaderSize + j*format.QWORDSize
			if entryOff+8 > len(subPayload) {
				break
			}
			nkRef := uint32(subPayload[entryOff]) | uint32(subPayload[entryOff+1])<<8 | uint32(subPayload[entryOff+2])<<16 | uint32(subPayload[entryOff+3])<<24
			hash := uint32(subPayload[entryOff+4]) | uint32(subPayload[entryOff+5])<<8 | uint32(subPayload[entryOff+6])<<16 | uint32(subPayload[entryOff+7])<<24
			if nkRef != targetRef {
				allEntries = append(allEntries, RefHashEntry{NKRef: nkRef, Hash: hash})
			}
		}
	}

	if len(allEntries) == 0 {
		return format.InvalidOffset, 0, nil
	}

	// Decide output format based on count
	if len(allEntries) <= RIThreshold {
		if len(allEntries) <= LFThreshold {
			return writeRefHashListLFLH(allocator, allEntries, 'f', alloc.ClassLF)
		}
		return writeRefHashListLFLH(allocator, allEntries, 'h', alloc.ClassLH)
	}

	// Still need RI - split into chunks
	return writeRefHashListRI(allocator, allEntries)
}

// writeRefHashListLFLH writes an LF or LH list from RefHashEntry slice.
func writeRefHashListLFLH(allocator alloc.Allocator, entries []RefHashEntry, sigByte byte, class alloc.Class) (uint32, int, error) {
	count := uint16(len(entries))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.QWORDSize)
	size := payloadSize + format.CellHeaderSize

	ref, buf, err := allocator.Alloc(size, class)
	if err != nil {
		return 0, 0, err
	}

	buf[0] = 'l'
	buf[1] = sigByte
	buf[2] = byte(count)
	buf[3] = byte(count >> 8)

	for i, e := range entries {
		off := format.ListHeaderSize + i*format.QWORDSize
		buf[off] = byte(e.NKRef)
		buf[off+1] = byte(e.NKRef >> 8)
		buf[off+2] = byte(e.NKRef >> 16)
		buf[off+3] = byte(e.NKRef >> 24)
		buf[off+4] = byte(e.Hash)
		buf[off+5] = byte(e.Hash >> 8)
		buf[off+6] = byte(e.Hash >> 16)
		buf[off+7] = byte(e.Hash >> 24)
	}

	return ref, len(entries), nil
}

// writeRefHashListLI writes an LI list from RefHashEntry slice.
func writeRefHashListLI(allocator alloc.Allocator, entries []RefHashEntry) (uint32, int, error) {
	count := uint16(len(entries))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.DWORDSize)
	size := payloadSize + format.CellHeaderSize

	ref, buf, err := allocator.Alloc(size, alloc.ClassLI)
	if err != nil {
		return 0, 0, err
	}

	buf[0] = 'l'
	buf[1] = 'i'
	buf[2] = byte(count)
	buf[3] = byte(count >> 8)

	for i, e := range entries {
		off := format.ListHeaderSize + i*format.DWORDSize
		buf[off] = byte(e.NKRef)
		buf[off+1] = byte(e.NKRef >> 8)
		buf[off+2] = byte(e.NKRef >> 16)
		buf[off+3] = byte(e.NKRef >> 24)
	}

	return ref, len(entries), nil
}

// writeRefHashListRI writes an RI list from RefHashEntry slice.
func writeRefHashListRI(allocator alloc.Allocator, entries []RefHashEntry) (uint32, int, error) {
	const chunkSize = 1024
	numChunks := (len(entries) + chunkSize - 1) / chunkSize

	subListRefs := make([]uint32, 0, numChunks)
	for i := 0; i < len(entries); i += chunkSize {
		end := i + chunkSize
		if end > len(entries) {
			end = len(entries)
		}
		chunk := entries[i:end]
		ref, _, err := writeRefHashListLFLH(allocator, chunk, 'h', alloc.ClassLH)
		if err != nil {
			return 0, 0, err
		}
		subListRefs = append(subListRefs, ref)
	}

	// Write RI header
	count := uint16(len(subListRefs))
	payloadSize := int32(format.ListHeaderSize + int(count)*format.DWORDSize)
	size := payloadSize + format.CellHeaderSize

	ref, buf, err := allocator.Alloc(size, alloc.ClassRI)
	if err != nil {
		return 0, 0, err
	}

	buf[0] = 'r'
	buf[1] = 'i'
	buf[2] = byte(count)
	buf[3] = byte(count >> 8)

	for i, subRef := range subListRefs {
		off := format.ListHeaderSize + i*format.DWORDSize
		buf[off] = byte(subRef)
		buf[off+1] = byte(subRef >> 8)
		buf[off+2] = byte(subRef >> 16)
		buf[off+3] = byte(subRef >> 24)
	}

	return ref, len(entries), nil
}
