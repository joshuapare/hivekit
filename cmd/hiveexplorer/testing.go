package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
)

// TestHelper provides utilities for testing TUI components
type TestHelper struct {
	model Model
}

// NewTestHelper creates a test helper with a model
func NewTestHelper(hivePath string) *TestHelper {
	return &TestHelper{
		model: NewModel(hivePath),
	}
}

// SendKey simulates a key press but does not execute async commands
// Tests should manually call LoadChildKeys, LoadValues, etc. to control async behavior
func (h *TestHelper) SendKey(keyType tea.KeyType) *TestHelper {
	msg := tea.KeyMsg{Type: keyType}
	updated, _ := h.model.Update(msg)
	h.model = updated.(Model)
	return h
}

// SendKeyRune simulates a character key press
func (h *TestHelper) SendKeyRune(r rune) *TestHelper {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
	updated, _ := h.model.Update(msg)
	h.model = updated.(Model)
	return h
}

// SendWindowSize simulates a window resize
func (h *TestHelper) SendWindowSize(width, height int) *TestHelper {
	msg := tea.WindowSizeMsg{Width: width, Height: height}
	updated, _ := h.model.Update(msg)
	h.model = updated.(Model)
	return h
}

// LoadRootKeys simulates loading root keys
func (h *TestHelper) LoadRootKeys(keys []keytree.KeyInfo) *TestHelper {
	msg := keytree.RootKeysLoadedMsg{Keys: keys}
	updatedTree, _ := h.model.keyTree.Update(msg)
	h.model.keyTree = updatedTree
	return h
}

// LoadChildKeys simulates loading child keys for a parent
func (h *TestHelper) LoadChildKeys(parentPath string, keys []keytree.KeyInfo) *TestHelper {
	msg := keytree.ChildKeysLoadedMsg{Parent: parentPath, Keys: keys}
	updatedTree, _ := h.model.keyTree.Update(msg)
	h.model.keyTree = updatedTree
	return h
}

// LoadValues simulates loading values
func (h *TestHelper) LoadValues(path string, values []ValueInfo) *TestHelper {
	msg := valuesLoadedMsg{Path: path, Values: values}
	updatedTable, _ := h.model.valueTable.Update(msg)
	h.model.valueTable = updatedTable
	return h
}

// GetModel returns the current model
func (h *TestHelper) GetModel() Model {
	return h.model
}

// GetView returns the rendered view
func (h *TestHelper) GetView() string {
	return h.model.View()
}

// GetFocusedPane returns the currently focused pane
func (h *TestHelper) GetFocusedPane() Pane {
	return h.model.focusedPane
}

// GetTreeItemCount returns the number of items in the tree
func (h *TestHelper) GetTreeItemCount() int {
	return len(h.model.keyTree.GetItems())
}

// GetValueItemCount returns the number of values in the table
func (h *TestHelper) GetValueItemCount() int {
	return len(h.model.valueTable.GetItems())
}

// GetTreeCursor returns the current tree cursor position
func (h *TestHelper) GetTreeCursor() int {
	return h.model.keyTree.GetCursor()
}

// GetValueCursor returns the current value table cursor position
func (h *TestHelper) GetValueCursor() int {
	return h.model.valueTable.GetCursor()
}

// GetCurrentTreeItem returns the currently selected tree item
func (h *TestHelper) GetCurrentTreeItem() *keytree.Item {
	return h.model.keyTree.CurrentItem()
}

// GetCurrentValueItem returns the currently selected value item
func (h *TestHelper) GetCurrentValueItem() *valuetable.ValueRow {
	return h.model.valueTable.CurrentItem()
}

// CreateTestKeys creates sample keys for testing
func CreateTestKeys(count int) []keytree.KeyInfo {
	keys := make([]keytree.KeyInfo, count)
	for i := 0; i < count; i++ {
		keys[i] = keytree.KeyInfo{
			Path:    string(rune('A' + i)),
			Name:    string(rune('A' + i)),
			SubkeyN: i * 2,
			ValueN:  i + 1,
		}
	}
	return keys
}

// CreateTestValues creates sample values for testing
func CreateTestValues(count int) []ValueInfo {
	values := make([]ValueInfo, count)
	for i := 0; i < count; i++ {
		values[i] = ValueInfo{
			Name:      string(rune('A' + i)),
			Type:      "REG_SZ",
			Size:      10,
			StringVal: "test value",
		}
	}
	return values
}

// createTestModelWithItems creates a Model with pre-populated tree items for testing.
// This bypasses the need for a real hive file and directly sets up the tree state.
func createTestModelWithItems(t interface{}, items []keytree.Item) Model {
	// Create a minimal model without a hive file
	m := Model{
		focusedPane: TreePane,
		inputMode:   NormalMode,
	}

	// Create a keytree model with empty path (testing mode)
	m.keyTree = keytree.NewModel("")

	// Set the items directly in the tree state
	m.keyTree.SetItems(items)

	return m
}
