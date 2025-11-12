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

// TestRendererCursorSync verifies that the virtuallist renderer's cursor
// stays in sync with the keytree's internal cursor position.
// This is the TRUE test of the visual cursor bug.
func TestRendererCursorSync(t *testing.T) {
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

	// Send all key events in sequence
	go func() {
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		time.Sleep(300 * time.Millisecond) // Wait for tree to load

		// Move down once with j
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'j' to move down...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(150 * time.Millisecond)

		// Move down again
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'j' again...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(150 * time.Millisecond)

		// Move up with k
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'k' to move up...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(150 * time.Millisecond)

		// Jump to end with G
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'G' to jump to end...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
		time.Sleep(500 * time.Millisecond)

		// Jump to start with g
		fmt.Fprintf(os.Stderr, "[TEST] Pressing 'g' to jump to start...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		time.Sleep(500 * time.Millisecond)

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

	// Now check the FINAL model state
	keyTreeCursor := m.GetKeyTree().GetCursor()
	rendererCursor := m.GetKeyTree().GetRendererCursor()
	fmt.Fprintf(os.Stderr, "[TEST] FINAL STATE: keyTree.cursor=%d, renderer.cursor=%d\n",
		keyTreeCursor, rendererCursor)

	if keyTreeCursor != rendererCursor {
		t.Errorf("FINAL STATE MISMATCH: keyTree cursor=%d but renderer cursor=%d",
			keyTreeCursor, rendererCursor)
	} else {
		t.Logf("✓ Cursors are in sync: both at position %d", keyTreeCursor)
	}

	t.Logf("Test completed")
}
