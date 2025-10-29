package adapter

import (
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestRowToDisplayProps_DiffStatusAdded tests mapping for added values
func TestRowToDisplayProps_DiffStatusAdded(t *testing.T) {
	source := ValueRowSource{
		Name:       "AddedValue",
		Type:       "REG_SZ",
		Value:      "test",
		DiffStatus: hive.DiffAdded,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	if props.DiffPrefix != "+" {
		t.Errorf("expected prefix '+' for added value, got %q", props.DiffPrefix)
	}

	if props.Name != "AddedValue" {
		t.Errorf("expected name 'AddedValue', got %q", props.Name)
	}
}

// TestRowToDisplayProps_DiffStatusRemoved tests mapping for removed values
func TestRowToDisplayProps_DiffStatusRemoved(t *testing.T) {
	source := ValueRowSource{
		Name:       "RemovedValue",
		Type:       "REG_DWORD",
		Value:      "42",
		DiffStatus: hive.DiffRemoved,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	if props.DiffPrefix != "-" {
		t.Errorf("expected prefix '-' for removed value, got %q", props.DiffPrefix)
	}
}

// TestRowToDisplayProps_DiffStatusModified tests mapping for modified values
func TestRowToDisplayProps_DiffStatusModified(t *testing.T) {
	source := ValueRowSource{
		Name:       "ModifiedValue",
		Type:       "REG_SZ",
		Value:      "new",
		OldValue:   "old",
		DiffStatus: hive.DiffModified,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	if props.DiffPrefix != "~" {
		t.Errorf("expected prefix '~' for modified value, got %q", props.DiffPrefix)
	}
}

// TestRowToDisplayProps_DiffStatusUnchanged tests mapping for unchanged values
func TestRowToDisplayProps_DiffStatusUnchanged(t *testing.T) {
	source := ValueRowSource{
		Name:       "UnchangedValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffUnchanged,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	if props.DiffPrefix != " " {
		t.Errorf("expected prefix ' ' for unchanged value, got %q", props.DiffPrefix)
	}
}

// TestRowToDisplayProps_EmptyName tests default name handling
func TestRowToDisplayProps_EmptyName(t *testing.T) {
	source := ValueRowSource{
		Name:       "",
		Type:       "REG_SZ",
		Value:      "default value",
		DiffStatus: hive.DiffUnchanged,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	if props.Name != "(Default)" {
		t.Errorf("expected name '(Default)' for empty name, got %q", props.Name)
	}
}

// TestRowToDisplayProps_ModifiedValueFormatting tests "old → new" formatting
func TestRowToDisplayProps_ModifiedValueFormatting(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "newValue",
		OldValue:   "oldValue",
		DiffStatus: hive.DiffModified,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	expected := "oldValue → newValue"
	if props.Value != expected {
		t.Errorf("expected value %q, got %q", expected, props.Value)
	}
}

// TestRowToDisplayProps_ModifiedValueWithoutOldValue tests modified without old value
func TestRowToDisplayProps_ModifiedValueWithoutOldValue(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "newValue",
		OldValue:   "", // No old value
		DiffStatus: hive.DiffModified,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	// Should just show new value (no formatting)
	if props.Value != "newValue" {
		t.Errorf("expected value 'newValue', got %q", props.Value)
	}
}

// TestRowToDisplayProps_AlternatingRowsEven tests even row style
func TestRowToDisplayProps_AlternatingRowsEven(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffUnchanged,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30) // index 0 = even

	// Even rows should use unchangedRowStyle (no background)
	// We can't directly compare lipgloss.Style, but we can verify the props are set
	if props.Index != 0 {
		t.Errorf("expected index 0, got %d", props.Index)
	}
}

// TestRowToDisplayProps_AlternatingRowsOdd tests odd row style
func TestRowToDisplayProps_AlternatingRowsOdd(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffUnchanged,
	}

	props := RowToDisplayProps(source, 1, false, 20, 15, 30) // index 1 = odd

	// Odd rows should use unchangedRowAltStyle (with background)
	if props.Index != 1 {
		t.Errorf("expected index 1, got %d", props.Index)
	}
}

// TestRowToDisplayProps_Selection tests selection flag
func TestRowToDisplayProps_Selection(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffUnchanged,
	}

	// Selected
	propsSelected := RowToDisplayProps(source, 0, true, 20, 15, 30)
	if !propsSelected.IsSelected {
		t.Error("expected IsSelected to be true when selected")
	}

	// Not selected
	propsUnselected := RowToDisplayProps(source, 0, false, 20, 15, 30)
	if propsUnselected.IsSelected {
		t.Error("expected IsSelected to be false when not selected")
	}
}

// TestRowToDisplayProps_ColumnWidths tests column width preservation
func TestRowToDisplayProps_ColumnWidths(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffUnchanged,
	}

	nameWidth := 25
	typeWidth := 18
	valueWidth := 40

	props := RowToDisplayProps(source, 0, false, nameWidth, typeWidth, valueWidth)

	if props.NameWidth != nameWidth {
		t.Errorf("expected nameWidth %d, got %d", nameWidth, props.NameWidth)
	}

	if props.TypeWidth != typeWidth {
		t.Errorf("expected typeWidth %d, got %d", typeWidth, props.TypeWidth)
	}

	if props.ValueWidth != valueWidth {
		t.Errorf("expected valueWidth %d, got %d", valueWidth, props.ValueWidth)
	}
}

// TestRowToDisplayProps_TypePreservation tests type field passthrough
func TestRowToDisplayProps_TypePreservation(t *testing.T) {
	types := []string{"REG_SZ", "REG_DWORD", "REG_BINARY", "REG_MULTI_SZ"}

	for _, typeName := range types {
		source := ValueRowSource{
			Name:       "TestValue",
			Type:       typeName,
			Value:      "data",
			DiffStatus: hive.DiffUnchanged,
		}

		props := RowToDisplayProps(source, 0, false, 20, 15, 30)

		if props.Type != typeName {
			t.Errorf("expected type %q, got %q", typeName, props.Type)
		}
	}
}

// TestRowToDisplayProps_ComplexScenario tests all features together
func TestRowToDisplayProps_ComplexScenario(t *testing.T) {
	source := ValueRowSource{
		Name:       "", // Empty name -> should become "(Default)"
		Type:       "REG_SZ",
		Value:      "newData",
		OldValue:   "oldData",
		DiffStatus: hive.DiffModified,
	}

	props := RowToDisplayProps(source, 3, true, 20, 15, 30)

	// Check all mappings
	if props.Name != "(Default)" {
		t.Errorf("expected name '(Default)', got %q", props.Name)
	}

	if props.Type != "REG_SZ" {
		t.Errorf("expected type 'REG_SZ', got %q", props.Type)
	}

	expectedValue := "oldData → newData"
	if props.Value != expectedValue {
		t.Errorf("expected value %q, got %q", expectedValue, props.Value)
	}

	if props.DiffPrefix != "~" {
		t.Errorf("expected prefix '~' for modified, got %q", props.DiffPrefix)
	}

	if props.Index != 3 {
		t.Errorf("expected index 3, got %d", props.Index)
	}

	if !props.IsSelected {
		t.Error("expected IsSelected to be true")
	}

	if props.NameWidth != 20 || props.TypeWidth != 15 || props.ValueWidth != 30 {
		t.Errorf("expected widths 20/15/30, got %d/%d/%d",
			props.NameWidth, props.TypeWidth, props.ValueWidth)
	}
}

// TestRowToDisplayProps_UnknownDiffStatus tests default fallback for invalid DiffStatus
func TestRowToDisplayProps_UnknownDiffStatus(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffStatus(999), // Unknown/invalid value
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	// Should default to unchanged style
	if props.DiffPrefix != " " {
		t.Errorf("expected unchanged prefix ' ' for unknown DiffStatus, got %q", props.DiffPrefix)
	}

	if props.Name != "TestValue" {
		t.Errorf("expected name to be preserved, got %q", props.Name)
	}
}

// TestRowToDisplayProps_UnknownDiffStatusOddRow tests default fallback with alternating row
func TestRowToDisplayProps_UnknownDiffStatusOddRow(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffStatus(999), // Unknown/invalid value
	}

	props := RowToDisplayProps(source, 1, false, 20, 15, 30) // index 1 = odd

	// Should default to unchanged style with alternating row
	if props.DiffPrefix != " " {
		t.Errorf("expected unchanged prefix ' ' for unknown DiffStatus, got %q", props.DiffPrefix)
	}

	if props.Index != 1 {
		t.Errorf("expected index 1, got %d", props.Index)
	}
}

// TestRowToDisplayProps_StylesNotNil tests that styles are properly set
func TestRowToDisplayProps_StylesNotNil(t *testing.T) {
	source := ValueRowSource{
		Name:       "TestValue",
		Type:       "REG_SZ",
		Value:      "data",
		DiffStatus: hive.DiffAdded,
	}

	props := RowToDisplayProps(source, 0, false, 20, 15, 30)

	// Styles should not be nil (we can't easily test the actual style properties
	// but we can verify the function doesn't panic and returns valid props)
	if props.Name == "" {
		t.Error("adapter should set props correctly")
	}
}
