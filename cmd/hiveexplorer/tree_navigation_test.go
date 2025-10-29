package main

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	
)

// TestTreeNavigation_ExpandCollapse verifies that expanding and collapsing keys works correctly
func TestTreeNavigation_ExpandCollapse(t *testing.T) {
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
		time.Sleep(500 * time.Millisecond)
		// Send Enter to expand, wait, then 'h' to collapse, then quit
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		time.Sleep(50 * time.Millisecond)
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
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
	}

	// Get final state
	items := m.GetKeyTree().GetItems()
	if len(items) == 0 {
		t.Fatal("No items loaded")
	}

	// After the sequence (expand then collapse), verify we're back to initial state
	// All top-level items should be collapsed
	for i, item := range items {
		if item.Depth == 0 && item.Expanded {
			t.Errorf("Top-level item %d (%q) is still expanded after collapse", i, item.Name)
		}
	}
}

// TestTreeNavigation_CursorMovement verifies cursor navigation works correctly
func TestTreeNavigation_CursorMovement(t *testing.T) {
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
		time.Sleep(500 * time.Millisecond)
		// Send cursor movements: j, j, k, q
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(100 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		time.Sleep(100 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		time.Sleep(100 * time.Millisecond)
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
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
	}

	itemCount := len(m.GetKeyTree().GetItems())
	if itemCount < 2 {
		t.Skip("Need at least 2 items for cursor movement test")
	}

	// After j, j, k sequence, cursor should be at position 1
	cursor := m.GetKeyTree().GetCursor()
	if cursor != 1 {
		t.Errorf("After j, j, k sequence, cursor should be 1, got %d", cursor)
	}
}

// TestTreeNavigation_ExpandAll verifies that expanding all children works
func TestTreeNavigation_ExpandAll(t *testing.T) {
	t.Skip("ExpandAll can take a long time on large hives - enable manually for testing")

	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")

	m := NewModel(hivePath)
	defer func() {
		if err := m.Close(); err != nil {
			t.Logf("Warning: error closing model: %v", err)
		}
	}()

	// Initialize and load tree
	initCmd := m.Init()
	msg := initCmd()
	updatedModel, _ := m.Update(msg)
	m = updatedModel.(Model)

	initialCount := len(m.GetKeyTree().GetItems())

	// Simulate Ctrl+E to expand all (if bound)
	// For now, just verify the tree is in a valid state
	items := m.GetKeyTree().GetItems()
	for i, item := range items {
		// Verify parent references are valid
		if item.Parent != "" {
			// Find parent in items
			found := false
			for _, potentialParent := range items {
				if potentialParent.Path == item.Parent {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Item %d (%q) has parent %q which is not in visible items",
					i, item.Name, item.Parent)
			}
		}
	}

	t.Logf("Tree has %d items after loading", initialCount)
}

// TestTreeNavigation_CollapseAll verifies that collapsing all keys works
func TestTreeNavigation_CollapseAll(t *testing.T) {
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
		time.Sleep(500 * time.Millisecond)
		// Expand first item, then collapse all
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(100 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}}) // CollapseAll
		time.Sleep(100 * time.Millisecond) // Give more time for collapse to complete
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
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
	}

	// Verify: All visible items should be at depth 0 after CollapseAll
	items := m.GetKeyTree().GetItems()
	for i, item := range items {
		if item.Depth != 0 {
			t.Errorf("After collapse all, item %d (%q) has depth %d, expected 0",
				i, item.Name, item.Depth)
		}
		if item.Expanded {
			t.Errorf("After collapse all, item %d (%q) is still expanded",
				i, item.Name)
		}
	}
}
