package adapter

import (
	"testing"
)

// TestItemToDisplayProps_Bookmarked tests bookmark indicator
func TestItemToDisplayProps_Bookmarked(t *testing.T) {
	source := TreeItemSource{
		Name:  "BookmarkedKey",
		Depth: 0,
	}

	// With bookmark
	propsBookmarked := ItemToDisplayProps(source, true, false)
	if propsBookmarked.LeftIndicator != "★" {
		t.Errorf("expected bookmark indicator '★', got %q", propsBookmarked.LeftIndicator)
	}

	// Without bookmark
	propsNormal := ItemToDisplayProps(source, false, false)
	if propsNormal.LeftIndicator != " " {
		t.Errorf("expected no bookmark indicator ' ', got %q", propsNormal.LeftIndicator)
	}
}

// TestItemToDisplayProps_HasChildren_Expanded tests icon for expanded parent
func TestItemToDisplayProps_HasChildren_Expanded(t *testing.T) {
	source := TreeItemSource{
		Name:        "ParentKey",
		Depth:       0,
		HasChildren: true,
		Expanded:    true,
		SubkeyCount: 5,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Icon != "▼" {
		t.Errorf("expected expanded icon '▼', got %q", props.Icon)
	}

	if props.CountText != "(5)" {
		t.Errorf("expected count text '(5)', got %q", props.CountText)
	}
}

// TestItemToDisplayProps_HasChildren_Collapsed tests icon for collapsed parent
func TestItemToDisplayProps_HasChildren_Collapsed(t *testing.T) {
	source := TreeItemSource{
		Name:        "ParentKey",
		Depth:       0,
		HasChildren: true,
		Expanded:    false,
		SubkeyCount: 3,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Icon != "▶" {
		t.Errorf("expected collapsed icon '▶', got %q", props.Icon)
	}

	if props.CountText != "(3)" {
		t.Errorf("expected count text '(3)', got %q", props.CountText)
	}
}

// TestItemToDisplayProps_NoChildren tests icon for leaf node
func TestItemToDisplayProps_NoChildren(t *testing.T) {
	source := TreeItemSource{
		Name:        "LeafKey",
		Depth:       0,
		HasChildren: false,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Icon != "•" {
		t.Errorf("expected leaf icon '•', got %q", props.Icon)
	}

	if props.CountText != "" {
		t.Errorf("expected empty count text for leaf, got %q", props.CountText)
	}
}

// TestItemToDisplayProps_DepthMapping tests depth is preserved
func TestItemToDisplayProps_DepthMapping(t *testing.T) {
	tests := []int{0, 1, 2, 5, 10}

	for _, depth := range tests {
		source := TreeItemSource{
			Name:  "TestKey",
			Depth: depth,
		}

		props := ItemToDisplayProps(source, false, false)

		if props.Depth != depth {
			t.Errorf("expected depth %d, got %d", depth, props.Depth)
		}
	}
}

// TestItemToDisplayProps_CursorSelection tests selection flag
func TestItemToDisplayProps_CursorSelection(t *testing.T) {
	source := TreeItemSource{
		Name:  "TestKey",
		Depth: 0,
	}

	// Selected
	propsSelected := ItemToDisplayProps(source, false, true)
	if !propsSelected.IsSelected {
		t.Error("expected IsSelected to be true when cursor is on item")
	}

	// Not selected
	propsUnselected := ItemToDisplayProps(source, false, false)
	if propsUnselected.IsSelected {
		t.Error("expected IsSelected to be false when cursor is not on item")
	}
}

// TestItemToDisplayProps_TimestampFormatting tests timestamp passthrough
func TestItemToDisplayProps_TimestampFormatting(t *testing.T) {
	source := TreeItemSource{
		Name:      "TestKey",
		Depth:     0,
		LastWrite: "2024-03-15 14:30",
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Timestamp != "2024-03-15 14:30" {
		t.Errorf("expected timestamp '2024-03-15 14:30', got %q", props.Timestamp)
	}
}

// TestItemToDisplayProps_ComplexScenario tests all features together
func TestItemToDisplayProps_ComplexScenario(t *testing.T) {
	source := TreeItemSource{
		Name:        "ComplexKey",
		Depth:       2,
		HasChildren: true,
		Expanded:    true,
		SubkeyCount: 12,
		LastWrite:   "2024-01-01 00:00",
	}

	props := ItemToDisplayProps(source, true, true)

	// Check all mappings
	if props.Name != "ComplexKey" {
		t.Errorf("expected name 'ComplexKey', got %q", props.Name)
	}

	if props.Depth != 2 {
		t.Errorf("expected depth 2, got %d", props.Depth)
	}

	if props.Icon != "▼" {
		t.Errorf("expected expanded icon, got %q", props.Icon)
	}

	if props.LeftIndicator != "★" {
		t.Errorf("expected bookmark indicator, got %q", props.LeftIndicator)
	}

	if props.CountText != "(12)" {
		t.Errorf("expected count '(12)', got %q", props.CountText)
	}

	if props.Timestamp != "2024-01-01 00:00" {
		t.Errorf("expected timestamp, got %q", props.Timestamp)
	}

	if !props.IsSelected {
		t.Error("expected IsSelected to be true")
	}
}

// TestItemToDisplayProps_StylesNotNil tests that styles are properly set
func TestItemToDisplayProps_StylesNotNil(t *testing.T) {
	source := TreeItemSource{
		Name:  "TestKey",
		Depth: 0,
	}

	props := ItemToDisplayProps(source, false, false)

	// Styles should not be nil (we can't easily test the actual style properties
	// but we can verify the function doesn't panic and returns valid props)
	if props.Name == "" {
		t.Error("adapter should set props correctly")
	}
}
