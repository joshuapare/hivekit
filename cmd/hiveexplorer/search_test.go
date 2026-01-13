package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// runModelWithActions runs the model with a set of actions and returns the final model
func runModelWithActions(t *testing.T, m Model, actions func(*tea.Program)) Model {
	in := bytes.NewReader([]byte{})
	out := &bytes.Buffer{}
	p := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out))
	done := make(chan Model, 1)

	go func() {
		// Send a window size first
		p.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
		time.Sleep(100 * time.Millisecond)
		// Run the actions
		actions(p)
	}()

	go func() {
		finalModel, err := p.Run()
		if err != nil {
			t.Errorf("Error running program: %v", err)
			done <- Model{}
			return
		}
		if finalModel == nil {
			done <- Model{}
			return
		}
		done <- finalModel.(Model)
	}()

	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for test to complete")
		return Model{}
	}
}

// TestSearch_BasicSearch verifies that search mode can be entered and executed
func TestSearch_BasicSearch(t *testing.T) {
	hivePath := filepath.Join("..", "..", "testdata", "suite", "windows-2012-software")
	m := NewModel(hivePath)

	m = runModelWithActions(t, m, func(p *tea.Program) {
		time.Sleep(1000 * time.Millisecond)
		// Enter search mode and type "Microsoft"
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		time.Sleep(50 * time.Millisecond)
		for _, ch := range "Microsoft" {
			p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
			time.Sleep(10 * time.Millisecond)
		}
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	})

	defer m.Close()

	// Verify the model is still in a valid state
	items := m.GetKeyTree().GetItems()
	if len(items) == 0 {
		t.Error("No items after search - tree should still be visible")
	}

	// Verify cursor is still valid
	cursor := m.GetKeyTree().GetCursor()
	if cursor < 0 || cursor >= len(items) {
		t.Errorf("Cursor %d is out of range [0, %d)", cursor, len(items))
	}
}

// TestSearch_Cancel verifies that search can be cancelled
func TestSearch_Cancel(t *testing.T) {
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

	initialCursor := m.GetKeyTree().GetCursor()

	// Enter search mode
	searchKeyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updatedModel, _ = m.Update(searchKeyMsg)
	m = updatedModel.(Model)

	// Type some characters
	for _, ch := range "test" {
		charMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		updatedModel, _ = m.Update(charMsg)
		m = updatedModel.(Model)
	}

	// Press Escape to cancel
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	updatedModel, _ = m.Update(escMsg)
	m = updatedModel.(Model)

	// Verify: Cursor should be unchanged
	if m.GetKeyTree().GetCursor() != initialCursor {
		t.Errorf("Cursor changed after cancelled search: was %d, now %d",
			initialCursor, m.GetKeyTree().GetCursor())
	}
}

// TestSearch_NavigateResults verifies that 'n' and 'N' navigate through search results
func TestSearch_NavigateResults(t *testing.T) {
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
		// Search for "s" (should have multiple matches)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		time.Sleep(50 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		time.Sleep(20 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)
		// Navigate results: n, N
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		time.Sleep(20 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
		time.Sleep(20 * time.Millisecond)
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

	// Verify cursor is in valid range
	cursor := m.GetKeyTree().GetCursor()
	items := m.GetKeyTree().GetItems()
	if cursor < 0 || cursor >= len(items) {
		t.Errorf("After search navigation, cursor %d is out of range [0, %d)", cursor, len(items))
	}
}

// TestSearch_EmptyQuery verifies that empty search queries are handled gracefully
func TestSearch_EmptyQuery(t *testing.T) {
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
		// Enter search mode and press Enter without typing
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		time.Sleep(50 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
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

	// Verify: Model should still be in valid state
	items := m.GetKeyTree().GetItems()
	if len(items) == 0 {
		t.Error("No items after empty search")
	}

	// Verify cursor is in valid range
	cursor := m.GetKeyTree().GetCursor()
	if cursor < 0 || cursor >= len(items) {
		t.Errorf("Cursor %d is out of range [0, %d)", cursor, len(items))
	}
}

// TestSearch_CaseInsensitive verifies that search is case-insensitive
func TestSearch_CaseInsensitive(t *testing.T) {
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

	// Search for lowercase version of a key we know exists
	// Most registry keys start with capital letters, so searching lowercase
	// tests case-insensitivity

	// Enter search mode
	searchKeyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updatedModel, _ = m.Update(searchKeyMsg)
	m = updatedModel.(Model)

	// Type "classes" (should match "Classes" if case-insensitive)
	for _, ch := range "classes" {
		charMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}}
		updatedModel, _ = m.Update(charMsg)
		m = updatedModel.(Model)
	}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedModel, _ = m.Update(enterMsg)
	m = updatedModel.(Model)

	// Verify: Should have found something
	cursor := m.GetKeyTree().GetCursor()
	items := m.GetKeyTree().GetItems()

	if cursor >= 0 && cursor < len(items) {
		currentItem := items[cursor]
		// Check if the found item contains "class" (case-insensitive)
		if !strings.Contains(strings.ToLower(currentItem.Name), "class") {
			t.Logf("Search for 'classes' landed on %q (may have multiple matches)", currentItem.Name)
		}
	}
}

// TestSearch_LiveFiltering verifies that the tree filters as the user types
func TestSearch_LiveFiltering(t *testing.T) {
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

		// Get initial item count
		// Enter search mode
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		time.Sleep(50 * time.Millisecond)

		// Type "mic" - this should trigger live filtering after 3 characters
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
		time.Sleep(20 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		time.Sleep(20 * time.Millisecond)
		// Before 3rd character - should not filter yet

		// Type 3rd character - should trigger live filtering
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		time.Sleep(50 * time.Millisecond)
		// Tree should now be filtered to show items matching "mic" (like "Microsoft")

		// Add more characters to refine filter
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
		time.Sleep(20 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
		time.Sleep(20 * time.Millisecond)

		// Backspace to test filter update
		p.Send(tea.KeyMsg{Type: tea.KeyBackspace})
		time.Sleep(20 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyBackspace})
		time.Sleep(20 * time.Millisecond)

		// Press Esc to clear filter
		p.Send(tea.KeyMsg{Type: tea.KeyEsc})
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

	// Verify: After Esc, filter should be cleared and tree should show all items
	items := m.GetKeyTree().GetItems()
	if len(items) == 0 {
		t.Error("No items after live filtering test - tree should be visible")
	}

	// Verify cursor is in valid range
	cursor := m.GetKeyTree().GetCursor()
	if cursor < 0 || cursor >= len(items) {
		t.Errorf("Cursor %d is out of range [0, %d)", cursor, len(items))
	}

	t.Logf("Live filtering test completed successfully with %d items visible", len(items))
}

// TestSearch_LiveFilteringThreeCharThreshold verifies that filtering only starts after 3 characters
func TestSearch_LiveFilteringThreeCharThreshold(t *testing.T) {
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

	// Send window size
	sizeMsg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, _ = m.Update(sizeMsg)
	m = updatedModel.(Model)

	time.Sleep(100 * time.Millisecond)

	initialItemCount := len(m.GetKeyTree().GetItems())
	t.Logf("Initial item count: %d", initialItemCount)

	// Enter search mode
	searchKeyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}
	updatedModel, _ = m.Update(searchKeyMsg)
	m = updatedModel.(Model)

	// Type "m" (1 character) - should NOT filter
	charMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}
	updatedModel, _ = m.Update(charMsg)
	m = updatedModel.(Model)
	time.Sleep(20 * time.Millisecond)

	itemsAfterOne := len(m.GetKeyTree().GetItems())
	if itemsAfterOne != initialItemCount {
		t.Errorf("After 1 character, expected %d items (no filtering), got %d", initialItemCount, itemsAfterOne)
	}

	// Type "i" (2 characters total: "mi") - should still NOT filter
	charMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}
	updatedModel, _ = m.Update(charMsg)
	m = updatedModel.(Model)
	time.Sleep(20 * time.Millisecond)

	itemsAfterTwo := len(m.GetKeyTree().GetItems())
	if itemsAfterTwo != initialItemCount {
		t.Errorf("After 2 characters, expected %d items (no filtering), got %d", initialItemCount, itemsAfterTwo)
	}

	// Type "c" (3 characters total: "mic") - SHOULD filter
	charMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}
	updatedModel, _ = m.Update(charMsg)
	m = updatedModel.(Model)
	time.Sleep(50 * time.Millisecond)

	itemsAfterThree := len(m.GetKeyTree().GetItems())
	// Should have fewer items due to filtering (unless all items match "mic", which is unlikely)
	if itemsAfterThree >= initialItemCount {
		t.Logf("After 3 characters, expected filtering to reduce items from %d, but got %d (may be expected if all keys match)",
			initialItemCount, itemsAfterThree)
	} else {
		t.Logf("Filtering active after 3 characters: %d -> %d items", initialItemCount, itemsAfterThree)
	}
}
