package subkeys

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/internal/buf"
	"github.com/joshuapare/hivekit/internal/format"
)

// MatchedEntry is a subkey entry matched by LH hash filtering.
type MatchedEntry struct {
	NKRef     uint32 // cell index of matched NK cell
	Hash      uint32 // the LH hash
	NameLower string // decoded lowercase name (needed by walkAndApply for child paths)
}

// AddHashTarget adds a name to the hash-bucketed target map, handling collisions.
func AddHashTarget(targets map[uint32][]string, name string) {
	h := Hash(name)
	targets[h] = append(targets[h], name)
}

// HashTargetCount returns the total number of target names across all hash buckets.
func HashTargetCount(targets map[uint32][]string) int {
	n := 0
	for _, names := range targets {
		n += len(names)
	}
	return n
}

// MatchByHash finds children in a subkey list using hash-first filtering.
// Instead of dereferencing every NK cell (as ReadOffsetsInto + MatchNKsFromOffsets does),
// it scans the {offset, hash} pairs stored in the LH list sequentially and only
// dereferences NK cells when the hash matches a target. For a parent with 200 children
// and 3 targets, this reduces ~200 random reads to ~3.
//
// targets maps pre-computed LH hash -> list of lowercase names with that hash.
// Multiple names may share the same hash (collision). All are checked on hash match.
// Build this map with AddHashTarget.
//
// For non-LH list types (LF, LI), falls back to dereferencing each entry.
// For RI lists, recurses into each leaf list.
//
// Hash collisions are handled correctly: after a hash match, the actual NK name
// is decoded and compared to ALL target names for that hash.
func MatchByHash(h *hive.Hive, listRef uint32, targets map[uint32][]string) ([]MatchedEntry, error) {
	if len(targets) == 0 {
		return nil, nil
	}

	if listRef == 0 || listRef == format.InvalidOffset {
		return nil, nil
	}

	payload, err := resolveCell(h, listRef)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve list cell: %w", err)
	}

	// Check if it's an RI (indirect) list
	if len(payload) >= signatureSize && payload[0] == 'r' && payload[1] == 'i' {
		return matchRIList(h, payload, targets)
	}

	return matchDirectList(h, payload, targets)
}

// matchDirectList handles a single LH, LF, or LI list.
func matchDirectList(h *hive.Hive, payload []byte, targets map[uint32][]string) ([]MatchedEntry, error) {
	if len(payload) < format.ListHeaderSize {
		return nil, ErrTruncated
	}

	count, err := format.CheckedReadU16(payload, listCountOffset)
	if err != nil {
		return nil, fmt.Errorf("list count: %w", err)
	}

	if count == 0 {
		return nil, nil
	}

	sig0, sig1 := payload[0], payload[1]

	if sig0 == 'l' && sig1 == 'h' {
		return matchLH(h, payload, count, targets)
	}
	if sig0 == 'l' && sig1 == 'f' {
		// LF lists store a 4-byte "name hint" (first 4 chars of name),
		// not a full hash. We cannot reliably filter by hash, so fall back
		// to dereferencing each entry.
		return matchLFLIFallback(h, payload, count, format.QWORDSize, targets)
	}
	if sig0 == 'l' && sig1 == 'i' {
		// LI lists have no hash at all — only offsets.
		return matchLFLIFallback(h, payload, count, format.DWORDSize, targets)
	}

	return nil, ErrInvalidSignature
}

// matchLH scans an LH list's {offset, hash} pairs and only dereferences NK cells
// on hash match. This is the fast path that provides the ~60x speedup.
func matchLH(h *hive.Hive, payload []byte, count uint16, targets map[uint32][]string) ([]MatchedEntry, error) {
	// Each LH entry is 8 bytes: 4 bytes NK offset + 4 bytes hash.
	if _, err := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), format.QWORDSize); err != nil {
		return nil, ErrTruncated
	}

	matched := make([]MatchedEntry, 0, len(targets))
	remaining := HashTargetCount(targets)

	for i := range count {
		if remaining == 0 {
			break // found all targets
		}

		entryOffset := format.ListHeaderSize + int(i)*format.QWORDSize

		// Read hash (bytes 4-7 of the entry) — cheap sequential read.
		// Error discarded: bounds pre-validated by CheckListBounds above.
		storedHash, _ := format.CheckedReadU32(payload, entryOffset+4)

		// Check if hash matches any target
		targetNames, found := targets[storedHash]
		if !found {
			continue // no hash match — skip this entry entirely (the fast path)
		}

		// Hash matches — now dereference the NK cell to verify the name.
		// This handles hash collisions correctly by checking all names
		// that share this hash bucket.
		// Error discarded: bounds pre-validated by CheckListBounds above.
		nkRef, _ := format.CheckedReadU32(payload, entryOffset)

		for _, targetName := range targetNames {
			nameLower, verified := verifyNKName(h, nkRef, targetName)
			if !verified {
				continue
			}

			matched = append(matched, MatchedEntry{
				NKRef:     nkRef,
				Hash:      storedHash,
				NameLower: nameLower,
			})
			remaining--
			break // one NK cell can only match one name
		}
	}

	return matched, nil
}

// matchLFLIFallback handles LF and LI lists by dereferencing each entry.
// LF lists store name hints (first 4 chars) instead of hashes, and LI lists
// have no hash data at all. Both require full NK dereference for name matching.
//
// entrySize is format.QWORDSize (8) for LF or format.DWORDSize (4) for LI.
func matchLFLIFallback(h *hive.Hive, payload []byte, count uint16, entrySize int, targets map[uint32][]string) ([]MatchedEntry, error) {
	if _, err := buf.CheckListBounds(len(payload), format.ListHeaderSize, int(count), entrySize); err != nil {
		return nil, ErrTruncated
	}

	// Build reverse map: lowercase name -> hash for output.
	nameToHash := make(map[string]uint32, HashTargetCount(targets))
	for hash, names := range targets {
		for _, name := range names {
			nameToHash[name] = hash
		}
	}

	matched := make([]MatchedEntry, 0, len(targets))
	remaining := HashTargetCount(targets)

	for i := range count {
		if remaining == 0 {
			break
		}

		entryOffset := format.ListHeaderSize + int(i)*entrySize
		nkRef, _ := format.CheckedReadU32(payload, entryOffset)

		// Dereference the NK cell and check name against all targets
		nkPayload, err := resolveCell(h, nkRef)
		if err != nil {
			continue
		}
		nk, err := hive.ParseNK(nkPayload)
		if err != nil {
			continue
		}

		nameBytes := nk.Name()
		if len(nameBytes) == 0 {
			continue
		}

		// Check each target name from all hash buckets
		for targetHash, targetNames := range targets {
			found := false
			for _, targetName := range targetNames {
				var nameMatches bool
				if nk.IsCompressedName() {
					nameMatches = compressedNameEqualsLower(nameBytes, targetName)
				} else {
					nameMatches = utf16NameEqualsLower(nameBytes, targetName)
				}

				if nameMatches {
					matched = append(matched, MatchedEntry{
						NKRef:     nkRef,
						Hash:      targetHash,
						NameLower: targetName,
					})
					remaining--
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	return matched, nil
}

// matchRIList handles an RI (indirect) list by recursing into each sub-list.
func matchRIList(h *hive.Hive, payload []byte, targets map[uint32][]string) ([]MatchedEntry, error) {
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
		return nil, ErrInvalidRI
	}

	var allMatched []MatchedEntry
	remaining := HashTargetCount(targets)

	for i := range count {
		if remaining == 0 {
			break
		}

		offset := format.ListHeaderSize + int(i)*format.DWORDSize
		subListRef, readErr := format.CheckedReadU32(payload, offset)
		if readErr != nil {
			continue
		}

		subPayload, subErr := resolveCell(h, subListRef)
		if subErr != nil {
			continue
		}

		subMatched, subReadErr := matchDirectList(h, subPayload, targets)
		if subReadErr != nil {
			continue
		}

		allMatched = append(allMatched, subMatched...)
		remaining -= len(subMatched)
	}

	return allMatched, nil
}

// verifyNKName dereferences an NK cell and checks if its lowercased name
// matches expectedLower. Returns the decoded lowercase name and true if matched.
func verifyNKName(h *hive.Hive, nkRef uint32, expectedLower string) (string, bool) {
	nkPayload, err := resolveCell(h, nkRef)
	if err != nil {
		return "", false
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		return "", false
	}

	nameBytes := nk.Name()
	if len(nameBytes) == 0 {
		return "", false
	}

	if nk.IsCompressedName() {
		if compressedNameEqualsLower(nameBytes, expectedLower) {
			return expectedLower, true
		}
	} else {
		if utf16NameEqualsLower(nameBytes, expectedLower) {
			return expectedLower, true
		}
	}

	return "", false
}
