package subkeys

import (
	"errors"
	"fmt"
	"sync"
	"unicode"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/buf"
	"github.com/joshuapare/hivekit/internal/format"
)

// Keep utf16 imported for the old functions (used by tests)

// byteBufferPool pools byte buffers for string operations to reduce GC pressure.
var byteBufferPool = sync.Pool{
	New: func() any {
		// Pre-allocate 256 bytes (typical subkey name length)
		b := make([]byte, 0, 256)
		return &b
	},
}


const (
	// signatureSize is the size of cell signature fields (e.g., "lf", "lh", "ri").
	signatureSize = 2

	// listCountOffset is the offset to the count field in list headers.
	listCountOffset = 2

	// utf16BytesPerChar is the number of bytes per UTF-16 character.
	utf16BytesPerChar = 2

	// asciiMax is the maximum value for an ASCII character.
	asciiMax = 0x7F
)

// win1252Table maps Windows-1252 bytes 0x80-0x9F to Unicode code points.
// Bytes 0x00-0x7F are ASCII (identity mapping).
// Bytes 0xA0-0xFF map to U+00A0..U+00FF (identity mapping).
// Only 0x80-0x9F need special handling.
var win1252Table = [32]rune{
	0x20AC, // 0x80 → € (Euro sign)
	0x0081, // 0x81 → unused control (keep as-is)
	0x201A, // 0x82 → ‚ (single low-9 quotation mark)
	0x0192, // 0x83 → ƒ (Latin small letter f with hook)
	0x201E, // 0x84 → „ (double low-9 quotation mark)
	0x2026, // 0x85 → … (horizontal ellipsis)
	0x2020, // 0x86 → † (dagger)
	0x2021, // 0x87 → ‡ (double dagger)
	0x02C6, // 0x88 → ˆ (modifier letter circumflex accent)
	0x2030, // 0x89 → ‰ (per mille sign)
	0x0160, // 0x8A → Š (Latin capital letter S with caron)
	0x2039, // 0x8B → ‹ (single left-pointing angle quotation mark)
	0x0152, // 0x8C → Œ (Latin capital ligature OE)
	0x008D, // 0x8D → unused control
	0x017D, // 0x8E → Ž (Latin capital letter Z with caron)
	0x008F, // 0x8F → unused control
	0x0090, // 0x90 → unused control
	0x2018, // 0x91 → ' (left single quotation mark)
	0x2019, // 0x92 → ' (right single quotation mark)
	0x201C, // 0x93 → " (left double quotation mark)
	0x201D, // 0x94 → " (right double quotation mark)
	0x2022, // 0x95 → • (bullet)
	0x2013, // 0x96 → – (en dash)
	0x2014, // 0x97 → — (em dash)
	0x02DC, // 0x98 → ˜ (small tilde)
	0x2122, // 0x99 → ™ (trade mark sign)
	0x0161, // 0x9A → š (Latin small letter s with caron)
	0x203A, // 0x9B → › (single right-pointing angle quotation mark)
	0x0153, // 0x9C → œ (Latin small ligature oe)
	0x009D, // 0x9D → unused control
	0x017E, // 0x9E → ž (Latin small letter z with caron)
	0x0178, // 0x9F → Ÿ (Latin capital letter Y with diaeresis)
}

// ReadOffsets reads a subkey list and returns only the NK cell offsets.
// This is a lightweight alternative to Read() that avoids decoding NK names.
// Use this when you only need the offsets and will decode names later.
//
// listRef is the HCELL_INDEX offset to the list cell.
// Returns a slice of NK cell offsets.
func ReadOffsets(h *hive.Hive, listRef uint32) ([]uint32, error) {
	if listRef == 0 || listRef == format.InvalidOffset {
		return nil, nil
	}

	payload, err := resolveCell(h, listRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve list cell: %w", err)
	}

	// Check if it's an RI (indirect) list
	if len(payload) >= signatureSize && payload[0] == 'r' && payload[1] == 'i' {
		return readRIListOffsets(h, payload)
	}

	// Read direct list offsets
	return readDirectListOffsets(payload)
}

// ReadOffsetsInto reads a subkey list and appends NK cell offsets to the provided buffer.
// This is the zero-allocation version of ReadOffsets for hot paths.
// The buffer is reset to zero length before appending and returned (may be reallocated if capacity exceeded).
//
// listRef is the HCELL_INDEX offset to the list cell.
// Returns the updated buffer slice (caller should assign back: dst = ReadOffsetsInto(h, ref, dst)).
func ReadOffsetsInto(h *hive.Hive, listRef uint32, dst []uint32) ([]uint32, error) {
	// Reset buffer to zero length (keep capacity)
	dst = dst[:0]

	if listRef == 0 || listRef == format.InvalidOffset {
		return dst, nil
	}

	payload, err := resolveCell(h, listRef)
	if err != nil {
		return dst, fmt.Errorf("failed to resolve list cell: %w", err)
	}

	// Check if it's an RI (indirect) list
	if len(payload) >= signatureSize && payload[0] == 'r' && payload[1] == 'i' {
		return readRIListOffsetsInto(h, payload, dst)
	}

	// Read direct list offsets
	return readDirectListOffsetsInto(payload, dst)
}

// readDirectListOffsets reads NK offsets from a single LF, LH, or LI list.
func readDirectListOffsets(payload []byte) ([]uint32, error) {
	if len(payload) < format.ListHeaderSize {
		return nil, ErrTruncated
	}

	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return nil, fmt.Errorf("list count: %w", err)
	}

	// Check signature bytes directly
	sig0, sig1 := payload[0], payload[1]
	if (sig0 == 'l' && sig1 == 'f') || (sig0 == 'l' && sig1 == 'h') {
		return readLFLH(payload, count)
	} else if sig0 == 'l' && sig1 == 'i' {
		return readLI(payload, count)
	}

	return nil, ErrInvalidSignature
}

// readDirectListOffsetsInto reads NK offsets from a single LF, LH, or LI list into the provided buffer.
func readDirectListOffsetsInto(payload []byte, dst []uint32) ([]uint32, error) {
	if len(payload) < format.ListHeaderSize {
		return dst, ErrTruncated
	}

	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return dst, fmt.Errorf("list count: %w", err)
	}

	// Check signature bytes directly
	sig0, sig1 := payload[0], payload[1]
	if (sig0 == 'l' && sig1 == 'f') || (sig0 == 'l' && sig1 == 'h') {
		return readLFLHInto(payload, count, dst)
	} else if sig0 == 'l' && sig1 == 'i' {
		return readLIInto(payload, count, dst)
	}

	return dst, ErrInvalidSignature
}

// readRIListOffsets reads NK offsets from an RI (indirect) list.
func readRIListOffsets(h *hive.Hive, payload []byte) ([]uint32, error) {
	if len(payload) < format.ListHeaderSize {
		return nil, ErrInvalidRI
	}

	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return nil, fmt.Errorf("ri list count: %w", err)
	}

	if uint32(count) > format.MaxRIListCount {
		return nil, fmt.Errorf("ri list count %d exceeds limit: %w", count, format.ErrSanityLimit)
	}

	if _, boundsErr := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.DWORDSize); boundsErr != nil {
		return nil, ErrTruncated
	}

	// Collect offsets from all sub-lists
	var allOffsets []uint32

	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.DWORDSize
		subListRef, readErr := format.CheckedReadU32(payload, offset)
		if readErr != nil {
			continue
		}

		subOffsets, err := ReadOffsets(h, subListRef)
		if err != nil {
			continue
		}

		allOffsets = append(allOffsets, subOffsets...)
	}

	return allOffsets, nil
}

// readRIListOffsetsInto reads NK offsets from an RI (indirect) list into the provided buffer.
func readRIListOffsetsInto(h *hive.Hive, payload []byte, dst []uint32) ([]uint32, error) {
	if len(payload) < format.ListHeaderSize {
		return dst, ErrInvalidRI
	}

	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return dst, fmt.Errorf("ri list count: %w", err)
	}

	if uint32(count) > format.MaxRIListCount {
		return dst, fmt.Errorf("ri list count %d exceeds limit: %w", count, format.ErrSanityLimit)
	}

	if _, boundsErr := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.DWORDSize); boundsErr != nil {
		return dst, ErrTruncated
	}

	// Use a temporary buffer for sub-list reads to avoid corrupting main buffer during recursion
	var tempBuf []uint32

	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.DWORDSize
		subListRef, readErr := format.CheckedReadU32(payload, offset)
		if readErr != nil {
			continue
		}

		// Read sub-list into temp buffer (recursive call)
		var subErr error
		tempBuf, subErr = ReadOffsetsInto(h, subListRef, tempBuf)
		if subErr != nil {
			continue
		}

		// Append sub-list offsets to main buffer
		dst = append(dst, tempBuf...)
	}

	return dst, nil
}

// Read reads a subkey list from the hive and returns a List with all entries.
// The list can be LF, LH, LI, or RI (indirect). For RI lists, this function
// automatically follows the indirection and reads all sub-lists.
//
// listRef is the HCELL_INDEX offset to the list cell.
// Returns a List with Entry{NameLower, NKRef} for each subkey.
func Read(h *hive.Hive, listRef uint32) (*List, error) {
	if listRef == 0 || listRef == format.InvalidOffset {
		return &List{Entries: []Entry{}}, nil
	}

	// Read the list cell
	payload, err := resolveCell(h, listRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve list cell: %w", err)
	}

	// Check if it's an RI (indirect) list
	// Optimized: Use byte comparison instead of string allocation
	if len(payload) >= signatureSize && payload[0] == 'r' && payload[1] == 'i' {
		return readRIList(h, payload)
	}

	// Read direct list (LF, LH, or LI)
	return readDirectList(h, payload)
}

// readDirectList reads a single LF, LH, or LI list.
func readDirectList(h *hive.Hive, payload []byte) (*List, error) {
	if len(payload) < format.ListHeaderSize {
		return nil, ErrTruncated
	}

	// Optimized: Use byte comparisons instead of string allocation
	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return nil, fmt.Errorf("list count: %w", err)
	}

	var nkRefs []uint32
	var readErr error

	// Check signature bytes directly (no string allocation)
	sig0, sig1 := payload[0], payload[1]
	if (sig0 == 'l' && sig1 == 'f') || (sig0 == 'l' && sig1 == 'h') {
		nkRefs, readErr = readLFLH(payload, count)
	} else if sig0 == 'l' && sig1 == 'i' {
		nkRefs, readErr = readLI(payload, count)
	} else {
		return nil, ErrInvalidSignature
	}

	if readErr != nil {
		return nil, readErr
	}

	// For each NK reference, read the NK cell and extract the name
	entries := make([]Entry, 0, len(nkRefs))
	for _, nkRef := range nkRefs {
		entry, entryErr := readNKEntry(h, nkRef)
		if entryErr != nil {
			// Skip invalid entries rather than failing entire list
			continue
		}
		entries = append(entries, entry)
	}

	return &List{Entries: entries}, nil
}

// readRIList reads an RI (indirect) list which contains references to other lists.
func readRIList(h *hive.Hive, payload []byte) (*List, error) {
	if len(payload) < format.ListHeaderSize {
		return nil, ErrInvalidRI
	}

	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return nil, fmt.Errorf("ri list count: %w", err)
	}

	// Sanity check: RI list count
	if uint32(count) > format.MaxRIListCount {
		return nil, fmt.Errorf("ri list count %d exceeds limit: %w", count, format.ErrSanityLimit)
	}

	// Overflow-safe bounds check
	if _, boundsErr := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.DWORDSize); boundsErr != nil {
		return nil, ErrTruncated
	}

	// First pass: count total entries to pre-allocate
	totalEntries := 0
	subLists := make([]*List, 0, count)

	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.DWORDSize
		subListRef, readErr := format.CheckedReadU32(payload, offset)
		if readErr != nil {
			// Skip malformed entry
			continue
		}

		// Read the sub-list
		subList, err := Read(h, subListRef)
		if err != nil {
			// Skip invalid sub-lists
			continue
		}

		subLists = append(subLists, subList)
		totalEntries += len(subList.Entries)
	}

	// Pre-allocate exact size needed (eliminates ~17GB of reallocs!)
	allEntries := make([]Entry, 0, totalEntries)
	for _, subList := range subLists {
		allEntries = append(allEntries, subList.Entries...)
	}

	return &List{Entries: allEntries}, nil
}

// readLFLH reads LF or LH list entries (with hash).
func readLFLH(payload []byte, count uint16) ([]uint32, error) {
	// Each entry is 8 bytes: 4 bytes offset + 4 bytes hash
	// Overflow-safe bounds check
	if _, err := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.QWORDSize); err != nil {
		return nil, ErrTruncated
	}

	refs := make([]uint32, count)
	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.QWORDSize
		val, err := format.CheckedReadU32(payload, offset)
		if err != nil {
			// Return partial results on read error
			return refs[:i], nil
		}
		refs[i] = val
		// Skip the 4-byte hash (we don't need it for reading)
	}

	return refs, nil
}

// readLI reads LI list entries (no hash).
func readLI(payload []byte, count uint16) ([]uint32, error) {
	// Each entry is 4 bytes: offset only (no hash)
	// Overflow-safe bounds check
	if _, err := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.DWORDSize); err != nil {
		return nil, ErrTruncated
	}

	refs := make([]uint32, count)
	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.DWORDSize
		val, err := format.CheckedReadU32(payload, offset)
		if err != nil {
			// Return partial results on read error
			return refs[:i], nil
		}
		refs[i] = val
	}

	return refs, nil
}

// readLFLHInto reads LF or LH list entries into the provided buffer.
func readLFLHInto(payload []byte, count uint16, dst []uint32) ([]uint32, error) {
	// Each entry is 8 bytes: 4 bytes offset + 4 bytes hash
	// Overflow-safe bounds check
	if _, err := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.QWORDSize); err != nil {
		return dst, ErrTruncated
	}

	// Ensure capacity
	if cap(dst) < int(count) {
		dst = make([]uint32, 0, count)
	}

	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.QWORDSize
		val, err := format.CheckedReadU32(payload, offset)
		if err != nil {
			// Return partial results on read error
			return dst, nil
		}
		dst = append(dst, val)
		// Skip the 4-byte hash (we don't need it for reading)
	}

	return dst, nil
}

// readLIInto reads LI list entries into the provided buffer.
func readLIInto(payload []byte, count uint16, dst []uint32) ([]uint32, error) {
	// Each entry is 4 bytes: offset only (no hash)
	// Overflow-safe bounds check
	if _, err := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.DWORDSize); err != nil {
		return dst, ErrTruncated
	}

	// Ensure capacity
	if cap(dst) < int(count) {
		dst = make([]uint32, 0, count)
	}

	for i := range count {
		offset := format.ListHeaderSize + int(i)*format.DWORDSize
		val, err := format.CheckedReadU32(payload, offset)
		if err != nil {
			// Return partial results on read error
			return dst, nil
		}
		dst = append(dst, val)
	}

	return dst, nil
}

// readNKEntry reads an NK cell and extracts the key name.
func readNKEntry(h *hive.Hive, nkRef uint32) (Entry, error) {
	payload, err := resolveCell(h, nkRef)
	if err != nil {
		return Entry{}, err
	}

	// Parse NK cell
	nk, err := hive.ParseNK(payload)
	if err != nil {
		return Entry{}, err
	}

	// Get the name bytes
	nameBytes := nk.Name()
	if len(nameBytes) == 0 {
		return Entry{}, errors.New("NK cell has empty name")
	}

	// Decode, lowercase, AND compute both hashes in one pass
	// This fuses decode + lowercase + dual hash to eliminate redundant iteration
	var nameLower string
	var regHash, fnvHash uint32
	var decodeErr error
	if nk.IsCompressedName() {
		// ASCII (or Windows-1252) - decode + lowercase + dual hash in one pass
		nameLower, regHash, fnvHash, decodeErr = decodeCompressedNameLowerWithHashes(nameBytes)
	} else {
		// UTF-16LE - decode + lowercase + hash in one pass with ASCII-16 fast path
		// Note: FNV32 not computed for UTF-16 (rare, fallback to string path)
		nameLower, regHash, decodeErr = decodeUTF16LENameLowerWithHash(nameBytes)
		fnvHash = 0 // Will use string-based path in index builder
	}

	if decodeErr != nil {
		return Entry{}, fmt.Errorf("failed to decode name: %w", decodeErr)
	}

	return Entry{
		NameLower: nameLower,
		NKRef:     nkRef,
		Hash:      regHash,
		FNV32:     fnvHash,
	}, nil
}

// compressedNameEqualsLower checks if an ASCII/Windows-1252 encoded NK name
// equals the given lowercase target string, without allocating or fully decoding.
//
// Fast path: pure ASCII byte-by-byte case-insensitive comparison (zero allocation).
// Falls back to full decode only for rare non-ASCII bytes (Windows-1252 range 0x80-0xFF).
func compressedNameEqualsLower(nameBytes []byte, targetLower string) bool {
	// Fast path: same byte length means we can try direct ASCII comparison.
	// For pure ASCII names, byte length == rune length.
	if len(nameBytes) == len(targetLower) {
		for i, b := range nameBytes {
			if b > asciiMax {
				// Non-ASCII byte encountered; fall through to slow path.
				return compressedNameEqualsLowerSlow(nameBytes, targetLower)
			}
			lower := b
			if b >= 'A' && b <= 'Z' {
				lower = b + ('a' - 'A')
			}
			if lower != targetLower[i] {
				return false
			}
		}
		return true
	}

	// Different lengths. For pure ASCII names this is an immediate rejection.
	// Only Windows-1252 names (with bytes 0x80-0xFF that expand to multi-byte
	// UTF-8) can have nameBytes shorter than the UTF-8 target.
	for _, b := range nameBytes {
		if b > asciiMax {
			return compressedNameEqualsLowerSlow(nameBytes, targetLower)
		}
	}
	return false
}

// compressedNameEqualsLowerSlow is the fallback for names containing
// non-ASCII Windows-1252 bytes. It performs a full decode and compares.
func compressedNameEqualsLowerSlow(nameBytes []byte, targetLower string) bool {
	decoded, _, _, err := decodeCompressedNameLowerWithHashes(nameBytes)
	if err != nil {
		return false
	}
	return decoded == targetLower
}

// utf16NameEqualsLower checks if a UTF-16LE encoded NK name equals the given
// lowercase target string, without allocating when possible.
//
// For pure ASCII-in-UTF-16 names (high byte zero, low byte <= 0x7F), performs
// an inline comparison. Falls back to full decode for non-ASCII characters.
func utf16NameEqualsLower(nameBytes []byte, targetLower string) bool {
	if len(nameBytes)%utf16BytesPerChar != 0 {
		return false
	}

	charCount := len(nameBytes) / utf16BytesPerChar

	// Quick check: if target is pure ASCII and same char count, try fast path
	if charCount == len(targetLower) {
		allASCII16 := true
		for i := 0; i < len(nameBytes); i += 2 {
			if nameBytes[i+1] != 0 || nameBytes[i] > asciiMax {
				allASCII16 = false
				break
			}
		}
		if allASCII16 {
			for i, j := 0, 0; i < len(nameBytes); i, j = i+2, j+1 {
				b := nameBytes[i]
				lower := b
				if b >= 'A' && b <= 'Z' {
					lower = b + ('a' - 'A')
				}
				if lower != targetLower[j] {
					return false
				}
			}
			return true
		}
	}

	// Slow path: full decode
	decoded, _, err := decodeUTF16LENameLowerWithHash(nameBytes)
	if err != nil {
		return false
	}
	return decoded == targetLower
}

// MatchNKsFromOffsets resolves NK cells from offsets and returns entries only
// for those whose names match a target in targetNames (all lowercase).
//
// This is dramatically faster than Read() when only a small fraction of siblings
// are needed, because it skips the full name decode + hash computation for
// non-matching entries. Only matching entries get the full readNKEntry treatment
// (needed for Hash/FNV32 fields used by index insertion).
func MatchNKsFromOffsets(h *hive.Hive, offsets []uint32, targetNames map[string]struct{}) ([]Entry, error) {
	if len(targetNames) == 0 {
		return nil, nil
	}

	// Pre-allocate for expected matches (at most len(targetNames))
	entries := make([]Entry, 0, len(targetNames))

	for _, nkRef := range offsets {
		payload, err := resolveCell(h, nkRef)
		if err != nil {
			continue // Skip invalid cells
		}

		nk, err := hive.ParseNK(payload)
		if err != nil {
			continue // Skip invalid NK cells
		}

		nameBytes := nk.Name()
		if len(nameBytes) == 0 {
			continue
		}

		// Check if this name matches any target using cheap comparison
		matched := false
		if nk.IsCompressedName() {
			for target := range targetNames {
				if compressedNameEqualsLower(nameBytes, target) {
					matched = true
					break
				}
			}
		} else {
			for target := range targetNames {
				if utf16NameEqualsLower(nameBytes, target) {
					matched = true
					break
				}
			}
		}

		if !matched {
			continue
		}

		// Full decode only for matches (need Hash/FNV32 for index insertion)
		entry, err := readNKEntry(h, nkRef)
		if err != nil {
			continue
		}

		entries = append(entries, entry)

		// If we've found all targets, stop early
		if len(entries) == len(targetNames) {
			break
		}
	}

	return entries, nil
}

// resolveCell resolves a cell reference to its payload.
func resolveCell(h *hive.Hive, ref uint32) ([]byte, error) {
	data := h.Bytes()
	offset := format.HeaderSize + int(ref)

	if offset < 0 || offset+format.CellHeaderSize > len(data) {
		return nil, fmt.Errorf("cell offset out of bounds: 0x%X", ref)
	}

	// Read cell size with bounds checking (4 bytes, little-endian, signed)
	sizeRaw, err := format.CheckedReadI32(data, offset)
	if err != nil {
		return nil, fmt.Errorf("cell size read failed: 0x%X: %w", ref, err)
	}

	if sizeRaw >= 0 {
		return nil, fmt.Errorf("cell is free (positive size): 0x%X", ref)
	}

	size := int(-sizeRaw)
	if size < format.CellHeaderSize {
		return nil, fmt.Errorf("cell size too small: %d", size)
	}

	// Sanity check: cell size should not exceed limit
	if size > format.MaxCellSizeLimit {
		return nil, fmt.Errorf("cell size %d exceeds limit: 0x%X", size, ref)
	}

	payloadOffset := offset + format.CellHeaderSize
	payloadEnd := offset + size
	if payloadEnd < 0 || payloadEnd > len(data) {
		return nil, fmt.Errorf("cell payload out of bounds: 0x%X", ref)
	}

	return data[payloadOffset:payloadEnd], nil
}

// FNV-1a constants for 32-bit hash (duplicated from index package to avoid import cycle).
const (
	fnvBasis32 uint32 = 2166136261
	fnvPrime32 uint32 = 16777619
)

// decodeCompressedNameLowerWithHash decodes an ASCII/Windows-1252 encoded name,
// lowercases it, AND computes its Windows Registry hash in a single pass.
// This fuses decode + lowercase + hash computation to eliminate redundant iteration.
//
// Returns: (lowercaseName, regHash, error).
// Note: Also computes FNV32 hash available via decodeCompressedNameLowerWithHashes.
func decodeCompressedNameLowerWithHash(data []byte) (string, uint32, error) {
	name, regHash, _, err := decodeCompressedNameLowerWithHashes(data)
	return name, regHash, err
}

// decodeCompressedNameLowerWithHashes decodes an ASCII/Windows-1252 encoded name,
// lowercases it, AND computes both Windows Registry hash AND FNV-1a hash in a single pass.
// This fuses decode + lowercase + dual hash computation to eliminate redundant iteration.
//
// Returns: (lowercaseName, regHash, fnvHash, error).
func decodeCompressedNameLowerWithHashes(data []byte) (string, uint32, uint32, error) {
	var regHash uint32
	fnvHash := fnvBasis32

	// Fast path: pure ASCII with inline lowercase + hash
	ascii := true
	hasUpper := false
	for _, c := range data {
		if c > asciiMax {
			ascii = false
			break
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
	}

	if ascii {
		// Compute both hashes: regHash on uppercase, fnvHash on lowercase
		if !hasUpper {
			// Already lowercase - compute both hashes
			for _, c := range data {
				upper := c
				if c >= 'a' && c <= 'z' {
					upper = c - ('a' - 'A')
				}
				regHash = regHash*hashMultiplier + uint32(upper)
				fnvHash ^= uint32(c)
				fnvHash *= fnvPrime32
			}
			return string(data), regHash, fnvHash, nil
		}

		// Has uppercase - lowercase while computing both hashes
		bufPtr := byteBufferPool.Get().(*[]byte) //nolint:errcheck // pool contains only *[]byte
		buf := *bufPtr
		if cap(buf) < len(data) {
			buf = make([]byte, len(data))
		} else {
			buf = buf[:len(data)]
		}

		for i, c := range data {
			var lower, upper byte
			if c >= 'A' && c <= 'Z' {
				lower = c + ('a' - 'A')
				upper = c
			} else if c >= 'a' && c <= 'z' {
				lower = c
				upper = c - ('a' - 'A')
			} else {
				lower = c
				upper = c
			}
			buf[i] = lower
			regHash = regHash*hashMultiplier + uint32(upper)
			fnvHash ^= uint32(lower)
			fnvHash *= fnvPrime32
		}

		result := string(buf)
		*bufPtr = buf[:0]
		byteBufferPool.Put(bufPtr)
		return result, regHash, fnvHash, nil
	}

	// Windows-1252 → UTF-8 with inline mapping table + dual hash computation
	bufPtr := byteBufferPool.Get().(*[]byte) //nolint:errcheck // pool contains only *[]byte
	buf := *bufPtr
	need := len(data) * 2
	if cap(buf) < need {
		buf = make([]byte, 0, need)
	} else {
		buf = buf[:0]
	}

	// Encode into buf with ASCII-lowercase, compute both hashes
	for _, c := range data {
		if c <= 0x7F {
			// ASCII: lowercase for string and fnvHash, uppercase for regHash
			var lower, upper byte
			if c >= 'A' && c <= 'Z' {
				lower = c + ('a' - 'A')
				upper = c
			} else if c >= 'a' && c <= 'z' {
				lower = c
				upper = c - ('a' - 'A')
			} else {
				lower = c
				upper = c
			}
			buf = append(buf, lower)
			regHash = regHash*hashMultiplier + uint32(upper)
			fnvHash ^= uint32(lower)
			fnvHash *= fnvPrime32
			continue
		}

		// Non-ASCII: decode to rune, append UTF-8, compute both hashes
		var r rune
		if c >= 0xA0 {
			r = rune(c)
		} else {
			r = win1252Table[int(c-0x80)]
		}
		buf = appendRuneUTF8(buf, r)

		// regHash uses uppercase version of the decoded rune
		// fnvHash uses the raw byte (for consistency with Fnv32LowerBytes)
		upperRune := unicode.ToUpper(r)
		regHash = regHash*hashMultiplier + uint32(upperRune)
		// For non-ASCII, we hash the original byte lowercased
		fnvHash ^= uint32(c)
		fnvHash *= fnvPrime32
	}

	result := string(buf)
	*bufPtr = buf[:0]
	byteBufferPool.Put(bufPtr)
	return result, regHash, fnvHash, nil
}

// decodeUTF16LENameLowerWithHash decodes a UTF-16LE encoded name, lowercases it,
// AND computes its Windows Registry hash in a single pass.
// Returns: (lowercaseName, hash, error).
func decodeUTF16LENameLowerWithHash(data []byte) (string, uint32, error) {
	if len(data)%utf16BytesPerChar != 0 {
		return "", 0, fmt.Errorf("UTF-16LE name has odd length: %d", len(data))
	}

	var hash uint32

	// Fast path: ASCII packed in UTF-16LE (low byte ASCII, high byte zero)
	ascii16 := true
	for i := 0; i < len(data); i += 2 {
		if data[i+1] != 0 || data[i] > asciiMax {
			ascii16 = false
			break
		}
	}

	if ascii16 {
		// Extract even bytes, lowercase, and compute hash in one pass
		bufPtr := byteBufferPool.Get().(*[]byte) //nolint:errcheck // pool contains only *[]byte
		buf := *bufPtr
		outLen := len(data) / 2
		if cap(buf) < outLen {
			buf = make([]byte, outLen)
		} else {
			buf = buf[:outLen]
		}

		for i, j := 0, 0; i < len(data); i, j = i+2, j+1 {
			c := data[i]
			var lower, upper byte
			if c >= 'A' && c <= 'Z' {
				lower = c + ('a' - 'A')
				upper = c
			} else if c >= 'a' && c <= 'z' {
				lower = c
				upper = c - ('a' - 'A')
			} else {
				lower = c
				upper = c
			}
			buf[j] = lower
			hash = hash*hashMultiplier + uint32(upper)
		}

		result := string(buf)
		*bufPtr = buf[:0]
		byteBufferPool.Put(bufPtr)
		return result, hash, nil
	}

	// General UTF-16LE path: decode to UTF-8 with surrogate handling + hash
	bufPtr := byteBufferPool.Get().(*[]byte) //nolint:errcheck // pool contains only *[]byte
	buf := (*bufPtr)[:0]

	for i := 0; i < len(data); i += 2 {
		w1 := uint16(data[i]) | uint16(data[i+1])<<8

		// Handle surrogates
		if w1 >= 0xD800 && w1 <= 0xDBFF {
			// High surrogate
			if i+2 >= len(data) {
				*bufPtr = buf[:0]
				byteBufferPool.Put(bufPtr)
				return "", 0, fmt.Errorf("dangling high surrogate at offset %d", i)
			}
			w2 := uint16(data[i+2]) | uint16(data[i+3])<<8
			if w2 < 0xDC00 || w2 > 0xDFFF {
				*bufPtr = buf[:0]
				byteBufferPool.Put(bufPtr)
				return "", 0, fmt.Errorf("invalid surrogate pair at offset %d", i)
			}
			i += 2
			// Decode surrogate pair
			r := rune(0x10000 + (uint32(w1-0xD800) << 10) + uint32(w2-0xDC00))
			// Append UTF-8 encoding
			var tmp [4]byte
			n := encodeRune(tmp[:], r)
			buf = append(buf, tmp[:n]...)
			// Hash with uppercase rune
			upperRune := unicode.ToUpper(r)
			hash = hash*hashMultiplier + uint32(upperRune)
		} else if w1 >= 0xDC00 && w1 <= 0xDFFF {
			// Unpaired low surrogate
			*bufPtr = buf[:0]
			byteBufferPool.Put(bufPtr)
			return "", 0, fmt.Errorf("unpaired low surrogate at offset %d", i)
		} else {
			// BMP character
			r := rune(w1)
			var lowerRune, upperRune rune
			if r >= 'A' && r <= 'Z' {
				lowerRune = r + ('a' - 'A')
				upperRune = r
			} else if r >= 'a' && r <= 'z' {
				lowerRune = r
				upperRune = r - ('a' - 'A')
			} else {
				lowerRune = r
				upperRune = unicode.ToUpper(r)
			}
			// Encode lowercase to UTF-8
			var tmp [4]byte
			n := encodeRune(tmp[:], lowerRune)
			buf = append(buf, tmp[:n]...)
			// Hash with uppercase
			hash = hash*hashMultiplier + uint32(upperRune)
		}
	}

	result := string(buf)
	*bufPtr = buf[:0]
	byteBufferPool.Put(bufPtr)
	return result, hash, nil
}

// encodeRune encodes a rune to UTF-8 into dst and returns the number of bytes written.
// This is a simplified version of utf8.EncodeRune optimized for our use case.
// Used by UTF-16 decoder.
func encodeRune(dst []byte, r rune) int {
	if r <= 0x7F {
		dst[0] = byte(r)
		return 1
	}
	if r <= 0x7FF {
		dst[0] = 0xC0 | byte(r>>6)
		dst[1] = 0x80 | byte(r&0x3F)
		return 2
	}
	if r <= 0xFFFF {
		dst[0] = 0xE0 | byte(r>>12)
		dst[1] = 0x80 | byte((r>>6)&0x3F)
		dst[2] = 0x80 | byte(r&0x3F)
		return 3
	}
	dst[0] = 0xF0 | byte(r>>18)
	dst[1] = 0x80 | byte((r>>12)&0x3F)
	dst[2] = 0x80 | byte((r>>6)&0x3F)
	dst[3] = 0x80 | byte(r&0x3F)
	return 4
}

// appendRuneUTF8 appends the UTF-8 encoding of r to buf.
// Used by Windows-1252 decoder.
func appendRuneUTF8(buf []byte, r rune) []byte {
	if r <= 0x7F {
		return append(buf, byte(r))
	}
	if r <= 0x7FF {
		return append(buf, 0xC0|byte(r>>6), 0x80|byte(r&0x3F))
	}
	if r <= 0xFFFF {
		return append(buf, 0xE0|byte(r>>12), 0x80|byte((r>>6)&0x3F), 0x80|byte(r&0x3F))
	}
	return append(
		buf,
		0xF0|byte(r>>18),
		0x80|byte((r>>12)&0x3F),
		0x80|byte((r>>6)&0x3F),
		0x80|byte(r&0x3F),
	)
}

// isASCII checks if all bytes are valid ASCII (0x00-0x7F).
func isASCII(data []byte) bool {
	for _, b := range data {
		if b > asciiMax {
			return false
		}
	}
	return true
}

// decodeCompressedName decodes an ASCII/Windows-1252 encoded name (without lowercasing).
// Kept for backward compatibility with tests. New code should use decodeCompressedNameLower.
func decodeCompressedName(data []byte) (string, error) {
	// Fast path: pure ASCII (most common)
	if isASCII(data) {
		return string(data), nil
	}

	// Slow path: Windows-1252 extended characters
	decoded, err := charmap.Windows1252.NewDecoder().Bytes(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// decodeUTF16LEName decodes a UTF-16LE encoded name (without lowercasing).
// Kept for backward compatibility with tests. New code should use decodeUTF16LENameLower.
func decodeUTF16LEName(data []byte) (string, error) {
	if len(data)%utf16BytesPerChar != 0 {
		return "", fmt.Errorf("UTF-16LE name has odd length: %d", len(data))
	}

	// Convert bytes to uint16 slice
	u16 := make([]uint16, len(data)/utf16BytesPerChar)
	for i := 0; i < len(data); i += utf16BytesPerChar {
		val, err := format.CheckedReadU16(data, i)
		if err != nil {
			// Return partial results on read error
			runes := utf16.Decode(u16[:i/utf16BytesPerChar])
			return string(runes), nil
		}
		u16[i/utf16BytesPerChar] = val
	}

	// Decode UTF-16 to UTF-8
	runes := utf16.Decode(u16)
	return string(runes), nil
}
