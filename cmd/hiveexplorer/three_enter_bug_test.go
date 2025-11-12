package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestThreeEnterPressesBeforeLoad tests the exact bug from debug6.log:
// User presses Enter multiple times before async children load completes
//
// From log lines 97-100:
// Line 97: Enter: expanding "A\\A giant\\A giant elephant"
// Line 98: Enter: collapsing "A\\A giant\\A giant elephant"
// Line 99: Enter: expanding "A\\A giant\\A giant elephant"
// Line 100: children loaded
//
// Bug: First Enter sets Expanded=true but children load async
//
//	Second Enter sees Expanded=true and collapses
//	Third Enter expands again, children finally load
func TestThreeEnterPressesBeforeLoad(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "A", Name: "A", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)

	// Expand A
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("A", []keytree.KeyInfo{
		{Path: "A\\B", Name: "B", SubkeyN: 1, ValueN: 0},
	})

	// Navigate to B
	helper.SendKey(tea.KeyDown)

	itemB := helper.GetCurrentTreeItem()
	if itemB.Name != "B" {
		t.Fatalf("Expected to be on B, got %q", itemB.Name)
	}

	// Verify B is collapsed initially
	if itemB.Expanded {
		t.Error("B should start collapsed")
	}

	t.Log("=== Simulating rapid Enter presses before async load completes ===")

	// Press Enter first time - should trigger load but NOT set Expanded yet
	t.Log("Enter press #1 (should start loading children)")
	model := helper.GetModel()

	// Get B's current state
	itemBeforeFirstEnter := model.keyTree.CurrentItem()
	t.Logf("Before first Enter: B.Expanded=%v", itemBeforeFirstEnter.Expanded)

	// First Enter
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	itemAfterFirstEnter := model.keyTree.CurrentItem()
	t.Logf("After first Enter: B.Expanded=%v, cmd=%v", itemAfterFirstEnter.Expanded, cmd != nil)

	// THE FIX: B.Expanded should still be FALSE because children haven't loaded yet
	if itemAfterFirstEnter.Expanded {
		t.Error("BUG: B.Expanded is true before children loaded - this causes the 3-Enter bug")
		t.Error("Second Enter will collapse it, third Enter will expand it")
	} else {
		t.Log("GOOD: B.Expanded is still false (children not loaded yet)")
	}

	// Press Enter second time BEFORE loading children (simulating fast user)
	t.Log("Enter press #2 (before async load completes)")
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	itemAfterSecondEnter := model.keyTree.CurrentItem()
	t.Logf("After second Enter: B.Expanded=%v", itemAfterSecondEnter.Expanded)

	// With the bug: second Enter would see Expanded=true and collapse it
	// With the fix: second Enter should also do nothing (still waiting for children)

	// NOW simulate children loading
	t.Log("Children loading...")
	childMsg := keytree.ChildKeysLoadedMsg{
		Parent: "A\\B",
		Keys: []keytree.KeyInfo{
			{Path: "A\\B\\C", Name: "C", SubkeyN: 0, ValueN: 1},
		},
	}
	keyTree, _ := (&model.keyTree).Update(childMsg)
	model.keyTree = keyTree

	itemAfterLoad := model.keyTree.CurrentItem()
	treeItems := len(model.keyTree.GetItems())
	t.Logf("After children load: B.Expanded=%v, tree has %d items", itemAfterLoad.Expanded, treeItems)

	// After children load, B should be expanded
	if !itemAfterLoad.Expanded {
		t.Error("BUG: B should be expanded after children load")
	}

	// And children should be visible
	if treeItems < 3 {
		t.Errorf("BUG: Expected at least 3 items (A, B, C), got %d", treeItems)
	}

	t.Log("PASS: Enter press correctly waits for children to load before setting Expanded=true")
}

// TestEnterToggleWhenAlreadyExpanded tests that Enter still toggles when item is already expanded
func TestEnterToggleWhenAlreadyExpanded(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "A", Name: "A", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)

	// Expand A and load children
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("A", []keytree.KeyInfo{
		{Path: "A\\B", Name: "B", SubkeyN: 0, ValueN: 1},
	})

	itemA := helper.GetCurrentTreeItem()
	if !itemA.Expanded {
		t.Error("A should be expanded")
	}

	itemsBefore := helper.GetTreeItemCount()
	t.Logf("A is expanded, tree has %d items", itemsBefore)

	// Press Enter again - should collapse
	helper.SendKey(tea.KeyEnter)

	itemAAfter := helper.GetCurrentTreeItem()
	itemsAfter := helper.GetTreeItemCount()

	t.Logf("After Enter: A.Expanded=%v, tree has %d items", itemAAfter.Expanded, itemsAfter)

	if itemAAfter.Expanded {
		t.Error("BUG: Enter should have collapsed A")
	}

	if itemsAfter >= itemsBefore {
		t.Error("BUG: Children should have been removed")
	}

	t.Log("PASS: Enter correctly toggles expanded item to collapsed")
}

// TestExpandWhenChildrenAlreadyLoaded tests the re-expand case
func TestExpandWhenChildrenAlreadyLoaded(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "A", Name: "A", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)

	// Expand A (first time - loads children)
	t.Log("First expand - loading children")
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("A", []keytree.KeyInfo{
		{Path: "A\\B", Name: "B", SubkeyN: 0, ValueN: 1},
	})

	itemA := helper.GetCurrentTreeItem()
	if !itemA.Expanded {
		t.Error("A should be expanded after loading children")
	}

	// Collapse A
	t.Log("Collapsing A")
	helper.SendKey(tea.KeyEnter)

	itemACollapsed := helper.GetCurrentTreeItem()
	if itemACollapsed.Expanded {
		t.Error("A should be collapsed")
	}

	// Re-expand A - NOTE: We clear the loaded flag on collapse (to prevent re-expand bugs)
	// So this will reload children, not show them immediately
	t.Log("Re-expanding A (will reload children)")
	model := helper.GetModel()

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)

	itemABeforeReload := model.keyTree.CurrentItem()
	t.Logf("After re-expand (before reload): A.Expanded=%v, cmd=%v", itemABeforeReload.Expanded, cmd != nil)

	// Should have triggered reload, so Expanded is still false
	if itemABeforeReload.Expanded {
		t.Error("BUG: Expanded should be false until children reload")
	}

	// Now simulate children reloading
	helper.LoadChildKeys("A", []keytree.KeyInfo{
		{Path: "A\\B", Name: "B", SubkeyN: 0, ValueN: 1},
	})

	// Get updated item from helper (not old model variable)
	itemAAfterReload := helper.GetCurrentTreeItem()
	if !itemAAfterReload.Expanded {
		t.Error("BUG: A should be expanded after children reload")
	}

	t.Log("PASS: Re-expand correctly reloads children and sets Expanded after load")
}
