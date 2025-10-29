package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MainViewModel wraps the main UI (tree + value panes) for use as overlay background
type MainViewModel struct {
	model *Model
}

func NewMainViewModel(m *Model) *MainViewModel {
	return &MainViewModel{model: m}
}

func (m *MainViewModel) Init() tea.Cmd {
	return nil
}

func (m *MainViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Main model updates are handled in the parent Model's Update
	// This model just provides the View() for overlay
	return m, nil
}

func (m *MainViewModel) View() string {
	// Render the main UI (this is the background for the overlay)
	header := m.model.renderHeader()
	content := m.model.renderContent()
	status := m.model.renderStatus()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
		status,
	)
}
