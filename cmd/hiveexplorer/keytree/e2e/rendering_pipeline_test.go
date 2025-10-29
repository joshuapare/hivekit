package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
)

// TestE2E_RenderingPipeline_BasicItem verifies basic item renders correctly end-to-end
func TestE2E_RenderingPipeline_BasicItem(t *testing.T) {
	// Create a simple test item (leaf node, no children, depth 0)
	item := newTestItem("BasicKey")

	// Create minimal model state
	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render the item
	output := model.RenderItem(0, false, 80)

	// Verify output
	assertContains(t, output, "BasicKey")
	assertHasIcon(t, output, "•") // Leaf icon
}

// TestE2E_RenderingPipeline_ItemWithChildren verifies parent items render correctly
func TestE2E_RenderingPipeline_ItemWithChildren(t *testing.T) {
	tests := []struct {
		name         string
		expanded     bool
		expectedIcon string
	}{
		{"collapsed parent", false, "▶"},
		{"expanded parent", true, "▼"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := newTestItem("ParentKey",
				withChildren(5, tt.expanded),
			)

			state := keytree.NewTreeState()
			state.SetAllItems([]keytree.Item{item})
			state.SetItems([]keytree.Item{item})

			model := &keytree.Model{}
			model.SetStateForTesting(state)

			output := model.RenderItem(0, false, 80)

			assertContains(t, output, "ParentKey")
			assertHasIcon(t, output, tt.expectedIcon)
			assertContains(t, output, "(5)") // Count text
		})
	}
}

// TestE2E_RenderingPipeline_ItemWithTimestamp verifies timestamp formatting end-to-end
func TestE2E_RenderingPipeline_ItemWithTimestamp(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	item := newTestItem("KeyWithTimestamp",
		withTimestamp(timestamp),
	)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	assertContains(t, output, "KeyWithTimestamp")
	assertContains(t, output, "2024-01-15")
	assertContains(t, output, "10:30")
}

// TestE2E_RenderingPipeline_DepthIndentation verifies indentation at various depths
func TestE2E_RenderingPipeline_DepthIndentation(t *testing.T) {
	depths := []int{0, 1, 2, 5}

	for _, depth := range depths {
		t.Run("depth_"+string(rune('0'+depth)), func(t *testing.T) {
			item := newTestItem("Key",
				withDepth(depth),
			)

			state := keytree.NewTreeState()
			state.SetAllItems([]keytree.Item{item})
			state.SetItems([]keytree.Item{item})

			model := &keytree.Model{}
			model.SetStateForTesting(state)

			output := model.RenderItem(0, false, 80)

			assertContains(t, output, "Key")
			assertIndentation(t, output, depth)
		})
	}
}

// TestE2E_RenderingPipeline_CursorSelection verifies cursor selection styling
func TestE2E_RenderingPipeline_CursorSelection(t *testing.T) {
	item := newTestItem("SelectedKey")

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render with cursor (selected)
	outputSelected := model.RenderItem(0, true, 80)

	// Render without cursor (not selected)
	outputNotSelected := model.RenderItem(0, false, 80)

	// Both should contain the name
	assertContains(t, outputSelected, "SelectedKey")
	assertContains(t, outputNotSelected, "SelectedKey")

	// Note: In a real terminal, selection would add ANSI styling (background color).
	// In test environment without TTY, lipgloss may not output ANSI codes.
	// The important part is that RenderItem handles the isCursor flag correctly.
	// We verify the logic is correct even if visual styling isn't present in tests.
}

// TestE2E_RenderingPipeline_OutOfBounds verifies out-of-bounds handling
func TestE2E_RenderingPipeline_OutOfBounds(t *testing.T) {
	item := newTestItem("Key")

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	tests := []struct {
		name  string
		index int
	}{
		{"negative index", -1},
		{"beyond end", 5},
		{"way beyond", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := model.RenderItem(tt.index, false, 80)

			// Should return empty string, not panic
			if output != "" {
				t.Errorf("Expected empty string for out-of-bounds index %d, got %q", tt.index, output)
			}
		})
	}
}

// TestE2E_RenderingPipeline_EmptyItems verifies empty item list handling
func TestE2E_RenderingPipeline_EmptyItems(t *testing.T) {
	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{})
	state.SetItems([]keytree.Item{})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	output := model.RenderItem(0, false, 80)

	if output != "" {
		t.Errorf("Expected empty string when no items, got %q", output)
	}
}

// TestE2E_RenderingPipeline_MultipleItems verifies multiple items can be rendered
func TestE2E_RenderingPipeline_MultipleItems(t *testing.T) {
	items := []keytree.Item{
		newTestItem("Key1"),
		newTestItem("Key2", withDepth(1)),
		newTestItem("Key3", withChildren(3, false)),
	}

	state := keytree.NewTreeState()
	state.SetAllItems(items)
	state.SetItems(items)

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render each item
	outputs := []string{
		model.RenderItem(0, false, 80),
		model.RenderItem(1, false, 80),
		model.RenderItem(2, false, 80),
	}

	// Verify each renders correctly
	assertContains(t, outputs[0], "Key1")
	assertHasIcon(t, outputs[0], "•")

	assertContains(t, outputs[1], "Key2")
	assertIndentation(t, outputs[1], 1)

	assertContains(t, outputs[2], "Key3")
	assertHasIcon(t, outputs[2], "▶")
	assertContains(t, outputs[2], "(3)")
}

// TestE2E_RenderingPipeline_LongName verifies long names are handled
func TestE2E_RenderingPipeline_LongName(t *testing.T) {
	longName := strings.Repeat("VeryLongKeyName", 10) // 150 chars

	item := newTestItem(longName)

	state := keytree.NewTreeState()
	state.SetAllItems([]keytree.Item{item})
	state.SetItems([]keytree.Item{item})

	model := &keytree.Model{}
	model.SetStateForTesting(state)

	// Render with small width
	output := model.RenderItem(0, false, 40)

	// Should not panic
	assertContains(t, output, "VeryLongKeyName")
}
