package keytree

import (
	"time"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Item represents a key in the tree
type Item struct {
	// Normal mode: Only NodeID is populated (from single reader)
	NodeID hive.NodeID // For efficient lookups (normal mode)

	// Diff mode: Both OldNodeID and NewNodeID may be populated based on DiffStatus:
	//   - DiffAdded: only NewNodeID (key doesn't exist in old hive)
	//   - DiffRemoved: only OldNodeID (key doesn't exist in new hive)
	//   - DiffModified/DiffUnchanged: BOTH (key exists in both hives)
	OldNodeID hive.NodeID // NodeID in old hive (for diff mode)
	NewNodeID hive.NodeID // NodeID in new hive (for diff mode)

	Path        string
	Name        string
	Depth       int
	HasChildren bool
	SubkeyCount int       // Number of subkeys (for display)
	ValueCount  int       // Number of values (for display)
	LastWrite   time.Time // Last write timestamp (for display)
	Expanded    bool
	Parent      string
	DiffStatus  hive.DiffStatus // Diff state for comparison mode
}
