package valuedetail

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable"
)

// TestNewValueDetailModel tests initialization with different display modes
func TestNewValueDetailModel(t *testing.T) {
	tests := []struct {
		name string
		mode DetailDisplayMode
	}{
		{"Modal mode", DetailModeModal},
		{"Pane mode", DetailModePane},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewValueDetailModel(tt.mode)

			if m.DisplayMode() != tt.mode {
				t.Errorf("expected display mode %v, got %v", tt.mode, m.DisplayMode())
			}

			if m.IsVisible() {
				t.Error("new model should not be visible")
			}

			if m.value != nil {
				t.Error("new model should have nil value")
			}
		})
	}
}

// TestShowAndHide tests showing and hiding the detail view
func TestShowAndHide(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)
	// Initialize viewport size
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if m.IsVisible() {
		t.Error("model should start hidden")
	}

	// Create a test value
	value := &valuetable.ValueRow{
		Name:  "TestValue",
		Type:  "REG_SZ",
		Value: "test string",
		Size:  11,
		Raw:   []byte("test string"),
	}

	// Show the detail
	m.Show(value)

	if !m.IsVisible() {
		t.Error("model should be visible after Show()")
	}

	if m.value != value {
		t.Error("model should store the provided value")
	}

	// Hide the detail
	m.Hide()

	if m.IsVisible() {
		t.Error("model should be hidden after Hide()")
	}

	if m.value != nil {
		t.Error("model should clear value after Hide()")
	}
}

// TestDisplayMode tests the DisplayMode getter
func TestDisplayMode(t *testing.T) {
	tests := []struct {
		name string
		mode DetailDisplayMode
	}{
		{"Modal", DetailModeModal},
		{"Pane", DetailModePane},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewValueDetailModel(tt.mode)

			if m.DisplayMode() != tt.mode {
				t.Errorf("DisplayMode() = %v, want %v", m.DisplayMode(), tt.mode)
			}
		})
	}
}

// TestUpdate tests the Update method
func TestUpdate(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)

	// Send window size message
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	updatedModel, cmd := m.Update(msg)

	if cmd != nil {
		t.Error("Update should not return a command for WindowSizeMsg")
	}

	// Type assert back to *ValueDetailModel
	updated := updatedModel.(*ValueDetailModel)

	if updated.width != 120 {
		t.Errorf("width = %d, want 120", updated.width)
	}

	if updated.height != 40 {
		t.Errorf("height = %d, want 40", updated.height)
	}
}

// TestViewWhenHidden tests that View returns empty string when hidden
func TestViewWhenHidden(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)

	view := m.View()

	if view != "" {
		t.Error("View() should return empty string when hidden")
	}
}

// TestViewWhenVisible tests that View returns content when visible
func TestViewWhenVisible(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	value := &valuetable.ValueRow{
		Name:  "TestValue",
		Type:  "REG_SZ",
		Value: "test string",
		Size:  11,
		Raw:   []byte("test string"),
	}

	m.Show(value)

	view := m.View()

	if view == "" {
		t.Error("View() should return content when visible")
	}

	// Check that the view contains the value name
	if !strings.Contains(view, "TestValue") {
		t.Error("View should contain value name")
	}

	// Check that the view contains the value type
	if !strings.Contains(view, "REG_SZ") {
		t.Error("View should contain value type")
	}
}

// TestFormatHexDump tests hex dump formatting
func TestFormatHexDump(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []string // Expected substrings in output
	}{
		{
			name:     "Empty data",
			data:     []byte{},
			expected: []string{"(empty)"},
		},
		{
			name:     "Single byte",
			data:     []byte{0x42},
			expected: []string{"00000000", "42", "|B|"},
		},
		{
			name:     "Multiple bytes",
			data:     []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f},
			expected: []string{"00000000", "48 65 6c 6c 6f", "|Hello|"},
		},
		{
			name:     "Non-printable bytes",
			data:     []byte{0x00, 0x01, 0x02, 0x03},
			expected: []string{"00000000", "00 01 02 03", "|....|"},
		},
		{
			name: "Multiple lines",
			data: []byte{
				0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
				0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
				0x10, 0x11,
			},
			expected: []string{"00000000", "00000010", "10 11"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewValueDetailModel(DetailModeModal)
			result := m.formatHexDump(tt.data)

			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("formatHexDump() output should contain %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

// TestDifferentValueTypes tests rendering different registry value types
func TestDifferentValueTypes(t *testing.T) {
	tests := []struct {
		name         string
		value        *valuetable.ValueRow
		expectedText []string // Expected substrings in the rendered view
	}{
		{
			name: "REG_SZ string value",
			value: &valuetable.ValueRow{
				Name:  "StringValue",
				Type:  "REG_SZ",
				Value: "Hello World",
				Size:  11,
				Raw:   []byte("Hello World"),
			},
			expectedText: []string{"StringValue", "REG_SZ", "String Value", "Hello World"},
		},
		{
			name: "REG_DWORD value",
			value: &valuetable.ValueRow{
				Name:  "DwordValue",
				Type:  "REG_DWORD",
				Value: "0x000000ff (255)",
				Size:  4,
				Raw:   []byte{0xff, 0x00, 0x00, 0x00},
			},
			expectedText: []string{"DwordValue", "REG_DWORD", "DWORD Value", "0x000000ff"},
		},
		{
			name: "REG_QWORD value",
			value: &valuetable.ValueRow{
				Name:  "QwordValue",
				Type:  "REG_QWORD",
				Value: "0x00000000deadbeef (3735928559)",
				Size:  8,
				Raw:   []byte{0xef, 0xbe, 0xad, 0xde, 0x00, 0x00, 0x00, 0x00},
			},
			expectedText: []string{"QwordValue", "REG_QWORD", "QWORD Value", "deadbeef"},
		},
		{
			name: "REG_BINARY value",
			value: &valuetable.ValueRow{
				Name:  "BinaryValue",
				Type:  "REG_BINARY",
				Value: "010203",
				Size:  3,
				Raw:   []byte{0x01, 0x02, 0x03},
			},
			expectedText: []string{"BinaryValue", "REG_BINARY", "Binary Data", "00000000"},
		},
		{
			name: "REG_MULTI_SZ value",
			value: &valuetable.ValueRow{
				Name:  "MultiStringValue",
				Type:  "REG_MULTI_SZ",
				Value: "value1, value2, value3",
				Size:  23,
				Raw:   []byte("value1\x00value2\x00value3\x00"),
			},
			expectedText: []string{"MultiStringValue", "REG_MULTI_SZ", "Multi-String Values", "[0]", "[1]", "[2]"},
		},
		{
			name: "Default value (empty name)",
			value: &valuetable.ValueRow{
				Name:  "",
				Type:  "REG_SZ",
				Value: "default",
				Size:  7,
				Raw:   []byte("default"),
			},
			expectedText: []string{"(Default)", "REG_SZ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewValueDetailModel(DetailModeModal)
			m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m.Show(tt.value)

			view := m.View()

			for _, expected := range tt.expectedText {
				if !strings.Contains(view, expected) {
					t.Errorf("View should contain %q\nGot view: %s", expected, view)
				}
			}
		})
	}
}

// TestModalSizing tests modal viewport sizing calculations
func TestModalSizing(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Modal should be 80% of screen minus border/padding (6 horizontal, 4 vertical)
	expectedWidth := int(float64(100)*0.8) - 6 // 74
	expectedHeight := int(float64(50)*0.8) - 4 // 36

	if m.viewport.Width != expectedWidth {
		t.Errorf("modal viewport width = %d, want %d", m.viewport.Width, expectedWidth)
	}

	if m.viewport.Height != expectedHeight {
		t.Errorf("modal viewport height = %d, want %d", m.viewport.Height, expectedHeight)
	}
}

// TestPaneSizing tests pane viewport sizing calculations
func TestPaneSizing(t *testing.T) {
	m := NewValueDetailModel(DetailModePane)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})

	// Pane should be full width minus padding (4), 1/3 height minus padding (4)
	expectedWidth := 120 - 4   // 116
	expectedHeight := 60/3 - 4 // 16

	if m.viewport.Width != expectedWidth {
		t.Errorf("pane viewport width = %d, want %d", m.viewport.Width, expectedWidth)
	}

	if m.viewport.Height != expectedHeight {
		t.Errorf("pane viewport height = %d, want %d", m.viewport.Height, expectedHeight)
	}
}

// TestViewportResize tests that viewport resizes correctly on window size change
func TestViewportResize(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)

	// Initial size
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	initialWidth := m.viewport.Width
	initialHeight := m.viewport.Height

	// Resize
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 100})
	newWidth := m.viewport.Width
	newHeight := m.viewport.Height

	if newWidth <= initialWidth {
		t.Error("viewport width should increase after window size increase")
	}

	if newHeight <= initialHeight {
		t.Error("viewport height should increase after window size increase")
	}
}

// TestContentUpdatesOnShow tests that content is updated when showing a new value
func TestContentUpdatesOnShow(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// First value
	value1 := &valuetable.ValueRow{
		Name:  "Value1",
		Type:  "REG_SZ",
		Value: "first",
		Size:  5,
		Raw:   []byte("first"),
	}
	m.Show(value1)
	view1 := m.View()

	// Second value
	value2 := &valuetable.ValueRow{
		Name:  "Value2",
		Type:  "REG_SZ",
		Value: "second",
		Size:  6,
		Raw:   []byte("second"),
	}
	m.Show(value2)
	view2 := m.View()

	// Views should be different
	if view1 == view2 {
		t.Error("views should be different for different values")
	}

	// Second view should contain second value's name
	if !strings.Contains(view2, "Value2") {
		t.Error("view should contain second value's name")
	}

	// Second view should not contain first value's name
	if strings.Contains(view2, "Value1") {
		t.Error("view should not contain first value's name after showing second value")
	}
}

// TestRawDataDisplay tests that raw hex data is always shown
func TestRawDataDisplay(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	value := &valuetable.ValueRow{
		Name:  "TestValue",
		Type:  "REG_SZ",
		Value: "test",
		Size:  4,
		Raw:   []byte{0x74, 0x65, 0x73, 0x74}, // "test" in hex
	}
	m.Show(value)

	view := m.View()

	// Should contain "Raw Data (Hex)" section
	if !strings.Contains(view, "Raw Data (Hex)") {
		t.Error("view should contain 'Raw Data (Hex)' section")
	}

	// Should contain hex dump with hex values
	if !strings.Contains(view, "74 65 73 74") {
		t.Error("view should contain hex representation of data")
	}
}

// TestEmptyValue tests handling of values with no data
func TestEmptyValue(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	value := &valuetable.ValueRow{
		Name:  "EmptyValue",
		Type:  "REG_SZ",
		Value: "",
		Size:  0,
		Raw:   []byte{},
	}
	m.Show(value)

	view := m.View()

	// Should not panic and should render something
	if view == "" {
		t.Error("view should not be empty even for empty value")
	}

	// Should contain the value name
	if !strings.Contains(view, "EmptyValue") {
		t.Error("view should contain value name")
	}
}

// TestInit tests the Init method
func TestInit(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)

	cmd := m.Init()

	if cmd != nil {
		t.Error("Init() should return nil command")
	}
}

// TestUpdateWithNilValue tests that Update handles nil value gracefully
func TestUpdateWithNilValue(t *testing.T) {
	m := NewValueDetailModel(DetailModeModal)

	// Update without showing a value
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if cmd != nil {
		t.Error("Update should not return command")
	}

	// View should be empty
	view := m.View()
	if view != "" {
		t.Error("view should be empty when no value is shown")
	}
}
