package display

import (
	"strings"
)

// RenderTreeItemDisplay is a PURE rendering function with zero business logic.
// It takes pre-computed display props and formats them into a string.
//
// This function has NO knowledge of:
// - What DiffStatus means
// - What bookmarks are
// - What "expanded" vs "collapsed" means
// - Any domain concepts
//
// It ONLY knows how to:
// - Format strings
// - Apply padding
// - Apply pre-decided styles
//
// All decisions about WHAT to display are made by the adapter layer.
//
// Parameters:
//   - props: Pre-computed display properties
//   - width: Available width for rendering
//
// Returns: Formatted string ready for display
func RenderTreeItemDisplay(props TreeItemDisplayProps, width int) string {
	// Indentation (depth * 2 spaces)
	indent := strings.Repeat("  ", props.Depth)

	// Calculate plain text lengths for padding calculation
	leftIndicatorWidth := 1 // Always 1 character
	prefixWidth := len(props.Prefix)
	iconWidth := len(props.Icon)
	countWidth := len(props.CountText)
	timestampWidth := len(props.Timestamp)

	// Calculate available width for name
	// Format: prefix + leftIndicator + space + indent + icon + space + name + [space + count] + padding + timestamp
	// Note: space before count is only present if count exists
	fixedWidth := prefixWidth + leftIndicatorWidth + 1 + len(indent) + iconWidth + 1
	if countWidth > 0 {
		fixedWidth += 1 + countWidth // Add space before count + count width
	}
	minPadding := 2 // Minimum padding between name/count and timestamp
	availableForName := width - 4 - fixedWidth - timestampWidth - minPadding

	// Truncate name if necessary
	name := props.Name
	nameWidth := len(props.Name)
	if availableForName < nameWidth {
		if availableForName > 3 {
			// Truncate with ellipsis
			name = props.Name[:availableForName-3] + "..."
			nameWidth = availableForName
		} else if availableForName > 0 {
			// Very small space, just truncate without ellipsis
			name = props.Name[:availableForName]
			nameWidth = availableForName
		} else {
			// No space at all, use empty string
			name = ""
			nameWidth = 0
		}
	}

	// Calculate padding for right-justified timestamp
	usedWidth := fixedWidth + nameWidth
	padding := width - 4 - usedWidth - timestampWidth
	if padding < 1 {
		padding = 1
	}

	// Build the line with styled components
	// Note: We apply styles to individual parts, then combine
	var parts []string

	// Prefix (with its style)
	if props.Prefix != "" && props.Prefix != " " {
		parts = append(parts, props.PrefixStyle.Render(props.Prefix))
	} else {
		parts = append(parts, props.Prefix)
	}

	// Left indicator + space
	parts = append(parts, props.LeftIndicator+" ")

	// Indentation
	parts = append(parts, indent)

	// Icon + space
	parts = append(parts, props.Icon+" ")

	// Name (potentially truncated)
	parts = append(parts, name)

	// Count (styled, if present)
	if props.CountText != "" {
		// Apply muted style to count
		countStyle := props.ItemStyle.Copy().Foreground(mutedColor).Italic(true)
		parts = append(parts, " "+countStyle.Render(props.CountText))
	}

	// Padding
	parts = append(parts, strings.Repeat(" ", padding))

	// Timestamp (styled)
	if props.Timestamp != "" {
		timestampStyle := props.ItemStyle.Copy().Foreground(mutedColor).Italic(true)
		parts = append(parts, timestampStyle.Render(props.Timestamp))
	}

	// Join all parts
	line := strings.Join(parts, "")

	// Apply overall item style (background, selection highlighting, etc.)
	if props.IsSelected {
		// Selection style overrides everything
		line = selectedStyle.Render(line)
	} else {
		// Apply the item style (which may include diff coloring)
		line = props.ItemStyle.Render(line)
	}

	return line
}
