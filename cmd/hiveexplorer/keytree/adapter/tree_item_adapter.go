package adapter

import (
	"fmt"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree/display"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TreeItemSource represents the domain data for a tree item.
// This is separate from the keytree.Item to avoid circular dependencies.
type TreeItemSource struct {
	Name        string
	Depth       int
	HasChildren bool
	Expanded    bool
	SubkeyCount int
	LastWrite   string // Pre-formatted timestamp from domain
	DiffStatus  hive.DiffStatus
}

// ItemToDisplayProps converts domain Item data to pure display props.
// This is where ALL business logic lives - the display layer is completely dumb.
//
// This function understands:
// - DiffStatus enum and what each value means visually
// - Bookmark semantics
// - Tree expansion state
// - Icon selection rules
// - Style selection rules
func ItemToDisplayProps(
	source TreeItemSource,
	isBookmarked bool,
	isCursor bool,
) display.TreeItemDisplayProps {
	props := display.TreeItemDisplayProps{
		Name:       source.Name,
		Depth:      source.Depth,
		IsSelected: isCursor,
		Timestamp:  source.LastWrite,
	}

	// Icon selection based on tree state
	if source.HasChildren {
		if source.Expanded {
			props.Icon = expandedIcon
			props.CountText = fmt.Sprintf("(%d)", source.SubkeyCount)
		} else {
			props.Icon = collapsedIcon
			props.CountText = fmt.Sprintf("(%d)", source.SubkeyCount)
		}
	} else {
		props.Icon = emptyIcon
		props.CountText = ""
	}

	// Bookmark indicator
	if isBookmarked {
		props.LeftIndicator = bookmarkedIndicator
	} else {
		props.LeftIndicator = unbookmarkedIndicator
	}

	// DiffStatus → visual style mapping
	// This is domain logic: we understand what "Added" means and choose visual representation
	switch source.DiffStatus {
	case hive.DiffAdded:
		props.Prefix = addedPrefix
		props.PrefixStyle = addedPrefixStyle
		props.ItemStyle = addedItemStyle

	case hive.DiffRemoved:
		props.Prefix = removedPrefix
		props.PrefixStyle = removedPrefixStyle
		props.ItemStyle = removedItemStyle

	case hive.DiffModified:
		props.Prefix = modifiedPrefix
		props.PrefixStyle = modifiedPrefixStyle
		props.ItemStyle = modifiedItemStyle

	case hive.DiffUnchanged:
		props.Prefix = unchangedPrefix
		props.PrefixStyle = unchangedPrefixStyle
		props.ItemStyle = unchangedItemStyle

	default:
		// Default to unchanged style
		props.Prefix = unchangedPrefix
		props.PrefixStyle = unchangedPrefixStyle
		props.ItemStyle = unchangedItemStyle
	}

	return props
}

// Icon constants (semantic names, defined here in adapter)
const (
	expandedIcon          = "▼"
	collapsedIcon         = "▶"
	emptyIcon             = "•"
	bookmarkedIndicator   = "★"
	unbookmarkedIndicator = " "
)

// Diff prefix constants
const (
	addedPrefix     = "+"
	removedPrefix   = "-"
	modifiedPrefix  = "~"
	unchangedPrefix = " "
)
