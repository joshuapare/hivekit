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

	addedRowStyle = lipgloss.NewStyle().
			Foreground(successColor)

	removedPrefixStyle = lipgloss.NewStyle().
				Foreground(errorColor)

	removedRowStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Strikethrough(true)

	modifiedPrefixStyle = lipgloss.NewStyle().
				Foreground(warningColor)

	modifiedRowStyle = lipgloss.NewStyle().
				Foreground(warningColor)

	unchangedPrefixStyle = lipgloss.NewStyle().
				Foreground(normalColor)

	unchangedRowStyle = lipgloss.NewStyle().
				Foreground(normalColor)

	unchangedRowAltStyle = lipgloss.NewStyle().
				Foreground(normalColor).
				Background(lipgloss.Color("#0A0A0A"))
)
