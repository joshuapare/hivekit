package edit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// setupMinimalHive opens the minimal test hive for testing.
// Returns hive, allocator, index, and cleanup function.
func setupMinimalHive(t testing.TB) (*hive.Hive, *alloc.FastAllocator, index.Index, func()) {
	t.Helper()

	minimalHivePath := filepath.Join("..", "..", "testdata", "minimal")
	if _, err := os.Stat(minimalHivePath); os.IsNotExist(err) {
		t.Skipf("Minimal test hive not found: %s", minimalHivePath)
	}

	// Create a copy in temp dir
	tempDir := t.TempDir()
	tempHivePath := filepath.Join(tempDir, "test-hive")

	src, err := os.Open(minimalHivePath)
	if err != nil {
		t.Fatalf("Failed to open source hive: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to create temp hive: %v", err)
	}

	_, err = io.Copy(dst, src)
	dst.Close()
	if err != nil {
		t.Fatalf("Failed to copy hive: %v", err)
	}

	h, err := hive.Open(tempHivePath)
	if err != nil {
		t.Fatalf("Failed to open hive: %v", err)
	}

	allocator, err := alloc.NewFast(h, nil, nil)
	if err != nil {
		h.Close()
		t.Fatalf("Failed to create allocator: %v", err)
	}

	idx := index.NewStringIndex(1000, 1000)

	cleanup := func() {
		h.Close()
	}

	return h, allocator, idx, cleanup
}

// Test_RawSubkeyLoad_InsertOnly verifies that the ReadRaw optimization for
// insertDeferredChild produces correctly sorted subkey lists.
//
// Steps:
// 1. Create a parent key with 20 children using immediate mode
// 2. Enable deferred mode
// 3. Add 3 more children (triggers raw-load path via ReadRaw)
// 4. Flush and verify the subkey list is correctly sorted with all 23 children
func Test_RawSubkeyLoad_InsertOnly(t *testing.T) {
	h, allocator, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	ke := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create a parent key under root
	parentRef, _, err := ke.EnsureKeyPath(rootRef, []string{"_RawLoadTest"})
	if err != nil {
		t.Fatalf("Failed to create parent key: %v", err)
	}

	// Phase 1: Create 20 children in immediate mode (builds initial sorted subkey list)
	initialChildren := make([]string, 20)
	for i := 0; i < 20; i++ {
		// Use names that ensure interesting sort order (not just numeric)
		initialChildren[i] = fmt.Sprintf("child_%c_%02d", 'a'+rune(i%26), i)
	}

	for _, name := range initialChildren {
		_, _, err := ke.EnsureKeyPath(parentRef, []string{name})
		if err != nil {
			t.Fatalf("Failed to create child %q: %v", name, err)
		}
	}

	// Verify initial children exist in index
	for _, name := range initialChildren {
		_, ok := idx.GetNK(parentRef, strings.ToLower(name))
		if !ok {
			t.Fatalf("Child %q not found in index after immediate creation", name)
		}
	}

	// Phase 2: Enable deferred mode and add 3 more children
	// This will trigger insertDeferredChild which should use ReadRaw
	ke.EnableDeferredMode()

	newChildren := []string{"child_x_new", "child_a_new", "child_m_new"}
	for _, name := range newChildren {
		_, _, err := ke.EnsureKeyPath(parentRef, []string{name})
		if err != nil {
			t.Fatalf("Failed to create deferred child %q: %v", name, err)
		}
	}

	// Verify that the deferred parent used rawChildren (insert-only path)
	dp, exists := ke.(*keyEditor).deferredParents[parentRef]
	if !exists {
		t.Fatal("Expected deferred parent entry to exist")
	}
	if dp.rawChildren == nil {
		t.Error("Expected rawChildren to be populated (insert-only optimization)")
	}
	if len(dp.rawChildren) != 20 {
		t.Errorf("Expected 20 raw children, got %d", len(dp.rawChildren))
	}
	if len(dp.children) != 3 {
		t.Errorf("Expected 3 new children, got %d", len(dp.children))
	}

	// Phase 3: Flush deferred children
	flushed, err := ke.FlushDeferredSubkeys()
	if err != nil {
		t.Fatalf("FlushDeferredSubkeys failed: %v", err)
	}
	if flushed == 0 {
		t.Error("Expected at least 1 parent to be flushed")
	}

	// Phase 4: Verify the resulting subkey list is correctly sorted
	// Re-read the parent NK to get the new subkey list
	parentPayload, err := ke.(*keyEditor).resolveCell(parentRef)
	if err != nil {
		t.Fatalf("Failed to resolve parent NK: %v", err)
	}

	parentNK, err := hive.ParseNK(parentPayload)
	if err != nil {
		t.Fatalf("Failed to parse parent NK: %v", err)
	}

	subkeyCount := parentNK.SubkeyCount()
	if subkeyCount != 23 {
		t.Errorf("Expected 23 subkeys, got %d", subkeyCount)
	}

	// Read the subkey list and verify sorting
	listRef := parentNK.SubkeyListOffsetRel()
	if listRef == format.InvalidOffset {
		t.Fatal("Parent NK has no subkey list after flush")
	}

	// Read the full subkey list to verify names and order
	subkeyList, err := subkeys.Read(h, listRef)
	if err != nil {
		t.Fatalf("Failed to read subkey list: %v", err)
	}

	if len(subkeyList.Entries) != 23 {
		t.Fatalf("Expected 23 entries in subkey list, got %d", len(subkeyList.Entries))
	}

	// Build expected sorted list of all children
	allChildren := make([]string, 0, 23)
	for _, name := range initialChildren {
		allChildren = append(allChildren, strings.ToLower(name))
	}
	for _, name := range newChildren {
		allChildren = append(allChildren, strings.ToLower(name))
	}
	sort.Strings(allChildren)

	// Verify entries are in sorted order and contain all expected names
	for i, entry := range subkeyList.Entries {
		if entry.NameLower != allChildren[i] {
			t.Errorf("Entry %d: got %q, want %q", i, entry.NameLower, allChildren[i])
		}
	}

	// Verify entries are strictly sorted
	for i := 1; i < len(subkeyList.Entries); i++ {
		if subkeyList.Entries[i].NameLower <= subkeyList.Entries[i-1].NameLower {
			t.Errorf("Entries not sorted at position %d: %q <= %q",
				i, subkeyList.Entries[i].NameLower, subkeyList.Entries[i-1].NameLower)
		}
	}

	t.Logf("All 23 children correctly sorted after raw-load merge")
}

// Test_RawSubkeyLoad_EmptyParent verifies ReadRaw optimization works when the
// parent starts with no children (no existing subkey list to load).
func Test_RawSubkeyLoad_EmptyParent(t *testing.T) {
	h, allocator, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	ke := NewKeyEditor(h, allocator, idx, dt)
	rootRef := h.RootCellOffset()

	// Create an empty parent key
	parentRef, _, err := ke.EnsureKeyPath(rootRef, []string{"_EmptyParentTest"})
	if err != nil {
		t.Fatalf("Failed to create parent key: %v", err)
	}

	// Enable deferred mode and add children to empty parent
	ke.EnableDeferredMode()

	childNames := []string{"zebra", "apple", "mango", "banana", "cherry"}
	for _, name := range childNames {
		_, _, err := ke.EnsureKeyPath(parentRef, []string{name})
		if err != nil {
			t.Fatalf("Failed to create child %q: %v", name, err)
		}
	}

	// Flush
	_, err = ke.FlushDeferredSubkeys()
	if err != nil {
		t.Fatalf("FlushDeferredSubkeys failed: %v", err)
	}

	// Verify
	parentPayload, err := ke.(*keyEditor).resolveCell(parentRef)
	if err != nil {
		t.Fatalf("Failed to resolve parent NK: %v", err)
	}

	parentNK, err := hive.ParseNK(parentPayload)
	if err != nil {
		t.Fatalf("Failed to parse parent NK: %v", err)
	}

	if parentNK.SubkeyCount() != 5 {
		t.Errorf("Expected 5 subkeys, got %d", parentNK.SubkeyCount())
	}

	listRef := parentNK.SubkeyListOffsetRel()
	subkeyList, err := subkeys.Read(h, listRef)
	if err != nil {
		t.Fatalf("Failed to read subkey list: %v", err)
	}

	expected := []string{"apple", "banana", "cherry", "mango", "zebra"}
	for i, entry := range subkeyList.Entries {
		if entry.NameLower != expected[i] {
			t.Errorf("Entry %d: got %q, want %q", i, entry.NameLower, expected[i])
		}
	}
}

// Test_RawSubkeyLoad_WithDeletions verifies that when deletions are involved,
// the code falls back to full Read() (not ReadRaw) for correctness.
func Test_RawSubkeyLoad_WithDeletions(t *testing.T) {
	h, allocator, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	ke := NewKeyEditor(h, allocator, idx, dt)
	keImpl := ke.(*keyEditor)
	rootRef := h.RootCellOffset()

	// Create parent with 5 children in immediate mode
	parentRef, _, err := ke.EnsureKeyPath(rootRef, []string{"_DeletionTest"})
	if err != nil {
		t.Fatalf("Failed to create parent key: %v", err)
	}

	var child3Ref NKRef
	childNames := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	for _, name := range childNames {
		ref, _, err := ke.EnsureKeyPath(parentRef, []string{name})
		if err != nil {
			t.Fatalf("Failed to create child %q: %v", name, err)
		}
		if name == "gamma" {
			child3Ref = ref
		}
	}

	// Enable deferred mode
	ke.EnableDeferredMode()

	// Delete one child (creates deferred parent with loaded=false)
	err = keImpl.removeDeferredChild(parentRef, child3Ref)
	if err != nil {
		t.Fatalf("removeDeferredChild failed: %v", err)
	}

	// Now add a new child (should trigger Read fallback, not ReadRaw)
	_, _, err = ke.EnsureKeyPath(parentRef, []string{"zeta"})
	if err != nil {
		t.Fatalf("Failed to create child after deletion: %v", err)
	}

	// Verify that rawChildren is nil (fell back to Read for full decode)
	dp, exists := keImpl.deferredParents[parentRef]
	if !exists {
		t.Fatal("Expected deferred parent entry")
	}
	if dp.rawChildren != nil {
		t.Error("Expected rawChildren to be nil when deletions are involved")
	}

	// Flush and verify
	_, err = ke.FlushDeferredSubkeys()
	if err != nil {
		t.Fatalf("FlushDeferredSubkeys failed: %v", err)
	}

	// Re-read parent NK
	parentPayload, err := keImpl.resolveCell(parentRef)
	if err != nil {
		t.Fatalf("Failed to resolve parent: %v", err)
	}
	parentNK, err := hive.ParseNK(parentPayload)
	if err != nil {
		t.Fatalf("Failed to parse parent NK: %v", err)
	}

	// Should have 5 (alpha,beta,delta,epsilon,zeta) - gamma deleted
	if parentNK.SubkeyCount() != 5 {
		t.Errorf("Expected 5 subkeys, got %d", parentNK.SubkeyCount())
	}

	listRef := parentNK.SubkeyListOffsetRel()
	subkeyList, err := subkeys.Read(h, listRef)
	if err != nil {
		t.Fatalf("Failed to read subkey list: %v", err)
	}

	expected := []string{"alpha", "beta", "delta", "epsilon", "zeta"}
	for i, entry := range subkeyList.Entries {
		if entry.NameLower != expected[i] {
			t.Errorf("Entry %d: got %q, want %q", i, entry.NameLower, expected[i])
		}
	}
}

// Test_MergeRawAndNewEntries verifies the merge logic directly at the unit level.
func Test_MergeRawAndNewEntries(t *testing.T) {
	h, allocator, idx, cleanup := setupMinimalHive(t)
	defer cleanup()

	dt := dirty.NewTracker(h)
	ke := NewKeyEditor(h, allocator, idx, dt)
	keImpl := ke.(*keyEditor)
	rootRef := h.RootCellOffset()

	// Create a parent and children in immediate mode
	parentRef, _, err := ke.EnsureKeyPath(rootRef, []string{"_MergeTest"})
	if err != nil {
		t.Fatalf("Failed to create parent: %v", err)
	}

	// Create children and collect their raw entries
	childNames := []string{"bravo", "delta", "foxtrot", "hotel", "juliet"}
	for _, name := range childNames {
		_, _, err := ke.EnsureKeyPath(parentRef, []string{name})
		if err != nil {
			t.Fatalf("Failed to create child %q: %v", name, err)
		}
	}

	// Read the existing subkey list as raw entries
	parentPayload, err := keImpl.resolveCell(parentRef)
	if err != nil {
		t.Fatalf("Failed to resolve parent: %v", err)
	}
	parentNK, err := hive.ParseNK(parentPayload)
	if err != nil {
		t.Fatalf("Failed to parse parent NK: %v", err)
	}

	rawEntries, err := subkeys.ReadRaw(h, parentNK.SubkeyListOffsetRel())
	if err != nil {
		t.Fatalf("ReadRaw failed: %v", err)
	}

	if len(rawEntries) != 5 {
		t.Fatalf("Expected 5 raw entries, got %d", len(rawEntries))
	}

	// Create new entries to merge in (alphabetically between existing ones)
	newEntries := []subkeys.Entry{
		{NameLower: "charlie", NKRef: 0x9999, Hash: subkeys.Hash("charlie")},
		{NameLower: "echo", NKRef: 0x9998, Hash: subkeys.Hash("echo")},
		{NameLower: "golf", NKRef: 0x9997, Hash: subkeys.Hash("golf")},
	}
	sortEntries(newEntries)

	// Merge
	merged, err := keImpl.mergeRawAndNewEntries(rawEntries, newEntries)
	if err != nil {
		t.Fatalf("mergeRawAndNewEntries failed: %v", err)
	}

	if len(merged) != 8 {
		t.Fatalf("Expected 8 merged entries, got %d", len(merged))
	}

	// Verify the merged entries are in correct order by reading back NK names
	// for the real entries and checking positions of fake entries
	expectedOrder := []string{"bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "juliet"}
	for i, me := range merged {
		var name string
		if me.NKRef == 0x9999 {
			name = "charlie"
		} else if me.NKRef == 0x9998 {
			name = "echo"
		} else if me.NKRef == 0x9997 {
			name = "golf"
		} else {
			// Real entry - decode name
			payload, err := keImpl.resolveCell(me.NKRef)
			if err != nil {
				t.Fatalf("Failed to resolve entry %d NK: %v", i, err)
			}
			nk, err := hive.ParseNK(payload)
			if err != nil {
				t.Fatalf("Failed to parse entry %d NK: %v", i, err)
			}
			name = decodeName(nk.Name(), nk.IsCompressedName())
		}
		if name != expectedOrder[i] {
			t.Errorf("Merged entry %d: got %q, want %q", i, name, expectedOrder[i])
		}
	}

	t.Logf("Merge correctly interleaved %d raw + %d new entries", len(rawEntries), len(newEntries))
}
