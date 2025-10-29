package e2e

import (
	"testing"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestE2E_DiffMode_AddedItem verifies added items render with correct styling
func TestE2E_DiffMode_AddedItem(t *testing.T) {
	item := newTestItem("AddedKey",
		withDiffStatus(hive.DiffAdded),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Verify "+" prefix
	assertHasPrefix(t, output, '+')

	// Verify name is present
	assertContains(t, output, "AddedKey")

	// Note: ANSI styling may not be present in test environment (lipgloss detects TTY)
	// The important part is that the prefix and content are correct
}

// TestE2E_DiffMode_RemovedItem verifies removed items render with correct styling
func TestE2E_DiffMode_RemovedItem(t *testing.T) {
	item := newTestItem("RemovedKey",
		withDiffStatus(hive.DiffRemoved),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Verify "-" prefix
	assertHasPrefix(t, output, '-')

	// Verify name is present
	assertContains(t, output, "RemovedKey")

	// Note: ANSI styling may not be present in test environment
}

// TestE2E_DiffMode_ModifiedItem verifies modified items render with correct styling
func TestE2E_DiffMode_ModifiedItem(t *testing.T) {
	item := newTestItem("ModifiedKey",
		withDiffStatus(hive.DiffModified),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Verify "~" prefix
	assertHasPrefix(t, output, '~')

	// Verify name is present
	assertContains(t, output, "ModifiedKey")

	// Note: ANSI styling may not be present in test environment
}

// TestE2E_DiffMode_UnchangedItem verifies unchanged items render without diff markers
func TestE2E_DiffMode_UnchangedItem(t *testing.T) {
	item := newTestItem("UnchangedKey",
		withDiffStatus(hive.DiffUnchanged),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Verify name is present
	assertContains(t, output, "UnchangedKey")

	// Unchanged items should not have visible diff prefixes (+, -, ~)
	// They have a space prefix which is not visible
	assertNotContains(t, output, "+")
	assertNotContains(t, output, "-")
	assertNotContains(t, output, "~")
}

// TestE2E_DiffMode_MixedTree verifies a tree with mixed diff statuses
func TestE2E_DiffMode_MixedTree(t *testing.T) {
	items := []keytree.Item{
		newTestItem("AddedKey", withDiffStatus(hive.DiffAdded)),
		newTestItem("RemovedKey", withDiffStatus(hive.DiffRemoved)),
		newTestItem("ModifiedKey", withDiffStatus(hive.DiffModified)),
		newTestItem("UnchangedKey", withDiffStatus(hive.DiffUnchanged)),
	}

	state := keytree.NewTreeState()
	state.SetAllItems(items)
	state.SetItems(items)

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render each item
	output0 := model.RenderItem(0, false, 80) // Added
	output1 := model.RenderItem(1, false, 80) // Removed
	output2 := model.RenderItem(2, false, 80) // Modified
	output3 := model.RenderItem(3, false, 80) // Unchanged

	// Verify added item
	assertHasPrefix(t, output0, '+')
	assertContains(t, output0, "AddedKey")

	// Verify removed item
	assertHasPrefix(t, output1, '-')
	assertContains(t, output1, "RemovedKey")

	// Verify modified item
	assertHasPrefix(t, output2, '~')
	assertContains(t, output2, "ModifiedKey")

	// Verify unchanged item (no visible prefix)
	assertContains(t, output3, "UnchangedKey")
	assertNotContains(t, output3, "+")
	assertNotContains(t, output3, "-")
	assertNotContains(t, output3, "~")
}

// TestE2E_DiffMode_AddedWithChildren verifies added parent items
func TestE2E_DiffMode_AddedWithChildren(t *testing.T) {
	item := newTestItem("NewParent",
		withDiffStatus(hive.DiffAdded),
		withChildren(5, false),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should have all features:
	// - "+" prefix for added
	// - "▶" icon for collapsed parent
	// - "(5)" count
	assertHasPrefix(t, output, '+')
	assertHasIcon(t, output, "▶")
	assertContains(t, output, "(5)")
	assertContains(t, output, "NewParent")
}

// TestE2E_DiffMode_RemovedWithChildren verifies removed parent items
func TestE2E_DiffMode_RemovedWithChildren(t *testing.T) {
	item := newTestItem("RemovedParent",
		withDiffStatus(hive.DiffRemoved),
		withChildren(3, false),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	// Should have:
	// - "-" prefix for removed
	// - "▶" icon for collapsed parent
	// - "(3)" count
	// - Strikethrough styling
	assertHasPrefix(t, output, '-')
	assertHasIcon(t, output, "▶")
	assertContains(t, output, "(3)")
	assertContains(t, output, "RemovedParent")
	// Note: Strikethrough styling may not be present in test environment
}

// TestE2E_DiffMode_ModifiedAtDepth verifies modified items at various depths
func TestE2E_DiffMode_ModifiedAtDepth(t *testing.T) {
	depths := []int{0, 1, 2}

	for _, depth := range depths {
		t.Run("depth_"+string(rune('0'+depth)), func(t *testing.T) {
			item := newTestItem("ModifiedKey",
				withDiffStatus(hive.DiffModified),
				withDepth(depth),
			)

			state := keytree.NewTreeState()
			state.SetAllItems([]keytree.Item{item})
			state.SetItems([]keytree.Item{item})

			model := &keytree.Model{}
			model.SetStateForTesting(state)

			output := model.RenderItem(0, false, 80)

			// Should have "~" prefix and correct indentation
			assertHasPrefix(t, output, '~')
			assertIndentation(t, output, depth)
			assertContains(t, output, "ModifiedKey")
		})
	}
}

// TestE2E_DiffMode_AllStatusesCombined verifies complex diff scenario
func TestE2E_DiffMode_AllStatusesCombined(t *testing.T) {
	items := []keytree.Item{
		newTestItem("Root", withDiffStatus(hive.DiffUnchanged)),
		newTestItem("AddedChild", withDiffStatus(hive.DiffAdded), withDepth(1)),
		newTestItem("RemovedChild", withDiffStatus(hive.DiffRemoved), withDepth(1)),
		newTestItem("ModifiedChild", withDiffStatus(hive.DiffModified), withDepth(1)),
		newTestItem("DeepAdded", withDiffStatus(hive.DiffAdded), withDepth(2)),
	}

	state := keytree.NewTreeState()
	state.SetAllItems(items)
	state.SetItems(items)

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render all items and verify each has correct attributes
	for i, item := range items {
		output := model.RenderItem(i, false, 80)

		// Verify name is present
		assertContains(t, output, item.Name)

		// Verify correct diff prefix based on status
		switch item.DiffStatus {
		case hive.DiffAdded:
			assertHasPrefix(t, output, '+')
		case hive.DiffRemoved:
			assertHasPrefix(t, output, '-')
		case hive.DiffModified:
			assertHasPrefix(t, output, '~')
		case hive.DiffUnchanged:
			// Unchanged has space prefix (not visible)
			assertNotContains(t, output, "+")
			assertNotContains(t, output, "-")
			assertNotContains(t, output, "~")
		}

		// Verify indentation if depth > 0
		if item.Depth > 0 {
			assertIndentation(t, output, item.Depth)
		}
	}
}
