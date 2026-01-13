package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestScrollingAtBottomBug tests the bug where continuing to press down at the bottom
// causes the visible list to shrink in the actual TUI.
func TestScrollingAtBottomBug(t *testing.T) {
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")

	m := NewModel(hivePath)
	defer func() {
		if err := m.Close(); err != nil {
			t.Logf("Warning: error closing model: %v", err)
		}
	}()

	// Use tea.Program to properly initialize
	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	done := make(chan Model, 1)

	// Set a small viewport height to make the bug easier to trigger
	viewportHeight := 15

	go func() {
		// Send WindowSizeMsg to set small viewport
		p.Send(tea.WindowSizeMsg{Width: 120, Height: viewportHeight + 10}) // +10 for headers/status
		time.Sleep(300 * time.Millisecond)                                 // Wait for tree to load

		// Expand multiple root items to get a longer tree
		fmt.Fprintf(os.Stderr, "[E2E TEST] Expanding multiple root items to create long list...\n")
		for i := 0; i < 5; i++ {
			p.Send(tea.KeyMsg{Type: tea.KeyEnter}) // Expand current item
			time.Sleep(150 * time.Millisecond)
			p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Move down
			time.Sleep(50 * time.Millisecond)
		}
		fmt.Fprintf(os.Stderr, "[E2E TEST] Expansion complete\n")

		// Press 'G' to jump to end
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
		time.Sleep(100 * time.Millisecond)

		// Press 'j' (down) 10 times past the end
		// This should trigger the bug if it exists
		for i := 0; i < 10; i++ {
			p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			time.Sleep(50 * time.Millisecond)
		}

		// Give it time to process
		time.Sleep(100 * time.Millisecond)

		// Quit (we'll check final state after program ends)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	}()

	go func() {
		finalModel, err := p.Run()
		if err != nil {
			t.Errorf("Error running program: %v", err)
			done <- Model{}
			return
		}
		done <- finalModel.(Model)
	}()

	select {
	case m = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
	}

	// Now check the final state
	items := m.GetKeyTree().GetItems()
	cursor := m.GetKeyTree().GetCursor()
	viewport := m.GetKeyTree().GetViewport()

	if len(items) == 0 {
		t.Fatal("No items loaded")
	}

	fmt.Fprintf(os.Stderr, "[E2E TEST] Final state: cursor=%d, itemCount=%d, YOffset=%d, viewport.Height=%d\n",
		cursor, len(items), viewport.YOffset, viewport.Height)

	// Cursor should be at the last item
	expectedCursor := len(items) - 1
	if cursor != expectedCursor {
		t.Errorf("Expected cursor at last item (%d), got %d", expectedCursor, cursor)
	}

	// YOffset should be clamped to not exceed (itemCount - visibleHeight)
	visibleHeight := viewport.Height // No header in the tree virtual list
	maxYOffset := len(items) - visibleHeight
	if maxYOffset < 0 {
		maxYOffset = 0
	}

	if viewport.YOffset > maxYOffset {
		t.Errorf("YOffset (%d) exceeds maximum (%d). This indicates scrolling past the end.",
			viewport.YOffset, maxYOffset)
	}

	// Render the final view and check it contains the expected items
	view := m.View()
	lines := strings.Split(view, "\n")

	// Count non-empty lines (actual rendered items)
	nonEmptyLines := 0
	var renderedItems []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "─") && !strings.Contains(line, "Registry Hive") {
			nonEmptyLines++
			renderedItems = append(renderedItems, line)
		}
	}

	fmt.Fprintf(os.Stderr, "[E2E TEST] Rendered %d non-empty item lines\n", nonEmptyLines)

	// The view should show close to visibleHeight items (allowing some margin for headers)
	// If the list is shrinking, we'd see far fewer items
	minExpectedLines := visibleHeight - 5 // Allow some margin for headers/borders
	if nonEmptyLines < minExpectedLines {
		t.Errorf("List appears to have shrunk! Expected at least %d lines, got %d. YOffset=%d\nRendered items:\n%s",
			minExpectedLines, nonEmptyLines, viewport.YOffset, strings.Join(renderedItems, "\n"))
	}

	// Verify we're showing items near the end of the list
	if len(items) > 0 {
		lastItem := items[len(items)-1]
		viewStr := strings.Join(renderedItems, "\n")
		if !strings.Contains(viewStr, lastItem.Name) {
			t.Errorf("View should contain the last item (%q), but doesn't.\nView:\n%s",
				lastItem.Name, viewStr)
		}
	}
}

// TestScrollingAtBottomBugDetailed captures the actual rendering at each step
func TestScrollingAtBottomBugDetailed(t *testing.T) {
	t.Skip("Skipping detailed test - needs rewrite to not access stale model. TestScrollingAtBottomBug provides coverage.")

	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-xp-software")

	m := NewModel(hivePath)
	defer func() {
		if err := m.Close(); err != nil {
			t.Logf("Warning: error closing model: %v", err)
		}
	}()

	// Use tea.Program to properly initialize
	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))

	type snapshot struct {
		cursor         int
		yOffset        int
		itemCount      int
		visibleLines   int
		viewportHeight int
	}

	snapshots := []snapshot{}
	captureSnapshot := func() {
		items := m.GetKeyTree().GetItems()
		cursor := m.GetKeyTree().GetCursor()
		viewport := m.GetKeyTree().GetViewport()
		view := m.View()

		lines := strings.Split(view, "\n")
		nonEmptyLines := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "─") && !strings.Contains(line, "Registry Hive") {
				nonEmptyLines++
			}
		}

		snap := snapshot{
			cursor:         cursor,
			yOffset:        viewport.YOffset,
			itemCount:      len(items),
			visibleLines:   nonEmptyLines,
			viewportHeight: viewport.Height,
		}
		snapshots = append(snapshots, snap)
		fmt.Fprintf(os.Stderr, "[SNAPSHOT %d] cursor=%d, yOffset=%d, visibleLines=%d, itemCount=%d, vpHeight=%d\n",
			len(snapshots), snap.cursor, snap.yOffset, snap.visibleLines, snap.itemCount, snap.viewportHeight)
	}

	done := make(chan Model, 1)

	go func() {
		// Send WindowSizeMsg
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 25})
		time.Sleep(300 * time.Millisecond)

		// Expand first few items to get more items than viewport height
		for i := 0; i < 3; i++ {
			p.Send(tea.KeyMsg{Type: tea.KeyEnter}) // Expand
			time.Sleep(100 * time.Millisecond)
			p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Move down
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)

		// Capture initial state
		captureSnapshot()

		// Jump to end with 'G'
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
		time.Sleep(100 * time.Millisecond)
		captureSnapshot()

		// Press down 10 times
		for i := 0; i < 10; i++ {
			p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			time.Sleep(50 * time.Millisecond)
			captureSnapshot()
		}

		// Quit
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	}()

	go func() {
		finalModel, err := p.Run()
		if err != nil {
			t.Errorf("Error running program: %v", err)
			done <- Model{}
			return
		}
		done <- finalModel.(Model)
	}()

	select {
	case m = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
	}

	// Analyze snapshots for shrinking
	if len(snapshots) < 2 {
		t.Fatal("Not enough snapshots captured")
	}

	// After jumping to end, visible lines should stay relatively constant
	// If the bug exists, visibleLines will decrease with each down press
	afterEndSnap := snapshots[1] // After 'G'
	baselineVisibleLines := afterEndSnap.visibleLines

	fmt.Fprintf(os.Stderr, "\n[ANALYSIS] Baseline visible lines at end: %d\n", baselineVisibleLines)

	for i := 2; i < len(snapshots); i++ {
		snap := snapshots[i]
		fmt.Fprintf(os.Stderr, "[ANALYSIS] Snapshot %d: visibleLines=%d (baseline=%d, diff=%d)\n",
			i, snap.visibleLines, baselineVisibleLines, snap.visibleLines-baselineVisibleLines)

		// If visible lines decreased by more than 2, the list is shrinking
		if snap.visibleLines < baselineVisibleLines-2 {
			t.Errorf("List is shrinking! After %d down presses at end: visibleLines went from %d to %d (YOffset=%d)",
				i-1, baselineVisibleLines, snap.visibleLines, snap.yOffset)
		}

		// Check YOffset bounds
		maxYOffset := snap.itemCount - snap.viewportHeight
		if maxYOffset < 0 {
			maxYOffset = 0
		}
		if snap.yOffset > maxYOffset {
			t.Errorf("Snapshot %d: YOffset (%d) exceeds maximum (%d)", i, snap.yOffset, maxYOffset)
		}
	}
}
