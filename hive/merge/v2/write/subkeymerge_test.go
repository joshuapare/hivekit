package write

import (
	"testing"

	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/subkeys"
)

func TestMergeSortedEntries_EmptyInputs(t *testing.T) {
	result := MergeSortedEntries(nil, nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestMergeSortedEntries_OldOnly(t *testing.T) {
	old := []subkeys.Entry{
		{NameLower: "alpha", NKRef: 100, Hash: 1},
		{NameLower: "beta", NKRef: 200, Hash: 2},
		{NameLower: "gamma", NKRef: 300, Hash: 3},
	}

	result := MergeSortedEntries(old, nil, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	assertEntryOrder(t, result, []string{"alpha", "beta", "gamma"})
}

func TestMergeSortedEntries_NewOnly(t *testing.T) {
	new := []subkeys.Entry{
		{NameLower: "delta", NKRef: 400, Hash: 4},
		{NameLower: "epsilon", NKRef: 500, Hash: 5},
	}

	result := MergeSortedEntries(nil, new, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	assertEntryOrder(t, result, []string{"delta", "epsilon"})
}

func TestMergeSortedEntries_Interleaved(t *testing.T) {
	old := []subkeys.Entry{
		{NameLower: "alpha", NKRef: 100, Hash: 1},
		{NameLower: "gamma", NKRef: 300, Hash: 3},
	}
	new := []subkeys.Entry{
		{NameLower: "beta", NKRef: 200, Hash: 2},
		{NameLower: "delta", NKRef: 400, Hash: 4},
	}

	result := MergeSortedEntries(old, new, nil)
	if len(result) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(result))
	}
	assertEntryOrder(t, result, []string{"alpha", "beta", "delta", "gamma"})
}

func TestMergeSortedEntries_Replacement(t *testing.T) {
	old := []subkeys.Entry{
		{NameLower: "alpha", NKRef: 100, Hash: 1},
		{NameLower: "beta", NKRef: 200, Hash: 2},
	}
	new := []subkeys.Entry{
		{NameLower: "beta", NKRef: 999, Hash: 22},
	}

	result := MergeSortedEntries(old, new, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	// "beta" should come from new (NKRef=999).
	if result[1].NKRef != 999 {
		t.Errorf("expected replaced beta NKRef=999, got %d", result[1].NKRef)
	}
}

func TestMergeSortedEntries_WithDeletes(t *testing.T) {
	old := []subkeys.Entry{
		{NameLower: "alpha", NKRef: 100, Hash: 1},
		{NameLower: "beta", NKRef: 200, Hash: 2},
		{NameLower: "gamma", NKRef: 300, Hash: 3},
	}
	deleted := map[uint32]bool{200: true}

	result := MergeSortedEntries(old, nil, deleted)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries after delete, got %d", len(result))
	}
	assertEntryOrder(t, result, []string{"alpha", "gamma"})
}

func TestMergeSortedEntries_DeleteAndInsert(t *testing.T) {
	old := []subkeys.Entry{
		{NameLower: "alpha", NKRef: 100, Hash: 1},
		{NameLower: "beta", NKRef: 200, Hash: 2},
		{NameLower: "gamma", NKRef: 300, Hash: 3},
	}
	new := []subkeys.Entry{
		{NameLower: "delta", NKRef: 400, Hash: 4},
	}
	deleted := map[uint32]bool{200: true}

	result := MergeSortedEntries(old, new, deleted)
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	assertEntryOrder(t, result, []string{"alpha", "delta", "gamma"})
}

func TestMergeSortedRawEntries_Basic(t *testing.T) {
	old := []subkeys.RawEntry{
		{NKRef: 100, Hash: 10},
		{NKRef: 200, Hash: 20},
		{NKRef: 300, Hash: 30},
	}
	new := []subkeys.RawEntry{
		{NKRef: 150, Hash: 15},
		{NKRef: 250, Hash: 25},
	}

	result := MergeSortedRawEntries(old, new, nil)
	if len(result) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(result))
	}
	// Verify hash order.
	for i := 1; i < len(result); i++ {
		if result[i].Hash < result[i-1].Hash {
			t.Errorf("entries not sorted by hash at index %d: %d < %d",
				i, result[i].Hash, result[i-1].Hash)
		}
	}
}

func TestMergeSortedRawEntries_WithDeletes(t *testing.T) {
	old := []subkeys.RawEntry{
		{NKRef: 100, Hash: 10},
		{NKRef: 200, Hash: 20},
		{NKRef: 300, Hash: 30},
	}
	deleted := map[uint32]bool{200: true}

	result := MergeSortedRawEntries(old, nil, deleted)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries after delete, got %d", len(result))
	}
	if result[0].NKRef != 100 || result[1].NKRef != 300 {
		t.Errorf("unexpected entries: %v", result)
	}
}

// TestCanPositionalMerge_NewEntryAmongOld verifies that CanPositionalMerge
// returns false when new entries need insertion among existing old entries.
// This triggers the name-resolving fallback in rebuildSubkeyList.
func TestCanPositionalMerge_NewEntryAmongOld(t *testing.T) {
	oldRaw := []subkeys.RawEntry{
		{NKRef: 10, Hash: 0x1}, // "a"
		{NKRef: 20, Hash: 0x2}, // "b"
		{NKRef: 30, Hash: 0x3}, // "c"
		{NKRef: 40, Hash: 0x4}, // "d"
	}
	bbNode := &trie.Node{
		Name: "BB", NameLower: "bb", Hash: 0xBB,
		CellIdx: 99, Exists: true,
	}
	trieChildren := []*trie.Node{bbNode}

	// Must return false: BB (ref=99) is not in oldRaw, so it's a new entry.
	if CanPositionalMerge(oldRaw, trieChildren) {
		t.Fatal("CanPositionalMerge should return false when new entries need insertion among old entries")
	}

	// The fallback (MergeSortedEntries) produces correct order.
	oldEntries := []subkeys.Entry{
		{NameLower: "a", NKRef: 10, Hash: 0x1},
		{NameLower: "b", NKRef: 20, Hash: 0x2},
		{NameLower: "c", NKRef: 30, Hash: 0x3},
		{NameLower: "d", NKRef: 40, Hash: 0x4},
	}
	newEntries := []subkeys.Entry{
		{NameLower: "bb", NKRef: 99, Hash: 0xBB},
	}

	merged := MergeSortedEntries(oldEntries, newEntries, nil)

	if len(merged) != 5 {
		t.Fatalf("expected 5 merged entries, got %d", len(merged))
	}
	expectedRefs := []uint32{10, 20, 99, 30, 40}
	for i, e := range merged {
		if e.NKRef != expectedRefs[i] {
			t.Errorf("merged[%d].NKRef = %d, want %d", i, e.NKRef, expectedRefs[i])
		}
	}
}

// TestCanPositionalMerge_AnchorWithNewEntry verifies that CanPositionalMerge
// returns false even when some trie children ARE anchors, if there is also
// a new entry that needs insertion. This is the gap-misordering bug:
// oldRaw=[A,C,E], trie=[A(exists),D(new),E(exists)] would produce [A,D,C,E]
// via positional merge instead of correct [A,C,D,E].
func TestCanPositionalMerge_AnchorWithNewEntry(t *testing.T) {
	oldRaw := []subkeys.RawEntry{
		{NKRef: 10, Hash: 0x1}, // "A"
		{NKRef: 30, Hash: 0x3}, // "C"
		{NKRef: 50, Hash: 0x5}, // "E"
	}
	trieChildren := []*trie.Node{
		{Name: "A", NameLower: "a", CellIdx: 10, Exists: true},  // anchor
		{Name: "D", NameLower: "d", CellIdx: 99, Exists: true},  // NEW - not in oldRaw
		{Name: "E", NameLower: "e", CellIdx: 50, Exists: true},  // anchor
	}

	// Must return false: D (ref=99) is not in oldRaw.
	if CanPositionalMerge(oldRaw, trieChildren) {
		t.Fatal("CanPositionalMerge should return false when new entry D exists among anchors")
	}
}

// TestCanPositionalMerge_DeleteOnly verifies positional merge is safe when
// trie only has delete operations (no new entries to position).
func TestCanPositionalMerge_DeleteOnly(t *testing.T) {
	oldRaw := []subkeys.RawEntry{
		{NKRef: 10, Hash: 0x1},
		{NKRef: 20, Hash: 0x2},
	}
	trieChildren := []*trie.Node{
		{Name: "X", NameLower: "x", CellIdx: 50, DeleteKey: true},
	}

	if !CanPositionalMerge(oldRaw, trieChildren) {
		t.Error("CanPositionalMerge should return true for delete-only trie children")
	}
}

// TestCanPositionalMerge_AllAnchors verifies positional merge is safe when
// all non-deleted trie children exist in the old list (pure anchor case).
func TestCanPositionalMerge_AllAnchors(t *testing.T) {
	oldRaw := []subkeys.RawEntry{
		{NKRef: 10, Hash: 0x1},
		{NKRef: 20, Hash: 0x2},
	}
	trieChildren := []*trie.Node{
		{Name: "Existing", NameLower: "existing", CellIdx: 10, Exists: true},
	}

	if !CanPositionalMerge(oldRaw, trieChildren) {
		t.Error("CanPositionalMerge should return true when all children are anchors")
	}
}

// TestCanPositionalMerge_EmptyOldRaw verifies positional merge is safe
// when there are no existing entries (new parent, only new children).
func TestCanPositionalMerge_EmptyOldRaw(t *testing.T) {
	var oldRaw []subkeys.RawEntry
	trieChildren := []*trie.Node{
		{Name: "New", NameLower: "new", CellIdx: 99, Exists: true},
	}

	if !CanPositionalMerge(oldRaw, trieChildren) {
		t.Error("CanPositionalMerge should return true when oldRaw is empty")
	}
}

func assertEntryOrder(t *testing.T, entries []subkeys.Entry, expectedNames []string) {
	t.Helper()
	if len(entries) != len(expectedNames) {
		t.Fatalf("length mismatch: got %d, want %d", len(entries), len(expectedNames))
	}
	for i, name := range expectedNames {
		if entries[i].NameLower != name {
			t.Errorf("entry[%d]: got %q, want %q", i, entries[i].NameLower, name)
		}
	}
}
