package subkeys

import (
	"fmt"
	"sort"

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
	sort.Slice(sortedEntries, func(i, j int) bool {
		return sortedEntries[i].NameLower < sortedEntries[j].NameLower
	})

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
	// Split entries into chunks of 512
	const chunkSize = 512
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
	idx := sort.Search(len(l.Entries), func(i int) bool {
		return l.Entries[i].NameLower >= nameLower
	})

	if idx < len(l.Entries) && l.Entries[idx].NameLower == nameLower {
		return l.Entries[idx], true
	}

	return Entry{}, false
}
