package keytree

import "github.com/charmbracelet/bubbles/key"

// Keys defines keyboard shortcuts for the key tree
type Keys struct {
	// Navigation
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding

	// Actions
	Enter key.Binding

	// Tree operations
	GoToParent      key.Binding
	ExpandAll       key.Binding
	CollapseAll     key.Binding
	ExpandLevel     key.Binding
	CollapseToLevel key.Binding

	// Clipboard
	Copy key.Binding

	// Bookmarks
	ToggleBookmark key.Binding
}
