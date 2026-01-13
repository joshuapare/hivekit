package keytree

import (
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keyselection"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/logger"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/virtuallist"
)

// CursorManager handles all cursor movements and navigation signal emissions.
// This is the single source of truth for cursor changes - all navigation methods
// funnel through MoveTo() to ensure signals are consistently emitted.
type CursorManager struct {
	nav      *Navigator
	state    *TreeState
	navBus   *keyselection.Bus
	hivePath string
}

// newCursorManager creates a new cursor manager.
func newCursorManager(nav *Navigator, state *TreeState, hivePath string) *CursorManager {
	return &CursorManager{
		nav:      nav,
		state:    state,
		hivePath: hivePath,
	}
}

// setNavigationBus sets the navigation bus for signal emissions.
func (cm *CursorManager) setNavigationBus(bus *keyselection.Bus) {
	cm.navBus = bus
}

// MoveTo moves the cursor to the specified position and emits a navigation signal.
// This is the single source of truth for all cursor movements.
// Returns true if the cursor was moved, false if the position was invalid.
// The renderer parameter should be the current model's renderer (may be nil during initialization).
func (cm *CursorManager) MoveTo(pos int, renderer *virtuallist.Renderer) bool {
	itemCount := cm.state.ItemCount()

	// Validate position
	if pos < 0 || pos >= itemCount {
		return false
	}

	// Don't move if already at position
	if cm.nav.Cursor() == pos {
		return false
	}

	// Update cursor in navigator
	cm.nav.SetCursor(pos)

	// Update cursor in renderer (for visual display)
	// Note: renderer.SetCursor() also handles scrolling the viewport
	if renderer != nil {
		logger.Debug("Cursor MoveTo", "pos", pos, "rendererExists", true)
		renderer.SetCursor(pos)
	} else {
		logger.Debug("Cursor MoveTo", "pos", pos, "rendererExists", false)
	}

	// Emit navigation signal
	cm.emitSignal()

	return true
}

// MoveUp moves the cursor up by one position.
// Returns true if the cursor was moved.
func (cm *CursorManager) MoveUp(renderer *virtuallist.Renderer) bool {
	if cm.nav.Cursor() <= 0 {
		return false
	}
	return cm.MoveTo(cm.nav.Cursor()-1, renderer)
}

// MoveDown moves the cursor down by one position.
// Returns true if the cursor was moved.
func (cm *CursorManager) MoveDown(renderer *virtuallist.Renderer) bool {
	if cm.nav.Cursor() >= cm.state.ItemCount()-1 {
		return false
	}
	return cm.MoveTo(cm.nav.Cursor()+1, renderer)
}

// JumpToStart moves the cursor to the first item.
func (cm *CursorManager) JumpToStart(renderer *virtuallist.Renderer) {
	cm.MoveTo(0, renderer)
}

// JumpToEnd moves the cursor to the last item.
func (cm *CursorManager) JumpToEnd(renderer *virtuallist.Renderer) {
	lastIdx := cm.state.ItemCount() - 1
	if lastIdx >= 0 {
		cm.MoveTo(lastIdx, renderer)
	}
}

// EmitSignal emits a navigation signal for the current cursor position.
// This is useful for cases where the cursor hasn't changed but we need to
// notify subscribers (e.g., after initial tree load).
func (cm *CursorManager) EmitSignal() {
	if cm.navBus == nil {
		return
	}

	item := cm.state.GetItem(cm.nav.Cursor())
	if item != nil {
		cm.navBus.Notify(item.NodeID, item.Path, cm.hivePath)
	}
}

// emitSignal is the internal version, called by MoveTo.
func (cm *CursorManager) emitSignal() {
	cm.EmitSignal()
}

// Cursor returns the current cursor position.
func (cm *CursorManager) Cursor() int {
	return cm.nav.Cursor()
}
