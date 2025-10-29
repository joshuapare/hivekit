package keytree

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

// TestNewNavigator verifies initialization
func TestNewNavigator(t *testing.T) {
	nav := NewNavigator()

	if nav == nil {
		t.Fatal("NewNavigator() returned nil")
	}

	if nav.cursor != 0 {
		t.Errorf("expected cursor 0, got %d", nav.cursor)
	}

	if nav.pendingNavigationTarget != "" {
		t.Errorf("expected empty pendingNavigationTarget, got %q", nav.pendingNavigationTarget)
	}
}

// TestCursorSetCursor tests cursor position
func TestCursorSetCursor(t *testing.T) {
	nav := NewNavigator()

	// Initially should be 0
	if nav.Cursor() != 0 {
		t.Errorf("expected cursor 0, got %d", nav.Cursor())
	}

	// Set cursor to various positions
	tests := []int{0, 1, 5, 10, 100}
	for _, pos := range tests {
		nav.SetCursor(pos)
		if nav.Cursor() != pos {
			t.Errorf("expected cursor %d, got %d", pos, nav.Cursor())
		}
	}
}

// TestMoveUp tests moving up (success and boundary)
func TestMoveUp(t *testing.T) {
	tests := []struct {
		name           string
		initialCursor  int
		expectedCursor int
		expectedMoved  bool
	}{
		{
			name:           "move from position 5",
			initialCursor:  5,
			expectedCursor: 4,
			expectedMoved:  true,
		},
		{
			name:           "move from position 1",
			initialCursor:  1,
			expectedCursor: 0,
			expectedMoved:  true,
		},
		{
			name:           "cannot move from position 0",
			initialCursor:  0,
			expectedCursor: 0,
			expectedMoved:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nav := NewNavigator()
			nav.SetCursor(tt.initialCursor)

			moved := nav.MoveUp()

			if moved != tt.expectedMoved {
				t.Errorf("expected moved=%v, got %v", tt.expectedMoved, moved)
			}

			if nav.Cursor() != tt.expectedCursor {
				t.Errorf("expected cursor %d, got %d", tt.expectedCursor, nav.Cursor())
			}
		})
	}
}

// TestMoveDown tests moving down (success and boundary)
func TestMoveDown(t *testing.T) {
	tests := []struct {
		name           string
		initialCursor  int
		maxItems       int
		expectedCursor int
		expectedMoved  bool
	}{
		{
			name:           "move from position 0 with 10 items",
			initialCursor:  0,
			maxItems:       10,
			expectedCursor: 1,
			expectedMoved:  true,
		},
		{
			name:           "move from position 5 with 10 items",
			initialCursor:  5,
			maxItems:       10,
			expectedCursor: 6,
			expectedMoved:  true,
		},
		{
			name:           "move from position 8 with 10 items",
			initialCursor:  8,
			maxItems:       10,
			expectedCursor: 9,
			expectedMoved:  true,
		},
		{
			name:           "cannot move from last position (9 with 10 items)",
			initialCursor:  9,
			maxItems:       10,
			expectedCursor: 9,
			expectedMoved:  false,
		},
		{
			name:           "cannot move with 1 item",
			initialCursor:  0,
			maxItems:       1,
			expectedCursor: 0,
			expectedMoved:  false,
		},
		{
			name:           "cannot move with 0 items",
			initialCursor:  0,
			maxItems:       0,
			expectedCursor: 0,
			expectedMoved:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nav := NewNavigator()
			nav.SetCursor(tt.initialCursor)

			moved := nav.MoveDown(tt.maxItems)

			if moved != tt.expectedMoved {
				t.Errorf("expected moved=%v, got %v", tt.expectedMoved, moved)
			}

			if nav.Cursor() != tt.expectedCursor {
				t.Errorf("expected cursor %d, got %d", tt.expectedCursor, nav.Cursor())
			}
		})
	}
}

// TestPendingNavigationTarget tests pending navigation state
func TestPendingNavigationTarget(t *testing.T) {
	nav := NewNavigator()

	// Initially should be empty
	if nav.PendingNavigationTarget() != "" {
		t.Errorf("expected empty target, got %q", nav.PendingNavigationTarget())
	}

	// Set a target
	testPath := "SOFTWARE\\Microsoft\\Windows"
	nav.SetPendingNavigationTarget(testPath)

	if nav.PendingNavigationTarget() != testPath {
		t.Errorf("expected target %q, got %q", testPath, nav.PendingNavigationTarget())
	}

	// Clear the target
	nav.ClearPendingNavigationTarget()

	if nav.PendingNavigationTarget() != "" {
		t.Errorf("expected empty target after clear, got %q", nav.PendingNavigationTarget())
	}

	// Test setting multiple targets
	targets := []string{"path1", "path2", "path3\\subpath"}
	for _, target := range targets {
		nav.SetPendingNavigationTarget(target)
		if nav.PendingNavigationTarget() != target {
			t.Errorf("expected target %q, got %q", target, nav.PendingNavigationTarget())
		}
	}
}

// TestEnsureCursorVisible tests viewport scrolling logic
func TestEnsureCursorVisible(t *testing.T) {
	tests := []struct {
		name              string
		vpWidth           int
		vpHeight          int
		vpYOffset         int
		headerHeight      int
		cursor            int
		expectedYOffset   int
		expectedScrolled  bool
	}{
		{
			name:             "cursor visible in middle, no scroll needed",
			vpWidth:          80,
			vpHeight:         20,
			vpYOffset:        5,
			headerHeight:     2,
			cursor:           10,
			expectedYOffset:  5,
			expectedScrolled: false,
		},
		{
			name:             "cursor above viewport, scroll up",
			vpWidth:          80,
			vpHeight:         20,
			vpYOffset:        10,
			headerHeight:     2,
			cursor:           5,
			expectedYOffset:  5,
			expectedScrolled: true,
		},
		{
			name:             "cursor below viewport, scroll down",
			vpWidth:          80,
			vpHeight:         20,
			vpYOffset:        0,
			headerHeight:     2,
			cursor:           25,
			expectedYOffset:  8, // cursor(25) - visibleHeight(18) + 1 = 25 - 18 + 1 = 8
			expectedScrolled: true,
		},
		{
			name:             "cursor at top edge",
			vpWidth:          80,
			vpHeight:         20,
			vpYOffset:        5,
			headerHeight:     2,
			cursor:           5,
			expectedYOffset:  5,
			expectedScrolled: false,
		},
		{
			name:             "cursor at bottom edge of viewport",
			vpWidth:          80,
			vpHeight:         20,
			vpYOffset:        5,
			headerHeight:     2,
			cursor:           22, // 5 + (20-2) - 1
			expectedYOffset:  5,
			expectedScrolled: false,
		},
		{
			name:             "zero height, no scroll",
			vpWidth:          80,
			vpHeight:         0,
			vpYOffset:        5,
			headerHeight:     0,
			cursor:           10,
			expectedYOffset:  5,
			expectedScrolled: false,
		},
		{
			name:             "header height equals viewport height, no scroll",
			vpWidth:          80,
			vpHeight:         10,
			vpYOffset:        5,
			headerHeight:     10,
			cursor:           10,
			expectedYOffset:  5,
			expectedScrolled: false,
		},
		{
			name:             "header height exceeds viewport height, no scroll",
			vpWidth:          80,
			vpHeight:         10,
			vpYOffset:        5,
			headerHeight:     15,
			cursor:           10,
			expectedYOffset:  5,
			expectedScrolled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nav := NewNavigator()
			nav.SetCursor(tt.cursor)

			vp := viewport.New(tt.vpWidth, tt.vpHeight)
			vp.YOffset = tt.vpYOffset

			// Use itemCount large enough to not interfere with existing test expectations
			itemCount := 100
			scrolled := nav.EnsureCursorVisible(&vp, tt.headerHeight, itemCount)

			if scrolled != tt.expectedScrolled {
				t.Errorf("expected scrolled=%v, got %v", tt.expectedScrolled, scrolled)
			}

			if vp.YOffset != tt.expectedYOffset {
				t.Errorf("expected YOffset %d, got %d", tt.expectedYOffset, vp.YOffset)
			}
		})
	}
}

// TestEnsureCursorVisibleEdgeCases tests edge cases for EnsureCursorVisible
func TestEnsureCursorVisibleEdgeCases(t *testing.T) {
	nav := NewNavigator()

	// Test with cursor at position 0
	nav.SetCursor(0)
	vp := viewport.New(80, 20)
	vp.YOffset = 5

	scrolled := nav.EnsureCursorVisible(&vp, 2, 100)
	if !scrolled {
		t.Error("expected scroll when cursor at 0 and offset is 5")
	}
	if vp.YOffset != 0 {
		t.Errorf("expected YOffset 0, got %d", vp.YOffset)
	}

	// Test scrolling to make cursor visible at bottom
	nav.SetCursor(100)
	vp = viewport.New(80, 20)
	vp.YOffset = 0

	// itemCount must be > cursor (100) to allow scrolling to it
	scrolled = nav.EnsureCursorVisible(&vp, 2, 200)
	if !scrolled {
		t.Error("expected scroll when cursor at 100 and offset is 0")
	}

	// Verify cursor is now visible
	visibleHeight := vp.Height - 2
	if nav.Cursor() < vp.YOffset || nav.Cursor() >= vp.YOffset+visibleHeight {
		t.Error("cursor should be visible after scroll")
	}
}

// TestNavigatorSequentialMoves tests a sequence of moves
func TestNavigatorSequentialMoves(t *testing.T) {
	nav := NewNavigator()
	maxItems := 10

	// Move down several times
	for i := 0; i < 5; i++ {
		if !nav.MoveDown(maxItems) {
			t.Errorf("move down %d should succeed", i)
		}
	}

	if nav.Cursor() != 5 {
		t.Errorf("expected cursor 5, got %d", nav.Cursor())
	}

	// Move up several times
	for i := 0; i < 3; i++ {
		if !nav.MoveUp() {
			t.Errorf("move up %d should succeed", i)
		}
	}

	if nav.Cursor() != 2 {
		t.Errorf("expected cursor 2, got %d", nav.Cursor())
	}

	// Move down to last position
	for nav.Cursor() < maxItems-1 {
		nav.MoveDown(maxItems)
	}

	if nav.Cursor() != maxItems-1 {
		t.Errorf("expected cursor %d, got %d", maxItems-1, nav.Cursor())
	}

	// Try to move down again (should fail)
	if nav.MoveDown(maxItems) {
		t.Error("move down should fail at last position")
	}

	// Move up to first position
	for nav.Cursor() > 0 {
		nav.MoveUp()
	}

	if nav.Cursor() != 0 {
		t.Errorf("expected cursor 0, got %d", nav.Cursor())
	}

	// Try to move up again (should fail)
	if nav.MoveUp() {
		t.Error("move up should fail at first position")
	}
}
