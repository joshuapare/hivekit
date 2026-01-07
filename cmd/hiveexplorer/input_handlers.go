package main

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
)

// handleInputMode handles input when in search or go-to-path mode
func (m Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Cancel input mode
		wasGlobalSearch := m.inputMode == GlobalValueSearchMode
		m.inputMode = NormalMode
		m.inputBuffer = ""
		m.searchQuery = ""
		// Clear search filter when exiting search mode
		if m.focusedPane == TreePane {
			m.keyTree.SetSearchFilter("")
		}
		// Clear global value search state
		if wasGlobalSearch {
			m.globalValueSearchActive = false
			m.globalValueSearchResults = nil
			if m.globalValueSearchDebounce != nil {
				m.globalValueSearchDebounce.Stop()
				m.globalValueSearchDebounce = nil
			}
		}
		return m, nil

	case tea.KeyEnter:
		// Execute the command
		switch m.inputMode {
		case SearchMode:
			m.searchQuery = m.inputBuffer
			m.inputMode = NormalMode
			// Perform search and update matches
			m.performSearch()
			return m, nil
		case GlobalValueSearchMode:
			query := m.inputBuffer
			m.inputMode = NormalMode
			m.inputBuffer = ""
			m.globalValueSearchInProgress = true
			// Cancel debounce timer if active
			if m.globalValueSearchDebounce != nil {
				m.globalValueSearchDebounce.Stop()
				m.globalValueSearchDebounce = nil
			}
			// Start global value search immediately on Enter
			return m, m.performGlobalValueSearch(query)
		case GoToPathMode:
			path := m.inputBuffer
			m.inputMode = NormalMode
			m.inputBuffer = ""
			return m.handleGoToPath(path)
		case DiffPathMode:
			comparePath := m.inputBuffer
			m.inputMode = NormalMode
			m.inputBuffer = ""
			return m.handleLoadDiff(comparePath)
		default:
			return m, nil
		}

	case tea.KeyBackspace, tea.KeyDelete:
		// Remove last character
		if len(m.inputBuffer) > 0 {
			m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
		}
		// Update live search filter for tree pane
		if m.inputMode == SearchMode && m.focusedPane == TreePane {
			m.keyTree.SetSearchFilter(m.inputBuffer)
		}
		// Debounce global value search
		if m.inputMode == GlobalValueSearchMode {
			return m, m.debounceGlobalValueSearch()
		}
		return m, nil

	case tea.KeyRunes:
		// Add character to buffer
		m.inputBuffer += string(msg.Runes)
		// Update live search filter for tree pane
		if m.inputMode == SearchMode && m.focusedPane == TreePane {
			m.keyTree.SetSearchFilter(m.inputBuffer)
		}
		// Debounce global value search
		if m.inputMode == GlobalValueSearchMode {
			return m, m.debounceGlobalValueSearch()
		}
		return m, nil
	}

	return m, nil
}

// debounceGlobalValueSearch creates a debounced command for global value search.
// It cancels any existing timer and starts a new one that will trigger after 500ms.
func (m *Model) debounceGlobalValueSearch() tea.Cmd {
	// Cancel existing timer
	if m.globalValueSearchDebounce != nil {
		m.globalValueSearchDebounce.Stop()
	}

	// If input is empty, clear results
	if m.inputBuffer == "" {
		m.globalValueSearchActive = false
		m.globalValueSearchResults = nil
		m.globalValueSearchDebounce = nil
		return nil
	}

	// Create new debounce timer (500ms)
	query := m.inputBuffer
	m.globalValueSearchDebounce = time.AfterFunc(500*time.Millisecond, func() {
		// This will be executed in a separate goroutine
		// We can't update the model here, so we do nothing
		// The actual search will be triggered by a tick message
	})

	// Return a command that waits 500ms then triggers search
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return globalValueSearchTriggerMsg{query: query}
	})
}

// globalValueSearchTriggerMsg is sent when debounce timer expires
type globalValueSearchTriggerMsg struct {
	query string
}

// handleGoToPath navigates to a specific path
func (m Model) handleGoToPath(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		return m, nil
	}

	// Use NavigateToPath which will expand parents if needed
	cmd := m.keyTree.NavigateToPath(path)
	m.focusedPane = TreePane

	// Check if we found it
	if item := m.keyTree.CurrentItem(); item != nil && item.Path == path {
		logger.Debug("Navigated to path", "path", path)
		// NavigateToPath emits navigation signal, value table will load values
	} else {
		logger.Debug("Path not found", "path", path)
		m.statusMessage = "Path not found: " + path
	}

	return m, cmd
}

// handleNextBookmark navigates to the next bookmarked key
func (m Model) handleNextBookmark() (tea.Model, tea.Cmd) {
	if len(m.bookmarks) == 0 {
		m.statusMessage = "No bookmarks found"
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return clearStatusMsg{}
		})
	}

	// Get all bookmark paths sorted
	var bookmarkPaths []string
	for path := range m.bookmarks {
		bookmarkPaths = append(bookmarkPaths, path)
	}
	sort.Strings(bookmarkPaths)

	// Get current item path
	currentPath := ""
	if item := m.keyTree.CurrentItem(); item != nil {
		currentPath = item.Path
	}

	// Find next bookmark after current path
	var targetPath string
	if currentPath == "" {
		// No current path, go to first bookmark
		targetPath = bookmarkPaths[0]
	} else {
		// Find current position in sorted bookmarks
		found := false
		for _, path := range bookmarkPaths {
			if path > currentPath {
				targetPath = path
				found = true
				break
			}
		}
		// If no bookmark found after current, wrap to first
		if !found {
			targetPath = bookmarkPaths[0]
		}
	}

	// Navigate to target bookmark
	if targetPath != "" {
		m.focusedPane = TreePane
		cmd := m.keyTree.NavigateToPath(targetPath)
		m.statusMessage = fmt.Sprintf("Navigating to bookmark: %s", targetPath)
		return m, tea.Batch(
			cmd,
			tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return clearStatusMsg{}
			}),
		)
	}

	return m, nil
}
