package keytree

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
)

// ExpandManager handles all expand/collapse operations on the tree.
// It coordinates between TreeState and Navigator to manage tree visibility.
type ExpandManager struct {
	state *TreeState
	nav   *Navigator
}

// NewExpandManager creates a new expand manager
func NewExpandManager(state *TreeState, nav *Navigator) *ExpandManager {
	return &ExpandManager{
		state: state,
		nav:   nav,
	}
}

// Expand expands the current item. Returns a tea.Cmd if children need to be loaded.
func (em *ExpandManager) Expand(loadChildrenCmd func(path string) tea.Cmd) tea.Cmd {
	item := em.state.GetItem(em.nav.Cursor())
	if item == nil || !item.HasChildren {
		return nil
	}

	// If already expanded, toggle to collapsed
	if item.Expanded {
		em.CollapseAt(em.nav.Cursor())
		return nil
	}

	// If we have allItems loaded (upfront tree load), use it
	allItems := em.state.AllItems()
	if len(allItems) > 0 {
		return em.expandFromAllItems(em.nav.Cursor())
	}

	// Fallback: Lazy loading
	if !em.state.IsLoaded(item.Path) {
		logger.Debug("Expand: loading children", "path", item.Path)
		return loadChildrenCmd(item.Path)
	}

	// Children already loaded, just set expanded
	items := em.state.Items()
	items[em.nav.Cursor()].Expanded = true
	em.state.SetExpanded(item.Path, true)
	em.state.SetItems(items)

	return nil
}

// expandFromAllItems expands an item using preloaded allItems
func (em *ExpandManager) expandFromAllItems(cursorPos int) tea.Cmd {
	items := em.state.Items()
	if cursorPos >= len(items) {
		return nil
	}

	item := &items[cursorPos]
	allItems := em.state.AllItems()

	// Find direct children from allItems
	children := make([]Item, 0)
	for _, child := range allItems {
		if child.Parent == item.Path {
			children = append(children, child)
		}
	}

	if len(children) > 0 {
		// Insert children after current item
		newItems := make([]Item, 0, len(items)+len(children))
		newItems = append(newItems, items[:cursorPos+1]...)
		newItems = append(newItems, children...)
		newItems = append(newItems, items[cursorPos+1:]...)

		// Mark as expanded
		newItems[cursorPos].Expanded = true
		em.state.SetItems(newItems)
		em.state.SetExpanded(item.Path, true)

		logger.Debug("Expand: inserted children from allItems", "count", len(children), "path", item.Path)
	}

	return nil
}

// CollapseAt collapses the item at the given cursor position
func (em *ExpandManager) CollapseAt(cursorPos int) {
	startTime := time.Now()
	items := em.state.Items()
	if cursorPos >= len(items) {
		return
	}

	item := &items[cursorPos]
	logger.Debug("CollapseAt: START", "path", item.Path, "cursor", cursorPos, "totalItems", len(items))

	if !item.Expanded {
		// If already collapsed, move to parent
		if item.Parent != "" {
			em.MoveToParent()
		}
		logger.Debug("CollapseAt: Item already collapsed, moved to parent")
		return
	}

	markStart := time.Now()
	items[cursorPos].Expanded = false
	em.state.SetExpanded(item.Path, false)
	em.state.SetLoaded(item.Path, false)
	em.state.SetItems(items)
	markDuration := time.Since(markStart)
	logger.Debug("CollapseAt: Mark as collapsed", "duration", markDuration)

	// Remove all children from view
	removeStart := time.Now()
	em.removeChildren(item.Path)
	removeDuration := time.Since(removeStart)

	totalDuration := time.Since(startTime)
	logger.Debug("CollapseAt: TOTAL", "duration", totalDuration, "markDuration", markDuration, "removeChildrenDuration", removeDuration)
}

// Collapse collapses the current item
func (em *ExpandManager) Collapse() {
	em.CollapseAt(em.nav.Cursor())
}

// removeChildren removes all children of a key from the visible items
func (em *ExpandManager) removeChildren(parentPath string) {
	startTime := time.Now()
	items := em.state.Items()
	logger.Debug("removeChildren: START", "path", parentPath, "totalItems", len(items))

	// Clear loaded/expanded state for all descendants in ONE pass (not per-item)
	clearStart := time.Now()
	em.state.ClearLoadedDescendants(parentPath)
	clearDuration := time.Since(clearStart)
	logger.Debug("removeChildren: ClearLoadedDescendants", "duration", clearDuration)

	// Filter out descendant items from visible list
	iterationStart := time.Now()
	newItems := make([]Item, 0)
	skipUntilDepth := -1
	removedCount := 0

	for _, item := range items {
		if item.Parent == parentPath && skipUntilDepth < 0 {
			skipUntilDepth = item.Depth
			removedCount++
			continue
		}

		if skipUntilDepth >= 0 && item.Depth >= skipUntilDepth {
			removedCount++
			continue
		}

		skipUntilDepth = -1
		newItems = append(newItems, item)
	}
	iterationDuration := time.Since(iterationStart)
	logger.Debug("removeChildren: Item iteration", "duration", iterationDuration, "removedCount", removedCount)

	setItemsStart := time.Now()
	em.state.SetItems(newItems)
	setItemsDuration := time.Since(setItemsStart)
	logger.Debug("removeChildren: SetItems", "duration", setItemsDuration, "newCount", len(newItems))

	// Adjust cursor if it's out of bounds
	if em.nav.Cursor() >= len(newItems) && len(newItems) > 0 {
		em.nav.SetCursor(len(newItems) - 1)
	}

	totalDuration := time.Since(startTime)
	logger.Debug("removeChildren: TOTAL", "duration", totalDuration, "removedCount", removedCount)
}

// RemoveDescendantsFromView removes all descendants of a path from visible items
func (em *ExpandManager) RemoveDescendantsFromView(parentPath string) {
	items := em.state.Items()
	newItems := make([]Item, 0, len(items))

	for _, item := range items {
		// Keep item if it's not a descendant of parentPath
		if !strings.HasPrefix(item.Path, parentPath+"\\") {
			newItems = append(newItems, item)
		}
	}

	em.state.SetItems(newItems)
}

// MoveToParent moves cursor to parent of current item
func (em *ExpandManager) MoveToParent() {
	items := em.state.Items()
	cursorPos := em.nav.Cursor()

	if cursorPos >= len(items) {
		return
	}

	currentItem := items[cursorPos]
	if currentItem.Parent == "" {
		return
	}

	// Find parent
	for i, item := range items {
		if item.Path == currentItem.Parent {
			em.nav.SetCursor(i)
			return
		}
	}
}
