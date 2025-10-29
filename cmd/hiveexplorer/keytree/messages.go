package keytree

import "time"

// ErrMsg represents an error message
type ErrMsg struct {
	Err error
}

func (e ErrMsg) Error() string {
	return e.Err.Error()
}

// TreeLoadedMsg is sent when the entire tree has been loaded
type TreeLoadedMsg struct {
	Items  []Item
	Reader interface{ Close() error }
}

// RootKeysLoadedMsg is sent when root keys have been loaded
type RootKeysLoadedMsg struct {
	Keys []KeyInfo
}

// ChildKeysLoadedMsg is sent when child keys have been loaded
type ChildKeysLoadedMsg struct {
	Parent string
	Keys   []KeyInfo
}

// KeyInfo is a simplified version of hive.KeyInfo for messages
type KeyInfo struct {
	Path      string
	Name      string
	SubkeyN   int
	ValueN    int
	LastWrite time.Time
}
