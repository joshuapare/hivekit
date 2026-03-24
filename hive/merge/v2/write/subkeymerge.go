package write

import (
	"cmp"
	"slices"

	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

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

type anchor struct {
	oldIdx int
	child  *trie.Node
}

type insertGroup struct {
	beforeAnchorIdx int
	entries         []subkeys.RawEntry
}

// MergeRawWithInserts merges old raw subkey entries with trie children using
// positional merge. It preserves the relative order of existing entries and
// inserts new entries near their trie-sibling anchors.
func MergeRawWithInserts(oldRaw []subkeys.RawEntry, trieChildren []*trie.Node, deletedRefs map[uint32]bool) []subkeys.RawEntry {
	if len(trieChildren) == 0 {
		return filterDeleted(oldRaw, deletedRefs)
	}

	oldRefToIdx := make(map[uint32]int, len(oldRaw))
	for i, raw := range oldRaw {
		oldRefToIdx[raw.NKRef] = i
	}

	var anchors []anchor
	var currentNew []subkeys.RawEntry
	var groups []insertGroup

	for _, child := range trieChildren {
		if child.DeleteKey {
			continue
		}
		if child.CellIdx == format.InvalidOffset {
			continue
		}

		if idx, found := oldRefToIdx[child.CellIdx]; found {
			if len(currentNew) > 0 {
				groups = append(groups, insertGroup{beforeAnchorIdx: len(anchors), entries: currentNew})
				currentNew = nil
			}
			anchors = append(anchors, anchor{oldIdx: idx, child: child})
		} else {
			hash := child.Hash
			if hash == 0 {
				hash = subkeys.Hash(child.Name)
			}
			currentNew = append(currentNew, subkeys.RawEntry{NKRef: child.CellIdx, Hash: hash})
		}
	}
	trailingNew := currentNew

	result := make([]subkeys.RawEntry, 0, len(oldRaw)+len(trailingNew)+countInserts(groups))

	slices.SortFunc(anchors, func(a, b anchor) int { return cmp.Compare(a.oldIdx, b.oldIdx) })

	groupIdx, anchorIdx, prevEnd := 0, 0, 0

	for anchorIdx < len(anchors) {
		a := anchors[anchorIdx]
		for groupIdx < len(groups) && groups[groupIdx].beforeAnchorIdx == anchorIdx {
			result = append(result, groups[groupIdx].entries...)
			groupIdx++
		}
		for i := prevEnd; i <= a.oldIdx; i++ {
			if deletedRefs != nil && deletedRefs[oldRaw[i].NKRef] {
				continue
			}
			result = append(result, oldRaw[i])
		}
		prevEnd = a.oldIdx + 1
		anchorIdx++
	}

	for groupIdx < len(groups) {
		result = append(result, groups[groupIdx].entries...)
		groupIdx++
	}

	for i := prevEnd; i < len(oldRaw); i++ {
		if deletedRefs != nil && deletedRefs[oldRaw[i].NKRef] {
			continue
		}
		result = append(result, oldRaw[i])
	}

	result = append(result, trailingNew...)
	return result
}

// CanPositionalMerge checks whether MergeRawWithInserts can produce correctly
// ordered output. The positional merge is safe when:
//   - oldRaw is empty (only new entries, trie order is correct)
//   - No new entries to insert (all non-deleted trie children exist in oldRaw)
//
// It is NOT safe when new entries must be inserted among existing old entries,
// because the positional merge cannot determine where new entries sort relative
// to untouched old entries between anchors. In that case the caller must fall
// back to name-resolving merge.
func CanPositionalMerge(oldRaw []subkeys.RawEntry, trieChildren []*trie.Node) bool {
	if len(oldRaw) == 0 {
		return true // no old entries, trie order is correct
	}

	oldRefSet := make(map[uint32]bool, len(oldRaw))
	for _, r := range oldRaw {
		oldRefSet[r.NKRef] = true
	}
	for _, child := range trieChildren {
		if child.DeleteKey || child.CellIdx == format.InvalidOffset {
			continue
		}
		if !oldRefSet[child.CellIdx] {
			return false // new entry needs insertion among existing entries
		}
	}
	return true // all non-deleted children are in the old list (anchors or no-ops)
}

func filterDeleted(old []subkeys.RawEntry, deleted map[uint32]bool) []subkeys.RawEntry {
	if len(deleted) == 0 {
		return old
	}
	result := make([]subkeys.RawEntry, 0, len(old))
	for _, r := range old {
		if !deleted[r.NKRef] {
			result = append(result, r)
		}
	}
	return result
}

func countInserts(groups []insertGroup) int {
	n := 0
	for _, g := range groups {
		n += len(g.entries)
	}
	return n
}
