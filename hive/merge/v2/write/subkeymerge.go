package write

import "github.com/joshuapare/hivekit/hive/subkeys"

// MergeSortedEntries merges old (existing) and new subkey entries, excluding
// any entries whose NKRef appears in the deleted set. Both old and new must
// be sorted by their natural order (name-based, as stored in the LH list).
// Result preserves the relative ordering of old entries and inserts new
// entries using a two-pointer merge on NameLower.
//
// The merge uses NameLower (case-insensitive) for ordering since that is how
// subkey lists are sorted in the hive binary format. Entries from old that
// share the same NameLower as a new entry are replaced by the new entry.
func MergeSortedEntries(old, new []subkeys.Entry, deleted map[uint32]bool) []subkeys.Entry {
	// Pre-allocate result with combined capacity.
	result := make([]subkeys.Entry, 0, len(old)+len(new))

	oi, ni := 0, 0
	for oi < len(old) && ni < len(new) {
		o := old[oi]
		n := new[ni]

		// Skip deleted old entries.
		if deleted != nil && deleted[o.NKRef] {
			oi++
			continue
		}

		if o.NameLower < n.NameLower {
			result = append(result, o)
			oi++
		} else if o.NameLower > n.NameLower {
			result = append(result, n)
			ni++
		} else {
			// Same name: new replaces old.
			result = append(result, n)
			oi++
			ni++
		}
	}

	// Drain remaining old entries.
	for oi < len(old) {
		o := old[oi]
		if deleted == nil || !deleted[o.NKRef] {
			result = append(result, o)
		}
		oi++
	}

	// Drain remaining new entries.
	for ni < len(new) {
		result = append(result, new[ni])
		ni++
	}

	return result
}

// MergeSortedRawEntries merges old (existing) and new raw subkey entries,
// excluding any entries whose NKRef appears in the deleted set.
// Both old and new must be sorted by Hash. Result is sorted by Hash.
//
// This is the raw-entry variant for cases where names are not available;
// sort order is by Hash (matching the LH binary format).
func MergeSortedRawEntries(old, new []subkeys.RawEntry, deleted map[uint32]bool) []subkeys.RawEntry {
	result := make([]subkeys.RawEntry, 0, len(old)+len(new))

	oi, ni := 0, 0
	for oi < len(old) && ni < len(new) {
		o := old[oi]
		n := new[ni]

		// Skip deleted old entries.
		if deleted != nil && deleted[o.NKRef] {
			oi++
			continue
		}

		if o.Hash < n.Hash {
			result = append(result, o)
			oi++
		} else if o.Hash > n.Hash {
			result = append(result, n)
			ni++
		} else {
			// Same hash: if same NKRef it's a replacement, otherwise keep both.
			if o.NKRef == n.NKRef {
				result = append(result, n)
				oi++
				ni++
			} else {
				// Different NKRef with same hash (hash collision).
				// Keep old first to preserve existing ordering.
				result = append(result, o)
				oi++
			}
		}
	}

	for oi < len(old) {
		o := old[oi]
		if deleted == nil || !deleted[o.NKRef] {
			result = append(result, o)
		}
		oi++
	}

	for ni < len(new) {
		result = append(result, new[ni])
		ni++
	}

	return result
}
