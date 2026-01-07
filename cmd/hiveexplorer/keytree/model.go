package keytree

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keyselection"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree/adapter"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree/display"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/virtuallist"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Model manages the hierarchical key tree
type Model struct {
	hivePath string
	reader   interface{ Close() error }
	navBus   *keyselection.Bus

	state         *TreeState
	nav           *Navigator
	expander      *ExpandManager
	renderer      *virtuallist.Renderer
	CursorManager *CursorManager // Embedded for clean access to cursor operations

	// Input handling
	keys      Keys
	bookmarks map[string]bool // Set of bookmarked key paths
}

// NewModel creates a new key tree model
func NewModel(hivePath string) Model {
	state := NewTreeState()
	nav := NewNavigator()
	expander := NewExpandManager(state, nav)
	cursorMgr := newCursorManager(nav, state, hivePath)

	return Model{
		hivePath:      hivePath,
		state:         state,
		nav:           nav,
		expander:      expander,
		renderer:      nil, // Lazy-initialized in View() to avoid dangling pointer
		CursorManager: cursorMgr,
	}
}

// Close closes the underlying hive reader
func (m *Model) Close() error {
	if m.reader != nil {
		err := m.reader.Close()
		m.reader = nil
		return err
	}
	return nil
}

// Init initializes the key tree
func (m Model) Init() tea.Cmd {
	return LoadEntireTree(m.hivePath)
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Initialize renderer early to avoid nil pointer panics
	if m.renderer == nil {
		m.renderer = virtuallist.New(m)
		logger.Debug("created new renderer", "renderer", fmt.Sprintf("%p", m.renderer))
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case TreeLoadedMsg:
		// Store complete tree and reader
		m.state.SetAllItems(msg.Items)
		m.reader = msg.Reader

		// Mark all items as loaded (since we have the complete tree)
		for _, item := range msg.Items {
			m.state.SetLoaded(item.Path, true)
		}

		// Show only root level items initially (depth 0)
		rootItems := make([]Item, 0)
		for _, item := range msg.Items {
			if item.Depth == 0 {
				rootItems = append(rootItems, item)
			}
		}
		m.state.SetItems(rootItems)

		logger.Debug("tree loaded", "total_items", len(msg.Items), "root_items", len(rootItems))

		m.updateViewport()

		// Emit navigation signal for initial item (cursor is at 0)
		m.CursorManager.EmitSignal()

		return *m, nil

	case RootKeysLoadedMsg:
		// Add root keys to the tree (legacy support)
		items := m.state.Items()
		for _, key := range msg.Keys {
			item := Item{
				Path:        key.Path,
				Name:        key.Name,
				Depth:       0,
				HasChildren: key.SubkeyN > 0,
				SubkeyCount: key.SubkeyN,
				ValueCount:  key.ValueN,
				LastWrite:   key.LastWrite,
				Expanded:    false,
				Parent:      "",
			}
			items = append(items, item)
		}
		m.state.SetItems(items)
		m.state.SetLoaded("", true)
		m.updateViewport()

		// Emit navigation signal for initial item (cursor is at 0)
		m.CursorManager.EmitSignal()

		return *m, nil

	case ChildKeysLoadedMsg:
		// Find parent index
		items := m.state.Items()
		parentIdx := -1
		for i, item := range items {
			if item.Path == msg.Parent {
				parentIdx = i
				break
			}
		}

		if parentIdx >= 0 {
			parent := items[parentIdx]

			// Mark parent as expanded BEFORE copying to newItems
			logger.Debug("children loaded: setting expanded", "parent", msg.Parent)
			items[parentIdx].Expanded = true
			m.state.SetExpanded(msg.Parent, true)

			// Create child items
			children := make([]Item, 0)
			for _, key := range msg.Keys {
				child := Item{
					Path:        key.Path,
					Name:        key.Name,
					Depth:       parent.Depth + 1,
					HasChildren: key.SubkeyN > 0,
					SubkeyCount: key.SubkeyN,
					ValueCount:  key.ValueN,
					LastWrite:   key.LastWrite,
					Expanded:    false,
					Parent:      msg.Parent,
				}
				children = append(children, child)
			}

			// Insert children after parent
			newItems := make([]Item, 0, len(items)+len(children))
			newItems = append(newItems, items[:parentIdx+1]...)
			newItems = append(newItems, children...)
			newItems = append(newItems, items[parentIdx+1:]...)
			m.state.SetItems(newItems)

			// Adjust cursor if it's after the insertion point
			oldCursor := m.nav.Cursor()
			if oldCursor > parentIdx {
				m.nav.SetCursor(oldCursor + len(children))
				logger.Debug("children loaded: adjusted cursor",
					"old_cursor", oldCursor, "new_cursor", m.nav.Cursor(),
					"children_count", len(children), "parent_idx", parentIdx)
			}

			logger.Debug("children loaded", "parent", msg.Parent,
				"children_count", len(children), "total_items", len(newItems))
			m.state.SetLoaded(msg.Parent, true)

			m.updateViewport()

			// Check if we have a pending navigation to complete
			pendingTarget := m.nav.PendingNavigationTarget()
			if pendingTarget != "" {
				logger.Debug("childKeysLoadedMsg: checking pending navigation", "target", pendingTarget)
				// Try to find and navigate to the pending target
				found := false
				currentItems := m.state.Items()
				for i := range currentItems {
					if currentItems[i].Path == pendingTarget {
						m.MoveTo(i)
						logger.Debug("childKeysLoadedMsg: completed pending navigation", "target", pendingTarget, "index", i)
						m.nav.ClearPendingNavigationTarget()
						found = true
						break
					}
				}

				// If target still not found, continue expanding parents
				if !found {
					logger.Debug("childKeysLoadedMsg: target not found yet, continuing parent expansion")
					cmd = m.ExpandParents(pendingTarget)
				}
			}
		}

	case tea.KeyMsg:
		// Handle keyboard input for tree navigation and operations
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.renderer.SetSize(msg.Width, msg.Height)
		m.updateViewport()
	}

	// Update viewport
	vpCmd := m.renderer.Update(msg)
	if cmd == nil {
		cmd = vpCmd
	}

	return *m, cmd
}

// handleKeyMsg handles keyboard input for tree navigation and operations
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.MoveUp()
		if item := m.CurrentItem(); item != nil {
			logger.Debug("up key", "cursor", m.GetCursor(), "path", item.Path)
		}

	case key.Matches(msg, m.keys.Down):
		m.MoveDown()
		if item := m.CurrentItem(); item != nil {
			logger.Debug("down key", "cursor", m.GetCursor(), "path", item.Path)
		}

	case key.Matches(msg, m.keys.Enter):
		// Enter toggles expand/collapse
		if item := m.CurrentItem(); item != nil && item.Expanded {
			logger.Debug("enter: collapsing", "path", item.Path)
			m.Collapse()
			return *m, nil
		}
		if item := m.CurrentItem(); item != nil {
			logger.Debug("enter: expanding", "path", item.Path)
		}
		return *m, m.Expand()

	case key.Matches(msg, m.keys.Right):
		// Right arrow only expands
		if item := m.CurrentItem(); item != nil {
			logger.Debug("right: expanding", "path", item.Path)
		}
		return *m, m.Expand()

	case key.Matches(msg, m.keys.Left):
		m.Collapse()
		return *m, nil

	case key.Matches(msg, m.keys.Home):
		// Jump to first item (emits single navigation signal)
		m.JumpToStart()

	case key.Matches(msg, m.keys.End):
		// Jump to last item (emits single navigation signal)
		m.JumpToEnd()

	case key.Matches(msg, m.keys.GoToParent):
		m.GoToParent() // Emits navigation signal

	case key.Matches(msg, m.keys.ExpandAll):
		return *m, m.ExpandAllChildren()

	case key.Matches(msg, m.keys.CollapseAll):
		m.CollapseAll()
		// Cursor position may change, navigation signal emitted

	case key.Matches(msg, m.keys.ExpandLevel):
		return *m, m.ExpandCurrentLevel()

	case key.Matches(msg, m.keys.CollapseToLevel):
		m.CollapseToCurrentLevel()
		// Cursor position may change, navigation signal emitted

	case key.Matches(msg, m.keys.Copy):
		// Copy current path - emit message for main Model to show status
		err := m.CopyCurrentPath()
		success := err == nil
		item := m.CurrentItem()
		path := ""
		if item != nil {
			path = item.Path
		}
		return *m, func() tea.Msg {
			return CopyPathRequestedMsg{
				Path:    path,
				Success: success,
				Err:     err,
			}
		}

	case key.Matches(msg, m.keys.ToggleBookmark):
		// Toggle bookmark - emit message for main Model to update bookmark map
		if item := m.CurrentItem(); item != nil {
			// Toggle in local map
			added := !m.bookmarks[item.Path]
			if added {
				m.bookmarks[item.Path] = true
			} else {
				delete(m.bookmarks, item.Path)
			}
			// Update tree state for display
			m.state.SetBookmarks(m.bookmarks)

			return *m, func() tea.Msg {
				return BookmarkToggledMsg{
					Path:  item.Path,
					Added: added,
				}
			}
		}
	}

	return *m, nil
}

// View renders the key tree
func (m *Model) View() string {
	// Lazy-initialize renderer to avoid dangling pointer from constructor
	if m.renderer == nil {
		m.renderer = virtuallist.New(m)
		logger.Debug("view: created new renderer",
			"renderer", fmt.Sprintf("%p", m.renderer),
			"model", fmt.Sprintf("%p", m))
	} else {
		logger.Debug("view: using existing renderer",
			"renderer", fmt.Sprintf("%p", m.renderer),
			"model", fmt.Sprintf("%p", m))
	}
	return m.renderer.View()
}

// updateViewport updates the viewport content
func (m *Model) updateViewport() {
	// With virtual scrolling, we just need to update the cursor position
	// The View() method will render only visible items on demand
	if m.renderer != nil {
		m.renderer.SetCursor(m.nav.Cursor())
	}
}

// SetNavigationBus sets the navigation bus for this tree model
func (m *Model) SetNavigationBus(bus *keyselection.Bus) {
	m.navBus = bus
	m.CursorManager.setNavigationBus(bus)
}

// MoveUp moves the cursor up
func (m *Model) MoveUp() {
	m.CursorManager.MoveUp(m.renderer)
}

// MoveDown moves the cursor down
func (m *Model) MoveDown() {
	m.CursorManager.MoveDown(m.renderer)
}

// MoveTo moves the cursor to the specified position
func (m *Model) MoveTo(pos int) bool {
	return m.CursorManager.MoveTo(pos, m.renderer)
}

// JumpToStart jumps the cursor to the first item
func (m *Model) JumpToStart() {
	m.CursorManager.JumpToStart(m.renderer)
}

// JumpToEnd jumps the cursor to the last item
func (m *Model) JumpToEnd() {
	m.CursorManager.JumpToEnd(m.renderer)
}

// Expand expands the current item
func (m *Model) Expand() tea.Cmd {
	cmd := m.expander.Expand(func(path string) tea.Cmd {
		return LoadChildren(m.hivePath, path)
	})
	m.updateViewport()
	return cmd
}

// Collapse collapses the current item
func (m *Model) Collapse() {
	m.expander.Collapse()
	m.updateViewport()
}

// CurrentItem returns the currently selected item
func (m *Model) CurrentItem() *Item {
	return m.state.GetItem(m.nav.Cursor())
}

// GetPath returns the path of the current item
func (m *Model) GetPath() string {
	item := m.CurrentItem()
	if item != nil {
		return item.Path
	}
	return ""
}

// GoToParent navigates to the parent of the current item
func (m *Model) GoToParent() {
	item := m.CurrentItem()
	if item == nil || item.Parent == "" {
		return
	}

	// Find parent in visible items
	items := m.state.Items()
	for i, parentItem := range items {
		if parentItem.Path == item.Parent {
			m.MoveTo(i)
			return
		}
	}
}

// ExpandAllChildren recursively expands all children of the current item
func (m *Model) ExpandAllChildren() tea.Cmd {
	item := m.CurrentItem()
	if item == nil || !item.HasChildren {
		return nil
	}

	// Store the starting cursor position
	startCursor := m.nav.Cursor()

	// In diff mode, use diffMap which is already in memory
	if len(m.state.diffMap) > 0 {
		m.expandAllFromDiffMap(item.Path)
		m.nav.SetCursor(startCursor)
		if m.renderer != nil {
			m.renderer.SetCursor(startCursor)
		}
		return nil
	}

	// If allItems is loaded (upfront tree load), use in-memory expansion
	allItems := m.state.AllItems()
	if len(allItems) > 0 {
		m.expandAllFromAllItems(item.Path)
		m.nav.SetCursor(startCursor)
		if m.renderer != nil {
			m.renderer.SetCursor(startCursor)
		}
		return nil
	}

	// Fallback: Normal mode with lazy loading - load children synchronously and recursively
	m.expandAllSync(item.Path)

	// Restore cursor to original position
	m.nav.SetCursor(startCursor)
	if m.renderer != nil {
		m.renderer.SetCursor(startCursor)
	}

	return nil
}

// expandAllSync synchronously loads and expands all descendants in normal mode
func (m *Model) expandAllSync(rootPath string) {
	logger.Debug("expandAllSync: expanding all descendants", "root", rootPath)

	// Queue of paths to expand
	toExpand := []string{rootPath}
	expanded := make(map[string]bool)

	for len(toExpand) > 0 {
		// Pop the first path
		currentPath := toExpand[0]
		toExpand = toExpand[1:]

		if expanded[currentPath] {
			continue
		}

		// Find the item in the tree
		items := m.state.Items()
		var currentIdx int
		var found bool
		for i, item := range items {
			if item.Path == currentPath {
				currentIdx = i
				found = true
				break
			}
		}

		if !found {
			logger.Debug("expandAllSync: path not found in tree", "path", currentPath)
			continue
		}

		currentItem := &items[currentIdx]
		if !currentItem.HasChildren {
			continue
		}

		// Load children if not already loaded
		if !m.state.IsLoaded(currentPath) {
			logger.Debug("expandAllSync: loading children", "path", currentPath)

			// Open reader if not already open
			if m.reader == nil {
				r, err := reader.Open(m.hivePath, hive.OpenOptions{})
				if err != nil {
					logger.Error("expandAllSync: error opening reader", "error", err)
					return
				}
				m.reader = r
			}

			// Get the hive.Reader interface
			r, ok := m.reader.(hive.Reader)
			if !ok {
				logger.Error("expandAllSync: reader does not implement hive.Reader")
				return
			}

			// Navigate to the key using the reader
			var node hive.NodeID
			var err error
			if currentPath == "" {
				node, err = r.Root()
			} else {
				node, err = r.Find(currentPath)
			}
			if err != nil {
				logger.Debug("expandAllSync: error finding key", "path", currentPath, "error", err)
				continue
			}

			// Get children using the reader
			childIDs, err := r.Subkeys(node)
			if err != nil {
				logger.Debug("expandAllSync: error getting subkeys", "path", currentPath, "error", err)
				continue
			}

			// Create child items
			children := make([]Item, 0, len(childIDs))
			for _, childID := range childIDs {
				meta, err := r.StatKey(childID)
				if err != nil {
					continue
				}

				childPath := meta.Name
				if currentPath != "" {
					childPath = currentPath + "\\" + meta.Name
				}

				child := Item{
					Path:        childPath,
					Name:        meta.Name,
					Depth:       currentItem.Depth + 1,
					HasChildren: meta.SubkeyN > 0,
					SubkeyCount: meta.SubkeyN,
					ValueCount:  meta.ValueN,
					LastWrite:   meta.LastWrite,
					Expanded:    false,
					Parent:      currentPath,
				}
				children = append(children, child)
			}

			// Insert children after current item
			items = m.state.Items() // Refresh items
			newItems := make([]Item, 0, len(items)+len(children))
			newItems = append(newItems, items[:currentIdx+1]...)
			newItems = append(newItems, children...)
			newItems = append(newItems, items[currentIdx+1:]...)
			m.state.SetItems(newItems)

			m.state.SetLoaded(currentPath, true)
			logger.Debug("expandAllSync: inserted children", "count", len(children), "path", currentPath)
		}

		// Mark as expanded (must be after potential slice rebuild)
		items = m.state.Items()
		items[currentIdx].Expanded = true
		m.state.SetExpanded(currentPath, true)
		m.state.SetItems(items)
		expanded[currentPath] = true

		// Add all children to the expansion queue
		items = m.state.Items()
		for _, item := range items {
			if item.Parent == currentPath && item.HasChildren {
				toExpand = append(toExpand, item.Path)
			}
		}
	}

	m.updateViewport()
	logger.Debug("expandAllSync: complete", "total_items", m.state.ItemCount())
}

// expandAllFromDiffMap expands all descendants using data from diffMap (diff mode)
func (m *Model) expandAllFromDiffMap(rootPath string) {
	logger.Debug("expandAllFromDiffMap: expanding all descendants", "root", rootPath)

	// Queue of paths to expand
	toExpand := []string{rootPath}
	expanded := make(map[string]bool)

	for len(toExpand) > 0 {
		// Pop the first path
		currentPath := toExpand[0]
		toExpand = toExpand[1:]

		if expanded[currentPath] {
			continue
		}

		// Find the item in the tree
		items := m.state.Items()
		var currentIdx int
		var found bool
		for i, item := range items {
			if item.Path == currentPath {
				currentIdx = i
				found = true
				break
			}
		}

		if !found {
			continue
		}

		currentItem := &items[currentIdx]
		if !currentItem.HasChildren {
			continue
		}

		// Expand from diffMap if not already loaded
		if !m.state.IsLoaded(currentPath) {
			m.nav.SetCursor(currentIdx)
			m.expandFromDiffMap(currentItem)
		} else {
			// Just mark as expanded
			items = m.state.Items()
			items[currentIdx].Expanded = true
			m.state.SetExpanded(currentPath, true)
			m.state.SetItems(items)
		}

		expanded[currentPath] = true

		// Add all children to the expansion queue
		items = m.state.Items()
		for _, item := range items {
			if item.Parent == currentPath && item.HasChildren {
				toExpand = append(toExpand, item.Path)
			}
		}
	}

	m.updateViewport()
	logger.Debug("expandAllFromDiffMap: complete", "total_items", m.state.ItemCount())
}

// expandAllFromAllItems recursively expands all descendants using preloaded allItems (in-memory)
func (m *Model) expandAllFromAllItems(rootPath string) {
	startTime := time.Now()
	logger.Debug("expandAllFromAllItems: START", "root", rootPath)

	allItems := m.state.AllItems()
	if len(allItems) == 0 {
		logger.Debug("expandAllFromAllItems: no allItems available")
		return
	}

	// Step 0: Build children lookup map ONCE to avoid O(n*m) scans
	step0Start := time.Now()
	childrenMap := make(map[string][]Item)
	for _, item := range allItems {
		childrenMap[item.Parent] = append(childrenMap[item.Parent], item)
	}
	step0Duration := time.Since(step0Start)
	logger.Debug("expandAllFromAllItems: Step 0 (build children map)",
		"duration", step0Duration, "items", len(allItems))

	// Step 1: Find all descendants of rootPath that should be expanded
	step1Start := time.Now()
	toExpand := make(map[string]bool)
	toExpand[rootPath] = true

	// BFS to find all descendants using childrenMap
	queue := []string{rootPath}
	bfsIterations := 0
	for len(queue) > 0 {
		currentPath := queue[0]
		queue = queue[1:]
		bfsIterations++

		// Find children using O(1) map lookup instead of O(n) scan
		for _, item := range childrenMap[currentPath] {
			if item.HasChildren {
				if !toExpand[item.Path] {
					toExpand[item.Path] = true
					queue = append(queue, item.Path)
				}
			}
		}
	}

	step1Duration := time.Since(step1Start)
	logger.Debug("expandAllFromAllItems: Step 1 (BFS)",
		"duration", step1Duration, "paths_to_expand", len(toExpand), "iterations", bfsIterations)

	// Step 2: Mark all paths as expanded in state
	step2Start := time.Now()
	for path := range toExpand {
		m.state.SetExpanded(path, true)
	}
	step2Duration := time.Since(step2Start)
	logger.Debug("expandAllFromAllItems: Step 2 (mark expanded)", "duration", step2Duration)

	// Step 3: Rebuild the entire visible tree in ONE operation
	step3Start := time.Now()
	currentItems := m.state.Items()
	newVisibleItems := make([]Item, 0, len(allItems)/2) // Estimate capacity

	// Helper to recursively add items and their visible descendants
	recursiveCalls := 0
	var addItemAndDescendants func(item Item)
	addItemAndDescendants = func(item Item) {
		recursiveCalls++
		// Mark as expanded if in toExpand set
		if toExpand[item.Path] {
			item.Expanded = true
		}
		newVisibleItems = append(newVisibleItems, item)

		// If this item is expanded, add its children recursively
		if item.Expanded {
			// Use O(1) map lookup instead of O(n) scan through allItems
			for _, child := range childrenMap[item.Path] {
				addItemAndDescendants(child)
			}
		}
	}

	// Process each root-level item from current visible items
	for _, item := range currentItems {
		// Only process items that aren't descendants of anything else in currentItems
		// (i.e., find the roots of the current visible tree)
		isRoot := true
		for _, otherItem := range currentItems {
			if item.Path != otherItem.Path && strings.HasPrefix(item.Path, otherItem.Path+"\\") {
				isRoot = false
				break
			}
		}

		if isRoot {
			addItemAndDescendants(item)
		}
	}

	step3Duration := time.Since(step3Start)
	logger.Debug("expandAllFromAllItems: Step 3 (rebuild tree)",
		"duration", step3Duration, "recursive_calls", recursiveCalls, "items_built", len(newVisibleItems))

	// Step 4: Set the new items list ONCE
	step4Start := time.Now()
	m.state.SetItems(newVisibleItems)
	step4Duration := time.Since(step4Start)
	logger.Debug("expandAllFromAllItems: Step 4 (set items)", "duration", step4Duration)

	// Update viewport
	step5Start := time.Now()
	m.updateViewport()
	step5Duration := time.Since(step5Start)
	logger.Debug("expandAllFromAllItems: Step 5 (updateViewport)", "duration", step5Duration)

	totalDuration := time.Since(startTime)
	logger.Debug("expandAllFromAllItems: TOTAL",
		"duration", totalDuration, "paths_expanded", len(toExpand), "visible_items", m.state.ItemCount())
}

// expandFromDiffMap expands a key using children from the diffMap
func (m *Model) expandFromDiffMap(item *Item) {
	if m.state.IsLoaded(item.Path) {
		// Already loaded, just expand
		items := m.state.Items()
		for i := range items {
			if items[i].Path == item.Path {
				items[i].Expanded = true
				m.state.SetExpanded(item.Path, true)
				m.state.SetItems(items)
				break
			}
		}
		return
	}

	logger.Debug("expandFromDiffMap: expanding from diffMap", "path", item.Path)

	// Find all direct children of this key in the diffMap
	children := make([]Item, 0)
	expectedPrefix := item.Path + "\\"

	for path, keyDiff := range m.state.diffMap {
		// Check if this is a direct child (not a grandchild)
		if !strings.HasPrefix(path, expectedPrefix) {
			continue
		}

		// Make sure it's a direct child by checking there are no more separators after the prefix
		remainder := path[len(expectedPrefix):]
		if strings.Contains(remainder, "\\") {
			continue
		}

		// This is a direct child - look up both NodeIDs
		oldNodeID, newNodeID, err := m.state.GetNodeIDsForDiffKey(keyDiff.Path, keyDiff.Status)
		if err != nil {
			// Failed to get NodeIDs - log and skip this child
			fmt.Fprintf(
				os.Stderr,
				"[DEBUG] expandFromDiffMap: failed to get NodeIDs for %q (status=%d): %v\n",
				keyDiff.Path,
				keyDiff.Status,
				err,
			)
			continue
		}

		child := Item{
			OldNodeID:   oldNodeID,
			NewNodeID:   newNodeID,
			Path:        keyDiff.Path,
			Name:        keyDiff.Name,
			Depth:       item.Depth + 1,
			HasChildren: keyDiff.SubkeyN > 0,
			SubkeyCount: keyDiff.SubkeyN,
			ValueCount:  keyDiff.ValueN,
			LastWrite:   keyDiff.LastWrite,
			Expanded:    false,
			Parent:      item.Path,
			DiffStatus:  keyDiff.Status,
		}
		children = append(children, child)
	}

	fmt.Fprintf(
		os.Stderr,
		"[DEBUG] expandFromDiffMap: found %d children for %q\n",
		len(children),
		item.Path,
	)

	// Sort children by name
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})

	// Find parent index in current items
	items := m.state.Items()
	parentIdx := -1
	for i := range items {
		if items[i].Path == item.Path {
			parentIdx = i
			break
		}
	}

	if parentIdx >= 0 {
		// Mark parent as expanded and loaded
		items[parentIdx].Expanded = true
		m.state.SetExpanded(item.Path, true)
		m.state.SetLoaded(item.Path, true)

		// Insert children after parent
		newItems := make([]Item, 0, len(items)+len(children))
		newItems = append(newItems, items[:parentIdx+1]...)
		newItems = append(newItems, children...)
		newItems = append(newItems, items[parentIdx+1:]...)
		m.state.SetItems(newItems)

		// Adjust cursor if needed
		if m.nav.Cursor() > parentIdx {
			m.nav.SetCursor(m.nav.Cursor() + len(children))
		}
	}

	m.updateViewport()
}

// CollapseAll collapses all expanded items in the tree
func (m *Model) CollapseAll() {
	// Collapse all items by removing all but root-level items
	items := m.state.Items()
	newItems := make([]Item, 0)

	for i := range items {
		item := &items[i]
		// Keep root-level items (depth 0)
		if item.Depth == 0 {
			item.Expanded = false
			m.state.SetExpanded(item.Path, false)
			newItems = append(newItems, *item)
		}
	}

	// Clear all loaded flags except root
	m.state.loaded = make(map[string]bool)
	m.state.SetLoaded("", true)

	m.state.SetItems(newItems)

	// Adjust cursor
	cursor := m.nav.Cursor()
	if cursor >= len(newItems) {
		if len(newItems) > 0 {
			cursor = len(newItems) - 1
		} else {
			cursor = 0
		}
		m.nav.SetCursor(cursor)
	}

	m.updateViewport()
}

// ExpandCurrentLevel expands all siblings at the current item's depth
func (m *Model) ExpandCurrentLevel() tea.Cmd {
	item := m.CurrentItem()
	if item == nil {
		return nil
	}

	currentDepth := item.Depth

	// Find all items at the same depth
	items := m.state.Items()
	for i := range items {
		levelItem := &items[i]
		if levelItem.Depth == currentDepth && levelItem.HasChildren && !levelItem.Expanded {
			// Save cursor
			oldCursor := m.nav.Cursor()
			m.nav.SetCursor(i)

			// Expand this item
			if len(m.state.diffMap) > 0 {
				m.expandFromDiffMap(levelItem)
			} else if m.state.IsLoaded(levelItem.Path) {
				items := m.state.Items()
				items[i].Expanded = true
				m.state.SetExpanded(levelItem.Path, true)
				m.state.SetItems(items)
			}

			// Restore cursor
			m.nav.SetCursor(oldCursor)
		}
	}

	m.updateViewport()
	return nil
}

// CollapseToCurrentLevel collapses all items deeper than the current level
func (m *Model) CollapseToCurrentLevel() {
	item := m.CurrentItem()
	if item == nil {
		return
	}

	currentDepth := item.Depth

	// Remove all items deeper than current level
	items := m.state.Items()
	newItems := make([]Item, 0)

	for i := range items {
		levelItem := &items[i]
		if levelItem.Depth <= currentDepth {
			// Keep this item
			if levelItem.Depth == currentDepth {
				// Collapse it if it's at current depth
				levelItem.Expanded = false
				m.state.SetExpanded(levelItem.Path, false)
			}
			newItems = append(newItems, *levelItem)
		} else {
			// Remove deeper items and clear their state
			m.state.SetExpanded(levelItem.Path, false)
			m.state.SetLoaded(levelItem.Path, false)
		}
	}

	m.state.SetItems(newItems)

	// Adjust cursor
	cursor := m.nav.Cursor()
	if cursor >= len(newItems) && len(newItems) > 0 {
		m.nav.SetCursor(len(newItems) - 1)
	}

	m.updateViewport()
}

// CopyCurrentPath copies the current key path to clipboard
func (m *Model) CopyCurrentPath() error {
	item := m.CurrentItem()
	if item == nil {
		return fmt.Errorf("no item selected")
	}

	return clipboard.WriteAll(item.Path)
}

// ExpandParents expands all parent keys of a given path to make it visible
func (m *Model) ExpandParents(targetPath string) tea.Cmd {
	logger.Debug("ExpandParents: expanding parents", "target", targetPath)

	// Build list of parent paths from root to target
	var parents []string
	parts := strings.Split(targetPath, "\\")
	for i := 1; i < len(parts); i++ {
		parentPath := strings.Join(parts[:i], "\\")
		parents = append(parents, parentPath)
	}

	logger.Debug("ExpandParents: parent paths", "paths", parents)

	// Expand each parent from root to target
	var cmds []tea.Cmd
	for _, parentPath := range parents {
		// Find the parent in the tree
		items := m.state.Items()
		for i := range items {
			if items[i].Path == parentPath {
				logger.Debug("ExpandParents: found parent",
					"path", parentPath, "index", i,
					"expanded", items[i].Expanded, "loaded", m.state.IsLoaded(parentPath))

				// Check if children are actually visible (verify expanded state is consistent)
				childrenVisible := false
				if i+1 < len(items) && strings.HasPrefix(items[i+1].Path, parentPath+"\\") {
					childrenVisible = true
				}

				// If not already expanded OR children not visible (inconsistent state), expand it
				if !items[i].Expanded || !childrenVisible {
					if !childrenVisible && items[i].Expanded {
						logger.Warn("ExpandParents: parent marked as expanded but children not visible")
					}
					// Check if children are already loaded (from previous expansion)
					if m.state.IsLoaded(parentPath) {
						// Synchronously re-insert children that were hidden by collapse
						logger.Debug("ExpandParents: re-expanding loaded children")
						m.expandLoadedChildren(parentPath)
					} else {
						// Async load needed
						logger.Debug("ExpandParents: async loading children")
						oldCursor := m.nav.Cursor()
						m.nav.SetCursor(i)
						cmd := m.Expand()
						m.nav.SetCursor(oldCursor)

						if cmd != nil {
							cmds = append(cmds, cmd)
						}
					}
				} else {
					logger.Debug("ExpandParents: children already visible, skipping")
				}
				break
			}
		}
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// expandLoadedChildren re-inserts children that were previously loaded but hidden by collapse
func (m *Model) expandLoadedChildren(parentPath string) {
	logger.Debug("expandLoadedChildren: re-expanding", "path", parentPath)

	// Find parent index
	items := m.state.Items()
	parentIdx := -1
	for i := range items {
		if items[i].Path == parentPath {
			parentIdx = i
			break
		}
	}

	if parentIdx == -1 {
		return
	}

	// Mark as expanded
	items[parentIdx].Expanded = true
	m.state.SetExpanded(parentPath, true)
	m.state.SetItems(items)

	// Get cached children
	children := m.state.GetChildren(parentPath)
	if len(children) == 0 {
		logger.Debug("expandLoadedChildren: no cached children found", "path", parentPath)
		return
	}
	logger.Debug("expandLoadedChildren: found cached children", "count", len(children))

	// In diff mode, use diffMap to get children
	if len(m.state.diffMap) > 0 {
		// Find children in diffMap (parent is determined by path prefix)
		var childItems []Item
		for path, keyDiff := range m.state.diffMap {
			// Check if this is a direct child of parentPath
			if isDirectChild(path, parentPath) {
				// Look up both NodeIDs for this child
				oldNodeID, newNodeID, err := m.state.GetNodeIDsForDiffKey(path, keyDiff.Status)
				if err != nil {
					// Failed to get NodeIDs - log and skip this child
					fmt.Fprintf(
						os.Stderr,
						"[DEBUG] expandLoadedChildren: failed to get NodeIDs for %q (status=%d): %v\n",
						path,
						keyDiff.Status,
						err,
					)
					continue
				}

				childItems = append(childItems, Item{
					OldNodeID:   oldNodeID,
					NewNodeID:   newNodeID,
					Path:        path,
					Name:        keyDiff.Name,
					Depth:       items[parentIdx].Depth + 1,
					HasChildren: keyDiff.SubkeyN > 0,
					SubkeyCount: keyDiff.SubkeyN,
					ValueCount:  keyDiff.ValueN,
					LastWrite:   keyDiff.LastWrite,
					Expanded:    m.state.IsExpanded(path),
					Parent:      parentPath,
					DiffStatus:  keyDiff.Status,
				})
			}
		}

		// Sort children
		sort.Slice(childItems, func(i, j int) bool {
			return strings.ToLower(childItems[i].Name) < strings.ToLower(childItems[j].Name)
		})

		// Insert children after parent
		items = m.state.Items()
		newItems := make([]Item, 0, len(items)+len(childItems))
		newItems = append(newItems, items[:parentIdx+1]...)
		newItems = append(newItems, childItems...)
		newItems = append(newItems, items[parentIdx+1:]...)
		m.state.SetItems(newItems)

		fmt.Fprintf(
			os.Stderr,
			"[DEBUG] expandLoadedChildren: inserted %d children from diffMap\n",
			len(childItems),
		)
	} else {
		// Normal mode - insert cached children
		items = m.state.Items()
		newItems := make([]Item, 0, len(items)+len(children))
		newItems = append(newItems, items[:parentIdx+1]...)
		newItems = append(newItems, children...)
		newItems = append(newItems, items[parentIdx+1:]...)
		m.state.SetItems(newItems)

		logger.Debug("expandLoadedChildren: inserted cached children", "count", len(children))
	}

	m.updateViewport()
}

// isDirectChild checks if childPath is a direct child of parentPath
func isDirectChild(childPath, parentPath string) bool {
	if parentPath == "" {
		// Root level - check if no backslash in path
		return !strings.Contains(childPath, "\\")
	}

	// Check if childPath starts with parentPath + "\"
	prefix := parentPath + "\\"
	if !strings.HasPrefix(childPath, prefix) {
		return false
	}

	// Check that there's no additional backslash after the prefix
	remainder := childPath[len(prefix):]
	return !strings.Contains(remainder, "\\")
}

// NavigateToPath expands all parents and navigates to the given path
func (m *Model) NavigateToPath(targetPath string) tea.Cmd {
	logger.Debug("NavigateToPath: navigating", "target", targetPath)

	// First, expand all parents
	cmd := m.ExpandParents(targetPath)

	// Try to find and navigate to the target immediately
	items := m.state.Items()
	logger.Debug("NavigateToPath: searching items for target", "item_count", len(items))
	found := false
	for i := range items {
		if items[i].Path == targetPath {
			m.MoveTo(i)
			logger.Debug("NavigateToPath: found target", "index", i)
			found = true
			break
		}
	}

	if !found {
		logger.Debug("NavigateToPath: target NOT FOUND in items list", "has_cmd", cmd != nil)
	}

	// If we have async commands and didn't find the target,
	// store it for completion after loading
	if cmd != nil && !found {
		m.nav.SetPendingNavigationTarget(targetPath)
		logger.Debug("NavigateToPath: storing pending navigation", "target", targetPath)
	} else if cmd == nil && !found {
		logger.Warn("NavigateToPath: target not found and no async cmd, navigation will fail!")
	}

	return cmd
}

// Expose internal state for tui package

// SetKeys sets the keyboard shortcuts
func (m *Model) SetKeys(keys Keys) {
	m.keys = keys
}

// SetBookmarks sets the bookmarks map
func (m *Model) SetBookmarks(bookmarks map[string]bool) {
	m.bookmarks = bookmarks
	m.state.SetBookmarks(bookmarks)
}

// SetSearchFilter sets the search filter and updates the visible items accordingly.
// If query is less than 3 characters, filtering is disabled and all items are shown.
// Otherwise, only matching items and their parent paths are visible.
func (m *Model) SetSearchFilter(query string) {
	// Update the filter in state
	m.state.SetSearchFilter(query)

	// Apply filtering to get filtered items
	filteredItems := m.state.FilterItemsWithParents(query)

	// Update visible items
	m.state.SetItems(filteredItems)

	// Reset cursor to first item if we have filtered results
	if len(filteredItems) > 0 && m.nav.Cursor() >= len(filteredItems) {
		m.nav.SetCursor(0)
		if m.renderer != nil {
			m.renderer.SetCursor(0)
		}
	}

	// Update viewport to reflect changes
	m.updateViewport()

	// Emit navigation signal so value table loads values for the current item
	// This ensures values are displayed when filter is applied or cleared
	if len(filteredItems) > 0 {
		m.CursorManager.EmitSignal()
	}
}

// FilterByPaths filters the tree to show only the specified paths and their parents.
// This is used for global value search where we have exact paths to match.
// Pass an empty slice to clear the filter.
func (m *Model) FilterByPaths(paths []string) {
	// Apply filtering to get filtered items
	filteredItems := m.state.FilterByPathsWithParents(paths)

	// Update visible items
	m.state.SetItems(filteredItems)

	// Reset cursor to first item if we have filtered results
	if len(filteredItems) > 0 && m.nav.Cursor() >= len(filteredItems) {
		m.nav.SetCursor(0)
		if m.renderer != nil {
			m.renderer.SetCursor(0)
		}
	}

	// Update viewport to reflect changes
	m.updateViewport()

	// Emit navigation signal so value table loads values for the current item
	if len(filteredItems) > 0 {
		m.CursorManager.EmitSignal()
	}
}

// SetDiffMap sets the diff map
func (m *Model) SetDiffMap(diffMap map[string]hive.KeyDiff) {
	m.state.SetDiffMap(diffMap)
}

// SetDiffReaders sets the diff readers for NodeID lookups
func (m *Model) SetDiffReaders(oldReader, newReader hive.Reader) {
	m.state.SetDiffReaders(oldReader, newReader)
}

// ClearDiffReaders clears the diff readers
func (m *Model) ClearDiffReaders() {
	m.state.ClearDiffReaders()
}

// SetWidth sets the width
func (m *Model) SetWidth(width int) {
	// Initialize renderer if needed
	if m.renderer == nil {
		m.renderer = virtuallist.New(m)
	}
	m.renderer.SetSize(width, m.renderer.Height())
}

// SetHeight sets the height
func (m *Model) SetHeight(height int) {
	// Initialize renderer if needed
	if m.renderer == nil {
		m.renderer = virtuallist.New(m)
	}
	m.renderer.SetSize(m.renderer.Width(), height)
}

// GetCursor returns the current cursor position
func (m *Model) GetCursor() int {
	return m.nav.Cursor()
}

// SetCursor sets the cursor position
func (m *Model) SetCursor(cursor int) {
	m.nav.SetCursor(cursor)
}

// GetItems returns the current items
func (m *Model) GetItems() []Item {
	return m.state.Items()
}

// SetItems sets the items
func (m *Model) SetItems(items []Item) {
	m.state.SetItems(items)
}

// GetViewport returns the viewport
func (m *Model) GetViewport() *viewport.Model {
	if m.renderer != nil {
		return m.renderer.Viewport()
	}
	return nil
}

// GetRendererCursor returns the cursor position in the renderer (for testing).
// This is the actual visual cursor position that's rendered on screen.
func (m *Model) GetRendererCursor() int {
	if m.renderer != nil {
		cursor := m.renderer.Cursor()
		logger.Debug("GetRendererCursor",
			"renderer", fmt.Sprintf("%p", m.renderer), "cursor", cursor)
		return cursor
	}
	logger.Warn("GetRendererCursor: renderer is nil")
	return 0
}

// GetLoaded returns the loaded map
func (m *Model) GetLoaded() map[string]bool {
	return m.state.loaded
}

// SetLoaded sets the loaded map
func (m *Model) SetLoaded(loaded map[string]bool) {
	m.state.loaded = loaded
}

// GetExpanded returns the expanded map
func (m *Model) GetExpanded() map[string]bool {
	return m.state.expanded
}

// SetExpanded sets the expanded map
func (m *Model) SetExpanded(expanded map[string]bool) {
	m.state.expanded = expanded
}

// SetExpandedPath sets the expanded state for a specific path
func (m *Model) SetExpandedPath(path string, expanded bool) {
	m.state.SetExpanded(path, expanded)
}

// SetLoadedPath sets the loaded state for a specific path
func (m *Model) SetLoadedPath(path string, loaded bool) {
	m.state.SetLoaded(path, loaded)
}

// UpdateViewport updates the viewport content (exposed for testing)
func (m *Model) UpdateViewport() {
	m.updateViewport()
}

// ItemCount implements virtuallist.VirtualList
func (m *Model) ItemCount() int {
	return m.state.ItemCount()
}

// RenderItem implements virtuallist.VirtualList
// Renders a single tree item at the given index
func (m *Model) RenderItem(index int, isCursor bool, width int) string {
	items := m.state.Items()
	if index < 0 || index >= len(items) {
		return ""
	}

	item := items[index]

	// Check if item is bookmarked
	isBookmarked := m.state.bookmarks != nil && m.state.bookmarks[item.Path]

	// Convert domain Item to display-ready structure
	// This is the adapter layer: domain → display props
	source := adapter.TreeItemSource{
		Name:        item.Name,
		Depth:       item.Depth,
		HasChildren: item.HasChildren,
		Expanded:    item.Expanded,
		SubkeyCount: item.SubkeyCount,
		DiffStatus:  item.DiffStatus,
	}

	// Format timestamp
	if !item.LastWrite.IsZero() {
		source.LastWrite = item.LastWrite.Format("2006-01-02 15:04")
	}

	// Adapter converts domain data to display props
	displayProps := adapter.ItemToDisplayProps(source, isBookmarked, isCursor)

	// Pure display function renders the props
	return display.RenderTreeItemDisplay(displayProps, width)
}

// SetStateForTesting allows tests to inject a TreeState for testing purposes.
// This is only used by e2e tests and should not be called in production code.
func (m *Model) SetStateForTesting(state *TreeState) {
	m.state = state
}

// Reader returns the hive reader (for global value search)
func (m *Model) Reader() hive.Reader {
	if m.reader == nil {
		return nil
	}
	r, ok := m.reader.(hive.Reader)
	if !ok {
		return nil
	}
	return r
}

// AllItems returns all items in the tree (for global value search)
func (m *Model) AllItems() []Item {
	return m.state.AllItems()
}

// GetExpandedKeys returns a copy of currently expanded keys
func (m *Model) GetExpandedKeys() map[string]bool {
	return m.state.GetExpandedKeys()
}

// RestoreExpandedKeys restores expanded state and re-expands all keys
func (m *Model) RestoreExpandedKeys(expandedKeys map[string]bool) {
	if len(expandedKeys) == 0 {
		return
	}

	fmt.Fprintf(
		os.Stderr,
		"[DEBUG] RestoreExpandedKeys: restoring %d expanded keys\n",
		len(expandedKeys),
	)

	// Set expanded state
	m.state.SetExpandedKeys(expandedKeys)

	// Re-expand each key to make children visible
	for path := range expandedKeys {
		// Find the key in visible items
		items := m.state.Items()
		for i := range items {
			if items[i].Path == path && m.state.IsLoaded(path) {
				// Re-expand loaded children
				m.expandLoadedChildren(path)
				break
			}
		}
	}

	logger.Debug("RestoreExpandedKeys: completed restoration")
}
