package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestExpandedKeyWithChildrenRemovedByAncestor tests this scenario:
// 1. Expand A > B > C > D
// 2. Collapse B (removes C and D from view)
// 3. C is removed from items array, but what if C is later added back?
//
// More specifically: Does C remember it was expanded?
// When we re-expand B, does C come back with Expanded=true or false?
func TestExpandedKeyWithChildrenRemovedByAncestor(t *testing.T) {
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

	// Expand B
	helper.SendKey(tea.KeyDown) // Move to B
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("A\\B", []keytree.KeyInfo{
		{Path: "A\\B\\C", Name: "C", SubkeyN: 1, ValueN: 0},
	})

	// Expand C
	helper.SendKey(tea.KeyDown) // Move to C
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("A\\B\\C", []keytree.KeyInfo{
		{Path: "A\\B\\C\\D", Name: "D", SubkeyN: 0, ValueN: 1},
	})

	t.Log("Tree: [A, B, C (expanded), D]")
	if helper.GetTreeItemCount() != 4 {
		t.Errorf("Expected 4 items, got %d", helper.GetTreeItemCount())
	}

	// Navigate back to B and collapse it
	helper.SendKey(tea.KeyUp) // C -> B
	helper.SendKey(tea.KeyUp) // B (should be at B now, but let's verify)

	itemB := helper.GetCurrentTreeItem()
	if itemB.Name != "B" {
		// We might have gone too far up, let's navigate properly
		for helper.GetCurrentTreeItem().Name != "B" {
			helper.SendKey(tea.KeyDown)
		}
	}

	t.Log("Collapsing B (which has expanded child C)")
	helper.SendKey(tea.KeyEnter) // Collapse B

	afterCollapse := helper.GetTreeItemCount()
	t.Logf("After collapsing B: %d items", afterCollapse)

	if afterCollapse != 2 { // Should be [A, B]
		t.Errorf("Expected 2 items after collapse, got %d", afterCollapse)
	}

	// Now re-expand B
	t.Log("Re-expanding B")
	helper.SendKey(tea.KeyEnter) // Expand B
	helper.LoadChildKeys("A\\B", []keytree.KeyInfo{
		{Path: "A\\B\\C", Name: "C", SubkeyN: 1, ValueN: 0},
	})

	afterReexpand := helper.GetTreeItemCount()
	t.Logf("After re-expanding B: %d items", afterReexpand)

	if afterReexpand != 3 { // Should be [A, B, C]
		t.Errorf("Expected 3 items after re-expand, got %d", afterReexpand)
	}

	// Navigate to C
	helper.SendKey(tea.KeyDown) // Move to C

	itemC := helper.GetCurrentTreeItem()
	if itemC.Name != "C" {
		t.Fatalf("Expected to be on C, but on %q", itemC.Name)
	}

	t.Logf("At C: Expanded=%v", itemC.Expanded)

	// THE KEY TEST: Is C expanded or collapsed?
	// It should be collapsed (false) because it's a new TreeItem
	// But if there's a bug, it might still be expanded
	if itemC.Expanded {
		t.Error("BUG: C is marked as expanded even though it was just re-added")
		t.Error("First Enter will collapse it (no children visible)")
		t.Error("Second Enter will expand it and load children")

		// Demonstrate the bug
		t.Log("Pressing Enter on C (first time)...")
		itemsBefore := helper.GetTreeItemCount()
		helper.SendKey(tea.KeyEnter)
		itemsAfterFirst := helper.GetTreeItemCount()
		itemCAfterFirst := helper.GetCurrentTreeItem()

		t.Logf("After first Enter: Expanded=%v, items: %d -> %d",
			itemCAfterFirst.Expanded, itemsBefore, itemsAfterFirst)

		if !itemCAfterFirst.Expanded {
			t.Log("First Enter collapsed C (BUG CONFIRMED)")

			t.Log("Pressing Enter on C (second time)...")
			helper.SendKey(tea.KeyEnter)
			// Would need to load children here
			helper.LoadChildKeys("A\\B\\C", []keytree.KeyInfo{
				{Path: "A\\B\\C\\D", Name: "D", SubkeyN: 0, ValueN: 1},
			})

			itemsAfterSecond := helper.GetTreeItemCount()
			t.Logf("After second Enter: items: %d", itemsAfterSecond)

			if itemsAfterSecond > itemsAfterFirst {
				t.Error("BUG CONFIRMED: Took TWO Enter presses to expand C")
			}
		}
	} else {
		t.Log("PASS: C correctly starts collapsed after re-adding")
	}
}

// TestNavigateToSiblingAfterCollapsingDeepTree tests the exact user scenario:
// "expand 5 keys inwards, go back up and close on key 2, then navigate to next key and try to expand"
func TestNavigateToSiblingAfterCollapsingDeepTree(t *testing.T) {
	helper := NewTestHelper("test.hive")

	// Create a tree with multiple siblings at each level for navigation
	rootKeys := []keytree.KeyInfo{
		{Path: "Root1", Name: "Root1", SubkeyN: 2, ValueN: 0},
		{Path: "Root2", Name: "Root2", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)

	// Expand Root1
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1", []keytree.KeyInfo{
		{Path: "Root1\\L1_A", Name: "L1_A", SubkeyN: 2, ValueN: 0},
		{Path: "Root1\\L1_B", Name: "L1_B", SubkeyN: 1, ValueN: 0}, // Second child at level 1
	})

	// Expand L1_A (first child at level 1)
	helper.SendKey(tea.KeyDown)
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1\\L1_A", []keytree.KeyInfo{
		{Path: "Root1\\L1_A\\L2_A", Name: "L2_A", SubkeyN: 1, ValueN: 0},
		{Path: "Root1\\L1_A\\L2_B", Name: "L2_B", SubkeyN: 0, ValueN: 1},
	})

	// Expand L2_A (going deeper to level 3)
	helper.SendKey(tea.KeyDown)
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1\\L1_A\\L2_A", []keytree.KeyInfo{
		{Path: "Root1\\L1_A\\L2_A\\L3", Name: "L3", SubkeyN: 1, ValueN: 0},
	})

	// Expand L3 (level 4)
	helper.SendKey(tea.KeyDown)
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1\\L1_A\\L2_A\\L3", []keytree.KeyInfo{
		{Path: "Root1\\L1_A\\L2_A\\L3\\L4", Name: "L4", SubkeyN: 1, ValueN: 0},
	})

	// Expand L4 (level 5 - this is the "5 keys inwards" from user description)
	helper.SendKey(tea.KeyDown)
	helper.SendKey(tea.KeyEnter)
	helper.LoadChildKeys("Root1\\L1_A\\L2_A\\L3\\L4", []keytree.KeyInfo{
		{Path: "Root1\\L1_A\\L2_A\\L3\\L4\\L5", Name: "L5", SubkeyN: 0, ValueN: 1},
	})

	t.Log("Expanded 5 levels deep: Root1 > L1_A > L2_A > L3 > L4 > L5")
	t.Logf("Tree has %d items", helper.GetTreeItemCount())

	// Navigate back up to "key 2" (L1_A at level 1)
	t.Log("Navigating back to L1_A (level 1)")
	for helper.GetCurrentTreeItem().Name != "L1_A" {
		helper.SendKey(tea.KeyUp)
	}

	// Collapse L1_A
	t.Log("Collapsing L1_A")
	helper.SendKey(tea.KeyEnter)

	itemsAfterCollapse := helper.GetTreeItemCount()
	t.Logf("After collapse: %d items", itemsAfterCollapse)

	// Navigate to next key (L1_B, sibling of L1_A)
	t.Log("Navigating to L1_B (sibling)")
	helper.SendKey(tea.KeyDown)

	itemL1B := helper.GetCurrentTreeItem()
	if itemL1B.Name != "L1_B" {
		t.Errorf("Expected to be on L1_B, but on %q", itemL1B.Name)
	}

	t.Logf("At L1_B: Expanded=%v, HasChildren=%v", itemL1B.Expanded, itemL1B.HasChildren)

	// Try to expand L1_B
	t.Log("Trying to expand L1_B (CRITICAL TEST)")

	// Press Enter to expand
	t.Log("Pressing Enter to expand L1_B...")
	itemsBefore := helper.GetTreeItemCount()
	helper.SendKey(tea.KeyEnter)

	// Load children (simulating async load completion)
	helper.LoadChildKeys("Root1\\L1_B", []keytree.KeyInfo{
		{Path: "Root1\\L1_B\\ChildOfB", Name: "ChildOfB", SubkeyN: 0, ValueN: 1},
	})

	itemsAfter := helper.GetTreeItemCount()
	currentItem := helper.GetCurrentTreeItem()

	t.Logf("After Enter and loading: Expanded=%v, items: %d -> %d",
		currentItem.Expanded, itemsBefore, itemsAfter)

	// Verify expansion worked
	if itemsAfter <= itemsBefore {
		t.Errorf("BUG: L1_B should have expanded, but items didn't increase (%d -> %d)", itemsBefore, itemsAfter)
	} else if !currentItem.Expanded {
		t.Errorf("BUG: L1_B children appeared but Expanded flag is false")
	} else {
		t.Log("PASS: L1_B expanded correctly on first Enter press")
	}
}
