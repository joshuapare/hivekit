package virtuallist

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// VirtualList is the interface that must be implemented by components
// that want to use virtual scrolling for large lists.
type VirtualList interface {
	// ItemCount returns the total number of items in the list
	ItemCount() int

	// RenderItem renders a single item at the given index.
	// isCursor indicates whether this item is currently selected.
	// width is the available width for rendering.
	RenderItem(index int, isCursor bool, width int) string
}

// Renderer manages viewport and virtual scrolling for large lists.
// It only renders items that are actually visible on screen, making
// navigation performance constant regardless of total item count.
type Renderer struct {
	list         VirtualList
	viewport     viewport.Model
	cursor       int
	width        int
	height       int
	scrollOffset int // Track scroll position independently of viewport.YOffset
}

// New creates a new virtual list renderer
func New(list VirtualList) *Renderer {
	vp := viewport.New(0, 0)
	return &Renderer{
		list:     list,
		viewport: vp,
		cursor:   0,
		width:    0,
		height:   0,
	}
}

// SetSize updates the renderer size
func (r *Renderer) SetSize(width, height int) {
	r.width = width
	r.height = height
	r.viewport.Width = width
	r.viewport.Height = height
}

// SetCursor updates the cursor position and scrolls viewport if needed
func (r *Renderer) SetCursor(cursor int) {
	r.cursor = cursor
	r.ensureCursorVisible()
}

// Cursor returns the current cursor position
func (r *Renderer) Cursor() int {
	return r.cursor
}

// Update updates the viewport with a message
// NOTE: We only forward WindowSizeMsg to viewport, not keyboard messages.
// Keyboard navigation is handled by the parent (which calls SetCursor),
// and we handle scrolling in ensureCursorVisible(). If we forward keyboard
// messages here, viewport will ALSO scroll, causing double-scrolling bugs.
func (r *Renderer) Update(msg tea.Msg) tea.Cmd {
	// Only forward WindowSizeMsg to viewport
	// Do NOT forward keyboard messages - that causes double scrolling
	switch msg.(type) {
	case tea.WindowSizeMsg:
		var cmd tea.Cmd
		r.viewport, cmd = r.viewport.Update(msg)
		return cmd
	}
	return nil
}

// View renders the visible portion of the list
func (r *Renderer) View() string {
	itemCount := r.list.ItemCount()

	if itemCount == 0 {
		return "Loading..."
	}

	// Calculate visible range
	visibleHeight := r.height
	if visibleHeight <= 0 {
		visibleHeight = 20 // Fallback during initialization
	}

	// Calculate which items are actually visible
	// We use scrollOffset to track scroll position (NOT viewport.YOffset)
	start := r.scrollOffset
	end := start + visibleHeight

	// Clamp end to valid range
	if end > itemCount {
		end = itemCount
	}

	// IMPORTANT: If end was clamped, adjust start backwards to maintain
	// full visibleHeight worth of items (when possible).
	// This prevents the list from shrinking when at the bottom.
	if end == itemCount && end-start < visibleHeight {
		start = end - visibleHeight
		if start < 0 {
			start = 0
		}
		// Update scrollOffset to match the adjusted start
		r.scrollOffset = start
	}

	// Clamp start to valid range
	if start < 0 {
		start = 0
	}

	// Build content for only visible items
	var b strings.Builder
	linesBuilt := 0
	for i := start; i < end; i++ {
		line := r.list.RenderItem(i, i == r.cursor, r.width)
		b.WriteString(line)
		linesBuilt++
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Set content and return view
	// For virtual scrolling, we build ONLY the visible content, so viewport's
	// YOffset should always be 0 for rendering (we've already calculated which items to show)
	r.viewport.SetContent(b.String())
	r.viewport.YOffset = 0 // CRITICAL: Must be 0 for viewport to show the content we gave it

	result := r.viewport.View()

	// AFTER rendering, set viewport.YOffset to scrollOffset for external code that reads it
	// This allows tests and other code to check the scroll position
	r.viewport.YOffset = r.scrollOffset

	return result
}

// ensureCursorVisible scrolls viewport to make cursor visible
func (r *Renderer) ensureCursorVisible() {
	visibleHeight := r.height
	if visibleHeight <= 0 {
		return
	}

	itemCount := r.list.ItemCount()

	// Scroll up if cursor is above visible area
	if r.cursor < r.scrollOffset {
		r.scrollOffset = r.cursor
	}

	// Scroll down if cursor is below visible area
	if r.cursor >= r.scrollOffset+visibleHeight {
		r.scrollOffset = r.cursor - visibleHeight + 1
	}

	// Ensure scrollOffset is clamped to valid range [0, itemCount - visibleHeight]
	// This prevents scrolling past the end of the list
	maxOffset := itemCount - visibleHeight
	if maxOffset < 0 {
		maxOffset = 0
	}

	if r.scrollOffset < 0 {
		r.scrollOffset = 0
	}
	if r.scrollOffset > maxOffset {
		r.scrollOffset = maxOffset
	}
}

// Width returns the current width
func (r *Renderer) Width() int {
	return r.width
}

// Height returns the current height
func (r *Renderer) Height() int {
	return r.height
}

// Viewport returns the underlying viewport for direct access if needed
func (r *Renderer) Viewport() *viewport.Model {
	return &r.viewport
}
