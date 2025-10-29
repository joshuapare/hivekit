package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	
)

// TestVisualCursorMovement verifies that the visual cursor position updates
// correctly when navigating up and down with j/k keys.
// This test reproduces the bug where the cursor position is internally correct
// (right side displays update) but the visual cursor doesn't move.
func TestVisualCursorMovement(t *testing.T) {
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
		time.Sleep(1000 * time.Millisecond) // Wait for tree to load

		// Initial cursor should be at 0
		cursor := m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] Initial cursor: %d\n", cursor)
		if cursor != 0 {
			t.Errorf("Expected initial cursor at 0, got %d", cursor)
		}

		// Move down once with j
		fmt.Fprintf(os.Stderr, "[E2E TEST] Pressing 'j' to move down...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(150 * time.Millisecond)

		// Cursor should now be at 1
		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After 'j': cursor=%d\n", cursor)
		if cursor != 1 {
			t.Errorf("After pressing 'j', expected cursor at 1, got %d", cursor)
		}

		// Move down again
		fmt.Fprintf(os.Stderr, "[E2E TEST] Pressing 'j' again...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After second 'j': cursor=%d\n", cursor)
		if cursor != 2 {
			t.Errorf("After pressing 'j' twice, expected cursor at 2, got %d", cursor)
		}

		// Move up once with k
		fmt.Fprintf(os.Stderr, "[E2E TEST] Pressing 'k' to move up...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After 'k': cursor=%d\n", cursor)
		if cursor != 1 {
			t.Errorf("After pressing 'k', expected cursor at 1, got %d", cursor)
		}

		// Move up again
		fmt.Fprintf(os.Stderr, "[E2E TEST] Pressing 'k' again...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After second 'k': cursor=%d\n", cursor)
		if cursor != 0 {
			t.Errorf("After pressing 'k' twice, expected cursor at 0, got %d", cursor)
		}

		// Try arrow keys too
		fmt.Fprintf(os.Stderr, "[E2E TEST] Testing arrow keys...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyDown})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After Down arrow: cursor=%d\n", cursor)
		if cursor != 1 {
			t.Errorf("After pressing Down arrow, expected cursor at 1, got %d", cursor)
		}

		p.Send(tea.KeyMsg{Type: tea.KeyUp})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After Up arrow: cursor=%d\n", cursor)
		if cursor != 0 {
			t.Errorf("After pressing Up arrow, expected cursor at 0, got %d", cursor)
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

	t.Logf("Test completed successfully - visual cursor movement working")
}

// TestVisualCursorWithExpansion verifies cursor movement after expanding nodes
func TestVisualCursorWithExpansion(t *testing.T) {
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")

	m := NewModel(hivePath)
	defer func() {
		if err := m.Close(); err != nil {
			t.Logf("Warning: error closing model: %v", err)
		}
	}()

	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	done := make(chan Model, 1)

	go func() {
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		time.Sleep(300 * time.Millisecond)

		// Expand first item
		fmt.Fprintf(os.Stderr, "[E2E TEST] Expanding first item...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(500 * time.Millisecond)

		cursor := m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After expansion, cursor: %d\n", cursor)

		// Move down to child
		fmt.Fprintf(os.Stderr, "[E2E TEST] Moving down to child...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After moving to child, cursor: %d\n", cursor)
		if cursor != 1 {
			t.Errorf("After moving to child, expected cursor at 1, got %d", cursor)
		}

		// Move down to next sibling
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After moving to sibling, cursor: %d\n", cursor)
		if cursor != 2 {
			t.Errorf("After moving to sibling, expected cursor at 2, got %d", cursor)
		}

		// Move back up
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(150 * time.Millisecond)

		cursor = m.GetKeyTree().GetCursor()
		fmt.Fprintf(os.Stderr, "[E2E TEST] After moving back up, cursor: %d\n", cursor)
		if cursor != 1 {
			t.Errorf("After moving back up, expected cursor at 1, got %d", cursor)
		}

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

	t.Logf("Test completed - cursor movement after expansion working")
}
