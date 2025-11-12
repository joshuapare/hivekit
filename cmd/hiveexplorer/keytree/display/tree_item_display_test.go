package display

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestRenderTreeItemDisplay_BasicRendering tests pure rendering without styles
func TestRenderTreeItemDisplay_BasicRendering(t *testing.T) {
	props := TreeItemDisplayProps{
		Name:          "TestKey",
		LeftIndicator: " ",
		Icon:          "•",
		Prefix:        " ",
		CountText:     "",
		Timestamp:     "",
		Depth:         0,
		ItemStyle:     lipgloss.NewStyle(),
		IsSelected:    false,
	}

	result := RenderTreeItemDisplay(props, 80)

	// Should contain the name
	if !strings.Contains(result, "TestKey") {
		t.Error("rendered output should contain item name")
	}

	// Should contain the icon
	if !strings.Contains(result, "•") {
		t.Error("rendered output should contain icon")
	}
}

// TestRenderTreeItemDisplay_WithPrefix tests different prefix characters
func TestRenderTreeItemDisplay_WithPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
	}{
		{"added", "+"},
		{"removed", "-"},
		{"modified", "~"},
		{"unchanged", " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props := TreeItemDisplayProps{
				Name:        "TestKey",
				Prefix:      tt.prefix,
				PrefixStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
				Icon:        "•",
				ItemStyle:   lipgloss.NewStyle(),
			}

			result := RenderTreeItemDisplay(props, 80)

			if !strings.Contains(result, "TestKey") {
				t.Errorf("rendered output should contain item name for prefix %q", tt.prefix)
			}
		})
	}
}

// TestRenderTreeItemDisplay_WithLeftIndicator tests bookmark indicator
func TestRenderTreeItemDisplay_WithLeftIndicator(t *testing.T) {
	// With bookmark
	propsBookmarked := TreeItemDisplayProps{
		Name:          "Bookmarked",
		LeftIndicator: "★",
		Icon:          "•",
		Prefix:        " ",
		ItemStyle:     lipgloss.NewStyle(),
	}

	resultBookmarked := RenderTreeItemDisplay(propsBookmarked, 80)

	if !strings.Contains(resultBookmarked, "★") {
		t.Error("rendered output should contain bookmark indicator when bookmarked")
	}

	// Without bookmark
	propsNormal := TreeItemDisplayProps{
		Name:          "Normal",
		LeftIndicator: " ",
		Icon:          "•",
		Prefix:        " ",
		ItemStyle:     lipgloss.NewStyle(),
	}

	resultNormal := RenderTreeItemDisplay(propsNormal, 80)

	if strings.Contains(resultNormal, "★") {
		t.Error("rendered output should NOT contain bookmark indicator when not bookmarked")
	}
}

// TestRenderTreeItemDisplay_WithIcons tests different tree icons
func TestRenderTreeItemDisplay_WithIcons(t *testing.T) {
	tests := []struct {
		name string
		icon string
	}{
		{"expanded", "▼"},
		{"collapsed", "▶"},
		{"empty", "•"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props := TreeItemDisplayProps{
				Name:      "TestKey",
				Icon:      tt.icon,
				Prefix:    " ",
				ItemStyle: lipgloss.NewStyle(),
			}

			result := RenderTreeItemDisplay(props, 80)

			if !strings.Contains(result, tt.icon) {
				t.Errorf("rendered output should contain icon %q", tt.icon)
			}
		})
	}
}

// TestRenderTreeItemDisplay_WithCount tests count display
func TestRenderTreeItemDisplay_WithCount(t *testing.T) {
	props := TreeItemDisplayProps{
		Name:      "Parent",
		Icon:      "▶",
		CountText: "(5)",
		Prefix:    " ",
		ItemStyle: lipgloss.NewStyle(),
	}

	result := RenderTreeItemDisplay(props, 80)

	if !strings.Contains(result, "(5)") {
		t.Error("rendered output should contain count text")
	}
}

// TestRenderTreeItemDisplay_WithTimestamp tests timestamp display
func TestRenderTreeItemDisplay_WithTimestamp(t *testing.T) {
	props := TreeItemDisplayProps{
		Name:      "TestKey",
		Icon:      "•",
		Timestamp: "2024-01-15 10:30",
		Prefix:    " ",
		ItemStyle: lipgloss.NewStyle(),
	}

	result := RenderTreeItemDisplay(props, 80)

	if !strings.Contains(result, "2024-01-15") {
		t.Error("rendered output should contain timestamp")
	}
}

// TestRenderTreeItemDisplay_WithDepth tests indentation
func TestRenderTreeItemDisplay_WithDepth(t *testing.T) {
	tests := []struct {
		depth int
	}{
		{0},
		{1},
		{2},
		{5},
	}

	for _, tt := range tests {
		props := TreeItemDisplayProps{
			Name:      "TestKey",
			Icon:      "•",
			Depth:     tt.depth,
			Prefix:    " ",
			ItemStyle: lipgloss.NewStyle(),
		}

		result := RenderTreeItemDisplay(props, 80)

		// Should still contain the name regardless of depth
		if !strings.Contains(result, "TestKey") {
			t.Errorf("rendered output should contain item name at depth %d", tt.depth)
		}
	}
}

// TestRenderTreeItemDisplay_Selection tests selected vs unselected
func TestRenderTreeItemDisplay_Selection(t *testing.T) {
	// Selected
	propsSelected := TreeItemDisplayProps{
		Name:       "Selected",
		Icon:       "•",
		Prefix:     " ",
		ItemStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		IsSelected: true,
	}

	resultSelected := RenderTreeItemDisplay(propsSelected, 80)

	// Unselected
	propsUnselected := TreeItemDisplayProps{
		Name:       "Unselected",
		Icon:       "•",
		Prefix:     " ",
		ItemStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		IsSelected: false,
	}

	resultUnselected := RenderTreeItemDisplay(propsUnselected, 80)

	// Both should contain names
	if !strings.Contains(resultSelected, "Selected") {
		t.Error("selected item should contain name")
	}

	if !strings.Contains(resultUnselected, "Unselected") {
		t.Error("unselected item should contain name")
	}
}

// TestRenderTreeItemDisplay_ComplexCombination tests all features together
func TestRenderTreeItemDisplay_ComplexCombination(t *testing.T) {
	props := TreeItemDisplayProps{
		Name:          "ComplexKey",
		LeftIndicator: "★",
		Icon:          "▼",
		Prefix:        "+",
		PrefixStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
		CountText:     "(12)",
		Timestamp:     "2024-03-15 14:30",
		Depth:         2,
		ItemStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
		IsSelected:    false,
	}

	result := RenderTreeItemDisplay(props, 80)

	// Should contain all components
	if !strings.Contains(result, "ComplexKey") {
		t.Error("should contain name")
	}

	if !strings.Contains(result, "★") {
		t.Error("should contain bookmark indicator")
	}

	if !strings.Contains(result, "▼") {
		t.Error("should contain expanded icon")
	}

	if !strings.Contains(result, "(12)") {
		t.Error("should contain count")
	}

	if !strings.Contains(result, "2024-03-15") {
		t.Error("should contain timestamp")
	}
}

// TestRenderTreeItemDisplay_EmptyValues tests rendering with minimal props
func TestRenderTreeItemDisplay_EmptyValues(t *testing.T) {
	props := TreeItemDisplayProps{
		Name:       "MinimalKey",
		Icon:       "•",
		Prefix:     " ",
		ItemStyle:  lipgloss.NewStyle(),
		CountText:  "",
		Timestamp:  "",
		IsSelected: false,
	}

	result := RenderTreeItemDisplay(props, 80)

	// Should still render successfully
	if !strings.Contains(result, "MinimalKey") {
		t.Error("should contain name even with empty optional fields")
	}
}

// TestRenderTreeItemDisplay_NegativePadding tests padding clamping for very long content
func TestRenderTreeItemDisplay_NegativePadding(t *testing.T) {
	props := TreeItemDisplayProps{
		Name:        strings.Repeat("VeryLongKeyName", 10), // 150 chars
		Icon:        "▼",
		CountText:   "(999)",
		Timestamp:   "2024-01-15 10:30:45",
		Prefix:      "+",
		PrefixStyle: lipgloss.NewStyle(),
		ItemStyle:   lipgloss.NewStyle(),
		Depth:       0,
	}

	// Render with small width - should trigger padding < 1 case
	result := RenderTreeItemDisplay(props, 40)

	// Should not panic and should still contain at least part of the name (truncated)
	// With width=40 and long timestamp, name will be heavily truncated
	if !strings.Contains(result, "Ve") && !strings.Contains(result, "VeryLong") {
		t.Logf("Result: %q", result)
		t.Error("should contain at least part of the name even with insufficient width")
	}

	// Should still contain timestamp even with insufficient width
	if !strings.Contains(result, "2024-01-15") {
		t.Error("should contain timestamp even with insufficient width")
	}
}
