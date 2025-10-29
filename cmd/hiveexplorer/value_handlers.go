package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// loadValuesForCurrentKey reloads values for the currently selected key.
// Used for refresh (F5) operations to force a reload of the current key's data.
func (m *Model) loadValuesForCurrentKey() tea.Cmd {
	item := m.keyTree.CurrentItem()
	if item == nil {
		return nil
	}

	// In diff mode, determine which hive to use based on key status
	if m.diffMode {
		fmt.Fprintf(os.Stderr, "[DIFF] loadValuesForCurrentKey: path=%q, status=%d, oldHive=%q, newHive=%q\n",
			item.Path, item.DiffStatus, m.hivePath, m.comparePath)

		// Use cached readers if available for better performance
		switch item.DiffStatus {
		case hive.DiffAdded:
			// Key only exists in new hive
			fmt.Fprintf(os.Stderr, "[DIFF] → Loading from NEW hive (DiffAdded) with cached reader\n")
			if m.newHiveReader != nil {
				return m.loadValuesWithReader(item.Path, m.newHiveReader)
			}
			return m.valueTable.LoadValuesFromHive(item.Path, m.comparePath)
		case hive.DiffRemoved:
			// Key only exists in old hive
			fmt.Fprintf(os.Stderr, "[DIFF] → Loading from OLD hive (DiffRemoved) with cached reader\n")
			if m.oldHiveReader != nil {
				return m.loadValuesWithReader(item.Path, m.oldHiveReader)
			}
			return m.valueTable.LoadValuesFromHive(item.Path, m.hivePath)
		case hive.DiffModified, hive.DiffUnchanged:
			// Key exists in both, load from new hive (comparePath) to show current state
			fmt.Fprintf(os.Stderr, "[DIFF] → Loading from NEW hive (DiffModified/DiffUnchanged) with cached reader\n")
			if m.newHiveReader != nil {
				return m.loadValuesWithReader(item.Path, m.newHiveReader)
			}
			return m.valueTable.LoadValuesFromHive(item.Path, m.comparePath)
		default:
			fmt.Fprintf(os.Stderr, "[DIFF] → UNKNOWN STATUS %d, using old hive\n", item.DiffStatus)
			return m.valueTable.LoadValuesFromHive(item.Path, m.hivePath)
		}
	}

	// Normal mode: Re-emit navigation signal to trigger reload
	// The bus architecture with context cancellation handles deduplication automatically
	fmt.Fprintf(os.Stderr, "[REFRESH] Normal mode: re-emitting navigation signal for path=%q\n", item.Path)
	m.keyTree.CursorManager.EmitSignal()
	return nil
}

// loadValuesWithReader loads values using an existing reader (no open/close overhead)
func (m *Model) loadValuesWithReader(keyPath string, r hive.Reader) tea.Cmd {
	return func() tea.Msg {
		values, err := hive.ListValuesWithReader(r, keyPath)
		if err != nil {
			return errMsg{fmt.Errorf("failed to load values for path %q: %w", keyPath, err)}
		}
		// Convert using helper function
		return valuesLoadedMsg{Path: keyPath, Values: convertValueInfos(values)}
	}
}
