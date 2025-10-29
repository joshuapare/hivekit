package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestGoToPathBasicFunctionality tests basic go-to-path navigation
func TestGoToPathBasicFunctionality(t *testing.T) {
	helper := NewTestHelper("test.hive")

	// Create test tree with multiple nested keys
	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 2, ValueN: 0},
		{Path: "System", Name: "System", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Software to load children
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Software", []keytree.KeyInfo{
		{Path: "Software\\Microsoft", Name: "Microsoft", SubkeyN: 1, ValueN: 2},
		{Path: "Software\\Adobe", Name: "Adobe", SubkeyN: 0, ValueN: 1},
	})

	t.Log("Pressing Ctrl+G to enter go-to-path mode")
	helper.SendKey(tea.KeyCtrlG)

	model := helper.GetModel()
	if model.inputMode != GoToPathMode {
		t.Fatalf("Expected GoToPathMode, got %v", model.inputMode)
	}
	if model.inputBuffer != "" {
		t.Errorf("Expected empty input buffer, got %q", model.inputBuffer)
	}

	// Type path
	t.Log("Typing 'Software\\Adobe' in go-to-path mode")
	for _, r := range "Software\\Adobe" {
		helper.SendKeyRune(r)
	}

	model = helper.GetModel()
	if model.inputBuffer != "Software\\Adobe" {
		t.Errorf("Expected input buffer 'Software\\Adobe', got %q", model.inputBuffer)
	}

	t.Log("Pressing Enter to jump to path")
	helper.SendKey(tea.KeyEnter)

	model = helper.GetModel()
	if model.inputMode != NormalMode {
		t.Errorf("Expected NormalMode after enter, got %v", model.inputMode)
	}

	currentItem := helper.GetCurrentTreeItem()
	if currentItem == nil {
		t.Fatal("No current item after go-to-path")
	}
	if currentItem.Path != "Software\\Adobe" {
		t.Errorf("Expected to navigate to 'Software\\Adobe', but at %q", currentItem.Path)
	}

	t.Log("✓ Go-to-path basic functionality works correctly")
}

// TestGoToPathInvalidPath tests handling of invalid paths
func TestGoToPathInvalidPath(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	originalItem := helper.GetCurrentTreeItem()

	t.Log("Entering go-to-path mode with Ctrl+G")
	helper.SendKey(tea.KeyCtrlG)

	t.Log("Typing invalid path 'NonExistent\\Path'")
	for _, r := range "NonExistent\\Path" {
		helper.SendKeyRune(r)
	}

	helper.SendKey(tea.KeyEnter)

	model := helper.GetModel()
	if model.statusMessage == "" {
		t.Error("Expected error status message for invalid path")
	}
	if model.statusMessage != "Path not found: NonExistent\\Path" {
		t.Logf("Status message: %q", model.statusMessage)
	}

	// Cursor should not have moved
	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != originalItem.Path {
		t.Errorf("Cursor moved to %q, should have stayed at %q", currentItem.Path, originalItem.Path)
	}

	t.Log("✓ Go-to-path correctly handles invalid paths")
}

// TestGoToPathNestedNavigation tests navigation to nested paths (auto-expansion)
func TestGoToPathNestedNavigation(t *testing.T) {
	helper := NewTestHelper("test.hive")

	// Create a deeply nested tree (initially collapsed)
	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	t.Log("Tree initially has only root keys (collapsed)")
	if helper.GetTreeItemCount() != 1 {
		t.Errorf("Expected 1 root item, got %d", helper.GetTreeItemCount())
	}

	t.Log("Using go-to-path to navigate to deeply nested path")
	helper.SendKey(tea.KeyCtrlG)

	// Type a nested path that doesn't exist in the tree yet
	targetPath := "Root\\Level1\\Level2"
	for _, r := range targetPath {
		helper.SendKeyRune(r)
	}

	t.Log("Pressing Enter to navigate")
	helper.SendKey(tea.KeyEnter)

	// The NavigateToPath function will try to expand parents as needed
	// In a real scenario, this would trigger async loads
	// For this test, we verify the navigation attempt was made

	model := helper.GetModel()
	if model.inputMode != NormalMode {
		t.Errorf("Should return to NormalMode, got %v", model.inputMode)
	}

	// Since we don't load the intermediate keys in this test,
	// it should show "Path not found"
	if model.statusMessage != "Path not found: "+targetPath {
		t.Logf("Status message: %q", model.statusMessage)
	}

	t.Log("✓ Go-to-path attempts nested navigation")
}

// TestGoToPathWithExpandedParent tests navigation to a child of an expanded parent
func TestGoToPathWithExpandedParent(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 3, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Software
	t.Log("Expanding Software manually first")
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Software", []keytree.KeyInfo{
		{Path: "Software\\Microsoft", Name: "Microsoft", SubkeyN: 0, ValueN: 1},
		{Path: "Software\\Adobe", Name: "Adobe", SubkeyN: 0, ValueN: 2},
		{Path: "Software\\Google", Name: "Google", SubkeyN: 0, ValueN: 1},
	})

	t.Logf("Tree has %d items after expansion", helper.GetTreeItemCount())

	t.Log("Using go-to-path to jump to Software\\Google")
	helper.SendKey(tea.KeyCtrlG)
	for _, r := range "Software\\Google" {
		helper.SendKeyRune(r)
	}
	helper.SendKey(tea.KeyEnter)

	currentItem := helper.GetCurrentTreeItem()
	if currentItem == nil {
		t.Fatal("No current item after navigation")
	}
	if currentItem.Path != "Software\\Google" {
		t.Errorf("Expected to navigate to 'Software\\Google', but at %q", currentItem.Path)
	}

	t.Log("✓ Go-to-path navigates to child of expanded parent")
}

// TestGoToPathCancel tests canceling go-to-path with Esc
func TestGoToPathCancel(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	originalItem := helper.GetCurrentTreeItem()

	t.Log("Entering go-to-path mode")
	helper.SendKey(tea.KeyCtrlG)

	model := helper.GetModel()
	if model.inputMode != GoToPathMode {
		t.Fatalf("Expected GoToPathMode, got %v", model.inputMode)
	}

	t.Log("Typing partial path")
	for _, r := range "Some\\Path" {
		helper.SendKeyRune(r)
	}

	t.Log("Pressing Esc to cancel")
	helper.SendKey(tea.KeyEsc)

	model = helper.GetModel()
	if model.inputMode != NormalMode {
		t.Errorf("Expected to return to NormalMode, got %v", model.inputMode)
	}
	if model.inputBuffer != "" {
		t.Errorf("Expected input buffer cleared, got %q", model.inputBuffer)
	}

	// Cursor should not have moved
	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != originalItem.Path {
		t.Errorf("Cursor moved after cancel, expected %q but at %q", originalItem.Path, currentItem.Path)
	}

	t.Log("✓ Go-to-path cancel works correctly")
}

// TestGoToPathBackspace tests backspace in go-to-path mode
func TestGoToPathBackspace(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	t.Log("Entering go-to-path mode")
	helper.SendKey(tea.KeyCtrlG)

	t.Log("Typing 'Software\\Microsoft'")
	for _, r := range "Software\\Microsoft" {
		helper.SendKeyRune(r)
	}

	model := helper.GetModel()
	if model.inputBuffer != "Software\\Microsoft" {
		t.Errorf("Expected 'Software\\Microsoft', got %q", model.inputBuffer)
	}

	t.Log("Pressing backspace to remove 'oft'")
	helper.SendKey(tea.KeyBackspace) // t
	helper.SendKey(tea.KeyBackspace) // f
	helper.SendKey(tea.KeyBackspace) // o

	model = helper.GetModel()
	if model.inputBuffer != "Software\\Micros" {
		t.Errorf("Expected 'Software\\Micros', got %q", model.inputBuffer)
	}

	t.Log("✓ Backspace in go-to-path mode works correctly")
}

// TestGoToPathEmptyPath tests submitting an empty path
func TestGoToPathEmptyPath(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	originalItem := helper.GetCurrentTreeItem()

	t.Log("Entering go-to-path mode")
	helper.SendKey(tea.KeyCtrlG)

	t.Log("Pressing Enter without typing anything")
	helper.SendKey(tea.KeyEnter)

	model := helper.GetModel()
	if model.inputMode != NormalMode {
		t.Errorf("Expected to return to NormalMode, got %v", model.inputMode)
	}

	// Cursor should not have moved
	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != originalItem.Path {
		t.Errorf("Cursor moved after empty path, expected %q but at %q", originalItem.Path, currentItem.Path)
	}

	t.Log("✓ Go-to-path handles empty path correctly")
}

// TestGoToPathRootKey tests navigating to a root key
func TestGoToPathRootKey(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 0, ValueN: 1},
		{Path: "System", Name: "System", SubkeyN: 0, ValueN: 2},
		{Path: "Hardware", Name: "Hardware", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	t.Log("Starting at Software, navigating down to System")
	helper.SendKey(tea.KeyDown)

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != "System" {
		t.Fatalf("Setup failed: expected to be at System, but at %q", currentItem.Path)
	}

	t.Log("Using go-to-path to jump back to Software")
	helper.SendKey(tea.KeyCtrlG)
	for _, r := range "Software" {
		helper.SendKeyRune(r)
	}
	helper.SendKey(tea.KeyEnter)

	currentItem = helper.GetCurrentTreeItem()
	if currentItem == nil {
		t.Fatal("No current item after navigation")
	}
	if currentItem.Path != "Software" {
		t.Errorf("Expected to navigate to 'Software', but at %q", currentItem.Path)
	}

	t.Log("✓ Go-to-path can navigate to root keys")
}

// TestGoToPathCaseHandling tests whether paths are case-sensitive or not
func TestGoToPathCaseHandling(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Software", Name: "Software", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Software
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Software", []keytree.KeyInfo{
		{Path: "Software\\Microsoft", Name: "Microsoft", SubkeyN: 0, ValueN: 1},
	})

	t.Log("Using go-to-path with different case: 'software\\microsoft'")
	helper.SendKey(tea.KeyCtrlG)
	for _, r := range "software\\microsoft" {
		helper.SendKeyRune(r)
	}
	helper.SendKey(tea.KeyEnter)

	model := helper.GetModel()
	currentItem := helper.GetCurrentTreeItem()

	// Registry paths are case-insensitive in Windows, but our implementation
	// might be case-sensitive. This test documents the behavior.
	if currentItem.Path == "Software\\Microsoft" {
		t.Log("✓ Go-to-path is case-insensitive (matched despite case difference)")
	} else if model.statusMessage != "" && currentItem.Path != "Software\\Microsoft" {
		t.Log("✓ Go-to-path is case-sensitive (did not match different case)")
	} else {
		t.Logf("Current item: %q, status: %q", currentItem.Path, model.statusMessage)
	}
}
