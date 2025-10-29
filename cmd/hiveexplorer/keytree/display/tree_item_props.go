package display

import "github.com/charmbracelet/lipgloss"

// TreeItemDisplayProps contains all pre-computed display data for rendering a tree item.
// This struct has ZERO domain knowledge - it only knows about visual presentation.
// All business logic (DiffStatus interpretation, bookmark rules, icon selection) happens
// in the adapter layer before creating these props.
type TreeItemDisplayProps struct {
	// Pre-formatted text fields (no logic needed to display)
	Name          string // The item name to display
	LeftIndicator string // Left marker (e.g., "★" for bookmarked, " " for none)
	Icon          string // Tree icon ("▼" expanded, "▶" collapsed, "•" leaf)
	Prefix        string // Left prefix (e.g., "+", "-", "~", " ")
	CountText     string // Count display (e.g., "(5)" or "")
	Timestamp     string // Formatted timestamp (e.g., "2024-01-15 10:30" or "")
	Depth         int    // Indentation depth (0 = root, 1 = one level deep, etc.)

	// Pre-decided visual styling (no conditional logic)
	PrefixStyle lipgloss.Style // Style for the prefix character
	ItemStyle   lipgloss.Style // Style for the entire item
	IsSelected  bool           // Whether this item is currently selected
}
