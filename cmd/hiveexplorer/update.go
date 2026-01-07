package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/displays"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuedetail"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Update handles all messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If help is showing, handle help keys
		if m.showHelp {
			if key.Matches(msg, m.keys.Esc) || key.Matches(msg, m.keys.Help) || key.Matches(msg, m.keys.Quit) {
				m.showHelp = false
				return m, nil
			}
			// Ignore other keys when help is showing
			return m, nil
		}

		// If detail view is open, handle its keys
		if m.valueDetail.IsVisible() {
			if key.Matches(msg, m.keys.Esc) {
				m.valueDetail.Hide()
				return m, nil
			}
			// Forward Up/Down/PageUp/PageDown to detail view for scrolling
			if key.Matches(msg, m.keys.Up) || key.Matches(msg, m.keys.Down) ||
				key.Matches(msg, m.keys.PageUp) || key.Matches(msg, m.keys.PageDown) {
				var model tea.Model
				model, cmd = (&m.valueDetail).Update(msg)
				m.valueDetail = *model.(*valuedetail.ValueDetailModel)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
			// Still allow quit
			if key.Matches(msg, m.keys.Quit) {
				m.keyTree.Close()
				return m, tea.Quit
			}
			// Ignore other keys when detail is open
			return m, nil
		}

		// Handle input modes (search, go to path, diff path, global value search)
		if m.inputMode == SearchMode || m.inputMode == GoToPathMode || m.inputMode == DiffPathMode || m.inputMode == GlobalValueSearchMode {
			return m.handleInputMode(msg)
		}

		// Global keys
		if key.Matches(msg, m.keys.Quit) {
			m.keyTree.Close()
			return m, tea.Quit
		}

		// Clear search (Esc in normal mode)
		if key.Matches(msg, m.keys.Esc) && m.searchQuery != "" {
			m.searchQuery = ""
			m.searchMatches = 0
			m.searchMatchIdx = 0
			m.statusMessage = "Search cleared"
			// Clear the tree filter to restore all items
			if m.focusedPane == TreePane {
				m.keyTree.SetSearchFilter("")
			}
			// Clear status message after 2 seconds
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		// Enter search mode
		if key.Matches(msg, m.keys.Search) {
			m.inputMode = SearchMode
			m.inputBuffer = ""
			return m, nil
		}

		// Enter global value search mode
		if key.Matches(msg, m.keys.GlobalValueSearch) {
			m.inputMode = GlobalValueSearchMode
			m.inputBuffer = ""
			m.globalValueSearchActive = false
			m.globalValueSearchInProgress = false
			m.globalValueSearchResults = nil
			return m, nil
		}

		// Enter go to path mode
		if key.Matches(msg, m.keys.Jump) {
			m.inputMode = GoToPathMode
			m.inputBuffer = ""
			return m, nil
		}

		// Next bookmark - navigate to next bookmarked key
		if key.Matches(msg, m.keys.NextBookmark) {
			return m.handleNextBookmark()
		}

		// Next search match (works for both regular search and global value search)
		if key.Matches(msg, m.keys.NextMatch) && (m.searchQuery != "" || m.globalValueSearchActive) {
			logger.Debug("Next match key pressed", "globalValueSearchActive", m.globalValueSearchActive, "searchQuery", m.searchQuery)
			return m.handleNextMatch()
		}

		// Previous search match (works for both regular search and global value search)
		if key.Matches(msg, m.keys.PrevMatch) && (m.searchQuery != "" || m.globalValueSearchActive) {
			return m.handlePrevMatch()
		}

		// First search match (works for both regular search and global value search)
		if key.Matches(msg, m.keys.FirstMatch) && (m.searchQuery != "" || m.globalValueSearchActive) {
			return m.handleFirstMatch()
		}

		// Last search match (works for both regular search and global value search)
		if key.Matches(msg, m.keys.LastMatch) && (m.searchQuery != "" || m.globalValueSearchActive) {
			return m.handleLastMatch()
		}

		// Show help overlay
		if key.Matches(msg, m.keys.Help) {
			m.showHelp = true
			return m, nil
		}

		// Refresh current view
		if key.Matches(msg, m.keys.Refresh) {
			// Reload values for current key
			if item := m.keyTree.CurrentItem(); item != nil {
				m.statusMessage = "Refreshing..."
				return m, m.loadValuesForCurrentKey()
			}
		}

		// Diff mode toggles (only active in diff mode)
		if m.diffMode {
			if key.Matches(msg, m.keys.ToggleAdded) {
				m.showAdded = !m.showAdded
				return m.reloadTreeWithDiff()
			}
			if key.Matches(msg, m.keys.ToggleRemoved) {
				m.showRemoved = !m.showRemoved
				return m.reloadTreeWithDiff()
			}
			if key.Matches(msg, m.keys.ToggleModified) {
				m.showModified = !m.showModified
				return m.reloadTreeWithDiff()
			}
			if key.Matches(msg, m.keys.ToggleUnchanged) {
				m.showUnchanged = !m.showUnchanged
				return m.reloadTreeWithDiff()
			}
			if key.Matches(msg, m.keys.ToggleDiffView) {
				m.diffOnlyView = !m.diffOnlyView
				return m.reloadTreeWithDiff()
			}
		}

		// Enter diff mode (asks for comparison hive path)
		if key.Matches(msg, m.keys.DiffMode) {
			if !m.diffMode {
				m.inputMode = DiffPathMode
				m.inputBuffer = ""
				return m, nil
			} else {
				// Exit diff mode - close cached readers
				if m.oldHiveReader != nil {
					m.oldHiveReader.Close()
					m.oldHiveReader = nil
				}
				if m.newHiveReader != nil {
					m.newHiveReader.Close()
					m.newHiveReader = nil
				}
				m.diffMode = false
				m.hiveDiff = nil

				// Reload tree in normal mode
				m.keyTree = keytree.NewModel(m.hivePath)
				// Reconnect navigation bus (required for value/metadata loading)
				m.keyTree.SetNavigationBus(m.navBus)
				// Restore bookmarks
				m.keyTree.SetBookmarks(m.bookmarks)
				// Configure keytree with keys
				m.keyTree.SetKeys(keytree.Keys{
					Up:              m.keys.Up,
					Down:            m.keys.Down,
					Left:            m.keys.Left,
					Right:           m.keys.Right,
					PageUp:          m.keys.PageUp,
					PageDown:        m.keys.PageDown,
					Home:            m.keys.Home,
					End:             m.keys.End,
					Enter:           m.keys.Enter,
					GoToParent:      m.keys.GoToParent,
					ExpandAll:       m.keys.ExpandAll,
					CollapseAll:     m.keys.CollapseAll,
					ExpandLevel:     m.keys.ExpandLevel,
					CollapseToLevel: m.keys.CollapseToLevel,
					Copy:            m.keys.Copy,
					ToggleBookmark:  m.keys.ToggleBookmark,
				})

				// Initialize tree and restore state asynchronously
				initCmd := m.keyTree.Init()

				// Create command to restore tree state after initialization
				restoreCmd := func() tea.Msg {
					return restoreTreeStateMsg{
						cursorPath:   m.preDiffCursorPath,
						expandedKeys: m.preDiffExpandedKeys,
					}
				}

				logger.Debug("Exiting diff mode, will restore state", "cursorPath", m.preDiffCursorPath, "expandedKeys", len(m.preDiffExpandedKeys))

				return m, tea.Batch(initCmd, restoreCmd)
			}
		}

		// Tab to switch panes (only if detail view not open)
		if key.Matches(msg, m.keys.Tab) {
			if m.focusedPane == TreePane {
				m.focusedPane = ValuePane
			} else {
				m.focusedPane = TreePane
			}
			return m, nil
		}

		// Handle keys based on focused pane
		switch m.focusedPane {
		case TreePane:
			// Forward key message to keytree component
			var updatedTree keytree.Model
			updatedTree, cmd = m.keyTree.Update(msg)
			m.keyTree = updatedTree
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			// Update current diff status when cursor moves in diff mode
			if m.diffMode {
				if item := m.keyTree.CurrentItem(); item != nil {
					m.currentDiffStatus = item.DiffStatus
				}
			}

		case ValuePane:
			// Forward key message to valuetable component
			var updatedTable valuetable.ValueTableModel
			updatedTable, cmd = m.valueTable.Update(msg)
			m.valueTable = updatedTable
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate pane sizes (50-50 split)
		treeWidth := msg.Width / 2
		valueWidth := msg.Width - treeWidth

		// Account for header and status bar
		paneHeight := msg.Height - 8 // Must match view.go calculation

		// Reserve space for hive info panel
		// Note: Since JoinVertical doesn't add spacing, we include the spacing in hive info height
		hiveInfoHeight := HiveInfoPanelHeight + HiveInfoPanelSpacing
		treeViewHeight := paneHeight - hiveInfoHeight

		if treeViewHeight < 5 {
			treeViewHeight = 5
		}

		// Update tree (with reduced height for hive info)
		m.keyTree.SetWidth(treeWidth)
		m.keyTree.SetHeight(treeViewHeight)

		// Update value table to match the height calculation in view.go
		// In view.go: valueViewHeight = leftColumnHeight - keyInfoBoxHeight - 3
		// leftColumnHeight = (treeViewHeight + 3) + (hiveInfoHeight)
		// keyInfoBoxHeight = NKInfoPanelHeight (8 with borders)
		// So: valueViewHeight = treeViewHeight + 3 + hiveInfoHeight - NKInfoPanelHeight - 3
		//                     = treeViewHeight + hiveInfoHeight - NKInfoPanelHeight
		valueViewHeight := treeViewHeight + hiveInfoHeight - NKInfoPanelHeight
		if valueViewHeight < 5 {
			valueViewHeight = 5
		}
		// Forward WindowSizeMsg to value table so it can resize its viewport
		// Account for pane borders/padding (valueWidth - 2 in view.go, then -2 more for pane border)
		valueTableMsg := tea.WindowSizeMsg{Width: valueWidth - 4, Height: valueViewHeight}
		m.valueTable, cmd = (&m.valueTable).Update(valueTableMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		// Update value detail
		var model tea.Model
		model, cmd = (&m.valueDetail).Update(msg)
		m.valueDetail = *model.(*valuedetail.ValueDetailModel)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case errMsg:
		logger.Error("Error occurred", "error", msg.err)
		m.err = msg.err
		return m, nil

	case keytree.TreeLoadedMsg:
		// Forward to key tree
		// Tree will emit navigation signal for initial item, which subscribers will pick up
		m.keyTree, cmd = (&m.keyTree).Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case keytree.RootKeysLoadedMsg, keytree.ChildKeysLoadedMsg:
		// Forward to key tree
		// For RootKeysLoadedMsg, tree will emit navigation signal which subscribers will pick up
		m.keyTree, cmd = (&m.keyTree).Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case keytree.CopyPathRequestedMsg:
		// Handle copy path request from keytree
		if msg.Success {
			m.statusMessage = fmt.Sprintf("✓ Copied: %s", msg.Path)
		} else {
			m.statusMessage = "Failed to copy path"
		}
		// Clear status after 2 seconds
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case keytree.BookmarkToggledMsg:
		// Handle bookmark toggle from keytree
		if msg.Added {
			m.bookmarks[msg.Path] = true
			m.statusMessage = "Bookmark added"
		} else {
			delete(m.bookmarks, msg.Path)
			m.statusMessage = "Bookmark removed"
		}
		// Update tree with new bookmarks
		m.keyTree.SetBookmarks(m.bookmarks)
		// Clear status after 2 seconds
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case valuetable.CopyValueRequestedMsg:
		// Handle copy value request from valuetable
		if msg.Success {
			m.statusMessage = "Value copied to clipboard"
		} else {
			m.statusMessage = "Failed to copy value"
		}
		// Clear status after 2 seconds
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case valuetable.ValueSelectedMsg:
		// Handle value selection from valuetable - show detail view
		if msg.Value != nil {
			m.valueDetail.Show(msg.Value)
		}

	case displays.KeyInfoSignalMsg:
		// Navigation signal received from KeyInfo subscriber
		// Forward ONLY to key info display
		if m.keyInfo != nil {
			var keyInfoCmd tea.Cmd
			m.keyInfo, keyInfoCmd = m.keyInfo.Update(msg)
			if keyInfoCmd != nil {
				cmds = append(cmds, keyInfoCmd)
			}
		}

	case valuetable.NavSignalReceivedMsg:
		// Navigation signal received from ValueTable subscriber
		// Forward ONLY to value table (don't forward to keyInfo - it has its own subscription)
		m.valueTable, cmd = m.valueTable.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case valuesLoadedMsg:
		// Forward to value table
		logger.Debug("valuesLoadedMsg received", "path", msg.Path, "valueCount", len(msg.Values))
		m.valueTable, cmd = (&m.valueTable).Update(msg)
		logger.Debug("Value table after update", "items", len(m.valueTable.GetItems()), "cursor", m.valueTable.GetCursor(), "yOffset", m.valueTable.GetViewportYOffset())
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Note: NK info/key metadata is now loaded by KeyInfo component via navigation bus
		// No need to load it here - eliminates redundant file I/O

	case clearStatusMsg:
		// Clear status message
		m.statusMessage = ""
		return m, nil

	case restoreTreeStateMsg:
		// Restore tree state after exiting diff mode
		logger.Debug("Restoring tree state", "cursorPath", msg.cursorPath, "expandedKeys", len(msg.expandedKeys))

		// Restore expanded keys
		if len(msg.expandedKeys) > 0 {
			m.keyTree.RestoreExpandedKeys(msg.expandedKeys)
		}

		// Navigate to saved cursor position
		if msg.cursorPath != "" {
			return m, m.keyTree.NavigateToPath(msg.cursorPath)
		}

		return m, nil

	case globalValueSearchTriggerMsg:
		// Debounce timer expired - start search if query matches current buffer
		if m.inputMode == GlobalValueSearchMode && msg.query == m.inputBuffer {
			m.globalValueSearchInProgress = true
			return m, m.performGlobalValueSearch(msg.query)
		}
		return m, nil

	case globalValueSearchCompleteMsg:
		// Global value search completed - expand parents to show matching keys
		m.globalValueSearchResults = msg.results
		m.globalValueSearchInProgress = false // Clear "searching..." indicator
		m.globalValueSearchActive = true      // Enable n/N navigation
		m.statusMessage = fmt.Sprintf("Value search complete: found %d keys with matches", len(msg.results))
		logger.Debug("Search complete", "results", len(msg.results), "globalValueSearchActive", m.globalValueSearchActive)

		// Expand all parents to make matching keys visible
		if len(msg.results) > 0 {
			// Build list of key paths that have matching values
			matchingPaths := make([]string, 0, len(msg.results))
			for _, result := range msg.results {
				matchingPaths = append(matchingPaths, result.KeyPath)
			}

			// Expand all parents to make matching keys visible
			var expandCmds []tea.Cmd
			for _, path := range matchingPaths {
				cmd := m.keyTree.ExpandParents(path)
				if cmd != nil {
					expandCmds = append(expandCmds, cmd)
				}
			}

			logger.Debug("Expanding parents for matching paths", "count", len(matchingPaths))

			// Navigate to first match after expansions complete
			if len(expandCmds) > 0 {
				expandCmds = append(expandCmds, m.keyTree.NavigateToPath(matchingPaths[0]))
			} else {
				// No expansions needed, navigate immediately
				return m, tea.Batch(
					m.keyTree.NavigateToPath(matchingPaths[0]),
					tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
						return clearStatusMsg{}
					}),
				)
			}

			// Return expand commands along with status clear
			return m, tea.Batch(append(expandCmds, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			}))...)
		} else {
			m.globalValueSearchActive = false // No results, disable n/N navigation
			m.statusMessage = "No matching values found"
		}

		// Clear status message after 3 seconds
		return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})

	case diffLoadedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to load diff: %v", msg.err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		// Diff loaded successfully - open readers for both hives and keep them open
		logger.Debug("Opening cached readers", "oldHive", m.hivePath, "newHive", m.comparePath)

		oldReader, err := reader.Open(m.hivePath, hive.OpenOptions{})
		if err != nil {
			m.statusMessage = fmt.Sprintf("Failed to open old hive reader: %v", err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		newReader, err := reader.Open(m.comparePath, hive.OpenOptions{})
		if err != nil {
			oldReader.Close() // Clean up
			m.statusMessage = fmt.Sprintf("Failed to open new hive reader: %v", err)
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			})
		}

		m.oldHiveReader = oldReader
		m.newHiveReader = newReader
		m.diffMode = true
		m.hiveDiff = msg.diff
		m.statusMessage = fmt.Sprintf("✓ Diff loaded: %s", m.comparePath)

		// Save current tree state before entering diff mode (for restoration on exit)
		if item := m.keyTree.CurrentItem(); item != nil {
			m.preDiffCursorPath = item.Path
		}
		m.preDiffExpandedKeys = m.keyTree.GetExpandedKeys()
		logger.Debug("Saved pre-diff state", "cursorPath", m.preDiffCursorPath, "expandedKeys", len(m.preDiffExpandedKeys))

		// Set diff context on CursorManager so navigation signals include diff info
		m.keyTree.CursorManager.SetDiffContext(true, oldReader, newReader)

		// Set diff readers on TreeState for NodeID lookups during tree operations
		m.keyTree.SetDiffReaders(oldReader, newReader)

		// Reload tree with diff data
		return m.reloadTreeWithDiff()
	}

	return m, tea.Batch(cmds...)
}
