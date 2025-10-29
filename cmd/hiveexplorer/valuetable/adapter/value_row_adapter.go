package adapter

import (
	"fmt"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/valuetable/display"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// ValueRowSource represents the domain data for a value table row.
// This is separate from the tui.ValueRow to avoid circular dependencies.
type ValueRowSource struct {
	Name       string
	Type       string
	Value      string
	DiffStatus hive.DiffStatus
	OldValue   string // Previous value (for DiffModified)
	OldType    string // Previous type (for DiffModified)
}

// RowToDisplayProps converts domain ValueRow data to pure display props.
// This is where ALL business logic lives - the display layer is completely dumb.
//
// This function understands:
// - DiffStatus enum and what each value means visually
// - Modified value display format ("old → new")
// - Default name handling ("(Default)" for empty names)
// - Alternating row colors
// - Style selection rules
//
// Parameters:
//   - source: Domain data from ValueRow
//   - index: Row index (for alternating colors)
//   - isSelected: Whether this row is currently selected
//   - nameWidth, typeWidth, valueWidth: Column widths for truncation
//
// Returns: Display properties ready for pure rendering
func RowToDisplayProps(
	source ValueRowSource,
	index int,
	isSelected bool,
	nameWidth, typeWidth, valueWidth int,
) display.ValueRowDisplayProps {
	props := display.ValueRowDisplayProps{
		Name:       source.Name,
		Type:       source.Type,
		Value:      source.Value,
		NameWidth:  nameWidth,
		TypeWidth:  typeWidth,
		ValueWidth: valueWidth,
		IsSelected: isSelected,
		Index:      index,
	}

	// Handle empty name (default value)
	if props.Name == "" {
		props.Name = "(Default)"
	}

	// Handle modified values - show "old → new" format
	if source.DiffStatus == hive.DiffModified && source.OldValue != "" {
		// Format: "old → new"
		// Note: We don't truncate here - the display layer will handle truncation
		// We just prepare the combined string
		props.Value = fmt.Sprintf("%s → %s", source.OldValue, source.Value)
	}

	// DiffStatus → visual style mapping
	// This is domain logic: we understand what "Added" means and choose visual representation
	switch source.DiffStatus {
	case hive.DiffAdded:
		props.DiffPrefix = addedPrefix
		props.PrefixStyle = addedPrefixStyle
		props.RowStyle = addedRowStyle

	case hive.DiffRemoved:
		props.DiffPrefix = removedPrefix
		props.PrefixStyle = removedPrefixStyle
		props.RowStyle = removedRowStyle

	case hive.DiffModified:
		props.DiffPrefix = modifiedPrefix
		props.PrefixStyle = modifiedPrefixStyle
		props.RowStyle = modifiedRowStyle

	case hive.DiffUnchanged:
		props.DiffPrefix = unchangedPrefix
		props.PrefixStyle = unchangedPrefixStyle
		// For unchanged items, use alternating row styles based on index
		// This provides visual separation without domain meaning
		if index%2 == 0 {
			props.RowStyle = unchangedRowStyle
		} else {
			props.RowStyle = unchangedRowAltStyle
		}

	default:
		// Default to unchanged style with alternating rows
		props.DiffPrefix = unchangedPrefix
		props.PrefixStyle = unchangedPrefixStyle
		if index%2 == 0 {
			props.RowStyle = unchangedRowStyle
		} else {
			props.RowStyle = unchangedRowAltStyle
		}
	}

	return props
}

// Diff prefix constants
const (
	addedPrefix     = "+"
	removedPrefix   = "-"
	modifiedPrefix  = "~"
	unchangedPrefix = " "
)
