package display

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// TestRenderValueRow_BasicRendering tests pure rendering without styles
func TestRenderValueRow_BasicRendering(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
		IsSelected: false,
	}

	result := RenderValueRow(props, 80)

	// Should contain the name
	if !strings.Contains(result, "TestValue") {
		t.Error("rendered output should contain value name")
	}

	// Should contain the type
	if !strings.Contains(result, "REG_SZ") {
		t.Error("rendered output should contain type")
	}

	// Should contain the value
	if !strings.Contains(result, "data") {
		t.Error("rendered output should contain value")
	}
}

// TestRenderValueRow_WithPrefix tests different prefix characters
func TestRenderValueRow_WithPrefix(t *testing.T) {
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
			props := ValueRowDisplayProps{
				Name:        "TestValue",
				Type:        "REG_SZ",
				Value:       "data",
				DiffPrefix:  tt.prefix,
				NameWidth:   20,
				TypeWidth:   15,
				ValueWidth:  30,
				RowStyle:    lipgloss.NewStyle(),
				PrefixStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")),
			}

			result := RenderValueRow(props, 80)

			if !strings.Contains(result, "TestValue") {
				t.Errorf("rendered output should contain value name for prefix %q", tt.prefix)
			}
		})
	}
}

// TestRenderValueRow_DefaultName tests empty name handling
func TestRenderValueRow_DefaultName(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       "(Default)",
		Type:       "REG_SZ",
		Value:      "default value",
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
	}

	result := RenderValueRow(props, 80)

	if !strings.Contains(result, "(Default)") {
		t.Error("rendered output should contain (Default) for empty name")
	}
}

// TestRenderValueRow_ModifiedValueFormat tests "old → new" display
func TestRenderValueRow_ModifiedValueFormat(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       "ModifiedValue",
		Type:       "REG_SZ",
		Value:      "oldValue → newValue",
		DiffPrefix: "~",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 40,
		RowStyle:   lipgloss.NewStyle(),
	}

	result := RenderValueRow(props, 80)

	// Should contain the arrow-formatted value
	if !strings.Contains(result, "→") {
		t.Error("rendered output should contain arrow for modified values")
	}

	if !strings.Contains(result, "oldValue") || !strings.Contains(result, "newValue") {
		t.Error("rendered output should contain both old and new values")
	}
}

// TestRenderValueRow_DifferentTypes tests various registry types
func TestRenderValueRow_DifferentTypes(t *testing.T) {
	types := []string{"REG_SZ", "REG_DWORD", "REG_BINARY", "REG_MULTI_SZ", "REG_QWORD"}

	for _, typeName := range types {
		t.Run(typeName, func(t *testing.T) {
			props := ValueRowDisplayProps{
				Name:       "TestValue",
				Type:       typeName,
				Value:      "data",
				DiffPrefix: " ",
				NameWidth:  20,
				TypeWidth:  15,
				ValueWidth: 30,
				RowStyle:   lipgloss.NewStyle(),
			}

			result := RenderValueRow(props, 80)

			if !strings.Contains(result, typeName) {
				t.Errorf("rendered output should contain type %q", typeName)
			}
		})
	}
}

// TestRenderValueRow_Selection tests selected vs unselected
func TestRenderValueRow_Selection(t *testing.T) {
	// Selected
	propsSelected := ValueRowDisplayProps{
		Name:       "Selected",
		Type:       "REG_SZ",
		Value:      "data",
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
		IsSelected: true,
	}

	resultSelected := RenderValueRow(propsSelected, 80)

	// Unselected
	propsUnselected := ValueRowDisplayProps{
		Name:       "Unselected",
		Type:       "REG_SZ",
		Value:      "data",
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
		IsSelected: false,
	}

	resultUnselected := RenderValueRow(propsUnselected, 80)

	// Both should contain names
	if !strings.Contains(resultSelected, "Selected") {
		t.Error("selected row should contain name")
	}

	if !strings.Contains(resultUnselected, "Unselected") {
		t.Error("unselected row should contain name")
	}
}

// TestRenderValueRow_LongValueTruncation tests truncation of long values
func TestRenderValueRow_LongValueTruncation(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      strings.Repeat("VeryLongValue", 20), // 260 chars
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
	}

	result := RenderValueRow(props, 80)

	// Should still render successfully
	if !strings.Contains(result, "TestValue") {
		t.Error("should contain name even with long value")
	}

	// The result should be truncated with ellipsis
	if !strings.Contains(result, "...") {
		t.Error("should contain ellipsis for truncated long value")
	}
}

// TestRenderValueRow_LongNameTruncation tests truncation of long names
func TestRenderValueRow_LongNameTruncation(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       strings.Repeat("VeryLongName", 10), // 120 chars
		Type:       "REG_SZ",
		Value:      "data",
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
	}

	result := RenderValueRow(props, 80)

	// Should still render successfully
	if !strings.Contains(result, "VeryLongName") {
		t.Error("should contain part of name even when truncated")
	}

	// The result should be truncated with ellipsis
	if !strings.Contains(result, "...") {
		t.Error("should contain ellipsis for truncated long name")
	}
}

// TestRenderValueRow_ComplexCombination tests all features together
func TestRenderValueRow_ComplexCombination(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:        "(Default)",
		Type:        "REG_MULTI_SZ",
		Value:       "oldData → newData",
		DiffPrefix:  "~",
		PrefixStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")),
		NameWidth:   20,
		TypeWidth:   15,
		ValueWidth:  40,
		RowStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")),
		IsSelected:  false,
		Index:       3,
	}

	result := RenderValueRow(props, 80)

	// Should contain all components
	if !strings.Contains(result, "(Default)") {
		t.Error("should contain (Default) name")
	}

	if !strings.Contains(result, "REG_MULTI_SZ") {
		t.Error("should contain type")
	}

	if !strings.Contains(result, "→") {
		t.Error("should contain arrow for modified value")
	}
}

// TestRenderValueRow_EmptyFields tests rendering with empty optional fields
func TestRenderValueRow_EmptyFields(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       "",
		Type:       "",
		Value:      "",
		DiffPrefix: " ",
		NameWidth:  20,
		TypeWidth:  15,
		ValueWidth: 30,
		RowStyle:   lipgloss.NewStyle(),
	}

	result := RenderValueRow(props, 80)

	// Should still render successfully (even if all fields are empty)
	// The rendering function should not panic
	if result == "" {
		t.Error("should produce some output even with empty fields")
	}
}

// TestRenderHeader_BasicRendering tests header rendering
func TestRenderHeader_BasicRendering(t *testing.T) {
	result := RenderHeader(20, 15, 30, 80)

	// Should contain column headers
	if !strings.Contains(result, "Name") {
		t.Error("header should contain 'Name'")
	}

	if !strings.Contains(result, "Type") {
		t.Error("header should contain 'Type'")
	}

	if !strings.Contains(result, "Value") {
		t.Error("header should contain 'Value'")
	}
}

// TestRenderHeader_DifferentWidths tests header with various column widths
func TestRenderHeader_DifferentWidths(t *testing.T) {
	tests := []struct {
		name       string
		nameWidth  int
		typeWidth  int
		valueWidth int
	}{
		{"small", 10, 10, 20},
		{"medium", 20, 15, 30},
		{"large", 30, 20, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderHeader(tt.nameWidth, tt.typeWidth, tt.valueWidth, 100)

			// Should still contain all headers
			if !strings.Contains(result, "Name") || !strings.Contains(result, "Type") || !strings.Contains(result, "Value") {
				t.Error("header should contain all column names regardless of width")
			}
		})
	}
}

// TestRenderSeparator_BasicRendering tests separator line rendering
func TestRenderSeparator_BasicRendering(t *testing.T) {
	result := RenderSeparator(80)

	// Should contain separator characters
	if !strings.Contains(result, "─") {
		t.Error("separator should contain horizontal line characters")
	}

	// Should be approximately the right length (allowing for potential styling)
	if len(result) < 70 {
		t.Error("separator should be approximately the specified width")
	}
}

// TestRenderSeparator_DifferentWidths tests separator with various widths
func TestRenderSeparator_DifferentWidths(t *testing.T) {
	widths := []int{40, 60, 80, 100, 120}

	for _, width := range widths {
		result := RenderSeparator(width)

		// Should produce output
		if result == "" {
			t.Errorf("separator should produce output for width %d", width)
		}

		// Should contain separator character
		if !strings.Contains(result, "─") {
			t.Errorf("separator should contain line character for width %d", width)
		}
	}
}

// TestRenderSeparator_ZeroWidth tests separator with edge case width
func TestRenderSeparator_ZeroWidth(t *testing.T) {
	result := RenderSeparator(0)

	// Should not panic and should produce empty or minimal output
	_ = result // Just verify it doesn't panic
}

// TestRenderValueRow_VerySmallColumnWidth tests truncation with very small column widths
func TestRenderValueRow_VerySmallColumnWidth(t *testing.T) {
	props := ValueRowDisplayProps{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffPrefix: " ",
		NameWidth:  2, // Very small width to trigger maxLen <= 3 branch
		TypeWidth:  2,
		ValueWidth: 3,
		RowStyle:   lipgloss.NewStyle(),
	}

	result := RenderValueRow(props, 20)

	// Should not panic with very small column widths
	if result == "" {
		t.Error("should produce output even with very small column widths")
	}
}
