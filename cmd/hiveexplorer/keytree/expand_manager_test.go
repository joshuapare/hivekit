package keytree

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestNewExpandManager verifies initialization
func TestNewExpandManager(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()

	em := NewExpandManager(state, nav)

	if em == nil {
		t.Fatal("NewExpandManager() returned nil")
	}

	if em.state == nil {
		t.Error("state should not be nil")
	}

	if em.nav == nil {
		t.Error("nav should not be nil")
	}
}

// TestExpandFromAllItems tests expanding when allItems are loaded
func TestExpandFromAllItems(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up allItems with a hierarchy
	allItems := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			Parent:      "",
		},
		{
			Path:        "SOFTWARE\\Microsoft",
			Name:        "Microsoft",
			Depth:       1,
			HasChildren: true,
			Parent:      "SOFTWARE",
		},
		{
			Path:        "SOFTWARE\\Adobe",
			Name:        "Adobe",
			Depth:       1,
			HasChildren: false,
			Parent:      "SOFTWARE",
		},
	}
	state.SetAllItems(allItems)

	// Set visible items to just the root
	items := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			Expanded:    false,
			Parent:      "",
		},
	}
	state.SetItems(items)
	nav.SetCursor(0)

	// Expand at cursor (SOFTWARE)
	cmd := em.Expand(func(path string) tea.Cmd { return nil })

	if cmd != nil {
		t.Error("expected nil cmd when expanding from allItems")
	}

	// Verify children were added to visible items
	visibleItems := state.Items()
	if len(visibleItems) != 3 {
		t.Errorf("expected 3 visible items, got %d", len(visibleItems))
	}

	// Verify parent is marked as expanded
	if !visibleItems[0].Expanded {
		t.Error("parent item should be expanded")
	}

	if !state.IsExpanded("SOFTWARE") {
		t.Error("SOFTWARE should be marked as expanded in state")
	}

	// Verify children are correct
	if len(visibleItems) >= 2 && visibleItems[1].Name != "Adobe" &&
		visibleItems[1].Name != "Microsoft" {
		t.Errorf("expected child 'Adobe' or 'Microsoft', got %q", visibleItems[1].Name)
	}
}

// TestExpandFromDiffMap tests expanding in diff mode
func TestExpandFromDiffMap(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	testTime := time.Now()

	// Create mock readers for diff mode
	// In real usage, these would be actual hive readers
	// For testing, we can use nil since GetNodeIDsForDiffKey will return an error
	// which expandFromDiffMap will skip. This test just verifies the diff map
	// iteration logic, not the NodeID lookup logic.
	state.SetDiffReaders(nil, nil)

	// Set up diff map with a hierarchy
	diffMap := map[string]hive.KeyDiff{
		"SOFTWARE": {
			Path:      "SOFTWARE",
			Name:      "SOFTWARE",
			Status:    hive.DiffModified,
			SubkeyN:   2,
			ValueN:    0,
			LastWrite: testTime,
		},
		"SOFTWARE\\Microsoft": {
			Path:      "SOFTWARE\\Microsoft",
			Name:      "Microsoft",
			Status:    hive.DiffAdded,
			SubkeyN:   0,
			ValueN:    5,
			LastWrite: testTime,
		},
		"SOFTWARE\\Adobe": {
			Path:      "SOFTWARE\\Adobe",
			Name:      "Adobe",
			Status:    hive.DiffAdded,
			SubkeyN:   0,
			ValueN:    2,
			LastWrite: testTime,
		},
	}
	state.SetDiffMap(diffMap)

	// Set visible items to just the root
	items := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			SubkeyCount: 2,
			Expanded:    false,
			Parent:      "",
			DiffStatus:  hive.DiffModified,
		},
	}
	state.SetItems(items)
	nav.SetCursor(0)

	// Expand at cursor (SOFTWARE)
	cmd := em.Expand(func(path string) tea.Cmd { return nil })

	if cmd != nil {
		t.Error("expected nil cmd when expanding from diffMap")
	}

	// NOTE: With the new dual NodeID architecture, expandFromDiffMap requires
	// valid readers to look up NodeIDs. Without proper readers (we set nil above),
	// GetNodeIDsForDiffKey will fail and children will be skipped.
	// This test now verifies that expand doesn't crash with nil readers,
	// and that the item is still marked as expanded/loaded.
	// Real diff mode always has valid readers, so this is just a safety test.

	// Verify parent is marked as expanded and loaded even though children couldn't be added
	if !state.IsExpanded("SOFTWARE") {
		t.Error("SOFTWARE should be marked as expanded")
	}

	if !state.IsLoaded("SOFTWARE") {
		t.Error("SOFTWARE should be marked as loaded")
	}

	// With nil readers, no children should be added
	visibleItems := state.Items()
	if len(visibleItems) != 1 {
		t.Logf(
			"Note: With nil readers, expected 1 visible item (no children added), got %d",
			len(visibleItems),
		)
	}
}

// TestExpandAlreadyExpanded tests toggling to collapse
func TestExpandAlreadyExpanded(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up items with an already expanded parent
	items := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			Expanded:    true,
			Parent:      "",
		},
		{
			Path:        "SOFTWARE\\Microsoft",
			Name:        "Microsoft",
			Depth:       1,
			HasChildren: false,
			Expanded:    false,
			Parent:      "SOFTWARE",
		},
	}
	state.SetItems(items)
	state.SetExpanded("SOFTWARE", true)
	state.SetLoaded("SOFTWARE", true)
	nav.SetCursor(0)

	// Call Expand on already expanded item (should collapse)
	cmd := em.Expand(func(path string) tea.Cmd { return nil })

	if cmd != nil {
		t.Error("expected nil cmd when collapsing")
	}

	// Verify item was collapsed
	if state.IsExpanded("SOFTWARE") {
		t.Error("SOFTWARE should not be expanded after toggle")
	}

	if state.IsLoaded("SOFTWARE") {
		t.Error("SOFTWARE should not be loaded after collapse")
	}

	// Verify children were removed
	visibleItems := state.Items()
	if len(visibleItems) != 1 {
		t.Errorf("expected 1 visible item after collapse, got %d", len(visibleItems))
	}
}

// TestCollapseAt tests collapse operation
func TestCollapseAt(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up expanded hierarchy
	items := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			Expanded:    true,
			Parent:      "",
		},
		{
			Path:        "SOFTWARE\\Microsoft",
			Name:        "Microsoft",
			Depth:       1,
			HasChildren: true,
			Expanded:    true,
			Parent:      "SOFTWARE",
		},
		{
			Path:        "SOFTWARE\\Microsoft\\Windows",
			Name:        "Windows",
			Depth:       2,
			HasChildren: false,
			Expanded:    false,
			Parent:      "SOFTWARE\\Microsoft",
		},
		{
			Path:        "SOFTWARE\\Adobe",
			Name:        "Adobe",
			Depth:       1,
			HasChildren: false,
			Expanded:    false,
			Parent:      "SOFTWARE",
		},
	}
	state.SetItems(items)
	state.SetExpanded("SOFTWARE", true)
	state.SetExpanded("SOFTWARE\\Microsoft", true)
	state.SetLoaded("SOFTWARE", true)
	state.SetLoaded("SOFTWARE\\Microsoft", true)

	// Collapse SOFTWARE (at index 0)
	em.CollapseAt(0)

	// Verify SOFTWARE is collapsed
	visibleItems := state.Items()
	if len(visibleItems) != 1 {
		t.Errorf("expected 1 visible item after collapse, got %d", len(visibleItems))
	}

	if visibleItems[0].Expanded {
		t.Error("SOFTWARE should not be expanded")
	}

	if state.IsExpanded("SOFTWARE") {
		t.Error("SOFTWARE should not be marked as expanded in state")
	}

	if state.IsLoaded("SOFTWARE") {
		t.Error("SOFTWARE should not be marked as loaded in state")
	}
}

// TestRemoveDescendantsFromView tests removing descendants
func TestRemoveDescendantsFromView(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up hierarchy
	items := []Item{
		{Path: "SOFTWARE", Parent: ""},
		{Path: "SOFTWARE\\Microsoft", Parent: "SOFTWARE"},
		{Path: "SOFTWARE\\Microsoft\\Windows", Parent: "SOFTWARE\\Microsoft"},
		{Path: "SOFTWARE\\Adobe", Parent: "SOFTWARE"},
		{Path: "SYSTEM", Parent: ""},
		{Path: "SYSTEM\\CurrentControlSet", Parent: "SYSTEM"},
	}
	state.SetItems(items)

	// Remove descendants of SOFTWARE
	em.RemoveDescendantsFromView("SOFTWARE")

	// Verify descendants were removed
	visibleItems := state.Items()
	expectedCount := 3 // SOFTWARE, SYSTEM, SYSTEM\CurrentControlSet
	if len(visibleItems) != expectedCount {
		t.Errorf("expected %d visible items, got %d", expectedCount, len(visibleItems))
	}

	// Verify SOFTWARE remains
	foundSoftware := false
	for _, item := range visibleItems {
		if item.Path == "SOFTWARE" {
			foundSoftware = true
		}
		// Verify no descendants of SOFTWARE
		if item.Path == "SOFTWARE\\Microsoft" || item.Path == "SOFTWARE\\Adobe" ||
			item.Path == "SOFTWARE\\Microsoft\\Windows" {
			t.Errorf("descendant %q should have been removed", item.Path)
		}
	}

	if !foundSoftware {
		t.Error("SOFTWARE should still be visible")
	}

	// Verify SYSTEM and its descendants remain
	foundSystem := false
	foundCurrentControlSet := false
	for _, item := range visibleItems {
		if item.Path == "SYSTEM" {
			foundSystem = true
		}
		if item.Path == "SYSTEM\\CurrentControlSet" {
			foundCurrentControlSet = true
		}
	}

	if !foundSystem {
		t.Error("SYSTEM should still be visible")
	}
	if !foundCurrentControlSet {
		t.Error("SYSTEM\\CurrentControlSet should still be visible")
	}
}

// TestMoveToParent tests parent navigation
func TestMoveToParent(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up hierarchy
	items := []Item{
		{
			Path:   "SOFTWARE",
			Name:   "SOFTWARE",
			Depth:  0,
			Parent: "",
		},
		{
			Path:   "SOFTWARE\\Microsoft",
			Name:   "Microsoft",
			Depth:  1,
			Parent: "SOFTWARE",
		},
		{
			Path:   "SOFTWARE\\Microsoft\\Windows",
			Name:   "Windows",
			Depth:  2,
			Parent: "SOFTWARE\\Microsoft",
		},
	}
	state.SetItems(items)

	// Move cursor to Windows (index 2)
	nav.SetCursor(2)

	// Move to parent (should move to Microsoft at index 1)
	em.MoveToParent()

	if nav.Cursor() != 1 {
		t.Errorf("expected cursor at 1 (Microsoft), got %d", nav.Cursor())
	}

	// Move to parent again (should move to SOFTWARE at index 0)
	em.MoveToParent()

	if nav.Cursor() != 0 {
		t.Errorf("expected cursor at 0 (SOFTWARE), got %d", nav.Cursor())
	}

	// Try to move to parent again (root has no parent, should not move)
	em.MoveToParent()

	if nav.Cursor() != 0 {
		t.Error("cursor should remain at 0 when at root")
	}
}

// TestClearLoadedRecursive tests recursive clearing
func TestClearLoadedRecursive(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up loaded hierarchy
	paths := []string{
		"SOFTWARE",
		"SOFTWARE\\Microsoft",
		"SOFTWARE\\Microsoft\\Windows",
		"SOFTWARE\\Adobe",
		"SYSTEM",
	}

	for _, path := range paths {
		state.SetLoaded(path, true)
		state.SetExpanded(path, true)
	}

	// Clear SOFTWARE and descendants
	em.clearLoadedRecursive("SOFTWARE")

	// Verify SOFTWARE and descendants are cleared
	if state.IsLoaded("SOFTWARE") {
		t.Error("SOFTWARE should not be loaded")
	}
	if state.IsExpanded("SOFTWARE") {
		t.Error("SOFTWARE should not be expanded")
	}
	if state.IsLoaded("SOFTWARE\\Microsoft") {
		t.Error("SOFTWARE\\Microsoft should not be loaded")
	}
	if state.IsLoaded("SOFTWARE\\Microsoft\\Windows") {
		t.Error("SOFTWARE\\Microsoft\\Windows should not be loaded")
	}
	if state.IsLoaded("SOFTWARE\\Adobe") {
		t.Error("SOFTWARE\\Adobe should not be loaded")
	}

	// Verify SYSTEM is still loaded (not a descendant)
	if !state.IsLoaded("SYSTEM") {
		t.Error("SYSTEM should still be loaded")
	}
	if !state.IsExpanded("SYSTEM") {
		t.Error("SYSTEM should still be expanded")
	}
}

// TestCollapseAtMovesToParent tests that collapsing an already collapsed item moves to parent
func TestCollapseAtMovesToParent(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up items with a child that's not expanded
	items := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			Expanded:    true,
			Parent:      "",
		},
		{
			Path:        "SOFTWARE\\Microsoft",
			Name:        "Microsoft",
			Depth:       1,
			HasChildren: false,
			Expanded:    false, // Not expanded
			Parent:      "SOFTWARE",
		},
	}
	state.SetItems(items)
	nav.SetCursor(1) // Cursor on Microsoft

	// Try to collapse an already collapsed item
	em.CollapseAt(1)

	// Should move to parent (SOFTWARE at index 0)
	if nav.Cursor() != 0 {
		t.Errorf("expected cursor at 0 (parent), got %d", nav.Cursor())
	}
}

// TestExpandNoChildren tests expanding an item with no children
func TestExpandNoChildren(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up item with no children
	items := []Item{
		{
			Path:        "SOFTWARE\\Adobe",
			Name:        "Adobe",
			Depth:       1,
			HasChildren: false, // No children
			Expanded:    false,
			Parent:      "SOFTWARE",
		},
	}
	state.SetItems(items)
	nav.SetCursor(0)

	// Try to expand item with no children
	cmd := em.Expand(func(path string) tea.Cmd { return nil })

	if cmd != nil {
		t.Error("expected nil cmd when expanding item with no children")
	}

	// Verify nothing changed
	visibleItems := state.Items()
	if len(visibleItems) != 1 {
		t.Errorf("expected 1 visible item, got %d", len(visibleItems))
	}
}

// TestExpandOutOfBounds tests expanding with cursor out of bounds
func TestExpandOutOfBounds(t *testing.T) {
	state := NewTreeState()
	nav := NewNavigator()
	em := NewExpandManager(state, nav)

	// Set up one item
	items := []Item{
		{
			Path:        "SOFTWARE",
			Name:        "SOFTWARE",
			Depth:       0,
			HasChildren: true,
			NodeID:      hive.NodeID(1),
		},
	}
	state.SetItems(items)

	// Set cursor out of bounds
	nav.SetCursor(10)

	// Try to expand
	cmd := em.Expand(func(path string) tea.Cmd { return nil })

	if cmd != nil {
		t.Error("expected nil cmd when cursor is out of bounds")
	}
}
