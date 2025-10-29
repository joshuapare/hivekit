package display

import "github.com/charmbracelet/lipgloss"

// ValueRowDisplayProps contains pre-computed display properties for a value row.
// All business logic (DiffStatus interpretation, value formatting, etc.) happens
// in the adapter layer. This struct contains only the FINAL display strings and styles.
//
// The display layer has NO knowledge of:
// - What DiffStatus means
// - How to format modified values
// - How to determine diff prefixes
// - Any domain concepts
//
// It ONLY knows how to:
// - Format strings with pre-computed data
// - Apply pre-decided styles
// - Handle column widths and truncation
type ValueRowDisplayProps struct {
	// Pre-formatted text fields
	DiffPrefix string // Diff indicator: "+", "-", "~", or " "
	Name       string // Value name (or "(Default)" if empty)
	Type       string // Value type
	Value      string // Formatted value (may include "old → new" for modified)

	// Column widths (decided by caller)
	NameWidth  int
	TypeWidth  int
	ValueWidth int

	// Pre-decided visual styling
	RowStyle    lipgloss.Style // Style for the entire row
	PrefixStyle lipgloss.Style // Style for diff prefix
	IsSelected  bool           // Whether this row is selected

	// Row index (for alternating colors)
	Index int
}
