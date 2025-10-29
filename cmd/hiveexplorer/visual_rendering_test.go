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

// TestVisualCursorRendering checks that the cursor is visually rendered on the correct row.
// This test examines the actual View() output to verify cursor highlighting.
func TestVisualCursorRendering(t *testing.T) {
	t.Skip("Skipping rendering test - needs rewrite to not access stale model. TestRendererCursorSync and TestVisualCursorMovement provide coverage.")

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

	go func() {
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		time.Sleep(300 * time.Millisecond) // Wait for tree to load

		// Get initial view
		initialView := m.View()
		fmt.Fprintf(os.Stderr, "[TEST] Initial view:\n%s\n", initialView)

		// Check that the FIRST row is visually highlighted (cursor at 0)
		if !containsCursorAtRow(initialView, 0) {
			t.Errorf("Initial view should show cursor at row 0, but doesn't")
		}

		// Move down once with j
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'j' to move down...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(100 * time.Millisecond)

		// Get view after move
		viewAfterJ := m.View()
		fmt.Fprintf(os.Stderr, "[TEST] View after 'j':\n%s\n", viewAfterJ)

		// Check that the SECOND row is now visually highlighted (cursor at 1)
		if !containsCursorAtRow(viewAfterJ, 1) {
			t.Errorf("After pressing 'j', cursor should be visually at row 1, but isn't")
		}

		// Move down again
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'j' again...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(100 * time.Millisecond)

		viewAfterJ2 := m.View()
		fmt.Fprintf(os.Stderr, "[TEST] View after second 'j':\n%s\n", viewAfterJ2)

		// Check that the THIRD row is now visually highlighted (cursor at 2)
		if !containsCursorAtRow(viewAfterJ2, 2) {
			t.Errorf("After pressing 'j' twice, cursor should be visually at row 2, but isn't")
		}

		// Move up with k
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'k' to move up...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(100 * time.Millisecond)

		viewAfterK := m.View()
		fmt.Fprintf(os.Stderr, "[TEST] View after 'k':\n%s\n", viewAfterK)

		// Check that the SECOND row is highlighted again (cursor at 1)
		if !containsCursorAtRow(viewAfterK, 1) {
			t.Errorf("After pressing 'k', cursor should be visually at row 1, but isn't")
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

	t.Logf("Test completed")
}

// containsCursorAtRow checks if the view has a cursor-styled row at the given row index.
// The cursor style in the tree uses lipgloss styles that add ANSI escape codes.
// We check for the row being wrapped in the cursor style (bold + background color).
func containsCursorAtRow(view string, rowIndex int) bool {
	// Split view into lines
	lines := strings.Split(view, "\n")

	// Find the tree section (before the right-side panel)
	// The tree is on the left side of the screen
	treeLines := []string{}
	for _, line := range lines {
		// Skip empty lines and header/footer lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Skip lines that are just borders
		if strings.Contains(line, "───") {
			continue
		}
		// Skip the "Registry Hive" header
		if strings.Contains(line, "Registry Hive") {
			continue
		}
		// Tree lines contain the bullet point "•"
		if strings.Contains(line, "•") {
			treeLines = append(treeLines, line)
		}
	}

	if len(treeLines) == 0 {
		fmt.Fprintf(os.Stderr, "[TEST] No tree lines found in view\n")
		return false
	}

	if rowIndex >= len(treeLines) {
		fmt.Fprintf(os.Stderr, "[TEST] Row index %d out of bounds (only %d tree lines)\n", rowIndex, len(treeLines))
		return false
	}

	targetLine := treeLines[rowIndex]
	fmt.Fprintf(os.Stderr, "[TEST] Checking row %d: %q\n", rowIndex, targetLine)

	// The cursor style adds ANSI escape codes for bold and background color
	// Look for ANSI escape sequences that indicate styling
	// The cursor row should have different styling than non-cursor rows

	// A simple heuristic: cursor rows have more ANSI codes (bold + bg color)
	// while non-cursor rows have fewer codes
	ansiCount := strings.Count(targetLine, "\x1b[")

	// Also check if other rows have FEWER ANSI codes
	// If the target row has significantly more codes, it's likely the cursor
	otherRowsHighlyStyled := 0
	for i, line := range treeLines {
		if i == rowIndex {
			continue
		}
		otherAnsiCount := strings.Count(line, "\x1b[")
		if otherAnsiCount >= ansiCount {
			otherRowsHighlyStyled++
		}
	}

	fmt.Fprintf(os.Stderr, "[TEST] Row %d has %d ANSI codes, %d other rows have >= codes\n",
		rowIndex, ansiCount, otherRowsHighlyStyled)

	// The cursor row should have MORE styling than most other rows
	// If more than half of other rows have equal or more styling, this isn't the cursor
	if otherRowsHighlyStyled > len(treeLines)/2 {
		fmt.Fprintf(os.Stderr, "[TEST] FAIL: Too many other rows have similar/more styling\n")
		return false
	}

	return ansiCount > 0
}
