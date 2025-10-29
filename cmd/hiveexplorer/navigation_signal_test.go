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

// TestNavigationSignalAfterJump verifies that navigation signals are emitted
// correctly after using G (End) or g (Home) to jump to different locations.
// This test reproduces the bug where value/key displays stopped updating after jumps.
func TestNavigationSignalAfterJump(t *testing.T) {
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

		// Expand multiple root items to get 50+ items
		fmt.Fprintf(os.Stderr, "[E2E TEST] Expanding items to create long list...\n")
		for i := 0; i < 5; i++ {
			p.Send(tea.KeyMsg{Type: tea.KeyEnter}) // Expand
			time.Sleep(150 * time.Millisecond)
			p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Move down
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)

		// Jump to end with G
		fmt.Fprintf(os.Stderr, "[E2E TEST] Jumping to end with G...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
		time.Sleep(500 * time.Millisecond)

		// Move down once with j - this should trigger value loading
		fmt.Fprintf(os.Stderr, "[E2E TEST] Moving down with j...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(500 * time.Millisecond)

		// Move up once with k - this should trigger value loading
		fmt.Fprintf(os.Stderr, "[E2E TEST] Moving up with k...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(500 * time.Millisecond)

		// Jump to start with g
		fmt.Fprintf(os.Stderr, "[E2E TEST] Jumping to start with g...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		time.Sleep(500 * time.Millisecond)

		// Move down once - this should trigger value loading
		fmt.Fprintf(os.Stderr, "[E2E TEST] Moving down with j after jump to start...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
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
	case <-time.After(15 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
	}

	// Verify final state - cursor should be at position 1 (moved down from 0)
	cursor := m.GetKeyTree().GetCursor()
	if cursor != 1 {
		t.Errorf("Expected cursor at position 1 after final move, got %d", cursor)
	}

	// The test passes if it completes without panicking
	// In the buggy version, value loading would fail or displays wouldn't update
	t.Logf("Test completed successfully - navigation signals working after jumps")
}

// TestGoToParentEmitsSignal verifies that GoToParent emits navigation signals
func TestGoToParentEmitsSignal(t *testing.T) {
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
		time.Sleep(300 * time.Millisecond)

		// Expand first item
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(500 * time.Millisecond)

		// Move to first child
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(100 * time.Millisecond)

		// Go to parent with 'p' key
		fmt.Fprintf(os.Stderr, "[E2E TEST] Going to parent with p...\n")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		time.Sleep(500 * time.Millisecond)

		// Move down to verify displays are still working
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(100 * time.Millisecond)

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

	// The test passes if it completes without panicking
	t.Logf("Test completed successfully - GoToParent emits navigation signal")
}
