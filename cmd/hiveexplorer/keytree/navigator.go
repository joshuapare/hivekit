package keytree

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/viewport"
)

// Navigator manages cursor position and navigation within the tree.
type Navigator struct {
	cursor                  int
	pendingNavigationTarget string // Path to navigate to after async loading completes
}

// NewNavigator creates a new navigator
func NewNavigator() *Navigator {
	return &Navigator{
		cursor: 0,
	}
}

// Cursor returns the current cursor position
func (n *Navigator) Cursor() int {
	return n.cursor
}

// SetCursor sets the cursor position
func (n *Navigator) SetCursor(pos int) {
	n.cursor = pos
}

// MoveUp moves the cursor up if possible
func (n *Navigator) MoveUp() bool {
	if n.cursor > 0 {
		n.cursor--
		return true
	}
	return false
}

// MoveDown moves the cursor down if possible, returns true if moved
func (n *Navigator) MoveDown(maxItems int) bool {
	if n.cursor < maxItems-1 {
		n.cursor++
		return true
	}
	return false
}

// PendingNavigationTarget returns the pending navigation target path
func (n *Navigator) PendingNavigationTarget() string {
	return n.pendingNavigationTarget
}

// SetPendingNavigationTarget sets the pending navigation target
func (n *Navigator) SetPendingNavigationTarget(path string) {
	n.pendingNavigationTarget = path
}

// ClearPendingNavigationTarget clears the pending navigation target
func (n *Navigator) ClearPendingNavigationTarget() {
	n.pendingNavigationTarget = ""
}

// EnsureCursorVisible scrolls the viewport to make the cursor visible.
// Returns true if the viewport was scrolled.
func (n *Navigator) EnsureCursorVisible(vp *viewport.Model, headerHeight int, itemCount int) bool {
	visibleHeight := vp.Height - headerHeight
	if visibleHeight <= 0 {
		return false
	}

	oldYOffset := vp.YOffset
	scrolled := false
	if n.cursor < vp.YOffset {
		vp.YOffset = n.cursor
		scrolled = true
		fmt.Fprintf(os.Stderr, "[NAVIGATOR] Scrolled UP: cursor=%d, YOffset %d -> %d\n", n.cursor, oldYOffset, vp.YOffset)
	} else if n.cursor >= vp.YOffset+visibleHeight {
		vp.YOffset = n.cursor - visibleHeight + 1
		scrolled = true
		fmt.Fprintf(os.Stderr, "[NAVIGATOR] Scrolled DOWN: cursor=%d, YOffset %d -> %d (visibleHeight=%d)\n", n.cursor, oldYOffset, vp.YOffset, visibleHeight)
	}

	// Clamp YOffset to valid range [0, itemCount - visibleHeight]
	// This prevents scrolling past the end of the list
	maxYOffset := itemCount - visibleHeight
	if maxYOffset < 0 {
		maxYOffset = 0
	}

	beforeClamp := vp.YOffset
	if vp.YOffset < 0 {
		vp.YOffset = 0
		scrolled = true
		fmt.Fprintf(os.Stderr, "[NAVIGATOR] Clamped YOffset to 0 (was %d)\n", beforeClamp)
	}
	if vp.YOffset > maxYOffset {
		fmt.Fprintf(os.Stderr, "[NAVIGATOR] Clamping YOffset: %d -> %d (maxYOffset=%d, itemCount=%d, visibleHeight=%d)\n",
			vp.YOffset, maxYOffset, maxYOffset, itemCount, visibleHeight)
		vp.YOffset = maxYOffset
		scrolled = true
	}

	return scrolled
}
