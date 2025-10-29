package virtuallist

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// mockVirtualList is a mock implementation of VirtualList for testing
type mockVirtualList struct {
	items []string
}

func (m *mockVirtualList) ItemCount() int {
	return len(m.items)
}

func (m *mockVirtualList) RenderItem(index int, isCursor bool, width int) string {
	if index < 0 || index >= len(m.items) {
		return ""
	}

	item := m.items[index]

	// Truncate or pad to width
	if len(item) > width {
		item = item[:width]
	} else if len(item) < width {
		item = item + strings.Repeat(" ", width-len(item))
	}

	// Add cursor indicator
	if isCursor {
		return "> " + item[:width-2]
	}
	return "  " + item[:width-2]
}

func TestRenderer_EmptyList(t *testing.T) {
	list := &mockVirtualList{items: []string{}}
	renderer := New(list)
	renderer.SetSize(50, 10)

	view := renderer.View()
	if view != "Loading..." {
		t.Errorf("Expected 'Loading...' for empty list, got: %q", view)
	}
}

func TestRenderer_SingleItem(t *testing.T) {
	list := &mockVirtualList{items: []string{"item1"}}
	renderer := New(list)
	renderer.SetSize(50, 10)
	renderer.SetCursor(0)

	view := renderer.View()
	if !strings.Contains(view, "item1") {
		t.Errorf("Expected view to contain 'item1', got: %q", view)
	}
	if !strings.Contains(view, ">") {
		t.Errorf("Expected cursor indicator '>' for selected item, got: %q", view)
	}
}

func TestRenderer_MultipleItems(t *testing.T) {
	items := []string{"item1", "item2", "item3", "item4", "item5"}
	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 10)
	renderer.SetCursor(0)

	view := renderer.View()

	// Should contain all items since viewport is large enough
	for _, item := range items {
		if !strings.Contains(view, item) {
			t.Errorf("Expected view to contain %q, got: %q", item, view)
		}
	}
}

func TestRenderer_VirtualScrolling(t *testing.T) {
	// Create 100 items
	items := make([]string, 100)
	for i := range 100 {
		items[i] = fmt.Sprintf("item%03d", i)
	}

	list := &mockVirtualList{items: items}
	renderer := New(list)

	// Set viewport to only show 10 items
	renderer.SetSize(50, 10)
	renderer.SetCursor(0)

	view := renderer.View()

	// Should contain first 10 items
	for i := range 10 {
		if !strings.Contains(view, fmt.Sprintf("item%03d", i)) {
			t.Errorf("Expected view to contain item%03d", i)
		}
	}

	// Should NOT contain items beyond visible range
	if strings.Contains(view, "item050") {
		t.Errorf("Should not render items outside visible range, found item050")
	}
	if strings.Contains(view, "item099") {
		t.Errorf("Should not render items outside visible range, found item099")
	}
}

func TestRenderer_CursorMovement(t *testing.T) {
	items := make([]string, 50)
	for i := range 50 {
		items[i] = fmt.Sprintf("item%02d", i)
	}

	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 10)

	tests := []struct {
		name             string
		cursor           int
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			name:             "cursor at start",
			cursor:           0,
			shouldContain:    []string{"item00", "item01", "item09"},
			shouldNotContain: []string{"item15", "item20"},
		},
		{
			name:             "cursor in middle",
			cursor:           25,
			shouldContain:    []string{"item25", "item24", "item20"},
			shouldNotContain: []string{"item00", "item49", "item10"},
		},
		{
			name:             "cursor at end",
			cursor:           49,
			shouldContain:    []string{"item49", "item48", "item40"},
			shouldNotContain: []string{"item00", "item10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer.SetCursor(tt.cursor)
			view := renderer.View()

			for _, item := range tt.shouldContain {
				if !strings.Contains(view, item) {
					t.Errorf("Expected view to contain %q when cursor=%d, got: %q",
						item, tt.cursor, view)
				}
			}

			for _, item := range tt.shouldNotContain {
				if strings.Contains(view, item) {
					t.Errorf("Expected view NOT to contain %q when cursor=%d, got: %q",
						item, tt.cursor, view)
				}
			}
		})
	}
}

func TestRenderer_ScrollingBehavior(t *testing.T) {
	items := make([]string, 100)
	for i := range 100 {
		items[i] = fmt.Sprintf("item%03d", i)
	}

	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 10) // Show 10 items at a time

	// Start at top
	renderer.SetCursor(0)
	view := renderer.View()
	if !strings.Contains(view, "item000") {
		t.Error("Should show first item when cursor at 0")
	}

	// Move cursor down to 5 - should still show items 0-9
	renderer.SetCursor(5)
	view = renderer.View()
	if !strings.Contains(view, "item000") || !strings.Contains(view, "item009") {
		t.Error("Should still show first 10 items when cursor at 5")
	}

	// Move cursor to 15 - should scroll viewport
	renderer.SetCursor(15)
	view = renderer.View()
	if strings.Contains(view, "item000") {
		t.Error("Should have scrolled past item000 when cursor at 15")
	}
	if !strings.Contains(view, "item015") {
		t.Error("Should show item015 when cursor at 15")
	}

	// Move to end
	renderer.SetCursor(99)
	view = renderer.View()
	if !strings.Contains(view, "item099") {
		t.Error("Should show last item when cursor at end")
	}
	if strings.Contains(view, "item050") {
		t.Error("Should not show middle items when cursor at end")
	}
}

func TestRenderer_ResizeWindow(t *testing.T) {
	items := make([]string, 50)
	for i := range 50 {
		items[i] = fmt.Sprintf("item%02d", i)
	}

	list := &mockVirtualList{items: items}
	renderer := New(list)

	// Start with small viewport
	renderer.SetSize(50, 5)
	renderer.SetCursor(10)
	view1 := renderer.View()
	lines1 := strings.Count(view1, "\n") + 1

	// Resize to larger viewport
	renderer.SetSize(50, 20)
	view2 := renderer.View()
	lines2 := strings.Count(view2, "\n") + 1

	// Should show more items after resize
	if lines2 <= lines1 {
		t.Errorf("Expected more lines after resize: before=%d, after=%d", lines1, lines2)
	}

	// Should still show cursor item
	if !strings.Contains(view2, "item10") {
		t.Error("Should still show cursor item after resize")
	}
}

func TestRenderer_BoundaryConditions(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 10)

	tests := []struct {
		name   string
		cursor int
	}{
		{"cursor at -1", -1},
		{"cursor at 0", 0},
		{"cursor at last item", 2},
		{"cursor beyond end", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer.SetCursor(tt.cursor)
			view := renderer.View()

			// Should not panic and should produce some output
			if view == "" {
				t.Error("View should not be empty")
			}
		})
	}
}

func TestRenderer_Update(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 10)

	// Test that Update passes through to viewport
	sizeMsg := tea.WindowSizeMsg{Width: 80, Height: 20}
	cmd := renderer.Update(sizeMsg)

	// Should return a command or nil (both are valid)
	_ = cmd

	// Verify size was updated
	if renderer.Width() != 50 || renderer.Height() != 10 {
		t.Error("Update should not change size (size is set via SetSize)")
	}
}

func TestRenderer_GettersSetters(t *testing.T) {
	items := []string{"item1"}
	list := &mockVirtualList{items: items}
	renderer := New(list)

	// Test SetSize and getters
	renderer.SetSize(100, 25)
	if renderer.Width() != 100 {
		t.Errorf("Expected width=100, got %d", renderer.Width())
	}
	if renderer.Height() != 25 {
		t.Errorf("Expected height=25, got %d", renderer.Height())
	}

	// Test SetCursor and Cursor
	renderer.SetCursor(0)
	if renderer.Cursor() != 0 {
		t.Errorf("Expected cursor=0, got %d", renderer.Cursor())
	}

	// Test Viewport access
	vp := renderer.Viewport()
	if vp == nil {
		t.Error("Viewport() should not return nil")
	}
}

func TestRenderer_YOffsetBounds(t *testing.T) {
	// Test that YOffset doesn't go negative
	items := make([]string, 10)
	for i := range 10 {
		items[i] = fmt.Sprintf("item%d", i)
	}

	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 5)

	// Set cursor to 0
	renderer.SetCursor(0)

	// YOffset should be 0 or positive
	if renderer.Viewport().YOffset < 0 {
		t.Errorf("YOffset should never be negative, got %d", renderer.Viewport().YOffset)
	}

	// Move cursor to middle
	renderer.SetCursor(5)
	if renderer.Viewport().YOffset < 0 {
		t.Errorf(
			"YOffset should never be negative after scroll, got %d",
			renderer.Viewport().YOffset,
		)
	}
}

func TestRenderer_RenderWidth(t *testing.T) {
	items := []string{"short", "this is a very long item that exceeds width"}
	list := &mockVirtualList{items: items}
	renderer := New(list)

	// Set narrow width
	renderer.SetSize(20, 10)
	renderer.SetCursor(0)

	view := renderer.View()
	lines := strings.Split(view, "\n")

	// Each line should respect the width constraint
	for i, line := range lines {
		if len(line) > 20 {
			t.Errorf("Line %d exceeds width: len=%d, line=%q", i, len(line), line)
		}
	}
}

// TestRenderer_ScrollingAtBottomBug tests the bug where continuing to move down
// at the bottom causes the visible list to shrink
func TestRenderer_ScrollingAtBottomBug(t *testing.T) {
	// Create 20 items
	items := make([]string, 20)
	for i := range 20 {
		items[i] = fmt.Sprintf("item%02d", i)
	}

	list := &mockVirtualList{items: items}
	renderer := New(list)
	renderer.SetSize(50, 10) // Show 10 items at a time

	// Move cursor to near the end
	renderer.SetCursor(15)
	view := renderer.View()
	initialLines := strings.Count(view, "\n") + 1
	t.Logf("At cursor=15, YOffset=%d, lines=%d", renderer.Viewport().YOffset, initialLines)

	// Move to last item
	renderer.SetCursor(19)
	view = renderer.View()
	lines := strings.Count(view, "\n") + 1
	t.Logf("At cursor=19 (last), YOffset=%d, lines=%d", renderer.Viewport().YOffset, lines)
	t.Logf("View at cursor=19:\n%s", view)

	// Should still show 10 items (or all remaining items if less than 10)
	expectedLines := 10 // We have 20 items total, viewport shows 10
	if lines < expectedLines {
		t.Errorf("At bottom, expected %d visible lines, got %d. View:\n%s",
			expectedLines, lines, view)
	}

	// Verify we're showing items 10-19 (the last 10 items)
	for i := 10; i < 20; i++ {
		expectedItem := fmt.Sprintf("item%02d", i)
		if !strings.Contains(view, expectedItem) {
			t.Errorf("Expected view to contain %q at cursor=19, view:\n%s", expectedItem, view)
		}
	}

	// Should NOT show early items
	if strings.Contains(view, "item00") {
		t.Errorf("Should not show item00 when at bottom, view:\n%s", view)
	}

	// Test the critical bug: Simulate pressing down repeatedly past the end
	// (This would happen if Update() is forwarding key messages to viewport)
	for i := 0; i < 5; i++ {
		// Simulate pressing down (which shouldn't move cursor past end)
		// but might scroll the viewport if Update() forwards the message
		renderer.Update(tea.KeyMsg{Type: tea.KeyDown})
		view = renderer.View()
		lines = strings.Count(view, "\n") + 1
		yOffset := renderer.Viewport().YOffset

		t.Logf("After %d extra downs: YOffset=%d, lines=%d", i+1, yOffset, lines)

		// The list should NOT shrink - should still show 10 lines
		if lines < expectedLines {
			t.Errorf("After %d extra down keys, list shrunk to %d lines (expected %d). YOffset=%d, View:\n%s",
				i+1, lines, expectedLines, yOffset, view)
		}

		// YOffset should be clamped to at most (itemCount - visibleHeight) = 20 - 10 = 10
		maxYOffset := 10
		if yOffset > maxYOffset {
			t.Errorf("After %d extra down keys, YOffset=%d exceeds maximum %d",
				i+1, yOffset, maxYOffset)
		}
	}
}