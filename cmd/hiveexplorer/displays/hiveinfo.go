package displays

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// HiveInfoDisplay displays registry hive header information
type HiveInfoDisplay struct {
	Info   hive.HiveInfo
	width  int
	height int
}

// NewHiveInfoDisplay creates a new hive info display
func NewHiveInfoDisplay(info hive.HiveInfo) *HiveInfoDisplay {
	return &HiveInfoDisplay{
		Info: info,
	}
}

// SetSize sets the display dimensions
func (h *HiveInfoDisplay) SetSize(width, height int) {
	h.width = width
	h.height = height
}

// View renders the hive info panel
func (h *HiveInfoDisplay) View() string {
	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4")).
		Width(12)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA"))

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#383838")).
		Padding(0, 1)

	// Format timestamp
	var timeStr string
	if h.Info.LastWrite.Unix() > 0 {
		timeStr = h.Info.LastWrite.Format("2006-01-02 15:04:05")
	} else {
		timeStr = "N/A"
	}

	// Determine hive type
	var typeStr string
	switch h.Info.Type {
	case 0:
		typeStr = "Primary"
	case 1:
		typeStr = "Alternate"
	default:
		typeStr = fmt.Sprintf("Unknown (%d)", h.Info.Type)
	}

	// Format sizes in KB/MB
	hbinsSizeKB := float64(h.Info.HiveBinsDataSize) / 1024.0
	var sizeStr string
	if hbinsSizeKB > 1024 {
		sizeStr = fmt.Sprintf("%.2f MB", hbinsSizeKB/1024.0)
	} else {
		sizeStr = fmt.Sprintf("%.2f KB", hbinsSizeKB)
	}

	// Build content
	content := titleStyle.Render("HIVE INFO") + "\n\n"

	content += labelStyle.Render("Version:") + " " +
		valueStyle.Render(fmt.Sprintf("%d.%d", h.Info.MajorVersion, h.Info.MinorVersion)) + "\n"

	content += labelStyle.Render("Type:") + " " +
		valueStyle.Render(typeStr) + "\n"

	content += labelStyle.Render("Modified:") + " " +
		valueStyle.Render(timeStr) + "\n"

	content += labelStyle.Render("Data Size:") + " " +
		valueStyle.Render(sizeStr) + "\n"

	content += labelStyle.Render("Seq:") + " " +
		valueStyle.Render(fmt.Sprintf("%d/%d", h.Info.PrimarySequence, h.Info.SecondarySequence)) + "\n"

	// Only show clustering factor if non-zero (rarely used)
	if h.Info.ClusteringFactor > 0 {
		content += labelStyle.Render("Cluster:") + " " +
			valueStyle.Render(fmt.Sprintf("%d", h.Info.ClusteringFactor)) + "\n"
	}

	// Apply border and fill the space
	// Note: lipgloss Width/Height set the TOTAL size including border and padding
	// So we pass the full dimensions and lipgloss handles the border/padding internally
	box := borderStyle.
		Width(h.width).
		Height(h.height).
		Render(content)

	return box
}
