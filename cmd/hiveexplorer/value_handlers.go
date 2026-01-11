package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
)

// loadValuesForCurrentKey reloads values for the currently selected key.
// Used for refresh (F5) operations to force a reload of the current key's data.
func (m *Model) loadValuesForCurrentKey() tea.Cmd {
	item := m.keyTree.CurrentItem()
	if item == nil {
		return nil
	}

	// Re-emit navigation signal to trigger reload
	// The bus architecture with context cancellation handles deduplication automatically
	logger.Debug("Re-emitting navigation signal for refresh", "path", item.Path)
	m.keyTree.CursorManager.EmitSignal()
	return nil
}
