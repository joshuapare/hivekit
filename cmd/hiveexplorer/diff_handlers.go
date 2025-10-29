package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// handleLoadDiff starts async diff loading
func (m Model) handleLoadDiff(comparePath string) (tea.Model, tea.Cmd) {
	fmt.Fprintf(os.Stderr, "[DIFF] handleLoadDiff called with comparePath=%q\n", comparePath)

	if comparePath == "" {
		m.statusMessage = "Diff cancelled: no path provided"
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	}

	m.comparePath = comparePath
	m.statusMessage = "Loading diff... (this may take a few seconds)"

	// Start async diff loading
	return m, m.loadDiffAsync(comparePath)
}

// loadDiffAsync creates a command that loads diff in background
func (m Model) loadDiffAsync(comparePath string) tea.Cmd {
	return func() tea.Msg {
		fmt.Fprintf(os.Stderr, "[DIFF] Loading diff: old=%q, new=%q\n", m.hivePath, comparePath)

		diff, err := hive.DiffHives(m.hivePath, comparePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[DIFF] Error loading diff: %v\n", err)
			return diffLoadedMsg{diff: nil, err: err}
		}

		fmt.Fprintf(os.Stderr, "[DIFF] Diff loaded successfully\n")
		return diffLoadedMsg{diff: diff, err: nil}
	}
}

// reloadTreeWithDiff reloads the tree view using current diff filter settings
func (m Model) reloadTreeWithDiff() (tea.Model, tea.Cmd) {
	fmt.Fprintf(os.Stderr, "[DIFF] reloadTreeWithDiff called\n")

	if m.hiveDiff == nil {
		fmt.Fprintf(os.Stderr, "[DIFF] No diff data, returning\n")
		return m, nil
	}

	// Tree structure is changing, navigation signal will trigger value reload

	// Determine which items to show based on diffOnlyView
	showUnchanged := m.showUnchanged
	if m.diffOnlyView {
		// In diff-only view, don't show unchanged unless explicitly enabled
		showUnchanged = false
	}

	fmt.Fprintf(os.Stderr, "[DIFF] Filtering keys: added=%v, removed=%v, modified=%v, unchanged=%v\n",
		m.showAdded, m.showRemoved, m.showModified, showUnchanged)

	// Filter diff keys
	filteredKeys := hive.FilterDiffKeys(m.hiveDiff, m.showAdded, m.showRemoved, m.showModified, showUnchanged)
	fmt.Fprintf(os.Stderr, "[DIFF] Filtered to %d keys\n", len(filteredKeys))

	// Build diff map for quick lookup
	diffMap := make(map[string]hive.KeyDiff)
	for _, kd := range filteredKeys {
		diffMap[kd.Path] = kd
	}

	// Store diff map in key tree for later use
	m.keyTree.SetDiffMap(diffMap)

	// Build tree items - only show ROOT level keys initially
	items := make([]keytree.Item, 0)

	// Sort by path for hierarchical display
	sortedKeys := make([]hive.KeyDiff, len(filteredKeys))
	copy(sortedKeys, filteredKeys)
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i].Path < sortedKeys[j].Path
	})

	// Only show root-level keys (depth 0)
	for _, keyDiff := range sortedKeys {
		// Calculate depth
		depth := 0
		if keyDiff.Path != "" {
			depth = len(strings.Split(keyDiff.Path, "\\")) - 1
		}

		// Only include root-level keys (depth 0)
		if depth != 0 {
			continue
		}

		// Look up NodeIDs from appropriate readers based on DiffStatus
		var oldNodeID, newNodeID hive.NodeID
		var oldErr, newErr error

		switch keyDiff.Status {
		case hive.DiffAdded:
			// Added key: only exists in NEW hive
			if m.newHiveReader != nil {
				newNodeID, newErr = m.newHiveReader.Find(keyDiff.Path)
			}
			if newErr != nil {
				fmt.Fprintf(os.Stderr, "[DIFF] Failed to find NodeID in new hive for added key %q: %v\n", keyDiff.Path, newErr)
				continue
			}

		case hive.DiffRemoved:
			// Removed key: only exists in OLD hive
			if m.oldHiveReader != nil {
				oldNodeID, oldErr = m.oldHiveReader.Find(keyDiff.Path)
			}
			if oldErr != nil {
				fmt.Fprintf(os.Stderr, "[DIFF] Failed to find NodeID in old hive for removed key %q: %v\n", keyDiff.Path, oldErr)
				continue
			}

		case hive.DiffModified, hive.DiffUnchanged:
			// Modified/Unchanged: exists in BOTH hives - look up both NodeIDs
			if m.oldHiveReader != nil {
				oldNodeID, oldErr = m.oldHiveReader.Find(keyDiff.Path)
			}
			if m.newHiveReader != nil {
				newNodeID, newErr = m.newHiveReader.Find(keyDiff.Path)
			}
			if oldErr != nil || newErr != nil {
				fmt.Fprintf(os.Stderr, "[DIFF] Failed to find NodeIDs for modified/unchanged key %q (oldErr=%v, newErr=%v)\n",
					keyDiff.Path, oldErr, newErr)
				continue
			}
		}

		item := keytree.Item{
			OldNodeID:   oldNodeID,
			NewNodeID:   newNodeID,
			Path:        keyDiff.Path,
			Name:        keyDiff.Name,
			Depth:       0,
			HasChildren: keyDiff.SubkeyN > 0,
			SubkeyCount: keyDiff.SubkeyN,
			ValueCount:  keyDiff.ValueN,
			LastWrite:   keyDiff.LastWrite,
			Expanded:    false,
			Parent:      "",
			DiffStatus:  keyDiff.Status,
		}

		items = append(items, item)
	}

	// Store current path to restore after rebuild
	currentPath := ""
	if currentItem := m.keyTree.CurrentItem(); currentItem != nil {
		currentPath = currentItem.Path
		fmt.Fprintf(os.Stderr, "[DIFF] Storing current path: %q\n", currentPath)
	}

	m.keyTree.SetItems(items)

	// Try to restore cursor to same path
	restored := false
	if currentPath != "" {
		for i, item := range m.keyTree.GetItems() {
			if item.Path == currentPath {
				m.keyTree.SetCursor(i)
				fmt.Fprintf(os.Stderr, "[DIFF] Restored cursor to path %q at index %d\n", currentPath, i)
				restored = true
				break
			}
		}
	}

	// If we couldn't restore, reset to 0
	if !restored {
		m.keyTree.SetCursor(0)
		m.keyTree.GetViewport().YOffset = 0
		fmt.Fprintf(os.Stderr, "[DIFF] Reset cursor to 0\n")
	}

	fmt.Fprintf(os.Stderr, "[DIFF] Tree rebuilt with %d items\n", len(m.keyTree.GetItems()))

	// Emit navigation signal to trigger value/metadata reload
	// This uses the new navigation signal architecture
	if len(m.keyTree.GetItems()) > 0 {
		fmt.Fprintf(os.Stderr, "[DIFF] Emitting navigation signal for current item\n")
		m.keyTree.CursorManager.EmitSignal()
	}

	// Set status message with filter info
	var filters []string
	if m.showAdded {
		filters = append(filters, "+added")
	}
	if m.showRemoved {
		filters = append(filters, "-removed")
	}
	if m.showModified {
		filters = append(filters, "~modified")
	}
	if m.showUnchanged && !m.diffOnlyView {
		filters = append(filters, "unchanged")
	}
	filterStr := strings.Join(filters, ", ")
	m.statusMessage = fmt.Sprintf("Diff view: %s", filterStr)

	return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}
