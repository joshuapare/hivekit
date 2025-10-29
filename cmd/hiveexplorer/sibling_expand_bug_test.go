package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestSiblingExpandAfterCollapsingExpandedDescendants tests this specific scenario:
// 1. Expand A > B > C > D > E (5 levels)
// 2. Navigate back to B (level 1)
// 3. Collapse B - this removes C, D, E from view
// 4. Navigate to B's sibling (B2)
// 5. Try to expand B2 - BUG: if B2 was previously expanded and then hidden by a parent collapse,
//    it might take multiple Enter presses
func TestSiblingExpandAfterCollapsingExpandedDescendants(t *testing.T) {
	helper := NewTestHelper("test.hive")

	// Create root with two siblings A and B, each with deep children
	rootKeys := []keytree.KeyInfo{
		{Path: "A", Name: "A", SubkeyN: 2, ValueN: 0}, // Has 2 children: A1 and A2
	}
	helper.LoadRootKeys(rootKeys)

	// Expand A
	t.Log("Expanding A")
	helper.SendKey(tea.KeyEnter)
	childrenA := []keytree.KeyInfo{
		{Path: "A\\A1", Name: "A1", SubkeyN: 2, ValueN: 0}, // Has children A1a, A1b
		{Path: "A\\A2", Name: "A2", SubkeyN: 1, ValueN: 0}, // Has child A2a
	}
	helper.LoadChildKeys("A", childrenA)

	// Now tree: [A, A1, A2]
	if helper.GetTreeItemCount() != 3 {
		t.Errorf("Expected 3 items, got %d", helper.GetTreeItemCount())
	}

	// Expand A1 (first child)
	t.Log("Navigating to A1 and expanding")
	helper.SendKey(tea.KeyDown) // Move to A1
	helper.SendKey(tea.KeyEnter) // Expand A1
	childrenA1 := []keytree.KeyInfo{
		{Path: "A\\A1\\A1a", Name: "A1a", SubkeyN: 1, ValueN: 0},
		{Path: "A\\A1\\A1b", Name: "A1b", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadChildKeys("A\\A1", childrenA1)

	// Now tree: [A, A1, A1a, A1b, A2]
	if helper.GetTreeItemCount() != 5 {
		t.Errorf("Expected 5 items, got %d", helper.GetTreeItemCount())
	}

	// Expand A1a (going deeper)
	t.Log("Navigating to A1a and expanding")
	helper.SendKey(tea.KeyDown) // Move to A1a
	helper.SendKey(tea.KeyEnter) // Expand A1a
	childrenA1a := []keytree.KeyInfo{
		{Path: "A\\A1\\A1a\\Deep", Name: "Deep", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadChildKeys("A\\A1\\A1a", childrenA1a)

	// Now tree: [A, A1, A1a, Deep, A1b, A2]
	expectedCount := 6
	if helper.GetTreeItemCount() != expectedCount {
		t.Errorf("Expected %d items, got %d", expectedCount, helper.GetTreeItemCount())
	}

	t.Logf("Current tree has %d items with A, A1 (expanded), A1a (expanded), Deep, A1b, A2", helper.GetTreeItemCount())

	// Also expand A2 for good measure
	t.Log("Navigating to A2 and expanding")
	// Current position: A1a, need to go: Deep, A1b, A2
	helper.SendKey(tea.KeyDown) // Deep
	helper.SendKey(tea.KeyDown) // A1b
	helper.SendKey(tea.KeyDown) // A2
	helper.SendKey(tea.KeyEnter) // Expand A2
	childrenA2 := []keytree.KeyInfo{
		{Path: "A\\A2\\A2a", Name: "A2a", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadChildKeys("A\\A2", childrenA2)

	// Now tree: [A, A1, A1a, Deep, A1b, A2, A2a]
	expectedCount = 7
	if helper.GetTreeItemCount() != expectedCount {
		t.Errorf("Expected %d items, got %d", expectedCount, helper.GetTreeItemCount())
	}

	// Verify A2 is expanded
	currentItem := helper.GetCurrentTreeItem()
	if !currentItem.Expanded {
		t.Error("A2 should be expanded")
	}
	t.Logf("A2 is expanded: %v", currentItem.Expanded)

	// Now navigate back to A1 and collapse it
	t.Log("\n=== Navigating back to A1 and collapsing ===")
	// Current: A2, need to go to A1
	// Path: A2 -> A1b -> Deep -> A1a -> A1
	for helper.GetCurrentTreeItem().Name != "A1" {
		helper.SendKey(tea.KeyUp)
	}

	t.Logf("At A1, collapsing...")
	helper.SendKey(tea.KeyEnter) // Collapse A1

	// After collapse: [A, A1, A2, A2a]
	// A1's children (A1a, Deep, A1b) should be removed
	expectedAfterCollapse := 4 // A, A1 (collapsed), A2 (still expanded), A2a
	actualAfterCollapse := helper.GetTreeItemCount()
	t.Logf("After collapsing A1: tree has %d items (expected %d)", actualAfterCollapse, expectedAfterCollapse)

	if actualAfterCollapse != expectedAfterCollapse {
		t.Errorf("Expected %d items after collapse, got %d", expectedAfterCollapse, actualAfterCollapse)
	}

	// Verify A1 is collapsed
	currentItem = helper.GetCurrentTreeItem()
	if currentItem.Expanded {
		t.Error("A1 should be collapsed")
	}

	// Now try to expand A2 again - but A2 is already expanded!
	// Navigate to A2
	t.Log("\n=== Navigating to A2 (which was expanded before A1 collapsed) ===")
	helper.SendKey(tea.KeyDown) // A1 -> A2

	currentItem = helper.GetCurrentTreeItem()
	if currentItem.Name != "A2" {
		t.Errorf("Expected to be on A2, but on %q", currentItem.Name)
	}

	t.Logf("At A2, Expanded=%v", currentItem.Expanded)

	// Test that Enter properly toggles A2 expand/collapse
	// A2 is currently expanded with children visible
	t.Log("\n=== Pressing Enter on A2 (should toggle to collapsed) ===")

	itemsBefore := helper.GetTreeItemCount()
	helper.SendKey(tea.KeyEnter)
	itemsAfter := helper.GetTreeItemCount()

	currentItem = helper.GetCurrentTreeItem()
	t.Logf("After Enter: Expanded=%v, items: %d -> %d", currentItem.Expanded, itemsBefore, itemsAfter)

	// A2 was expanded, so Enter should collapse it (toggle behavior)
	if itemsAfter < itemsBefore && !currentItem.Expanded {
		t.Log("PASS: Enter correctly toggled A2 from expanded to collapsed")
	} else if itemsAfter == itemsBefore && currentItem.Expanded {
		t.Error("BUG: A2 stayed expanded even though Enter should toggle it")
	} else if itemsAfter == itemsBefore && !currentItem.Expanded {
		t.Log("A2 toggled to collapsed but children count unchanged - checking state")

		// Press Enter again to verify toggle works both ways
		helper.SendKey(tea.KeyEnter)
		itemsAfterSecond := helper.GetTreeItemCount()

		if itemsAfterSecond > itemsAfter {
			t.Error("BUG: Required TWO Enter presses - first collapsed, second expanded")
		}
	}
}

// TestMultipleEnterPressesNeeded creates a simpler scenario:
// - Expand A > B
// - Collapse A (B is removed from view but B.Expanded might still be true)
// - Re-expand A
// - Navigate to B
// - Check if B.Expanded is still true - if so, first Enter will collapse, second will expand
func TestMultipleEnterPressesNeeded(t *testing.T) {
	helper := NewTestHelper("test.hive")

	rootKeys := []keytree.KeyInfo{
		{Path: "A", Name: "A", SubkeyN: 2, ValueN: 0},
	}
	helper.LoadRootKeys(rootKeys)

	// Expand A
	helper.SendKey(tea.KeyEnter)
	childrenA := []keytree.KeyInfo{
		{Path: "A\\B", Name: "B", SubkeyN: 1, ValueN: 0},
		{Path: "A\\C", Name: "C", SubkeyN: 1, ValueN: 0},
	}
	helper.LoadChildKeys("A", childrenA)

	// Expand B
	helper.SendKey(tea.KeyDown) // Move to B
	helper.SendKey(tea.KeyEnter) // Expand B
	childrenB := []keytree.KeyInfo{
		{Path: "A\\B\\B1", Name: "B1", SubkeyN: 0, ValueN: 1},
	}
	helper.LoadChildKeys("A\\B", childrenB)

	t.Logf("Tree: [A, B (expanded), B1, C]")

	// Navigate back to A and collapse
	helper.SendKey(tea.KeyUp) // B -> A
	helper.SendKey(tea.KeyEnter) // Collapse A

	t.Logf("After collapsing A, tree has %d items", helper.GetTreeItemCount())

	// Re-expand A
	helper.SendKey(tea.KeyEnter) // Expand A
	// Need to reload children since we cleared loaded flag
	helper.LoadChildKeys("A", childrenA)

	t.Logf("After re-expanding A, tree has %d items", helper.GetTreeItemCount())

	// Navigate to B
	helper.SendKey(tea.KeyDown) // A -> B

	itemB := helper.GetCurrentTreeItem()
	t.Logf("At B: Expanded=%v", itemB.Expanded)

	// THE BUG: If B still has Expanded=true from before, first Enter will collapse
	if itemB.Expanded {
		t.Error("BUG CONFIRMED: B.Expanded is still true even though it was removed when A collapsed")
		t.Error("This means first Enter will collapse B (no visible change since no children loaded)")
		t.Error("Second Enter will expand B")

		// Test it
		t.Log("Pressing Enter first time...")
		helper.SendKey(tea.KeyEnter)
		itemBAfter1 := helper.GetCurrentTreeItem()
		t.Logf("After first Enter: Expanded=%v", itemBAfter1.Expanded)

		if !itemBAfter1.Expanded {
			t.Log("First Enter collapsed B as expected (BUG)")

			t.Log("Pressing Enter second time...")
			helper.SendKey(tea.KeyEnter)
			itemBAfter2 := helper.GetCurrentTreeItem()
			t.Logf("After second Enter: Expanded=%v", itemBAfter2.Expanded)

			if itemBAfter2.Expanded {
				t.Error("BUG CONFIRMED: Required TWO Enter presses to expand B")
			}
		}
	} else {
		t.Log("PASS: B.Expanded was properly reset when A collapsed")
	}
}
