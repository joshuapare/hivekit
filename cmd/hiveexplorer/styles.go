package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/joshuapare/hivekit/pkg/hive"
)

var (
	// Color palette
	primaryColor   = lipgloss.Color("#7D56F4")
	secondaryColor = lipgloss.Color("#00D7FF")
	accentColor    = lipgloss.Color("#FF00FF")
	successColor   = lipgloss.Color("#04B575")
	warningColor   = lipgloss.Color("#FFA500")
	errorColor     = lipgloss.Color("#FF4B4B")
	mutedColor  = lipgloss.Color("#666666")
	borderColor = lipgloss.Color("#383838")

	// Header styles
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(lipgloss.Color("#1A1A1A")).
			Padding(0, 1).
			MarginBottom(1)

	pathStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Italic(true)

	// Pane styles
	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1)

	activePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1)

	// Value table styles
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)


	tableRowStyle = lipgloss.NewStyle()

	tableRowAltStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#0A0A0A"))

	tableSelectedStyle = lipgloss.NewStyle().
				Background(primaryColor).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	// Status bar styles
	statusStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Background(lipgloss.Color("#1A1A1A")).
			Padding(0, 1).
			MarginTop(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	statusCountStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	// Help overlay styles
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			Background(lipgloss.Color("#1A1A1A")).
			Padding(0, 1).
			MarginBottom(1)

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true).
			Width(15)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA"))

	// Modal styles
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2).
			Background(lipgloss.Color("#1A1A1A"))

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	// Search styles
	searchPromptStyle = lipgloss.NewStyle().
				Foreground(accentColor).
				Bold(true)

	searchMatchStyle = lipgloss.NewStyle().
				Background(warningColor).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)

	// Error styles
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	// Diff styles
	diffAddedStyle = lipgloss.NewStyle().
			Foreground(successColor) // Green for additions

	diffRemovedStyle = lipgloss.NewStyle().
				Foreground(errorColor). // Red for removals
				Strikethrough(true)

	diffModifiedStyle = lipgloss.NewStyle().
				Foreground(warningColor) // Orange for modifications

	diffUnchangedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FAFAFA")) // Normal color

	// Diff prefixes/indicators
	diffAddedPrefix    = "+"
	diffRemovedPrefix  = "-"
	diffModifiedPrefix = "~"
	diffUnchangedPrefix = " "
)

// getDiffStyle returns the appropriate style for a diff status
func getDiffStyle(status hive.DiffStatus) lipgloss.Style {
	switch status {
	case hive.DiffAdded:
		return diffAddedStyle
	case hive.DiffRemoved:
		return diffRemovedStyle
	case hive.DiffModified:
		return diffModifiedStyle
	case hive.DiffUnchanged:
		return diffUnchangedStyle
	default:
		return diffUnchangedStyle
	}
}

// getDiffPrefix returns the prefix character for a diff status
func getDiffPrefix(status hive.DiffStatus) string {
	switch status {
	case hive.DiffAdded:
		return diffAddedPrefix
	case hive.DiffRemoved:
		return diffRemovedPrefix
	case hive.DiffModified:
		return diffModifiedPrefix
	case hive.DiffUnchanged:
		return diffUnchangedPrefix
	default:
		return diffUnchangedPrefix
	}
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

