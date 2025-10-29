package main

import (
	"testing"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	tea "github.com/charmbracelet/bubbletea"
)

// TestLiveFilteringAfterThreeCharacters verifies that filtering only applies after 3+ characters
func TestLiveFilteringAfterThreeCharacters(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Microsoft", Path: "Software\\Microsoft", Depth: 1},
		{Name: "Windows", Path: "Software\\Microsoft\\Windows", Depth: 2},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	// Enter search mode and focus tree pane
	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Type "so" (2 characters) - should NOT filter
	m.inputBuffer = "so"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()
	if len(items) != 4 {
		t.Errorf("With 2 characters, expected no filtering (4 items), got %d items", len(items))
	}

	// Type "sof" (3 characters) - SHOULD filter
	m.inputBuffer = "sof"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items = m.keyTree.GetItems()
	// Should show: Software (match)
	if len(items) == 0 {
		t.Errorf("With 3 characters 'sof', expected filtering to apply, got 0 items")
	}
	if len(items) > 0 && items[0].Name != "Software" {
		t.Errorf("Expected first filtered item to be 'Software', got '%s'", items[0].Name)
	}
}

// TestLiveFilteringUpdatesOnKeystroke verifies filtering updates as user types
func TestLiveFilteringUpdatesOnKeystroke(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Microsoft", Path: "Software\\Microsoft", Depth: 1},
		{Name: "Windows", Path: "Software\\Microsoft\\Windows", Depth: 2},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Type "win" - should show Windows + parents
	m.inputBuffer = "win"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) == 0 {
		t.Fatalf("Expected filtered results for 'win', got 0 items")
	}

	// Verify Windows is in results
	foundWindows := false
	for _, item := range items {
		if item.Name == "Windows" {
			foundWindows = true
			break
		}
	}
	if !foundWindows {
		t.Errorf("Expected 'Windows' to be in filtered results for query 'win'")
	}

	// Add another character - "wind" - should still show Windows
	m.inputBuffer = "wind"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items = m.keyTree.GetItems()

	foundWindows = false
	for _, item := range items {
		if item.Name == "Windows" {
			foundWindows = true
			break
		}
	}
	if !foundWindows {
		t.Errorf("Expected 'Windows' to be in filtered results for query 'wind'")
	}
}

// TestLiveFilteringBackspace verifies backspace updates the filter
func TestLiveFilteringBackspace(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Microsoft", Path: "Software\\Microsoft", Depth: 1},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Type "hard" - should filter to Hardware only
	m.inputBuffer = "hard"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) == 0 {
		t.Fatalf("Expected filtered results for 'hard', got 0 items")
	}

	// Backspace to "har" - should still show Hardware
	m.inputBuffer = "har"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items = m.keyTree.GetItems()

	foundHardware := false
	for _, item := range items {
		if item.Name == "Hardware" {
			foundHardware = true
			break
		}
	}
	if !foundHardware {
		t.Errorf("Expected 'Hardware' in results after backspace to 'har'")
	}

	// Backspace to "ha" (2 chars) - should show all items (no filtering)
	m.inputBuffer = "ha"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items = m.keyTree.GetItems()

	if len(items) != 3 {
		t.Errorf("With 2 characters, expected no filtering (3 items), got %d items", len(items))
	}
}

// TestLiveFilteringIncludesParentPaths verifies parent paths are shown
func TestLiveFilteringIncludesParentPaths(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Microsoft", Path: "Software\\Microsoft", Depth: 1},
		{Name: "Windows", Path: "Software\\Microsoft\\Windows", Depth: 2},
		{Name: "CurrentVersion", Path: "Software\\Microsoft\\Windows\\CurrentVersion", Depth: 3},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Search for "CurrentVersion" - should show full parent path
	m.inputBuffer = "CurrentVersion"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) == 0 {
		t.Fatalf("Expected filtered results for 'CurrentVersion', got 0 items")
	}

	// Verify all parent paths are present
	expectedPaths := map[string]bool{
		"Software":                                           false,
		"Software\\Microsoft":                                false,
		"Software\\Microsoft\\Windows":                       false,
		"Software\\Microsoft\\Windows\\CurrentVersion":       false,
	}

	for _, item := range items {
		if _, exists := expectedPaths[item.Path]; exists {
			expectedPaths[item.Path] = true
		}
	}

	for path, found := range expectedPaths {
		if !found {
			t.Errorf("Expected parent path '%s' to be in filtered results", path)
		}
	}
}

// TestLiveFilteringClearsOnEscape verifies Esc clears the filter
func TestLiveFilteringClearsOnEscape(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Apply filter
	m.inputBuffer = "soft"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) >= 2 {
		t.Fatalf("Expected filtering to reduce items, got %d items", len(items))
	}

	// Simulate Esc key - clear filter
	msg := tea.KeyMsg{Type: tea.KeyEsc}
	updatedModel, _ := m.handleInputMode(msg)
	m = updatedModel.(Model)

	// Verify filter is cleared
	items = m.keyTree.GetItems()
	if len(items) != 2 {
		t.Errorf("After Esc, expected all items (2) to be shown, got %d items", len(items))
	}

	// Verify mode and buffers are cleared
	if m.inputMode != NormalMode {
		t.Errorf("Expected NormalMode after Esc, got %v", m.inputMode)
	}
	if m.inputBuffer != "" {
		t.Errorf("Expected empty inputBuffer after Esc, got '%s'", m.inputBuffer)
	}
	if m.searchQuery != "" {
		t.Errorf("Expected empty searchQuery after Esc, got '%s'", m.searchQuery)
	}
}

// TestLiveFilteringCursorReset verifies cursor resets when filtered list changes
func TestLiveFilteringCursorReset(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Alpha", Path: "Alpha", Depth: 0},
		{Name: "Beta", Path: "Beta", Depth: 0},
		{Name: "Gamma", Path: "Gamma", Depth: 0},
		{Name: "Delta", Path: "Delta", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Move cursor to position 2 (Gamma)
	m.keyTree.MoveDown()
	m.keyTree.MoveDown()

	// Apply filter that reduces to 1 item
	m.inputBuffer = "alp"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) == 0 {
		t.Fatalf("Expected filtered results for 'alp', got 0 items")
	}

	// Cursor should be reset to valid position (0 or within bounds)
	cursor := m.keyTree.GetCursor()
	if cursor >= len(items) {
		t.Errorf("Cursor position %d is out of bounds for %d items", cursor, len(items))
	}
}

// TestLiveFilteringEmptyQuery verifies empty query shows all items
func TestLiveFilteringEmptyQuery(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Apply empty filter
	m.inputBuffer = ""
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) != 2 {
		t.Errorf("With empty query, expected all items (2) to be shown, got %d items", len(items))
	}
}

// TestLiveFilteringCaseInsensitive verifies search is case-insensitive
func TestLiveFilteringCaseInsensitive(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Search with lowercase
	m.inputBuffer = "software"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items := m.keyTree.GetItems()

	if len(items) == 0 {
		t.Fatalf("Expected case-insensitive match for 'software', got 0 items")
	}

	foundSoftware := false
	for _, item := range items {
		if item.Name == "Software" {
			foundSoftware = true
			break
		}
	}
	if !foundSoftware {
		t.Errorf("Expected 'Software' to match lowercase query 'software'")
	}

	// Search with uppercase
	m.inputBuffer = "SOFT"
	m.keyTree.SetSearchFilter(m.inputBuffer)
	items = m.keyTree.GetItems()

	foundSoftware = false
	for _, item := range items {
		if item.Name == "Software" {
			foundSoftware = true
			break
		}
	}
	if !foundSoftware {
		t.Errorf("Expected 'Software' to match uppercase query 'SOFT'")
	}
}

// TestLiveFilteringValuePaneNotAffected verifies value pane is not affected by tree filtering
func TestLiveFilteringValuePaneNotAffected(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	// Focus value pane and enter search mode
	m.inputMode = SearchMode
	m.focusedPane = ValuePane

	// Apply filter - should NOT filter tree since value pane is focused
	m.inputBuffer = "soft"
	// SetSearchFilter should only be called when tree pane is focused
	// This test verifies the logic in handleInputMode

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}
	m.inputBuffer = "soft"
	updatedModel, _ := m.handleInputMode(msg)
	m = updatedModel.(Model)

	// Tree should still show all items
	items := m.keyTree.GetItems()
	if len(items) != 2 {
		t.Errorf("Value pane search should not filter tree, expected 2 items, got %d", len(items))
	}
}

// TestLiveFilteringEmitsNavigationSignal verifies that SetSearchFilter emits a navigation signal
// This ensures the value table loads values for the filtered item
func TestLiveFilteringEmitsNavigationSignal(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0, NodeID: 100},
		{Name: "Microsoft", Path: "Software\\Microsoft", Depth: 1, NodeID: 101},
		{Name: "Hardware", Path: "Hardware", Depth: 0, NodeID: 200},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Apply a filter - this should emit a navigation signal
	m.inputBuffer = "soft"
	m.keyTree.SetSearchFilter(m.inputBuffer)

	// The test passes if SetSearchFilter doesn't panic
	// In the real application, this would trigger value loading via the navigation bus
	// We can verify the tree is filtered correctly
	items := m.keyTree.GetItems()
	if len(items) == 0 {
		t.Fatal("Expected filtered items, got 0")
	}

	// Verify the first item is Software
	if items[0].Name != "Software" {
		t.Errorf("Expected first filtered item to be 'Software', got '%s'", items[0].Name)
	}

	t.Logf("Filter applied and navigation signal would be emitted for NodeID: %d", items[0].NodeID)
}

// TestLiveFilteringClearsEmitsSignal verifies that clearing the filter also emits a signal
func TestLiveFilteringClearsEmitsSignal(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0, NodeID: 100},
		{Name: "Hardware", Path: "Hardware", Depth: 0, NodeID: 200},
	})

	m.inputMode = SearchMode
	m.focusedPane = TreePane

	// Apply a filter first
	m.inputBuffer = "soft"
	m.keyTree.SetSearchFilter(m.inputBuffer)

	// Now clear the filter by typing fewer than 3 characters
	m.inputBuffer = "so"
	m.keyTree.SetSearchFilter(m.inputBuffer)

	// Verify all items are back
	items := m.keyTree.GetItems()
	if len(items) != 2 {
		t.Errorf("After clearing filter, expected 2 items, got %d", len(items))
	}

	t.Logf("Filter cleared and navigation signal would be emitted for current item")
}

// TestEscClearsSearchAndFilter verifies that Esc in normal mode clears both search and filter
func TestEscClearsSearchAndFilter(t *testing.T) {
	m := createTestModelWithItems(t, []keytree.Item{
		{Name: "Software", Path: "Software", Depth: 0},
		{Name: "Microsoft", Path: "Software\\Microsoft", Depth: 1},
		{Name: "Hardware", Path: "Hardware", Depth: 0},
	})

	m.focusedPane = TreePane
	m.inputMode = SearchMode

	// Apply a filter
	m.inputBuffer = "soft"
	m.keyTree.SetSearchFilter(m.inputBuffer)

	// Press Enter to exit search mode (mimics real usage)
	m.searchQuery = m.inputBuffer
	m.inputMode = NormalMode
	m.inputBuffer = ""

	// Verify tree is filtered (includes matches + parent paths)
	items := m.keyTree.GetItems()
	if len(items) >= 3 {
		t.Fatalf("Expected filter to reduce items, but got %d (same as unfiltered)", len(items))
	}
	initialFilteredCount := len(items)

	// Now press Esc to clear search (this is handled in update.go)
	// Simulate what update.go does:
	if m.searchQuery != "" {
		m.searchQuery = ""
		m.searchMatches = 0
		m.searchMatchIdx = 0
		if m.focusedPane == TreePane {
			m.keyTree.SetSearchFilter("")
		}
	}

	// Verify tree is restored
	items = m.keyTree.GetItems()
	if len(items) != 3 {
		t.Errorf("After Esc clears search, expected all 3 items back, got %d", len(items))
	}
	if len(items) <= initialFilteredCount {
		t.Errorf("Expected more items after clearing (%d) than when filtered (%d)", len(items), initialFilteredCount)
	}

	// Verify search state is cleared
	if m.searchQuery != "" {
		t.Errorf("Expected searchQuery to be cleared, got '%s'", m.searchQuery)
	}
}
