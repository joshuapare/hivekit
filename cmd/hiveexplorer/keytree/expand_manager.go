package keytree

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

	// In diff mode, load children from diffMap
	if _, hasDiff := em.state.GetDiff(item.Path); hasDiff {
		return em.expandFromDiffMap(em.nav.Cursor())
	}

	// If we have allItems loaded (upfront tree load), use it
	allItems := em.state.AllItems()
	if len(allItems) > 0 {
		return em.expandFromAllItems(em.nav.Cursor())
	}

	// Fallback: Lazy loading
	if !em.state.IsLoaded(item.Path) {
		fmt.Fprintf(os.Stderr, "[DEBUG] Expand: loading children for %q\n", item.Path)
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

		fmt.Fprintf(os.Stderr, "[DEBUG] Expand: inserted %d children for %q from allItems\n", len(children), item.Path)
	}

	return nil
}

// expandFromDiffMap expands an item using diff map data
func (em *ExpandManager) expandFromDiffMap(cursorPos int) tea.Cmd {
	items := em.state.Items()
	if cursorPos >= len(items) {
		return nil
	}

	item := &items[cursorPos]

	if em.state.IsLoaded(item.Path) {
		// Already loaded, just expand
		items[cursorPos].Expanded = true
		em.state.SetExpanded(item.Path, true)
		em.state.SetItems(items)
		return nil
	}

	fmt.Fprintf(os.Stderr, "[DEBUG] expandFromDiffMap: expanding %q from diffMap\n", item.Path)

	// Find all direct children in the diffMap
	children := make([]Item, 0)
	expectedPrefix := item.Path + "\\"

	// Get all diff data
	diffMap := em.state.diffMap // Direct access for iteration
	for path, keyDiff := range diffMap {
		if !strings.HasPrefix(path, expectedPrefix) {
			continue
		}

		remainder := path[len(expectedPrefix):]
		if strings.Contains(remainder, "\\") {
			continue // Not a direct child
		}

		// Look up NodeIDs for this child based on its DiffStatus
		oldNodeID, newNodeID, err := em.state.GetNodeIDsForDiffKey(keyDiff.Path, keyDiff.Status)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[DEBUG] expandFromDiffMap: failed to get NodeIDs for %q (status=%d): %v\n",
				keyDiff.Path, keyDiff.Status, err)
			continue // Skip this child if we can't get its NodeIDs
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

	fmt.Fprintf(os.Stderr, "[DEBUG] expandFromDiffMap: found %d children for %q\n", len(children), item.Path)

	// Sort children by name
	sort.Slice(children, func(i, j int) bool {
		return children[i].Name < children[j].Name
	})

	// Mark as expanded and loaded
	items[cursorPos].Expanded = true
	em.state.SetExpanded(item.Path, true)
	em.state.SetLoaded(item.Path, true)

	// Insert children after parent
	newItems := make([]Item, 0, len(items)+len(children))
	newItems = append(newItems, items[:cursorPos+1]...)
	newItems = append(newItems, children...)
	newItems = append(newItems, items[cursorPos+1:]...)
	em.state.SetItems(newItems)

	// Adjust cursor if needed
	if em.nav.Cursor() > cursorPos {
		em.nav.SetCursor(em.nav.Cursor() + len(children))
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
	fmt.Fprintf(os.Stderr, "[TIMING] CollapseAt: START collapsing path=%q at cursor=%d (total items=%d)\n", item.Path, cursorPos, len(items))

	if !item.Expanded {
		// If already collapsed, move to parent
		if item.Parent != "" {
			em.MoveToParent()
		}
		fmt.Fprintf(os.Stderr, "[TIMING] CollapseAt: Item already collapsed, moved to parent\n")
		return
	}

	markStart := time.Now()
	items[cursorPos].Expanded = false
	em.state.SetExpanded(item.Path, false)
	em.state.SetLoaded(item.Path, false)
	em.state.SetItems(items)
	markDuration := time.Since(markStart)
	fmt.Fprintf(os.Stderr, "[TIMING] CollapseAt: Mark as collapsed took %v\n", markDuration)

	// Remove all children from view
	removeStart := time.Now()
	em.removeChildren(item.Path)
	removeDuration := time.Since(removeStart)

	totalDuration := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[TIMING] CollapseAt: TOTAL took %v (mark=%v, removeChildren=%v)\n",
		totalDuration, markDuration, removeDuration)
}

// Collapse collapses the current item
func (em *ExpandManager) Collapse() {
	em.CollapseAt(em.nav.Cursor())
}

// removeChildren removes all children of a key from the visible items
func (em *ExpandManager) removeChildren(parentPath string) {
	startTime := time.Now()
	items := em.state.Items()
	fmt.Fprintf(os.Stderr, "[TIMING] removeChildren: START for path=%q, %d total items\n", parentPath, len(items))

	// Clear loaded/expanded state for all descendants in ONE pass (not per-item)
	clearStart := time.Now()
	em.state.ClearLoadedDescendants(parentPath)
	clearDuration := time.Since(clearStart)
	fmt.Fprintf(os.Stderr, "[TIMING] removeChildren: ClearLoadedDescendants (single call) took %v\n", clearDuration)

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
	fmt.Fprintf(os.Stderr, "[TIMING] removeChildren: Item iteration (filtering) took %v, removed %d items\n",
		iterationDuration, removedCount)

	setItemsStart := time.Now()
	em.state.SetItems(newItems)
	setItemsDuration := time.Since(setItemsStart)
	fmt.Fprintf(os.Stderr, "[TIMING] removeChildren: SetItems took %v (new count: %d)\n", setItemsDuration, len(newItems))

	// Adjust cursor if it's out of bounds
	if em.nav.Cursor() >= len(newItems) && len(newItems) > 0 {
		em.nav.SetCursor(len(newItems) - 1)
	}

	totalDuration := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "[TIMING] removeChildren: TOTAL took %v (removed %d items, 1 ClearLoadedDescendants call)\n",
		totalDuration, removedCount)
}

// clearLoadedRecursive clears the loaded flag for a path and all its descendants
func (em *ExpandManager) clearLoadedRecursive(path string) {
	fmt.Fprintf(os.Stderr, "[DEBUG] clearLoadedRecursive: clearing loaded flag for %q and descendants\n", path)
	em.state.ClearLoadedDescendants(path)
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
