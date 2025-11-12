package display

import "github.com/charmbracelet/lipgloss"

// Basic color palette for display components
// These are pure presentation colors with no domain meaning
var (
	mutedColor = lipgloss.Color("#666666")
)

// Basic display styles
var (
	selectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#7D56F4")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true)
)
