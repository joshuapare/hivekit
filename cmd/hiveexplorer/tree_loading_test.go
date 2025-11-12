package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestTreeLoading_TopLevelKeys verifies that when a hive is loaded,
// the top-level items shown are the actual registry keys (children of root),
// not the root node itself (e.g., "CsiTool-CreateHive-{00000000-...}")
func TestTreeLoading_TopLevelKeys(t *testing.T) {
	// Use a real hive file
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")

	// Create the TUI model
	m := NewModel(hivePath)

	// We need to use the actual tea.Program to properly handle the Init batch
	// Create a program that will run briefly and quit
	// Use WithInput/WithOutput to avoid requiring a TTY
	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))

	// Run in a goroutine
	done := make(chan Model, 1)
	go func() {
		// Send a window size first
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		// Give it more time to initialize and load tree
		time.Sleep(1000 * time.Millisecond)

		// Send quit
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

	// Wait for program to finish
	select {
	case m = <-done:
		items := m.GetKeyTree().GetItems()
		t.Logf("Tree loaded with %d items", len(items))
		if len(items) == 0 {
			t.Fatal("No items loaded - model may have error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for tree to load")
	}

	// Get the displayed items
	items := m.GetKeyTree().GetItems()
	if len(items) == 0 {
		t.Fatal("No items loaded in tree")
	}

	// Verify: The top-level items should NOT be the root node itself
	// Instead, they should be actual registry keys like "Classes", "Clients", etc.

	// Get the expected top-level keys from the hive directly
	expectedKeys, err := hive.ListKeys(hivePath, "", false, 1)
	if err != nil {
		t.Fatalf("Failed to get expected top-level keys: %v", err)
	}

	// Verify we have the expected number of top-level keys
	if len(items) != len(expectedKeys) {
		t.Errorf("Expected %d top-level keys, got %d", len(expectedKeys), len(items))
		t.Logf("Displayed items:")
		for i, item := range items {
			t.Logf("  [%d] Name=%q Path=%q Depth=%d", i, item.Name, item.Path, item.Depth)
		}
	}

	// Verify: All displayed items should be at depth 0 (since we haven't expanded anything)
	for i, item := range items {
		if item.Depth != 0 {
			t.Errorf("Item %d has depth %d, expected 0", i, item.Depth)
		}
	}

	// Verify: The displayed items should match the expected keys
	displayedNames := make(map[string]bool)
	for _, item := range items {
		displayedNames[item.Name] = true
	}

	for _, expected := range expectedKeys {
		if !displayedNames[expected.Name] {
			t.Errorf("Expected top-level key %q not found in displayed items", expected.Name)
		}
	}

	// Verify: No item should have a name that looks like a root node
	// (contains "CsiTool-CreateHive" or similar UUID patterns)
	for i, item := range items {
		if strings.Contains(item.Name, "CsiTool-CreateHive") {
			t.Errorf(
				"Item %d appears to be root node (Name=%q), should not be displayed at top level",
				i,
				item.Name,
			)
		}

		// Check for UUID pattern in name (another indicator of root node)
		if strings.Contains(item.Name, "{") && strings.Contains(item.Name, "}") &&
			strings.Contains(item.Name, "-") {
			t.Logf(
				"Warning: Item %d name %q looks like it might contain a UUID (possible root node)",
				i,
				item.Name,
			)
		}
	}

	// Clean up
	if err := m.Close(); err != nil {
		t.Logf("Warning: error closing model: %v", err)
	}
}

// TestTreeLoading_InitialState verifies the initial state after loading a hive
func TestTreeLoading_InitialState(t *testing.T) {
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")

	m := NewModel(hivePath)

	// Use tea.Program to properly handle Init
	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	done := make(chan Model, 1)

	go func() {
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		time.Sleep(500 * time.Millisecond)
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
		t.Fatal("Timeout waiting for tree to load")
	}

	items := m.GetKeyTree().GetItems()

	// Verify: All top-level items should be collapsed initially
	for i, item := range items {
		if item.Expanded {
			t.Errorf("Item %d (%q) is expanded, but all items should be collapsed initially",
				i, item.Name)
		}
	}

	// Verify: Cursor should be at first item
	cursor := m.GetKeyTree().GetCursor()
	if cursor != 0 {
		t.Errorf("Cursor is at position %d, expected 0", cursor)
	}

	// Clean up
	if err := m.Close(); err != nil {
		t.Logf("Warning: error closing model: %v", err)
	}
}

// TestTreeLoading_WithWait is a helper that waits for async tree loading
// This is useful when the tree loader is truly async
func TestTreeLoading_WithWait(t *testing.T) {
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")

	m := NewModel(hivePath)

	// Create a program to handle async messages
	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))

	// Run in a goroutine so we can send quit after a delay
	done := make(chan Model)
	go func() {
		finalModel, err := p.Run()
		if err != nil {
			t.Errorf("Error running program: %v", err)
		}
		done <- finalModel.(Model)
	}()

	// Send window size and wait for tree to load
	p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	time.Sleep(1000 * time.Millisecond)

	// Send quit
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Wait for program to finish
	select {
	case finalModel := <-done:
		items := finalModel.GetKeyTree().GetItems()
		if len(items) == 0 {
			t.Error("No items loaded in tree after waiting")
		}

		// Verify no root node in display
		for i, item := range items[:min(5, len(items))] {
			if strings.Contains(item.Name, "CsiTool") ||
				(strings.Contains(item.Name, "{") && strings.Contains(item.Name, "}")) {
				t.Errorf("Top-level item %d appears to be root node: %q", i, item.Name)
			}
		}

		// Clean up
		if err := finalModel.Close(); err != nil {
			t.Logf("Warning: error closing model: %v", err)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for program to finish")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
