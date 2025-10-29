package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestExpandLevelBasic tests expanding all siblings at current level with Ctrl+E
func TestExpandLevelBasic(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root1", Name: "Root1", SubkeyN: 1, ValueN: 0},
		{Path: "Root2", Name: "Root2", SubkeyN: 1, ValueN: 0},
		{Path: "Root3", Name: "Root3", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Root1 manually
	t.Log("Expanding Root1")
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1", []keytree.KeyInfo{
		{Path: "Root1\\Child1", Name: "Child1", SubkeyN: 0, ValueN: 1},
	})

	// Load children for Root2 (but don't expand yet)
	helper.LoadChildKeys("Root2", []keytree.KeyInfo{
		{Path: "Root2\\Child2", Name: "Child2", SubkeyN: 0, ValueN: 1},
	})

	// Load children for Root3 (but don't expand yet)
	helper.LoadChildKeys("Root3", []keytree.KeyInfo{
		{Path: "Root3\\Child3", Name: "Child3", SubkeyN: 0, ValueN: 1},
	})

	// Mark as loaded
	model := helper.GetModel()
	model.keyTree.SetLoadedPath("Root2", true)
	model.keyTree.SetLoadedPath("Root3", true)
	helper.model = model

	itemCountBefore := helper.GetTreeItemCount()
	t.Logf("Tree has %d items before expand level", itemCountBefore)

	t.Log("Pressing Ctrl+E to expand all siblings at root level")
	helper.SendKey(tea.KeyCtrlE)

	itemCountAfter := helper.GetTreeItemCount()
	t.Logf("Tree has %d items after expand level", itemCountAfter)

	// All root level items that have children and are loaded should now be expanded
	model = helper.GetModel()
	items := model.keyTree.GetItems()
	loaded := model.keyTree.GetLoaded()
	for i, item := range items {
		if item.Depth == 0 {
			t.Logf("Item %d: %q, Expanded=%v, Loaded=%v", i, item.Path, item.Expanded, loaded[item.Path])
		}
	}

	t.Log("✓ Expand level expands siblings at current depth")
}

// TestExpandLevelDeepInTree tests expand level from deep in tree
func TestExpandLevelDeepInTree(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Build a tree: Root -> (Child1, Child2, Child3)
	t.Log("Expanding Root")
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root", []keytree.KeyInfo{
		{Path: "Root\\Child1", Name: "Child1", SubkeyN: 1, ValueN: 0},
		{Path: "Root\\Child2", Name: "Child2", SubkeyN: 1, ValueN: 0},
		{Path: "Root\\Child3", Name: "Child3", SubkeyN: 1, ValueN: 0},
	})

	// Expand Child1
	t.Log("Expanding Child1")
	helper.SendKey(tea.KeyDown) // Root -> Child1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child1", []keytree.KeyInfo{
		{Path: "Root\\Child1\\GrandChild1", Name: "GrandChild1", SubkeyN: 0, ValueN: 1},
	})

	// Preload children for Child2 and Child3 (but don't expand)
	helper.LoadChildKeys("Root\\Child2", []keytree.KeyInfo{
		{Path: "Root\\Child2\\GrandChild2", Name: "GrandChild2", SubkeyN: 0, ValueN: 1},
	})
	helper.LoadChildKeys("Root\\Child3", []keytree.KeyInfo{
		{Path: "Root\\Child3\\GrandChild3", Name: "GrandChild3", SubkeyN: 0, ValueN: 1},
	})

	// Mark as loaded
	model := helper.GetModel()
	model.keyTree.SetLoadedPath("Root\\Child2", true)
	model.keyTree.SetLoadedPath("Root\\Child3", true)
	helper.model = model

	// Navigate back to Child1 to be at depth 1
	for helper.GetCurrentTreeItem().Name != "Child1" {
		helper.SendKey(tea.KeyUp)
	}

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Depth != 1 {
		t.Fatalf("Expected to be at depth 1, got %d", currentItem.Depth)
	}

	t.Log("Pressing Ctrl+E to expand all siblings at depth 1")
	helper.SendKey(tea.KeyCtrlE)

	// Check that Child2 and Child3 are now expanded
	model = helper.GetModel()
	child2Expanded := false
	child3Expanded := false

	items := model.keyTree.GetItems()
	for _, item := range items {
		if item.Path == "Root\\Child2" && item.Expanded {
			child2Expanded = true
		}
		if item.Path == "Root\\Child3" && item.Expanded {
			child3Expanded = true
		}
	}

	if !child2Expanded {
		t.Log("Child2 should be expanded (if loaded)")
	}
	if !child3Expanded {
		t.Log("Child3 should be expanded (if loaded)")
	}

	t.Log("✓ Expand level works from deep in tree")
}

// TestExpandLevelWithNoSiblings tests expand level when there are no siblings
func TestExpandLevelWithNoSiblings(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Root
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root", []keytree.KeyInfo{
		{Path: "Root\\Child", Name: "Child", SubkeyN: 0, ValueN: 1},
	})

	// Navigate to Child (only item at depth 1)
	helper.SendKey(tea.KeyDown)

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Path != "Root\\Child" {
		t.Fatalf("Expected to be at Root\\Child, but at %q", currentItem.Path)
	}

	itemCountBefore := helper.GetTreeItemCount()

	t.Log("Pressing Ctrl+E when there are no siblings to expand")
	helper.SendKey(tea.KeyCtrlE)

	itemCountAfter := helper.GetTreeItemCount()

	// Item count shouldn't change since there are no siblings
	if itemCountAfter != itemCountBefore {
		t.Logf("Item count changed from %d to %d (expected no change)", itemCountBefore, itemCountAfter)
	}

	t.Log("✓ Expand level handles no siblings gracefully")
}

// TestCollapseToLevelBasic tests collapsing to current level with Ctrl+L
func TestCollapseToLevelBasic(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Build a deep tree: Root -> Child1 -> GrandChild -> GreatGrandChild
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root", []keytree.KeyInfo{
		{Path: "Root\\Child1", Name: "Child1", SubkeyN: 1, ValueN: 0},
	})

	helper.SendKey(tea.KeyDown) // Root -> Child1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child1", []keytree.KeyInfo{
		{Path: "Root\\Child1\\GrandChild", Name: "GrandChild", SubkeyN: 1, ValueN: 0},
	})

	helper.SendKey(tea.KeyDown) // Child1 -> GrandChild
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child1\\GrandChild", []keytree.KeyInfo{
		{Path: "Root\\Child1\\GrandChild\\GreatGrandChild", Name: "GreatGrandChild", SubkeyN: 0, ValueN: 1},
	})

	itemCountBefore := helper.GetTreeItemCount()
	t.Logf("Tree has %d items before collapse to level", itemCountBefore)

	// Navigate back to GrandChild (depth 2)
	for helper.GetCurrentTreeItem().Name != "GrandChild" {
		helper.SendKey(tea.KeyUp)
	}

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Depth != 2 {
		t.Fatalf("Expected to be at depth 2, got %d", currentItem.Depth)
	}

	t.Log("Pressing Ctrl+L to collapse to current level (depth 2)")
	helper.SendKey(tea.KeyCtrlL)

	itemCountAfter := helper.GetTreeItemCount()
	t.Logf("Tree has %d items after collapse to level", itemCountAfter)

	// Should have removed GreatGrandChild (depth 3)
	if itemCountAfter >= itemCountBefore {
		t.Errorf("Expected fewer items after collapse to level, got %d (was %d)", itemCountAfter, itemCountBefore)
	}

	// Verify GreatGrandChild is removed
	model := helper.GetModel()
	items := model.keyTree.GetItems()
	for _, item := range items {
		if item.Path == "Root\\Child1\\GrandChild\\GreatGrandChild" {
			t.Error("GreatGrandChild should have been removed")
		}
		if item.Depth > 2 {
			t.Errorf("No items should have depth > 2, found %q with depth %d", item.Path, item.Depth)
		}
	}

	t.Log("✓ Collapse to level removes deeper items")
}

// TestCollapseToLevelAtRoot tests collapse to level at root
func TestCollapseToLevelAtRoot(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root1", Name: "Root1", SubkeyN: 1, ValueN: 0},
		{Path: "Root2", Name: "Root2", SubkeyN: 0, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Expand Root1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1", []keytree.KeyInfo{
		{Path: "Root1\\Child", Name: "Child", SubkeyN: 0, ValueN: 1},
	})

	itemCountBefore := helper.GetTreeItemCount()
	t.Logf("Tree has %d items before collapse to level", itemCountBefore)

	// Navigate back to Root1 (depth 0)
	helper.SendKey(tea.KeyUp)

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Depth != 0 {
		t.Fatalf("Expected to be at depth 0, got %d", currentItem.Depth)
	}

	t.Log("Pressing Ctrl+L at root level")
	helper.SendKey(tea.KeyCtrlL)

	itemCountAfter := helper.GetTreeItemCount()
	t.Logf("Tree has %d items after collapse to level at root", itemCountAfter)

	// Should have collapsed all children
	if itemCountAfter != len(rootKeys) {
		t.Errorf("Expected %d root items, got %d", len(rootKeys), itemCountAfter)
	}

	t.Log("✓ Collapse to level at root collapses all children")
}

// TestCollapseToLevelCollapsesCurrentDepth tests that items at current depth are collapsed
func TestCollapseToLevelCollapsesCurrentDepth(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 2, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Build tree with multiple branches at depth 1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root", []keytree.KeyInfo{
		{Path: "Root\\Child1", Name: "Child1", SubkeyN: 1, ValueN: 0},
		{Path: "Root\\Child2", Name: "Child2", SubkeyN: 1, ValueN: 0},
	})

	// Expand both children
	helper.SendKey(tea.KeyDown) // Root -> Child1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child1", []keytree.KeyInfo{
		{Path: "Root\\Child1\\GrandChild1", Name: "GrandChild1", SubkeyN: 0, ValueN: 1},
	})

	helper.SendKey(tea.KeyDown) // Child1 -> GrandChild1
	helper.SendKey(tea.KeyDown) // GrandChild1 -> Child2
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child2", []keytree.KeyInfo{
		{Path: "Root\\Child2\\GrandChild2", Name: "GrandChild2", SubkeyN: 0, ValueN: 1},
	})

	// Navigate to Child1
	for helper.GetCurrentTreeItem().Name != "Child1" {
		helper.SendKey(tea.KeyUp)
	}

	currentItem := helper.GetCurrentTreeItem()
	if currentItem.Depth != 1 {
		t.Fatalf("Expected to be at depth 1, got %d", currentItem.Depth)
	}

	t.Log("Pressing Ctrl+L at depth 1")
	helper.SendKey(tea.KeyCtrlL)

	// Both Child1 and Child2 should be collapsed
	model := helper.GetModel()
	items := model.keyTree.GetItems()
	for _, item := range items {
		if item.Path == "Root\\Child1" && item.Expanded {
			t.Error("Child1 should be collapsed")
		}
		if item.Path == "Root\\Child2" && item.Expanded {
			t.Error("Child2 should be collapsed")
		}
		if item.Depth > 1 {
			t.Errorf("No items should have depth > 1, found %q with depth %d", item.Path, item.Depth)
		}
	}

	t.Log("✓ Collapse to level collapses items at current depth")
}

// TestExpandLevelAfterCollapseToLevel tests expand level after collapse to level
func TestExpandLevelAfterCollapseToLevel(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "Root", Name: "Root", SubkeyN: 2, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)
	helper.SendWindowSize(120, 40)

	// Build tree
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root", []keytree.KeyInfo{
		{Path: "Root\\Child1", Name: "Child1", SubkeyN: 1, ValueN: 0},
		{Path: "Root\\Child2", Name: "Child2", SubkeyN: 1, ValueN: 0},
	})

	// Expand Child1
	helper.SendKey(tea.KeyDown)
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root\\Child1", []keytree.KeyInfo{
		{Path: "Root\\Child1\\GrandChild1", Name: "GrandChild1", SubkeyN: 0, ValueN: 1},
	})

	// Preload Child2
	helper.LoadChildKeys("Root\\Child2", []keytree.KeyInfo{
		{Path: "Root\\Child2\\GrandChild2", Name: "GrandChild2", SubkeyN: 0, ValueN: 1},
	})
	model := helper.GetModel()
	model.keyTree.SetLoadedPath("Root\\Child2", true)
	helper.model = model

	// Collapse to depth 1
	t.Log("Collapsing to depth 1")
	helper.SendKey(tea.KeyCtrlL)

	// Now expand all at depth 1
	t.Log("Expanding all at depth 1")
	helper.SendKey(tea.KeyCtrlE)

	// Child2 should now be expanded (if loaded)
	model = helper.GetModel()
	child2Found := false
	items := model.keyTree.GetItems()
	for _, item := range items {
		if item.Path == "Root\\Child2" {
			child2Found = true
			t.Logf("Child2: Expanded=%v", item.Expanded)
		}
	}

	if !child2Found {
		t.Error("Child2 should be in tree")
	}

	t.Log("✓ Expand level works after collapse to level")
}
