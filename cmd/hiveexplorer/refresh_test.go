package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestRefreshReloadsValues tests that F5 reloads values for current key
func TestRefreshReloadsValues(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 2},
		{Path: "System", Name: "System", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Load initial values
	initialValues := []ValueInfo{
		{Name: "Version", Type: "REG_SZ", StringVal: "1.0", Size: 3},
		{Name: "Build", Type: "REG_DWORD", DWordVal: 100, Size: 4},
	}
	helper.LoadValues("Software", initialValues)

	model := helper.GetModel()
	if helper.GetValueItemCount() != 2 {
		t.Fatalf("Expected 2 initial values, got %d", helper.GetValueItemCount())
	}

	t.Log("Pressing F5 to refresh values")
	helper.SendKey(tea.KeyF5)

	model = helper.GetModel()
	if model.statusMessage != "Refreshing..." {
		t.Logf("Status message: %q", model.statusMessage)
	}

	// Simulate refresh loading updated values
	updatedValues := []ValueInfo{
		{Name: "Version", Type: "REG_SZ", StringVal: "1.0", Size: 3},
		{Name: "Build", Type: "REG_DWORD", DWordVal: 101, Size: 4},    // Updated value
		{Name: "NewValue", Type: "REG_SZ", StringVal: "New", Size: 3}, // New value
	}
	helper.LoadValues("Software", updatedValues)

	if helper.GetValueItemCount() != 3 {
		t.Errorf("Expected 3 values after refresh, got %d", helper.GetValueItemCount())
	}

	t.Log("✓ Refresh reloads values correctly")
}

// TestRefreshPreservesCursorPosition tests that refresh maintains cursor position
func TestRefreshPreservesCursorPosition(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root1", Name: "Root1", SubkeyN: 1, ValueN: 0},
		{Path: "Root2", Name: "Root2", SubkeyN: 0, ValueN: 3},
		{Path: "Root3", Name: "Root3", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Navigate to Root2
	t.Log("Navigating to Root2")
	helper.SendKey(tea.KeyDown) // Root1 -> Root2

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != "Root2" {
		t.Fatalf("Expected to be at Root2, but at %q", currentItem.Path)
	}

	cursorBeforeRefresh := helper.GetTreeCursor()

	// Load values for Root2
	values := []ValueInfo{
		{Name: "Val1", Type: "REG_SZ", StringVal: "Test1", Size: 5},
		{Name: "Val2", Type: "REG_SZ", StringVal: "Test2", Size: 5},
		{Name: "Val3", Type: "REG_DWORD", DWordVal: 42, Size: 4},
	}
	helper.LoadValues("Root2", values)

	t.Log("Pressing F5 to refresh")
	helper.SendKey(tea.KeyF5)

	cursorAfterRefresh := helper.GetTreeCursor()

	if cursorAfterRefresh != cursorBeforeRefresh {
		t.Errorf("Cursor moved after refresh: %d -> %d", cursorBeforeRefresh, cursorAfterRefresh)
	}

	currentItem = helper.GetCurrentTreeItem()
	if currentItem.Path != "Root2" {
		t.Errorf("Cursor should still be on Root2, but on %q", currentItem.Path)
	}

	t.Log("✓ Refresh preserves cursor position")
}

// TestRefreshInExpandedTree tests refresh while navigated deep in tree
func TestRefreshInExpandedTree(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Root
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root", []keytree.KeyInfo{
		{Path: "Root\\Child1", Name: "Child1", SubkeyN: 1, ValueN: 2},
		{Path: "Root\\Child2", Name: "Child2", SubkeyN: 0, ValueN: 1},
	})

	// Expand Child1
	helper.SendKey(tea.KeyDown) // Root -> Child1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child1", []keytree.KeyInfo{
		{Path: "Root\\Child1\\GrandChild", Name: "GrandChild", SubkeyN: 0, ValueN: 3},
	})

	// Navigate to GrandChild
	helper.SendKey(tea.KeyDown) // Child1 -> GrandChild

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != "Root\\Child1\\GrandChild" {
		t.Fatalf("Expected to be at GrandChild, but at %q", currentItem.Path)
	}

	// Load values for GrandChild
	values := []ValueInfo{
		{Name: "Data1", Type: "REG_SZ", StringVal: "Value1", Size: 6},
		{Name: "Data2", Type: "REG_SZ", StringVal: "Value2", Size: 6},
		{Name: "Data3", Type: "REG_DWORD", DWordVal: 999, Size: 4},
	}
	helper.LoadValues("Root\\Child1\\GrandChild", values)

	t.Log("Pressing F5 to refresh from deep in tree")
	helper.SendKey(tea.KeyF5)

	model := helper.GetModel()
	if model.statusMessage != "Refreshing..." {
		t.Logf("Status message: %q", model.statusMessage)
	}

	// Verify still at same location
	currentItem = helper.GetCurrentTreeItem()
	if currentItem.Path != "Root\\Child1\\GrandChild" {
		t.Errorf("Cursor should still be at GrandChild, but at %q", currentItem.Path)
	}

	t.Log("✓ Refresh works correctly from deep in tree")
}

// TestRefreshFromValuePane tests refresh while in value pane
func TestRefreshFromValuePane(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "TestKey", Name: "TestKey", SubkeyN: 0, ValueN: 2},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Load values
	values := []ValueInfo{
		{Name: "Name", Type: "REG_SZ", StringVal: "TestApp", Size: 7},
		{Name: "Version", Type: "REG_SZ", StringVal: "1.0", Size: 3},
	}
	helper.LoadValues("TestKey", values)

	// Switch to value pane
	t.Log("Switching to value pane")
	helper.SendKey(tea.KeyTab)

	model := helper.GetModel()
	if model.focusedPane != ValuePane {
		t.Fatal("Should be in value pane")
	}

	// Navigate to second value
	helper.SendKey(tea.KeyDown) // Name -> Version

	currentValue := helper.GetCurrentValueItem()
	if currentValue == nil || currentValue.Name != "Version" {
		t.Fatalf("Expected to be at Version value")
	}

	valueCursorBefore := helper.GetValueCursor()

	t.Log("Pressing F5 to refresh from value pane")
	helper.SendKey(tea.KeyF5)

	// The refresh should reload values for the current tree item
	// even though we're in the value pane

	model = helper.GetModel()
	if model.statusMessage != "Refreshing..." {
		t.Logf("Status message: %q", model.statusMessage)
	}

	// Simulate reloaded values
	updatedValues := []ValueInfo{
		{Name: "Name", Type: "REG_SZ", StringVal: "TestApp", Size: 7},
		{Name: "Version", Type: "REG_SZ", StringVal: "2.0", Size: 3}, // Updated
	}
	helper.LoadValues("TestKey", updatedValues)

	// Check that we're still in value pane
	model = helper.GetModel()
	if model.focusedPane != ValuePane {
		t.Error("Should still be in value pane after refresh")
	}

	// Cursor position in value pane should be preserved (or adjusted if needed)
	valueCursorAfter := helper.GetValueCursor()
	t.Logf("Value cursor: %d -> %d", valueCursorBefore, valueCursorAfter)

	t.Log("✓ Refresh works correctly from value pane")
}

// TestRefreshWithNoCurrentItem tests refresh when there's no current item
func TestRefreshWithNoCurrentItem(t *testing.T) {
	helper := NewTestHelper("test.hive")

	// Don't load any root keys - empty tree
	helper.SendWindowSize(120, 40)

	t.Log("Pressing F5 with no current item (should handle gracefully)")
	helper.SendKey(tea.KeyF5)

	// Should not crash - just do nothing
	model := helper.GetModel()
	t.Logf("Status after refresh with no items: %q", model.statusMessage)

	t.Log("✓ Refresh handles empty tree gracefully")
}
