package adapter

import (
	"fmt"

	"github.com/joshuapare/hivekit/cmd/hiveexplorer/keytree/display"
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
}

// ItemToDisplayProps converts domain Item data to pure display props.
// This is where ALL business logic lives - the display layer is completely dumb.
//
// This function understands:
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
		Name:        source.Name,
		Depth:       source.Depth,
		IsSelected:  isCursor,
		Timestamp:   source.LastWrite,
		Prefix:      " ", // Default prefix (space for alignment)
		PrefixStyle: normalPrefixStyle,
		ItemStyle:   normalItemStyle,
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
