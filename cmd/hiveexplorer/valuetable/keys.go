package valuetable

import "github.com/charmbracelet/bubbles/key"

// Keys defines keyboard shortcuts for the value table
type Keys struct {
	// Navigation
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding

	// Actions
	Enter key.Binding

	// Clipboard
	CopyValue key.Binding
}
