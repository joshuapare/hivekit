package display

import "github.com/charmbracelet/lipgloss"

// Basic display styles (presentation-only, no domain meaning)
var (
	// Selection style
	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#7D56F4")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)

	// Alternating row styles
	rowStyle = lipgloss.NewStyle()

	rowAltStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0A0A0A"))

	// Header style
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))
)
