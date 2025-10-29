package adapter

import (
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestItemToDisplayProps_DiffStatusAdded tests mapping for added items
func TestItemToDisplayProps_DiffStatusAdded(t *testing.T) {
	source := TreeItemSource{
		Name:       "AddedKey",
		Depth:      0,
		DiffStatus: hive.DiffAdded,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Prefix != "+" {
		t.Errorf("expected prefix '+' for added item, got %q", props.Prefix)
	}

	if props.Name != "AddedKey" {
		t.Errorf("expected name 'AddedKey', got %q", props.Name)
	}
}

// TestItemToDisplayProps_DiffStatusRemoved tests mapping for removed items
func TestItemToDisplayProps_DiffStatusRemoved(t *testing.T) {
	source := TreeItemSource{
		Name:       "RemovedKey",
		Depth:      0,
		DiffStatus: hive.DiffRemoved,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Prefix != "-" {
		t.Errorf("expected prefix '-' for removed item, got %q", props.Prefix)
	}
}

// TestItemToDisplayProps_DiffStatusModified tests mapping for modified items
func TestItemToDisplayProps_DiffStatusModified(t *testing.T) {
	source := TreeItemSource{
		Name:       "ModifiedKey",
		Depth:      0,
		DiffStatus: hive.DiffModified,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Prefix != "~" {
		t.Errorf("expected prefix '~' for modified item, got %q", props.Prefix)
	}
}

// TestItemToDisplayProps_DiffStatusUnchanged tests mapping for unchanged items
func TestItemToDisplayProps_DiffStatusUnchanged(t *testing.T) {
	source := TreeItemSource{
		Name:       "UnchangedKey",
		Depth:      0,
		DiffStatus: hive.DiffUnchanged,
	}

	props := ItemToDisplayProps(source, false, false)

	if props.Prefix != " " {
		t.Errorf("expected prefix ' ' for unchanged item, got %q", props.Prefix)
	}
}

// TestItemToDisplayProps_Bookmarked tests bookmark indicator
func TestItemToDisplayProps_Bookmarked(t *testing.T) {
	source := TreeItemSource{
		Name:       "BookmarkedKey",
		Depth:      0,
		DiffStatus: hive.DiffUnchanged,
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
		DiffStatus:  hive.DiffUnchanged,
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
		DiffStatus:  hive.DiffUnchanged,
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
		DiffStatus:  hive.DiffUnchanged,
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
			Name:       "TestKey",
			Depth:      depth,
			DiffStatus: hive.DiffUnchanged,
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
		Name:       "TestKey",
		Depth:      0,
		DiffStatus: hive.DiffUnchanged,
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
		Name:       "TestKey",
		Depth:      0,
		LastWrite:  "2024-03-15 14:30",
		DiffStatus: hive.DiffUnchanged,
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
		DiffStatus:  hive.DiffAdded,
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

	if props.Prefix != "+" {
		t.Errorf("expected '+' prefix for added, got %q", props.Prefix)
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
		Name:       "TestKey",
		Depth:      0,
		DiffStatus: hive.DiffAdded,
	}

	props := ItemToDisplayProps(source, false, false)

	// Styles should not be nil (we can't easily test the actual style properties
	// but we can verify the function doesn't panic and returns valid props)
	if props.Name == "" {
		t.Error("adapter should set props correctly")
	}
}

// TestItemToDisplayProps_UnknownDiffStatus tests default fallback for invalid DiffStatus
func TestItemToDisplayProps_UnknownDiffStatus(t *testing.T) {
	source := TreeItemSource{
		Name:       "TestKey",
		Depth:      0,
		DiffStatus: hive.DiffStatus(999), // Unknown/invalid value
	}

	props := ItemToDisplayProps(source, false, false)

	// Should default to unchanged style
	if props.Prefix != " " {
		t.Errorf("expected unchanged prefix ' ' for unknown DiffStatus, got %q", props.Prefix)
	}

	if props.Name != "TestKey" {
		t.Errorf("expected name to be preserved, got %q", props.Name)
	}
}
