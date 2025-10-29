package valuedetail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
)

// DetailDisplayMode determines how the detail view is shown
type DetailDisplayMode int

const (
	DetailModeModal DetailDisplayMode = iota // Popup overlay
	DetailModePane                           // Bottom pane
)

// ValueDetailModel shows detailed information about a selected value
type ValueDetailModel struct {
	value       *valuetable.ValueRow
	displayMode DetailDisplayMode
	viewport    viewport.Model
	width       int
	height      int
	visible     bool
}

// NewValueDetailModel creates a new value detail model
func NewValueDetailModel(mode DetailDisplayMode) ValueDetailModel {
	return ValueDetailModel{
		displayMode: mode,
		viewport:    viewport.New(0, 0),
		visible:     false,
	}
}

// Init implements tea.Model
func (m ValueDetailModel) Init() tea.Cmd {
	return nil
}

// Show displays details for a value
func (m *ValueDetailModel) Show(value *valuetable.ValueRow) {
	m.value = value
	m.visible = true
	m.updateContent()
}

// Hide closes the detail view
func (m *ValueDetailModel) Hide() {
	m.visible = false
	m.value = nil
}

// IsVisible returns whether the detail view is currently shown
func (m *ValueDetailModel) IsVisible() bool {
	return m.visible
}

// DisplayMode returns the current display mode
func (m *ValueDetailModel) DisplayMode() DetailDisplayMode {
	return m.displayMode
}

// Update handles messages
func (m *ValueDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSize()
		m.updateContent()
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// updateViewportSize adjusts viewport dimensions based on display mode
func (m *ValueDetailModel) updateViewportSize() {
	switch m.displayMode {
	case DetailModeModal:
		// Modal takes 80% of screen, centered
		// Account for: border (2 lines) + padding (2 lines top+bottom) = 4 vertical
		//             border (2 cols) + padding (4 cols left+right) = 6 horizontal
		m.viewport.Width = int(float64(m.width)*0.8) - 6
		m.viewport.Height = int(float64(m.height)*0.8) - 4
	case DetailModePane:
		// Pane takes full width, 1/3 of height
		m.viewport.Width = m.width - 4
		m.viewport.Height = m.height/3 - 4
	}
}

// updateContent generates the detailed view content
func (m *ValueDetailModel) updateContent() {
	if m.value == nil {
		m.viewport.SetContent("")
		return
	}

	var b strings.Builder

	// Title
	name := m.value.Name
	if name == "" {
		name = "(Default)"
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	b.WriteString(titleStyle.Render(fmt.Sprintf("Value: %s", name)))
	b.WriteString("\n\n")

	// Type
	b.WriteString(fmt.Sprintf("Type:  %s\n", m.value.Type))
	b.WriteString(fmt.Sprintf("Size:  %d bytes\n", m.value.Size))
	b.WriteString("\n")

	// Formatted value based on type
	switch m.value.Type {
	case "REG_SZ", "REG_EXPAND_SZ":
		b.WriteString("String Value:\n")
		b.WriteString(strings.Repeat("─", m.viewport.Width-2))
		b.WriteString("\n")
		b.WriteString(m.value.Value)
		b.WriteString("\n\n")

	case "REG_MULTI_SZ":
		// For multi-string, the Value field has them comma-separated
		// We want to show them on separate lines
		b.WriteString("Multi-String Values:\n")
		b.WriteString(strings.Repeat("─", m.viewport.Width-2))
		b.WriteString("\n")
		// Parse from the stored format
		parts := strings.Split(m.value.Value, ", ")
		for i, part := range parts {
			b.WriteString(fmt.Sprintf("[%d] %s\n", i, part))
		}
		b.WriteString("\n")

	case "REG_DWORD", "REG_DWORD_BE":
		b.WriteString("DWORD Value:\n")
		b.WriteString(strings.Repeat("─", m.viewport.Width-2))
		b.WriteString("\n")
		b.WriteString(m.value.Value) // Already formatted as "0x12345678 (305419896)"
		b.WriteString("\n\n")

	case "REG_QWORD":
		b.WriteString("QWORD Value:\n")
		b.WriteString(strings.Repeat("─", m.viewport.Width-2))
		b.WriteString("\n")
		b.WriteString(m.value.Value) // Already formatted
		b.WriteString("\n\n")

	case "REG_BINARY":
		b.WriteString("Binary Data:\n")
		b.WriteString(strings.Repeat("─", m.viewport.Width-2))
		b.WriteString("\n")
		b.WriteString(m.formatHexDump(m.value.Raw))
		b.WriteString("\n")

	default:
		if len(m.value.Raw) > 0 {
			b.WriteString("Data (Hex):\n")
			b.WriteString(strings.Repeat("─", m.viewport.Width-2))
			b.WriteString("\n")
			b.WriteString(m.formatHexDump(m.value.Raw))
			b.WriteString("\n")
		} else {
			b.WriteString("(No data)\n")
		}
	}

	// Always show raw hex at the bottom for all types
	if len(m.value.Raw) > 0 && m.value.Type != "REG_BINARY" {
		b.WriteString("\n")
		b.WriteString("Raw Data (Hex):\n")
		b.WriteString(strings.Repeat("─", m.viewport.Width-2))
		b.WriteString("\n")
		b.WriteString(m.formatHexDump(m.value.Raw))
	}

	m.viewport.SetContent(b.String())
}

// formatHexDump creates a hex dump with ASCII sidebar
func (m *ValueDetailModel) formatHexDump(data []byte) string {
	if len(data) == 0 {
		return "(empty)"
	}

	var b strings.Builder
	const bytesPerLine = 16

	for offset := 0; offset < len(data); offset += bytesPerLine {
		// Offset
		b.WriteString(fmt.Sprintf("%08x  ", offset))

		// Hex bytes
		lineEnd := offset + bytesPerLine
		if lineEnd > len(data) {
			lineEnd = len(data)
		}

		for i := offset; i < lineEnd; i++ {
			b.WriteString(fmt.Sprintf("%02x ", data[i]))
			if i == offset+7 {
				b.WriteString(" ") // Extra space in the middle
			}
		}

		// Padding for incomplete lines
		remaining := bytesPerLine - (lineEnd - offset)
		for i := 0; i < remaining; i++ {
			b.WriteString("   ")
		}
		if remaining > 8 {
			b.WriteString(" ")
		}

		// ASCII representation
		b.WriteString(" |")
		for i := offset; i < lineEnd; i++ {
			if data[i] >= 32 && data[i] <= 126 {
				b.WriteByte(data[i])
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteString("|")

		if lineEnd < len(data) {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// View renders the detail view
func (m ValueDetailModel) View() string {
	if !m.visible || m.value == nil {
		return ""
	}

	switch m.displayMode {
	case DetailModeModal:
		return m.viewModal()
	case DetailModePane:
		return m.viewPane()
	default:
		return ""
	}
}

// viewModal renders as a centered popup
func (m ValueDetailModel) viewModal() string {
	// Create border style
	// Note: The overlay package handles centering, so we just render the box
	// The viewport is already sized to fit within the border+padding
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(1, 2)

	// Render the content box (overlay package will center it)
	return borderStyle.Render(m.viewport.View())
}

// viewPane renders as a bottom pane
func (m ValueDetailModel) viewPane() string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	return borderStyle.Render(m.viewport.View())
}
