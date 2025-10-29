package keytree

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/virtuallist"
)

// TestScrollingAtBottomBug tests the bug where continuing to press down at the bottom
// causes the visible list to shrink.
func TestScrollingAtBottomBug(t *testing.T) {
	// Create a tree with 20 items
	items := make([]Item, 20)
	for i := 0; i < 20; i++ {
		items[i] = Item{
			Path:        fmt.Sprintf("key%02d", i),
			Name:        fmt.Sprintf("key%02d", i),
			Depth:       0,
			HasChildren: false,
		}
	}

	// Create model
	m := NewModel("/fake/path")
	m.state.SetItems(items)

	// Set viewport size to show 10 items at a time
	viewportHeight := 10
	viewportWidth := 50
	m.renderer = nil // Force re-initialization
	m.renderer = virtuallist.New(&m)
	m.renderer.SetSize(viewportWidth, viewportHeight)

	// Move cursor to item 15 (near the end)
	m.MoveTo(15)

	// Render and parse view
	view := m.View()
	lines := strings.Split(view, "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}

	t.Logf("At cursor=15: YOffset=%d, non-empty lines=%d", m.renderer.Viewport().YOffset, nonEmptyLines)
	t.Logf("View:\n%s", view)

	// Record what the second visible item is
	if len(lines) < 2 {
		t.Fatal("Not enough lines in view")
	}
	secondItem := lines[1]
	t.Logf("Second item at cursor=15: %q", secondItem)

	// Move to last item (19)
	m.MoveTo(19)

	view = m.View()
	lines = strings.Split(view, "\n")
	nonEmptyLines = 0
	var visibleItems []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			nonEmptyLines++
			visibleItems = append(visibleItems, line)
		}
	}

	t.Logf("At cursor=19 (last): YOffset=%d, non-empty lines=%d", m.renderer.Viewport().YOffset, nonEmptyLines)
	t.Logf("Visible items: %v", visibleItems)
	t.Logf("Full view:\n%s", view)

	// Should still show 10 items (or close to it)
	if nonEmptyLines < viewportHeight-1 {
		t.Errorf("At cursor=19, expected ~%d lines, got %d. This suggests list is shrinking.", viewportHeight, nonEmptyLines)
	}

	// When at the last item (19), with 20 total items and viewport showing 10,
	// we should see items 10-19
	// Verify the view contains "key10" through "key19"
	viewStr := strings.Join(visibleItems, "\n")
	for i := 10; i < 20; i++ {
		expected := fmt.Sprintf("key%02d", i)
		if !strings.Contains(viewStr, expected) {
			t.Errorf("Expected view to contain %q when at last item, but it doesn't. View:\n%s", expected, viewStr)
		}
	}

	// Should NOT contain early items
	if strings.Contains(viewStr, "key00") || strings.Contains(viewStr, "key01") {
		t.Errorf("View should not contain early items when at bottom. View:\n%s", viewStr)
	}

	// Now the critical test: press down 5 more times past the end
	// The list should NOT shrink
	for i := 0; i < 5; i++ {
		// Try to move down (will be clamped at 19)
		m.MoveDown()

		view = m.View()
		lines = strings.Split(view, "\n")
		nonEmptyLines = 0
		visibleItems = []string{}
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				nonEmptyLines++
				visibleItems = append(visibleItems, line)
			}
		}

		yOffset := m.renderer.Viewport().YOffset
		cursor := m.nav.Cursor()

		t.Logf("After %d extra down presses: cursor=%d, YOffset=%d, non-empty lines=%d",
			i+1, cursor, yOffset, nonEmptyLines)

		// The list should NOT shrink - should still show ~10 lines
		if nonEmptyLines < viewportHeight-1 {
			t.Errorf("After %d extra down presses, list shrunk to %d lines (expected ~%d). YOffset=%d, cursor=%d\nView:\n%s",
				i+1, nonEmptyLines, viewportHeight, yOffset, cursor, view)
		}

		// YOffset should be clamped to at most (itemCount - visibleHeight) = 20 - 10 = 10
		maxYOffset := m.state.ItemCount() - viewportHeight
		if maxYOffset < 0 {
			maxYOffset = 0
		}
		if yOffset > maxYOffset {
			t.Errorf("After %d extra down presses, YOffset=%d exceeds maximum %d",
				i+1, yOffset, maxYOffset)
		}

		// Verify we're still showing the last 10 items
		viewStr := strings.Join(visibleItems, "\n")
		for j := 10; j < 20; j++ {
			expected := fmt.Sprintf("key%02d", j)
			if !strings.Contains(viewStr, expected) {
				t.Errorf("After %d extra downs, expected view to contain %q. View:\n%s",
					i+1, expected, viewStr)
			}
		}

		// Verify the second visible item stays consistent (first item should be key10)
		if len(visibleItems) >= 2 {
			firstItem := visibleItems[0]
			if !strings.Contains(firstItem, "key10") {
				t.Errorf("After %d extra downs, expected first visible item to contain 'key10', got: %q",
					i+1, firstItem)
			}
		}
	}
}

// TestScrollingAtBottomBugWithDeepTree tests with a deeper tree (expanded items)
func TestScrollingAtBottomBugWithDeepTree(t *testing.T) {
	// Create a tree with nested items (simulating expanded view)
	items := make([]Item, 50)
	for i := 0; i < 50; i++ {
		depth := i / 10 // Every 10 items increases depth
		items[i] = Item{
			Path:        fmt.Sprintf("root\\sub%d\\key%02d", depth, i),
			Name:        fmt.Sprintf("key%02d", i),
			Depth:       depth,
			HasChildren: false,
		}
	}

	// Create model
	m := NewModel("/fake/path")
	m.state.SetItems(items)

	// Set viewport size to show 10 items at a time
	viewportHeight := 10
	viewportWidth := 80
	m.renderer = nil
	m.renderer = virtuallist.New(&m)
	m.renderer.SetSize(viewportWidth, viewportHeight)

	// Jump to near the end
	targetCursor := 45
	m.MoveTo(targetCursor)

	fmt.Fprintf(os.Stderr, "[TEST] Starting at cursor=%d, itemCount=%d\n", targetCursor, m.state.ItemCount())

	// Move to last item
	lastIdx := m.state.ItemCount() - 1
	m.MoveTo(lastIdx)

	view := m.View()
	lines := strings.Split(view, "\n")
	nonEmptyLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmptyLines++
		}
	}

	t.Logf("At last item (cursor=%d): YOffset=%d, lines=%d", lastIdx, m.renderer.Viewport().YOffset, nonEmptyLines)

	if nonEmptyLines < viewportHeight-1 {
		t.Errorf("At last item, expected ~%d lines, got %d", viewportHeight, nonEmptyLines)
	}

	// Press down multiple times past the end
	for i := 0; i < 10; i++ {
		m.MoveDown()

		view = m.View()
		lines = strings.Split(view, "\n")
		nonEmptyLines = 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				nonEmptyLines++
			}
		}

		yOffset := m.renderer.Viewport().YOffset

		t.Logf("After %d extra downs: YOffset=%d, lines=%d", i+1, yOffset, nonEmptyLines)

		if nonEmptyLines < viewportHeight-1 {
			t.Errorf("After %d extra downs, list shrunk to %d lines (expected ~%d). View:\n%s",
				i+1, nonEmptyLines, viewportHeight, view)
		}

		// Check YOffset bounds
		maxYOffset := m.state.ItemCount() - viewportHeight
		if maxYOffset < 0 {
			maxYOffset = 0
		}
		if yOffset > maxYOffset {
			t.Errorf("After %d extra downs, YOffset=%d exceeds max %d", i+1, yOffset, maxYOffset)
		}
	}
}
