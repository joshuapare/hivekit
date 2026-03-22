package write

import (
	"testing"

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
