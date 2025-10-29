package displays

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// NKRecordDisplay displays registry NK (Node Key) record information
type NKRecordDisplay struct {
	KeyPath   string
	KeyDetail hive.KeyDetail
	width     int
	height    int
}

// NewNKRecordDisplay creates a new NK record display
func NewNKRecordDisplay(keyPath string, detail hive.KeyDetail) *NKRecordDisplay {
	return &NKRecordDisplay{
		KeyPath:   keyPath,
		KeyDetail: detail,
	}
}

// SetSize sets the display dimensions
func (n *NKRecordDisplay) SetSize(width, height int) {
	n.width = width
	n.height = height
}

// View renders the NK record info panel
func (n *NKRecordDisplay) View() string {
	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Width(10)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA"))

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#383838")).
		Padding(0, 1)

	// Format timestamp
	var timeStr string
	if !n.KeyDetail.LastWrite.IsZero() {
		timeStr = n.KeyDetail.LastWrite.Format("2006-01-02 15:04")
	} else {
		timeStr = "N/A"
	}

	// Decode flags
	flagsStr := fmt.Sprintf("0x%04X", n.KeyDetail.Flags)
	if n.KeyDetail.Flags&0x0020 != 0 {
		flagsStr += " (Compressed)"
	}

	// Build content - show all NK fields, one per line
	content := titleStyle.Render("NK RECORD") + "\n"

	content += labelStyle.Render("Name:") + " " +
		valueStyle.Render(n.KeyDetail.Name) + "\n"

	content += labelStyle.Render("Modified:") + " " +
		valueStyle.Render(timeStr) + "\n"

	content += labelStyle.Render("Subkeys:") + " " +
		valueStyle.Render(fmt.Sprintf("%d", n.KeyDetail.SubkeyN)) + "\n"

	content += labelStyle.Render("Values:") + " " +
		valueStyle.Render(fmt.Sprintf("%d", n.KeyDetail.ValueN)) + "\n"

	content += labelStyle.Render("Flags:") + " " +
		valueStyle.Render(flagsStr) + "\n"

	content += labelStyle.Render("Parent:") + " " +
		valueStyle.Render(fmt.Sprintf("0x%X", n.KeyDetail.ParentOffset)) + "\n"

	content += labelStyle.Render("Security:") + " " +
		valueStyle.Render(fmt.Sprintf("0x%X", n.KeyDetail.SecurityOffset)) + "\n"

	// Apply border and fill the space
	box := borderStyle.
		Width(n.width).
		Height(n.height).
		Render(content)

	return box
}
