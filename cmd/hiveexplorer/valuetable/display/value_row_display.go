package display

import (
	"fmt"
	"strings"
)

// RenderValueRow is a PURE rendering function with zero business logic.
// It takes pre-computed display props and formats them into a string.
//
// This function has NO knowledge of:
// - What DiffStatus means
// - How to format modified values
// - How to determine diff prefixes
// - Any domain concepts
//
// It ONLY knows how to:
// - Format strings with provided data
// - Apply padding and truncation
// - Apply pre-decided styles
//
// All decisions about WHAT to display are made by the adapter layer.
//
// Parameters:
//   - props: Pre-computed display properties
//   - contentWidth: Total width available for the row
//
// Returns: Formatted string ready for display
func RenderValueRow(props ValueRowDisplayProps, contentWidth int) string {
	// Truncate text to fit column widths
	name := truncate(props.Name, props.NameWidth)
	typeText := truncate(props.Type, props.TypeWidth)
	value := truncate(props.Value, props.ValueWidth)

	// Build line with proper padding BEFORE applying styles
	// Format: diffPrefix + space + name + spaces + type + spaces + value
	// Pad value to fill available width so row extends to edge
	line := fmt.Sprintf("%s %-*s  %-*s  %-*s",
		props.DiffPrefix,
		props.NameWidth, name,
		props.TypeWidth, typeText,
		props.ValueWidth, value,
	)

	// Apply selection or row style with full width
	if props.IsSelected {
		return selectedStyle.Width(contentWidth).Render(line)
	}

	// Apply the pre-decided row style
	return props.RowStyle.Width(contentWidth).Render(line)
}

// RenderHeader renders the table header
//
// Parameters:
//   - nameWidth: Width of name column
//   - typeWidth: Width of type column
//   - valueWidth: Width of value column
//   - contentWidth: Total width available
//
// Returns: Formatted header string
func RenderHeader(nameWidth, typeWidth, valueWidth, contentWidth int) string {
	header := fmt.Sprintf("  %-*s  %-*s  %-*s",
		nameWidth, "Name",
		typeWidth, "Type",
		valueWidth, "Value",
	)
	return headerStyle.Width(contentWidth).Render(header)
}

// RenderSeparator renders the separator line
//
// Parameters:
//   - contentWidth: Total width available
//
// Returns: Separator string
func RenderSeparator(contentWidth int) string {
	return strings.Repeat("─", contentWidth)
}

// truncate truncates a string to the specified length with ellipsis
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
