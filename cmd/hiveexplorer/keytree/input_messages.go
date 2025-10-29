package keytree

// Input-related messages that the keytree component emits for coordination with main Model

// CopyPathRequestedMsg is emitted when the user requests to copy the current path
type CopyPathRequestedMsg struct {
	Path    string
	Success bool
	Err     error
}

// BookmarkToggledMsg is emitted when the user toggles a bookmark
type BookmarkToggledMsg struct {
	Path  string
	Added bool // true if bookmark was added, false if removed
}
