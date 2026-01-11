package main

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard shortcuts
type KeyMap struct {
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
	Tab   key.Binding
	Esc   key.Binding

	// Commands
	Search            key.Binding
	GlobalValueSearch key.Binding
	Jump              key.Binding
	Copy              key.Binding
	CopyValue         key.Binding
	Export            key.Binding
	Refresh           key.Binding
	Stats             key.Binding
	Help              key.Binding
	Quit              key.Binding

	// Search navigation
	NextMatch  key.Binding
	PrevMatch  key.Binding
	FirstMatch key.Binding
	LastMatch  key.Binding

	// Tree navigation helpers
	GoToParent      key.Binding
	ExpandAll       key.Binding
	CollapseAll     key.Binding
	ExpandLevel     key.Binding
	CollapseToLevel key.Binding

	// Bookmarks
	ToggleBookmark key.Binding
	NextBookmark   key.Binding
}

// DefaultKeyMap returns the default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		// Navigation
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "collapse/go up"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "expand"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("g", "go to top"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("G", "go to bottom"),
		),

		// Actions
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "expand/collapse"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch pane"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),

		// Commands
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		GlobalValueSearch: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "global value search"),
		),
		Jump: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "jump to path"),
		),
		Copy: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "copy path"),
		),
		CopyValue: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy value"),
		),
		Export: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "export"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("f5"),
			key.WithHelp("f5", "refresh"),
		),
		Stats: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "statistics"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),

		// Search navigation
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		FirstMatch: key.NewBinding(
			key.WithKeys("ctrl+home"),
			key.WithHelp("ctrl+home", "first match"),
		),
		LastMatch: key.NewBinding(
			key.WithKeys("ctrl+end"),
			key.WithHelp("ctrl+end", "last match"),
		),

		// Tree navigation helpers
		GoToParent: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "go to parent"),
		),
		ExpandAll: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "expand all children"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "collapse all"),
		),
		ExpandLevel: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "expand siblings"),
		),
		CollapseToLevel: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "collapse to level"),
		),

		// Bookmarks
		ToggleBookmark: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "toggle bookmark"),
		),
		NextBookmark: key.NewBinding(
			key.WithKeys("B"),
			key.WithHelp("B", "next bookmark"),
		),
	}
}

// ShortHelp returns key bindings for the short help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Help,
		k.Quit,
	}
}

// FullHelp returns all key bindings for the full help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.Tab, k.Search, k.Quit},
	}
}
