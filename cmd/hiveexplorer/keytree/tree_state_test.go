package keytree

import (
	"testing"
	"time"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestNewTreeState verifies initialization
func TestNewTreeState(t *testing.T) {
	ts := NewTreeState()

	if ts == nil {
		t.Fatal("NewTreeState() returned nil")
	}

	if ts.items == nil {
		t.Error("items should be initialized")
	}

	if len(ts.items) != 0 {
		t.Errorf("expected 0 items, got %d", len(ts.items))
	}

	if ts.expanded == nil {
		t.Error("expanded map should be initialized")
	}

	if ts.loaded == nil {
		t.Error("loaded map should be initialized")
	}

	if ts.diffMap == nil {
		t.Error("diffMap should be initialized")
	}

	if ts.bookmarks == nil {
		t.Error("bookmarks should be initialized")
	}
}

// TestAllItemsSetAllItems tests getter/setter for all items
func TestAllItemsSetAllItems(t *testing.T) {
	ts := NewTreeState()

	// Initially should be nil/empty
	allItems := ts.AllItems()
	if allItems != nil {
		t.Errorf("expected nil allItems initially, got %v", allItems)
	}

	// Set some items
	testItems := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
		},
		{
			Path:        "SOFTWARE\\Microsoft",
			Name:        "Microsoft",
			Depth:       1,
			HasChildren: true,
		},
	}

	ts.SetAllItems(testItems)

	// Verify getter returns the same items
	retrievedItems := ts.AllItems()
	if len(retrievedItems) != len(testItems) {
		t.Errorf("expected %d items, got %d", len(testItems), len(retrievedItems))
	}

	for i, item := range retrievedItems {
		if item.Path != testItems[i].Path {
			t.Errorf("item %d: expected Path %q, got %q", i, testItems[i].Path, item.Path)
		}
		if item.Name != testItems[i].Name {
			t.Errorf("item %d: expected Name %q, got %q", i, testItems[i].Name, item.Name)
		}
	}
}

// TestItemsSetItems tests visible items management
func TestItemsSetItems(t *testing.T) {
	ts := NewTreeState()

	// Initially should be empty
	items := ts.Items()
	if len(items) != 0 {
		t.Errorf("expected 0 items initially, got %d", len(items))
	}

	// Set visible items
	testItems := []Item{
		{
			Path:  "SOFTWARE",
			Name:  "SOFTWARE",
			Depth: 0,
		},
		{
			Path:  "SYSTEM",
			Name:  "SYSTEM",
			Depth: 0,
		},
	}

	ts.SetItems(testItems)

	// Verify getter returns the same items
	retrievedItems := ts.Items()
	if len(retrievedItems) != len(testItems) {
		t.Errorf("expected %d items, got %d", len(testItems), len(retrievedItems))
	}

	for i, item := range retrievedItems {
		if item.Path != testItems[i].Path {
			t.Errorf("item %d: expected Path %q, got %q", i, testItems[i].Path, item.Path)
		}
	}
}

// TestItemCount tests count method
func TestItemCount(t *testing.T) {
	ts := NewTreeState()

	// Initially should be 0
	if ts.ItemCount() != 0 {
		t.Errorf("expected ItemCount 0, got %d", ts.ItemCount())
	}

	// Add items
	ts.SetItems([]Item{
		{Path: "item1"},
		{Path: "item2"},
		{Path: "item3"},
	})

	if ts.ItemCount() != 3 {
		t.Errorf("expected ItemCount 3, got %d", ts.ItemCount())
	}

	// Clear items
	ts.SetItems([]Item{})

	if ts.ItemCount() != 0 {
		t.Errorf("expected ItemCount 0 after clear, got %d", ts.ItemCount())
	}
}

// TestGetItem tests getting item by index (valid/invalid)
func TestGetItem(t *testing.T) {
	ts := NewTreeState()

	testItems := []Item{
		{Path: "item0", Name: "Item 0"},
		{Path: "item1", Name: "Item 1"},
		{Path: "item2", Name: "Item 2"},
	}
	ts.SetItems(testItems)

	tests := []struct {
		name         string
		index        int
		expectNil    bool
		expectedPath string
		expectedName string
	}{
		{
			name:         "valid index 0",
			index:        0,
			expectNil:    false,
			expectedPath: "item0",
			expectedName: "Item 0",
		},
		{
			name:         "valid index 1",
			index:        1,
			expectNil:    false,
			expectedPath: "item1",
			expectedName: "Item 1",
		},
		{
			name:         "valid index 2",
			index:        2,
			expectNil:    false,
			expectedPath: "item2",
			expectedName: "Item 2",
		},
		{
			name:      "invalid negative index",
			index:     -1,
			expectNil: true,
		},
		{
			name:      "invalid index out of bounds",
			index:     3,
			expectNil: true,
		},
		{
			name:      "invalid index far out of bounds",
			index:     100,
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := ts.GetItem(tt.index)

			if tt.expectNil {
				if item != nil {
					t.Errorf("expected nil item for index %d, got %v", tt.index, item)
				}
			} else {
				if item == nil {
					t.Errorf("expected non-nil item for index %d", tt.index)
					return
				}
				if item.Path != tt.expectedPath {
					t.Errorf("expected Path %q, got %q", tt.expectedPath, item.Path)
				}
				if item.Name != tt.expectedName {
					t.Errorf("expected Name %q, got %q", tt.expectedName, item.Name)
				}
			}
		})
	}
}

// TestIsExpandedSetExpanded tests expanded state
func TestIsExpandedSetExpanded(t *testing.T) {
	ts := NewTreeState()

	testPath := "SOFTWARE\\Microsoft"

	// Initially should not be expanded
	if ts.IsExpanded(testPath) {
		t.Error("path should not be expanded initially")
	}

	// Set expanded
	ts.SetExpanded(testPath, true)

	if !ts.IsExpanded(testPath) {
		t.Error("path should be expanded after SetExpanded(true)")
	}

	// Set not expanded
	ts.SetExpanded(testPath, false)

	if ts.IsExpanded(testPath) {
		t.Error("path should not be expanded after SetExpanded(false)")
	}

	// Test multiple paths
	ts.SetExpanded("path1", true)
	ts.SetExpanded("path2", true)
	ts.SetExpanded("path3", false)

	if !ts.IsExpanded("path1") {
		t.Error("path1 should be expanded")
	}
	if !ts.IsExpanded("path2") {
		t.Error("path2 should be expanded")
	}
	if ts.IsExpanded("path3") {
		t.Error("path3 should not be expanded")
	}
}

// TestIsLoadedSetLoaded tests loaded state
func TestIsLoadedSetLoaded(t *testing.T) {
	ts := NewTreeState()

	testPath := "SOFTWARE\\Microsoft"

	// Initially should not be loaded
	if ts.IsLoaded(testPath) {
		t.Error("path should not be loaded initially")
	}

	// Set loaded
	ts.SetLoaded(testPath, true)

	if !ts.IsLoaded(testPath) {
		t.Error("path should be loaded after SetLoaded(true)")
	}

	// Set not loaded
	ts.SetLoaded(testPath, false)

	if ts.IsLoaded(testPath) {
		t.Error("path should not be loaded after SetLoaded(false)")
	}
}

// TestClearLoaded tests clearing loaded state
func TestClearLoaded(t *testing.T) {
	ts := NewTreeState()

	testPath := "SOFTWARE\\Microsoft"

	// Set loaded
	ts.SetLoaded(testPath, true)

	if !ts.IsLoaded(testPath) {
		t.Error("path should be loaded before clear")
	}

	// Clear loaded
	ts.ClearLoaded(testPath)

	if ts.IsLoaded(testPath) {
		t.Error("path should not be loaded after ClearLoaded")
	}

	// Clearing non-existent path should not panic
	ts.ClearLoaded("nonexistent\\path")
}

// TestGetDiffSetDiffMap tests diff map management
func TestGetDiffSetDiffMap(t *testing.T) {
	ts := NewTreeState()

	// Initially should not have any diffs
	_, ok := ts.GetDiff("SOFTWARE")
	if ok {
		t.Error("should not have diff data initially")
	}

	// Create test diff map
	testTime := time.Now()
	diffMap := map[string]hive.KeyDiff{
		"SOFTWARE": {
			Path:      "SOFTWARE",
			Name:      "SOFTWARE",
			Status:    hive.DiffModified,
			SubkeyN:   10,
			ValueN:    5,
			LastWrite: testTime,
		},
		"SYSTEM": {
			Path:      "SYSTEM",
			Name:      "SYSTEM",
			Status:    hive.DiffAdded,
			SubkeyN:   20,
			ValueN:    15,
			LastWrite: testTime,
		},
	}

	ts.SetDiffMap(diffMap)

	// Verify we can retrieve diffs
	softwareDiff, ok := ts.GetDiff("SOFTWARE")
	if !ok {
		t.Error("should have diff for SOFTWARE")
	}
	if softwareDiff.Status != hive.DiffModified {
		t.Errorf("expected status DiffModified, got %v", softwareDiff.Status)
	}
	if softwareDiff.SubkeyN != 10 {
		t.Errorf("expected SubkeyN 10, got %d", softwareDiff.SubkeyN)
	}

	systemDiff, ok := ts.GetDiff("SYSTEM")
	if !ok {
		t.Error("should have diff for SYSTEM")
	}
	if systemDiff.Status != hive.DiffAdded {
		t.Errorf("expected status DiffAdded, got %v", systemDiff.Status)
	}

	// Non-existent path
	_, ok = ts.GetDiff("NONEXISTENT")
	if ok {
		t.Error("should not have diff for nonexistent path")
	}
}

// TestIsBookmarkedSetBookmarks tests bookmarks
func TestIsBookmarkedSetBookmarks(t *testing.T) {
	ts := NewTreeState()

	// Initially should not be bookmarked
	if ts.IsBookmarked("SOFTWARE") {
		t.Error("path should not be bookmarked initially")
	}

	// Set bookmarks
	bookmarks := map[string]bool{
		"SOFTWARE":            true,
		"SOFTWARE\\Microsoft": true,
		"SYSTEM":              true,
	}

	ts.SetBookmarks(bookmarks)

	// Verify bookmarked paths
	if !ts.IsBookmarked("SOFTWARE") {
		t.Error("SOFTWARE should be bookmarked")
	}
	if !ts.IsBookmarked("SOFTWARE\\Microsoft") {
		t.Error("SOFTWARE\\Microsoft should be bookmarked")
	}
	if !ts.IsBookmarked("SYSTEM") {
		t.Error("SYSTEM should be bookmarked")
	}

	// Non-bookmarked path
	if ts.IsBookmarked("HARDWARE") {
		t.Error("HARDWARE should not be bookmarked")
	}
}

// TestClearLoadedDescendants tests recursive clearing of descendants
func TestClearLoadedDescendants(t *testing.T) {
	ts := NewTreeState()

	// Set up a hierarchy of loaded and expanded paths
	paths := []string{
		"SOFTWARE",
		"SOFTWARE\\Microsoft",
		"SOFTWARE\\Microsoft\\Windows",
		"SOFTWARE\\Microsoft\\Windows\\CurrentVersion",
		"SOFTWARE\\Adobe",
		"SYSTEM",
		"SYSTEM\\CurrentControlSet",
	}

	for _, path := range paths {
		ts.SetLoaded(path, true)
		ts.SetExpanded(path, true)
	}

	// Verify all are loaded and expanded
	for _, path := range paths {
		if !ts.IsLoaded(path) {
			t.Errorf("path %q should be loaded", path)
		}
		if !ts.IsExpanded(path) {
			t.Errorf("path %q should be expanded", path)
		}
	}

	// Clear descendants of SOFTWARE
	ts.ClearLoadedDescendants("SOFTWARE")

	// SOFTWARE and its descendants should be cleared
	if ts.IsLoaded("SOFTWARE") {
		t.Error("SOFTWARE should not be loaded")
	}
	if ts.IsExpanded("SOFTWARE") {
		t.Error("SOFTWARE should not be expanded")
	}
	if ts.IsLoaded("SOFTWARE\\Microsoft") {
		t.Error("SOFTWARE\\Microsoft should not be loaded")
	}
	if ts.IsExpanded("SOFTWARE\\Microsoft") {
		t.Error("SOFTWARE\\Microsoft should not be expanded")
	}
	if ts.IsLoaded("SOFTWARE\\Microsoft\\Windows") {
		t.Error("SOFTWARE\\Microsoft\\Windows should not be loaded")
	}
	if ts.IsLoaded("SOFTWARE\\Adobe") {
		t.Error("SOFTWARE\\Adobe should not be loaded")
	}

	// SYSTEM paths should still be loaded and expanded (not descendants of SOFTWARE)
	if !ts.IsLoaded("SYSTEM") {
		t.Error("SYSTEM should still be loaded")
	}
	if !ts.IsExpanded("SYSTEM") {
		t.Error("SYSTEM should still be expanded")
	}
	if !ts.IsLoaded("SYSTEM\\CurrentControlSet") {
		t.Error("SYSTEM\\CurrentControlSet should still be loaded")
	}
}

// TestClearLoadedDescendantsEdgeCases tests edge cases for ClearLoadedDescendants
func TestClearLoadedDescendantsEdgeCases(t *testing.T) {
	ts := NewTreeState()

	// Test with similar prefixes that shouldn't match
	ts.SetLoaded("SOFTWARE", true)
	ts.SetLoaded("SOFTWARE2", true) // Similar prefix but not a descendant
	ts.SetLoaded("SOFTWARE\\Test", true)

	ts.ClearLoadedDescendants("SOFTWARE")

	// SOFTWARE and its descendants should be cleared
	if ts.IsLoaded("SOFTWARE") {
		t.Error("SOFTWARE should not be loaded")
	}
	if ts.IsLoaded("SOFTWARE\\Test") {
		t.Error("SOFTWARE\\Test should not be loaded")
	}

	// SOFTWARE2 should still be loaded (not a descendant, just similar prefix)
	if !ts.IsLoaded("SOFTWARE2") {
		t.Error("SOFTWARE2 should still be loaded (not a descendant)")
	}
}

// TestTreeStateGetItemBoundaryConditions tests GetItem with edge cases
func TestTreeStateGetItemBoundaryConditions(t *testing.T) {
	ts := NewTreeState()

	// Test with empty items
	item := ts.GetItem(0)
	if item != nil {
		t.Error("GetItem should return nil for empty items")
	}

	// Add single item
	ts.SetItems([]Item{{Path: "single", NodeID: hive.NodeID(1)}})

	// Test boundary
	item = ts.GetItem(0)
	if item == nil {
		t.Error("GetItem(0) should return item")
	}

	item = ts.GetItem(1)
	if item != nil {
		t.Error("GetItem(1) should return nil for single item")
	}
}
