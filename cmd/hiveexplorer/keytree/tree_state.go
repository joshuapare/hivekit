package keytree

import (
	"time"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
)

// TreeState manages the tree data structure and visibility state.
// It tracks all items, visible items, expanded/loaded state, and auxiliary data like bookmarks.
type TreeState struct {
	allItems       []Item          // Complete tree structure (all keys, loaded upfront)
	items          []Item          // Visible items (filtered based on expand/collapse and search)
	preFilterItems []Item          // Items before search filter was applied (for restoring)
	expanded       map[string]bool // Which paths are expanded
	loaded         map[string]bool // Which keys have loaded their children
	bookmarks      map[string]bool // Bookmarked key paths
	searchFilter   string          // Search filter query (live filtering)
}

// NewTreeState creates a new tree state manager
func NewTreeState() *TreeState {
	return &TreeState{
		items:     make([]Item, 0),
		expanded:  make(map[string]bool),
		loaded:    make(map[string]bool),
		bookmarks: make(map[string]bool),
	}
}

// AllItems returns all items in the tree
func (ts *TreeState) AllItems() []Item {
	return ts.allItems
}

// SetAllItems sets all items in the tree
func (ts *TreeState) SetAllItems(items []Item) {
	ts.allItems = items
}

// Items returns the currently visible items
func (ts *TreeState) Items() []Item {
	return ts.items
}

// SetItems sets the visible items
func (ts *TreeState) SetItems(items []Item) {
	ts.items = items
}

// GetChildren returns direct children of a parent from allItems
func (ts *TreeState) GetChildren(parentPath string) []Item {
	var children []Item
	for _, item := range ts.allItems {
		if item.Parent == parentPath {
			children = append(children, item)
		}
	}
	return children
}

// GetExpandedKeys returns a copy of currently expanded keys
func (ts *TreeState) GetExpandedKeys() map[string]bool {
	expandedCopy := make(map[string]bool, len(ts.expanded))
	for k, v := range ts.expanded {
		expandedCopy[k] = v
	}
	return expandedCopy
}

// SetExpandedKeys sets the expanded keys map
func (ts *TreeState) SetExpandedKeys(expandedKeys map[string]bool) {
	ts.expanded = make(map[string]bool, len(expandedKeys))
	for k, v := range expandedKeys {
		ts.expanded[k] = v
	}
}

// ItemCount returns the number of visible items
func (ts *TreeState) ItemCount() int {
	return len(ts.items)
}

// GetItem returns the item at the given index, or nil if out of bounds
func (ts *TreeState) GetItem(index int) *Item {
	if index >= 0 && index < len(ts.items) {
		return &ts.items[index]
	}
	return nil
}

// IsExpanded checks if a path is expanded
func (ts *TreeState) IsExpanded(path string) bool {
	return ts.expanded[path]
}

// SetExpanded sets the expanded state for a path
func (ts *TreeState) SetExpanded(path string, expanded bool) {
	ts.expanded[path] = expanded
}

// IsLoaded checks if a path has loaded its children
func (ts *TreeState) IsLoaded(path string) bool {
	return ts.loaded[path]
}

// SetLoaded sets the loaded state for a path
func (ts *TreeState) SetLoaded(path string, loaded bool) {
	ts.loaded[path] = loaded
}

// ClearLoaded clears the loaded state for a path
func (ts *TreeState) ClearLoaded(path string) {
	delete(ts.loaded, path)
}

// IsBookmarked checks if a path is bookmarked
func (ts *TreeState) IsBookmarked(path string) bool {
	return ts.bookmarks[path]
}

// SetBookmarks sets the entire bookmarks map
func (ts *TreeState) SetBookmarks(bookmarks map[string]bool) {
	ts.bookmarks = bookmarks
}

// ClearLoadedDescendants clears the loaded and expanded state for a path and all its descendants
func (ts *TreeState) ClearLoadedDescendants(path string) {
	startTime := time.Now()
	prefix := path + "\\"
	mapSize := len(ts.loaded)

	// Collect keys to delete (can't modify map during iteration)
	collectStart := time.Now()
	toDelete := make([]string, 0)
	scannedKeys := 0
	for key := range ts.loaded {
		scannedKeys++
		if key == path || (len(key) > len(prefix) && key[:len(prefix)] == prefix) {
			toDelete = append(toDelete, key)
		}
	}
	collectDuration := time.Since(collectStart)

	// Delete collected keys from both maps
	deleteStart := time.Now()
	for _, key := range toDelete {
		delete(ts.loaded, key)
		delete(ts.expanded, key)
	}
	deleteDuration := time.Since(deleteStart)

	totalDuration := time.Since(startTime)
	logger.Debug("ClearLoadedDescendants",
		"path", path,
		"mapSize", mapSize,
		"scanned", scannedKeys,
		"deleted", len(toDelete),
		"collectDuration", collectDuration,
		"deleteDuration", deleteDuration,
		"totalDuration", totalDuration,
	)
}

// SetSearchFilter sets the search filter query
func (ts *TreeState) SetSearchFilter(query string) {
	ts.searchFilter = query
}

// SearchFilter returns the current search filter query
func (ts *TreeState) SearchFilter() string {
	return ts.searchFilter
}

// FilterItemsWithParents filters items based on search query, including parent paths.
// If query is less than 3 characters, returns all currently visible items (no filtering).
// Otherwise, returns items that match the query plus their parent paths to maintain hierarchy.
func (ts *TreeState) FilterItemsWithParents(query string) []Item {
	// No filtering if query is too short
	if len(query) < 3 {
		// Restore pre-filter items if they exist (clearing the filter)
		if ts.preFilterItems != nil {
			result := ts.preFilterItems
			ts.preFilterItems = nil // Clear saved state
			return result
		}
		// Otherwise, just return current items
		return ts.items
	}

	// Save pre-filter items if this is the first filter operation
	if ts.preFilterItems == nil {
		ts.preFilterItems = make([]Item, len(ts.items))
		copy(ts.preFilterItems, ts.items)
	}

	// Find all matching items - search in preFilterItems to avoid filtering already-filtered items
	sourceItems := ts.preFilterItems
	if sourceItems == nil {
		sourceItems = ts.items
	}

	matches := make(map[string]bool) // Set of matched paths
	for _, item := range sourceItems {
		if matchesQuery(item, query) {
			matches[item.Path] = true
		}
	}

	// If no matches, return empty list
	if len(matches) == 0 {
		return []Item{}
	}

	// Add parent paths for each match
	parentsNeeded := make(map[string]bool)
	for path := range matches {
		// Add all parent paths
		parentPath := getParentPath(path)
		for parentPath != "" {
			parentsNeeded[parentPath] = true
			parentPath = getParentPath(parentPath)
		}
	}

	// Build filtered list maintaining tree order
	filtered := make([]Item, 0, len(matches)+len(parentsNeeded))
	for _, item := range sourceItems {
		// Include if it's a match or a needed parent
		if matches[item.Path] || parentsNeeded[item.Path] {
			filtered = append(filtered, item)
		}
	}

	logger.Debug("Filter applied", "query", query, "matches", len(matches), "parentsAdded", len(parentsNeeded), "total", len(filtered))

	return filtered
}

// FilterByPathsWithParents filters items to show only the specified paths and their parents.
// This is used for global value search where we have exact paths to match.
// Returns empty list if no paths provided.
func (ts *TreeState) FilterByPathsWithParents(paths []string) []Item {
	// No filtering if no paths provided
	if len(paths) == 0 {
		// Restore pre-filter items if they exist (clearing the filter)
		if ts.preFilterItems != nil {
			result := ts.preFilterItems
			ts.preFilterItems = nil // Clear saved state
			return result
		}
		// Otherwise, just return current items
		return ts.items
	}

	// Save pre-filter items if this is the first filter operation
	if ts.preFilterItems == nil {
		ts.preFilterItems = make([]Item, len(ts.items))
		copy(ts.preFilterItems, ts.items)
	}

	// Build set of paths to match
	sourceItems := ts.preFilterItems
	if sourceItems == nil {
		sourceItems = ts.items
	}

	matches := make(map[string]bool)
	for _, path := range paths {
		matches[path] = true
	}

	// Add parent paths for each match
	parentsNeeded := make(map[string]bool)
	for path := range matches {
		// Add all parent paths
		parentPath := getParentPath(path)
		for parentPath != "" {
			parentsNeeded[parentPath] = true
			parentPath = getParentPath(parentPath)
		}
	}

	// Build filtered list maintaining tree order
	filtered := make([]Item, 0, len(matches)+len(parentsNeeded))
	for _, item := range sourceItems {
		// Include if it's a match or a needed parent
		if matches[item.Path] || parentsNeeded[item.Path] {
			filtered = append(filtered, item)
		}
	}

	logger.Debug("FilterByPaths applied", "paths", len(paths), "matches", len(matches), "parentsAdded", len(parentsNeeded), "total", len(filtered))

	return filtered
}

// matchesQuery checks if an item matches the search query (case-insensitive)
func matchesQuery(item Item, query string) bool {
	// Convert to lowercase for case-insensitive search
	queryLower := ""
	nameLower := ""
	pathLower := ""

	// Manual lowercase conversion to avoid imports
	for _, r := range query {
		if r >= 'A' && r <= 'Z' {
			queryLower += string(r + 32)
		} else {
			queryLower += string(r)
		}
	}

	for _, r := range item.Name {
		if r >= 'A' && r <= 'Z' {
			nameLower += string(r + 32)
		} else {
			nameLower += string(r)
		}
	}

	for _, r := range item.Path {
		if r >= 'A' && r <= 'Z' {
			pathLower += string(r + 32)
		} else {
			pathLower += string(r)
		}
	}

	// Check if query is contained in name or path
	return contains(nameLower, queryLower) || contains(pathLower, queryLower)
}

// contains checks if s contains substr (simple substring check)
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// getParentPath returns the parent path of a given path
// e.g., "Software\\Microsoft\\Windows" -> "Software\\Microsoft"
func getParentPath(path string) string {
	// Find last backslash
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' {
			return path[:i]
		}
	}
	return "" // Root level has no parent
}
