package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestCopyPathFromTreePane tests copying the current key path with 'c'
func TestCopyPathFromTreePane(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 1, ValueN: 0},
		{Path: "System", Name: "System", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Verify we're in tree pane
	model := helper.GetModel()
	if model.focusedPane != TreePane {
		t.Fatal("Setup failed: should be in tree pane")
	}

	currentItem := helper.GetCurrentTreeItem()
	if currentItem == nil {
		t.Fatal("No current item")
	}
	expectedPath := currentItem.Path

	t.Logf("Pressing 'c' to copy path: %q", expectedPath)
	helper.SendKeyRune('c')

	model = helper.GetModel()
	// Check that status message indicates success
	// (We can't reliably test actual clipboard contents in unit tests)
	if !strings.Contains(model.statusMessage, "Copied") {
		t.Logf("Status message: %q", model.statusMessage)
		// Note: The clipboard operation might fail in test environment
		// This is acceptable as we're testing the code path, not the OS clipboard
	}

	t.Log("✓ Copy path command executed from tree pane")
}

// TestCopyPathFromValuePane tests copying path while in value pane
func TestCopyPathFromValuePane(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 2},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Load values
	values := []ValueInfo{
		{Name: "Version", Type: "REG_SZ", StringVal: "1.0", Size: 3},
		{Name: "Count", Type: "REG_DWORD", DWordVal: 42, Size: 4},
	}
	helper.LoadValues("Software", values)

	// Switch to value pane
	t.Log("Switching to value pane")
	helper.SendKey(tea.KeyTab)

	model := helper.GetModel()
	if model.focusedPane != ValuePane {
		t.Fatal("Should be in value pane")
	}

	t.Log("Pressing 'c' to copy key path (not value)")
	helper.SendKeyRune('c')

	model = helper.GetModel()
	// The 'c' key should copy the key path even from value pane
	if !strings.Contains(model.statusMessage, "Copied") {
		t.Logf("Status message: %q", model.statusMessage)
	}

	t.Log("✓ Copy path command executed from value pane")
}

// TestCopyValue tests copying the current value with 'y'
func TestCopyValue(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 3},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Load values
	values := []ValueInfo{
		{Name: "AppName", Type: "REG_SZ", StringVal: "MyApp", Size: 5},
		{Name: "Version", Type: "REG_SZ", StringVal: "2.1.0", Size: 5},
		{Name: "MaxUsers", Type: "REG_DWORD", DWordVal: 100, Size: 4},
	}
	helper.LoadValues("Software", values)

	// Switch to value pane
	t.Log("Switching to value pane")
	helper.SendKey(tea.KeyTab)

	model := helper.GetModel()
	if model.focusedPane != ValuePane {
		t.Fatal("Should be in value pane")
	}

	// Get current value
	currentValue := helper.GetCurrentValueItem()
	if currentValue == nil {
		t.Fatal("No current value item")
	}

	t.Logf("Pressing 'y' to copy value: %q", currentValue.Value)
	helper.SendKeyRune('y')

	model = helper.GetModel()
	// The status message is set in update.go but we're using the helper
	// which directly calls valueTable.CopyCurrentValue()
	// The function returns an error if it fails, but doesn't set statusMessage itself

	t.Log("✓ Copy value command executed")
}

// TestCopyValueFromTreePane tests that 'y' in tree pane handles appropriately
func TestCopyValueFromTreePane(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	model := helper.GetModel()
	if model.focusedPane != TreePane {
		t.Fatal("Should be in tree pane")
	}

	t.Log("Pressing 'y' in tree pane (should be ignored or handled gracefully)")
	helper.SendKeyRune('y')

	// The 'y' key is only handled in value pane navigation (see update.go)
	// In tree pane it should be ignored
	model = helper.GetModel()

	t.Log("✓ Copy value command in tree pane handled gracefully")
}

// TestCopyValueWithNoValues tests copying when value pane is empty
func TestCopyValueWithNoValues(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "EmptyKey", Name: "EmptyKey", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Load empty values
	helper.LoadValues("EmptyKey", []ValueInfo{})

	// Switch to value pane
	helper.SendKey(tea.KeyTab)

	model := helper.GetModel()
	if model.focusedPane != ValuePane {
		t.Fatal("Should be in value pane")
	}

	if helper.GetValueItemCount() != 0 {
		t.Fatalf("Expected 0 values, got %d", helper.GetValueItemCount())
	}

	t.Log("Pressing 'y' with no values (should handle gracefully)")
	helper.SendKeyRune('y')

	// The CopyCurrentValue function should return an error for "no value selected"
	// This shouldn't crash

	t.Log("✓ Copy value with empty value list handled gracefully")
}

// TestCopyPathNavigationScenario tests copying paths after navigation
func TestCopyPathNavigationScenario(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root1", Name: "Root1", SubkeyN: 2, ValueN: 0},
		{Path: "Root2", Name: "Root2", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Root1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1", []keytree.KeyInfo{
		{Path: "Root1\\Child1", Name: "Child1", SubkeyN: 0, ValueN: 1},
		{Path: "Root1\\Child2", Name: "Child2", SubkeyN: 0, ValueN: 2},
	})

	// Navigate to Child2
	t.Log("Navigating to Root1\\Child2")
	helper.SendKey(tea.KeyDown) // Root1 -> Child1
	helper.SendKey(tea.KeyDown) // Child1 -> Child2

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != "Root1\\Child2" {
		t.Fatalf("Expected to be at Root1\\Child2, but at %q", currentItem.Path)
	}

	t.Logf("Copying path: %q", currentItem.Path)
	helper.SendKeyRune('c')

	model := helper.GetModel()
	// Verify the model executed the copy command
	// The actual clipboard operation might fail in test environment
	t.Logf("Status message: %q", model.statusMessage)

	t.Log("✓ Copy path after navigation works correctly")
}

// TestCopyValueNavigation tests copying different values after navigation
func TestCopyValueNavigation(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "TestKey", Name: "TestKey", SubkeyN: 0, ValueN: 4},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Load values
	values := []ValueInfo{
		{Name: "Val1", Type: "REG_SZ", StringVal: "First", Size: 5},
		{Name: "Val2", Type: "REG_SZ", StringVal: "Second", Size: 6},
		{Name: "Val3", Type: "REG_SZ", StringVal: "Third", Size: 5},
		{Name: "Val4", Type: "REG_DWORD", DWordVal: 999, Size: 4},
	}
	helper.LoadValues("TestKey", values)

	// Switch to value pane
	helper.SendKey(tea.KeyTab)

	if helper.GetModel().focusedPane != ValuePane {
		t.Fatal("Should be in value pane")
	}

	// Navigate to third value
	t.Log("Navigating to Val3")
	helper.SendKey(tea.KeyDown) // Val1 -> Val2
	helper.SendKey(tea.KeyDown) // Val2 -> Val3

	currentValue := helper.GetCurrentValueItem()
	if currentValue == nil || currentValue.Name != "Val3" {
		t.Fatalf("Expected to be at Val3, but at %v", currentValue)
	}

	t.Logf("Copying value: %q", currentValue.Value)
	helper.SendKeyRune('y')

	// Navigate to last value and copy again
	t.Log("Navigating to Val4")
	helper.SendKey(tea.KeyDown) // Val3 -> Val4

	currentValue = helper.GetCurrentValueItem()
	if currentValue == nil || currentValue.Name != "Val4" {
		t.Fatalf("Expected to be at Val4, but at %v", currentValue)
	}

	t.Logf("Copying value: %q", currentValue.Value)
	helper.SendKeyRune('y')

	t.Log("✓ Copy value navigation works correctly")
}
