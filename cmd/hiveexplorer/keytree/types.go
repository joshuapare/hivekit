package keytree

import (
	"time"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Item represents a key in the tree
type Item struct {
	NodeID      hive.NodeID // For efficient lookups
	Path        string
	Name        string
	Depth       int
	HasChildren bool
	SubkeyCount int       // Number of subkeys (for display)
	ValueCount  int       // Number of values (for display)
	LastWrite   time.Time // Last write timestamp (for display)
	Expanded    bool
	Parent      string
}
