package adapter

import "github.com/charmbracelet/lipgloss"

// Normal styles for tree items
var (
	normalColor = lipgloss.Color("#FAFAFA")

	normalPrefixStyle = lipgloss.NewStyle().
				Foreground(normalColor)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(normalColor)
)
