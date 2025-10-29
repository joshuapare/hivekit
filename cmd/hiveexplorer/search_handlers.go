package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// performSearch performs a search and updates match counters.
// For tree pane: filtering is already applied via SetSearchFilter during typing,
// so we just need to count matches and position cursor.
// For value pane: we use the old search approach (jump to matches with n/N).
func (m *Model) performSearch() {
	if m.searchQuery == "" {
		m.searchMatches = 0
		m.searchMatchIdx = 0
		return
	}

	// Search in the currently focused pane
	if m.focusedPane == TreePane {
		// Tree pane: items are already filtered by SetSearchFilter()
		// Count how many items are currently visible (these are the matches)
		visibleItems := m.keyTree.GetItems()

		// Count actual matches (excluding parent paths that were added for hierarchy)
		matches := 0
		for _, item := range visibleItems {
			// Check if this item actually matches the query
			results := SearchKeys([]keytree.Item{item}, m.searchQuery)
			if len(results) > 0 {
				matches++
			}
		}

		m.searchMatches = matches
		m.searchMatchIdx = 0

		// Cursor is already at first item due to filtering, so no need to move
		fmt.Fprintf(
			os.Stderr,
			"[SEARCH] Found %d key matches (showing %d items with parents) for: %s\n",
			matches,
			len(visibleItems),
			m.searchQuery,
		)
	} else {
		// Value pane: use traditional search (no live filtering for values)
		results := SearchValues(m.valueTable.GetItems(), m.searchQuery)
		m.searchMatches = len(results)
		m.searchMatchIdx = 0

		// Jump to first match
		if len(results) > 0 {
			m.valueTable.SetCursor(results[0].Index)
			m.valueTable.EnsureCursorVisible()
		}

		fmt.Fprintf(os.Stderr, "[SEARCH] Found %d value matches for: %s\n", m.searchMatches, m.searchQuery)
	}
}

// handleNextMatch jumps to the next search match
func (m Model) handleNextMatch() (tea.Model, tea.Cmd) {
	fmt.Fprintf(os.Stderr, "[NAV] handleNextMatch called - globalValueSearchActive=%v, results=%d, searchMatches=%d\n",
		m.globalValueSearchActive, len(m.globalValueSearchResults), m.searchMatches)

	// Handle global value search separately
	if m.globalValueSearchActive && len(m.globalValueSearchResults) > 0 {
		fmt.Fprintf(os.Stderr, "[NAV] Delegating to handleNextGlobalValueMatch\n")
		return m.handleNextGlobalValueMatch()
	}

	if m.searchMatches == 0 {
		return m, nil
	}

	// Get all matches
	var results []SearchResult
	if m.focusedPane == TreePane {
		results = SearchKeys(m.keyTree.GetItems(), m.searchQuery)
	} else {
		results = SearchValues(m.valueTable.GetItems(), m.searchQuery)
	}

	if len(results) == 0 {
		return m, nil
	}

	// Move to next match (wrap around)
	m.searchMatchIdx = (m.searchMatchIdx + 1) % len(results)

	// Jump to the match
	if m.focusedPane == TreePane {
		m.keyTree.MoveTo(results[m.searchMatchIdx].Index)
	} else {
		m.valueTable.SetCursor(results[m.searchMatchIdx].Index)
		m.valueTable.EnsureCursorVisible()
	}

	return m, nil
}

// handlePrevMatch jumps to the previous search match
func (m Model) handlePrevMatch() (tea.Model, tea.Cmd) {
	// Handle global value search separately
	if m.globalValueSearchActive && len(m.globalValueSearchResults) > 0 {
		return m.handlePrevGlobalValueMatch()
	}

	if m.searchMatches == 0 {
		return m, nil
	}

	// Get all matches
	var results []SearchResult
	if m.focusedPane == TreePane {
		results = SearchKeys(m.keyTree.GetItems(), m.searchQuery)
	} else {
		results = SearchValues(m.valueTable.GetItems(), m.searchQuery)
	}

	if len(results) == 0 {
		return m, nil
	}

	// Move to previous match (wrap around)
	m.searchMatchIdx--
	if m.searchMatchIdx < 0 {
		m.searchMatchIdx = len(results) - 1
	}

	// Jump to the match
	if m.focusedPane == TreePane {
		m.keyTree.MoveTo(results[m.searchMatchIdx].Index)
	} else {
		m.valueTable.SetCursor(results[m.searchMatchIdx].Index)
		m.valueTable.EnsureCursorVisible()
	}

	return m, nil
}

// handleFirstMatch jumps to the first search match
func (m Model) handleFirstMatch() (tea.Model, tea.Cmd) {
	// Handle global value search separately
	if m.globalValueSearchActive && len(m.globalValueSearchResults) > 0 {
		return m.handleFirstGlobalValueMatch()
	}

	if m.searchMatches == 0 {
		return m, nil
	}

	// Get all matches
	var results []SearchResult
	if m.focusedPane == TreePane {
		results = SearchKeys(m.keyTree.GetItems(), m.searchQuery)
	} else {
		results = SearchValues(m.valueTable.GetItems(), m.searchQuery)
	}

	if len(results) == 0 {
		return m, nil
	}

	// Jump to first match
	m.searchMatchIdx = 0
	if m.focusedPane == TreePane {
		m.keyTree.MoveTo(results[0].Index)
	} else {
		m.valueTable.SetCursor(results[0].Index)
		m.valueTable.EnsureCursorVisible()
	}

	return m, nil
}

// handleLastMatch jumps to the last search match
func (m Model) handleLastMatch() (tea.Model, tea.Cmd) {
	// Handle global value search separately
	if m.globalValueSearchActive && len(m.globalValueSearchResults) > 0 {
		return m.handleLastGlobalValueMatch()
	}

	if m.searchMatches == 0 {
		return m, nil
	}

	// Get all matches
	var results []SearchResult
	if m.focusedPane == TreePane {
		results = SearchKeys(m.keyTree.GetItems(), m.searchQuery)
	} else {
		results = SearchValues(m.valueTable.GetItems(), m.searchQuery)
	}

	if len(results) == 0 {
		return m, nil
	}

	// Jump to last match
	m.searchMatchIdx = len(results) - 1
	if m.focusedPane == TreePane {
		m.keyTree.MoveTo(results[m.searchMatchIdx].Index)
	} else {
		m.valueTable.SetCursor(results[m.searchMatchIdx].Index)
		m.valueTable.EnsureCursorVisible()
	}

	return m, nil
}

// performGlobalValueSearch searches for values across ALL keys in the hive
// Returns a tea.Cmd that will emit globalValueSearchCompleteMsg when done
func (m *Model) performGlobalValueSearch(query string) tea.Cmd {
	if query == "" {
		return nil
	}

	return func() tea.Msg {
		fmt.Fprintf(os.Stderr, "[GLOBAL_VALUE_SEARCH] Starting search for: %s\n", query)

		var results []GlobalValueSearchResult
		queryLower := strings.ToLower(query)

		// Get all keys from the tree
		allItems := m.keyTree.AllItems()
		totalKeys := len(allItems)

		fmt.Fprintf(os.Stderr, "[GLOBAL_VALUE_SEARCH] Searching %d keys...\n", totalKeys)

		// Search each key for matching values
		for _, item := range allItems {
			// Load values for this key
			values, err := m.loadValuesForNode(item.NodeID)
			if err != nil {
				// Skip keys where we can't load values
				continue
			}

			// Check if any values match
			var matchingValues []ValueMatch
			for _, val := range values {
				var matchedIn string

				// Check value name
				if strings.Contains(strings.ToLower(val.Name), queryLower) {
					matchedIn = "name"
				}

				// Check value type
				if matchedIn == "" && strings.Contains(strings.ToLower(val.Type), queryLower) {
					matchedIn = "type"
				}

				// Check value content
				if matchedIn == "" && strings.Contains(strings.ToLower(val.StringVal), queryLower) {
					matchedIn = "value"
				}

				// If this value matched, add it to results
				if matchedIn != "" {
					// Truncate value content if too long
					valueStr := val.StringVal
					if len(valueStr) > 100 {
						valueStr = valueStr[:100] + "..."
					}

					matchingValues = append(matchingValues, ValueMatch{
						Name:      val.Name,
						Type:      val.Type,
						Value:     valueStr,
						MatchedIn: matchedIn,
					})
				}
			}

			// If this key has matching values, add to results
			if len(matchingValues) > 0 {
				results = append(results, GlobalValueSearchResult{
					KeyPath:        item.Path,
					MatchingValues: matchingValues,
					MatchCount:     len(matchingValues),
				})
			}
		}

		fmt.Fprintf(
			os.Stderr,
			"[GLOBAL_VALUE_SEARCH] Found %d keys with matching values\n",
			len(results),
		)

		return globalValueSearchCompleteMsg{
			results: results,
			query:   query,
		}
	}
}

// loadValuesForNode loads values for a specific node ID
// This is a helper for global value search
func (m *Model) loadValuesForNode(nodeID hive.NodeID) ([]ValueInfo, error) {
	reader := m.keyTree.Reader()
	if reader == nil {
		return nil, fmt.Errorf("no reader available")
	}

	// Get value IDs for this node
	valueIDs, err := reader.Values(nodeID)
	if err != nil {
		return nil, err
	}

	// Load value metadata and data
	var valueInfos []ValueInfo
	for _, valID := range valueIDs {
		meta, err := reader.StatValue(valID)
		if err != nil {
			continue // Skip broken values
		}

		data, err := reader.ValueBytes(valID, hive.ReadOptions{CopyData: true})
		if err != nil {
			data = []byte{} // Empty data on error
		}

		valInfo := ValueInfo{
			Name: meta.Name,
			Type: meta.Type.String(),
			Size: len(data),
			Data: data,
		}

		// Format value to string based on type (for search matching)
		valInfo.StringVal = formatValueToString(meta.Type, data)

		valueInfos = append(valueInfos, valInfo)
	}

	return valueInfos, nil
}

// formatValueToString formats a registry value to a searchable string representation
func formatValueToString(regType hive.RegType, data []byte) string {
	switch regType {
	case hive.REG_SZ, hive.REG_EXPAND_SZ:
		return string(data)
	case hive.REG_DWORD, hive.REG_DWORD_BE:
		if len(data) >= 4 {
			dwordVal := uint32(
				data[0],
			) | uint32(
				data[1],
			)<<8 | uint32(
				data[2],
			)<<16 | uint32(
				data[3],
			)<<24
			return fmt.Sprintf("0x%08x (%d)", dwordVal, dwordVal)
		}
		return ""
	case hive.REG_QWORD:
		if len(data) >= 8 {
			qwordVal := uint64(
				data[0],
			) | uint64(
				data[1],
			)<<8 | uint64(
				data[2],
			)<<16 | uint64(
				data[3],
			)<<24 |
				uint64(
					data[4],
				)<<32 | uint64(
				data[5],
			)<<40 | uint64(
				data[6],
			)<<48 | uint64(
				data[7],
			)<<56
			return fmt.Sprintf("0x%016x (%d)", qwordVal, qwordVal)
		}
		return ""
	case hive.REG_BINARY:
		// For binary data, just return a short hex representation for searching
		if len(data) > 32 {
			return fmt.Sprintf("%x...", data[:32])
		}
		return fmt.Sprintf("%x", data)
	default:
		// For unknown types, return hex representation
		if len(data) > 32 {
			return fmt.Sprintf("%x...", data[:32])
		} else if len(data) > 0 {
			return fmt.Sprintf("%x", data)
		}
		return "(empty)"
	}
}

// Global value search navigation handlers

// handleNextGlobalValueMatch navigates to the next key with matching values
func (m Model) handleNextGlobalValueMatch() (tea.Model, tea.Cmd) {
	fmt.Fprintf(os.Stderr, "[NAV] handleNextGlobalValueMatch called, %d results\n", len(m.globalValueSearchResults))

	if len(m.globalValueSearchResults) == 0 {
		fmt.Fprintf(os.Stderr, "[NAV] No results available\n")
		return m, nil
	}

	// Get current key path
	currentPath := ""
	if item := m.keyTree.CurrentItem(); item != nil {
		currentPath = item.Path
	}
	fmt.Fprintf(os.Stderr, "[NAV] Current path: %s\n", currentPath)

	// Find next match after current path
	foundCurrent := false
	for i, result := range m.globalValueSearchResults {
		if foundCurrent {
			// Navigate to this key
			fmt.Fprintf(os.Stderr, "[NAV] Navigating to result[%d]: %s\n", i, result.KeyPath)
			return m, m.keyTree.NavigateToPath(result.KeyPath)
		}
		if result.KeyPath == currentPath {
			fmt.Fprintf(os.Stderr, "[NAV] Found current at index %d\n", i)
			foundCurrent = true
		}
	}

	// Wrap around to first match if we're at the end
	if len(m.globalValueSearchResults) > 0 {
		fmt.Fprintf(os.Stderr, "[NAV] Wrapping to first result: %s\n", m.globalValueSearchResults[0].KeyPath)
		return m, m.keyTree.NavigateToPath(m.globalValueSearchResults[0].KeyPath)
	}

	return m, nil
}

// handlePrevGlobalValueMatch navigates to the previous key with matching values
func (m Model) handlePrevGlobalValueMatch() (tea.Model, tea.Cmd) {
	if len(m.globalValueSearchResults) == 0 {
		return m, nil
	}

	// Get current key path
	currentPath := ""
	if item := m.keyTree.CurrentItem(); item != nil {
		currentPath = item.Path
	}

	// Find previous match before current path (search backwards)
	for i := len(m.globalValueSearchResults) - 1; i >= 0; i-- {
		result := m.globalValueSearchResults[i]
		if result.KeyPath == currentPath {
			// Found current, return previous if it exists
			if i > 0 {
				return m, m.keyTree.NavigateToPath(m.globalValueSearchResults[i-1].KeyPath)
			}
			// Wrap around to last match
			return m, m.keyTree.NavigateToPath(
				m.globalValueSearchResults[len(m.globalValueSearchResults)-1].KeyPath,
			)
		}
	}

	// If current path not found, go to last match
	if len(m.globalValueSearchResults) > 0 {
		return m, m.keyTree.NavigateToPath(
			m.globalValueSearchResults[len(m.globalValueSearchResults)-1].KeyPath,
		)
	}

	return m, nil
}

// handleFirstGlobalValueMatch navigates to the first key with matching values
func (m Model) handleFirstGlobalValueMatch() (tea.Model, tea.Cmd) {
	if len(m.globalValueSearchResults) == 0 {
		return m, nil
	}

	return m, m.keyTree.NavigateToPath(m.globalValueSearchResults[0].KeyPath)
}

// handleLastGlobalValueMatch navigates to the last key with matching values
func (m Model) handleLastGlobalValueMatch() (tea.Model, tea.Cmd) {
	if len(m.globalValueSearchResults) == 0 {
		return m, nil
	}

	lastIdx := len(m.globalValueSearchResults) - 1
	return m, m.keyTree.NavigateToPath(m.globalValueSearchResults[lastIdx].KeyPath)
}
