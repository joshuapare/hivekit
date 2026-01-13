package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestFullNavigationFlow tests a complete user navigation flow
func TestFullNavigationFlow(t *testing.T) {
	helper := NewTestHelper("test.hive")

	// 1. Set window size
	helper.SendWindowSize(80, 24)

	// 2. Load root keys
	keys := CreateTestKeys(3)
	helper.LoadRootKeys(keys)

	if helper.GetTreeItemCount() != 3 {
		t.Fatalf("expected 3 tree items, got %d", helper.GetTreeItemCount())
	}

	// 3. Initially cursor should be at 0
	if helper.GetTreeCursor() != 0 {
		t.Errorf("expected cursor at 0, got %d", helper.GetTreeCursor())
	}

	// 4. Press Down to move cursor
	helper.SendKey(tea.KeyDown)

	if helper.GetTreeCursor() != 1 {
		t.Errorf("expected cursor at 1 after Down, got %d", helper.GetTreeCursor())
	}

	// 5. Press Up to move back
	helper.SendKey(tea.KeyUp)

	if helper.GetTreeCursor() != 0 {
		t.Errorf("expected cursor at 0 after Up, got %d", helper.GetTreeCursor())
	}

	// 6. Press Tab to switch to value pane
	helper.SendKey(tea.KeyTab)

	if helper.GetFocusedPane() != ValuePane {
		t.Errorf("expected ValuePane focus after Tab, got %v", helper.GetFocusedPane())
	}

	// 7. Press Tab again to go back to tree
	helper.SendKey(tea.KeyTab)

	if helper.GetFocusedPane() != TreePane {
		t.Errorf("expected TreePane focus after second Tab, got %v", helper.GetFocusedPane())
	}
}

// TestValueTableNavigation tests navigation in the value table
func TestValueTableNavigation(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Load some values
	values := CreateTestValues(5)
	helper.LoadValues("TestKey", values)

	if helper.GetValueItemCount() != 5 {
		t.Fatalf("expected 5 value items, got %d", helper.GetValueItemCount())
	}

	// Switch to value pane
	helper.SendKey(tea.KeyTab)

	// Navigate down
	helper.SendKey(tea.KeyDown)
	if helper.GetValueCursor() != 1 {
		t.Errorf("expected cursor at 1, got %d", helper.GetValueCursor())
	}

	// Navigate down again
	helper.SendKey(tea.KeyDown)
	if helper.GetValueCursor() != 2 {
		t.Errorf("expected cursor at 2, got %d", helper.GetValueCursor())
	}

	// Navigate up
	helper.SendKey(tea.KeyUp)
	if helper.GetValueCursor() != 1 {
		t.Errorf("expected cursor at 1 after up, got %d", helper.GetValueCursor())
	}
}

// TestViewContainsExpectedContent verifies the rendered view
func TestViewContainsExpectedContent(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Load test data
	keys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 10, ValueN: 5},
		{Path: "System", Name: "System", SubkeyN: 20, ValueN: 3},
	}
	helper.LoadRootKeys(keys)

	view := helper.GetView()

	// Check that view contains expected elements
	expectedStrings := []string{
		"test.hive", // Hive path in header
		"Software",  // Key name
		"System",    // Key name
		"Keys",      // Pane title
		"Values",    // Pane title
		"Navigate",  // Help text
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(view, expected) {
			t.Errorf("expected view to contain %q, but it doesn't", expected)
		}
	}
}

// TestTreeExpansion tests expanding and collapsing tree nodes
func TestTreeExpansion(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Load root keys with children
	keys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 5, ValueN: 0},
	}
	helper.LoadRootKeys(keys)

	if helper.GetTreeItemCount() != 1 {
		t.Fatalf("expected 1 item initially, got %d", helper.GetTreeItemCount())
	}

	// Get current item
	item := helper.GetCurrentTreeItem()
	if item == nil {
		t.Fatal("expected current item, got nil")
	}

	if item.Expanded {
		t.Error("item should not be expanded initially")
	}

	// Note: Actual expansion would require loading child keys
	// which requires a real hive file, so this test is limited
	// to checking the initial state
}

// TestDifferentValueTypes tests rendering different registry value types
func TestDifferentValueTypes(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Create values of different types
	values := []ValueInfo{
		{Name: "StringValue", Type: "REG_SZ", StringVal: "test string", Size: 11},
		{Name: "DwordValue", Type: "REG_DWORD", DWordVal: 255, Size: 4},
		{Name: "QwordValue", Type: "REG_QWORD", QWordVal: 0xDEADBEEF, Size: 8},
		{Name: "BinaryValue", Type: "REG_BINARY", Data: []byte{0x01, 0x02, 0x03}, Size: 3},
	}
	helper.LoadValues("TestKey", values)

	model := helper.GetModel()

	// Verify each value was formatted correctly
	items := model.valueTable.GetItems()
	if len(items) != 4 {
		t.Fatalf("expected 4 value items, got %d", len(items))
	}

	// Check string value
	if items[0].Value != "test string" {
		t.Errorf("expected string value 'test string', got %q", items[0].Value)
	}

	// Check DWORD formatting (should be hex and decimal)
	expectedDword := "0x000000ff (255)"
	if items[1].Value != expectedDword {
		t.Errorf("expected DWORD %q, got %q", expectedDword, items[1].Value)
	}

	// Check QWORD formatting
	expectedQword := "0x00000000deadbeef (3735928559)"
	if items[2].Value != expectedQword {
		t.Errorf("expected QWORD %q, got %q", expectedQword, items[2].Value)
	}

	// Check binary value (should be hex encoded)
	expectedBinary := "010203"
	if items[3].Value != expectedBinary {
		t.Errorf("expected binary %q, got %q", expectedBinary, items[3].Value)
	}
}

// TestEmptyState tests behavior with no data
func TestEmptyState(t *testing.T) {
	helper := NewTestHelper("test.hive")
	helper.SendWindowSize(80, 24)

	// Don't load any data
	view := helper.GetView()

	// Should show some default state
	if view == "" {
		t.Error("view should not be empty even with no data")
	}

	// Tree should show "Loading..." before data arrives
	if helper.GetTreeItemCount() != 0 {
		t.Errorf("expected 0 tree items initially, got %d", helper.GetTreeItemCount())
	}
}
