package adapter

import "github.com/charmbracelet/lipgloss"

// Domain color palette
// These colors have domain meaning: "added" = green, "removed" = red, etc.
var (
	successColor = lipgloss.Color("#04B575") // For additions
	errorColor   = lipgloss.Color("#FF4B4B") // For removals
	warningColor = lipgloss.Color("#FFA500") // For modifications
	normalColor  = lipgloss.Color("#FAFAFA") // For unchanged
)

// Diff-related styles (have domain meaning)
var (
	addedPrefixStyle = lipgloss.NewStyle().
				Foreground(successColor)

	addedItemStyle = lipgloss.NewStyle().
			Foreground(successColor)

	removedPrefixStyle = lipgloss.NewStyle().
				Foreground(errorColor)

	removedItemStyle = lipgloss.NewStyle().
				Foreground(errorColor).
				Strikethrough(true)

	modifiedPrefixStyle = lipgloss.NewStyle().
				Foreground(warningColor)

	modifiedItemStyle = lipgloss.NewStyle().
				Foreground(warningColor)

	unchangedPrefixStyle = lipgloss.NewStyle().
				Foreground(normalColor)

	unchangedItemStyle = lipgloss.NewStyle().
				Foreground(normalColor)
)
