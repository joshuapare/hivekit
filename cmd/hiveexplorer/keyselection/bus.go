package keyselection

import (
	"context"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Event represents a key selection event when the user selects a different registry key.
// It includes the NodeID for efficient value/metadata lookups, the hive path for
// loading from the correct hive (important for diff/merge), and a context for cancellation.
//
// In diff mode, it includes additional context for loading values from the appropriate hive:
// - DiffMode indicates whether diff comparison is active
// - DiffStatus indicates whether this key was Added/Removed/Modified/Unchanged
// - OldNodeID and NewNodeID are the NodeIDs in respective hives (may be 0 if key doesn't exist)
// - OldReader and NewReader provide access to both hives for comparison
type Event struct {
	// Normal mode: Only NodeID is used
	NodeID   hive.NodeID
	Path     string          // Key path (e.g., "SOFTWARE\Microsoft")
	HivePath string          // Path to hive file (for diff/merge support)
	Ctx      context.Context // For cancelling in-progress loads

	// Diff mode context (nil/zero values when not in diff mode)
	DiffMode   bool            // Whether diff comparison is active
	DiffStatus hive.DiffStatus // Key's diff status (Added/Removed/Modified/Unchanged)
	OldNodeID  hive.NodeID     // NodeID in old hive (0 if key doesn't exist there)
	NewNodeID  hive.NodeID     // NodeID in new hive (0 if key doesn't exist there)
	OldReader  hive.Reader     // Reader for old hive (original)
	NewReader  hive.Reader     // Reader for new hive (comparison)
}

// Bus coordinates key selection events between the tree and value/metadata loaders.
// When a key is selected in the tree, this notifies value table and key info display
// to load data for the new key, with cancellation support for rapid navigation.
type Bus struct {
	subscribers []chan Event
	cancel      context.CancelFunc // Cancels the current load operation
}

// NewBus creates a new key selection bus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make([]chan Event, 0),
	}
}

// Subscribe returns a channel that receives key selection events.
// Components (value table, key info display) subscribe to be notified when a key is selected.
func (b *Bus) Subscribe() <-chan Event {
	ch := make(chan Event, 1) // Buffered to prevent blocking
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Notify broadcasts a key selection event to all subscribers.
// It cancels any previous load operation and creates a new context for the new selection.
// This allows components to cancel in-progress loads when the user navigates quickly.
//
// For diff mode support, pass diffMode=true along with diffStatus, both NodeIDs, and both readers.
// When not in diff mode, pass diffMode=false, 0 for old/new NodeIDs, and nil for the readers.
func (b *Bus) Notify(
	nodeID hive.NodeID,
	path string,
	hivePath string,
	diffMode bool,
	diffStatus hive.DiffStatus,
	oldNodeID, newNodeID hive.NodeID,
	oldReader, newReader hive.Reader,
) {
	// Cancel previous load if any
	if b.cancel != nil {
		b.cancel()
	}

	// Create new context for this selection
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	event := Event{
		NodeID:     nodeID,
		Path:       path,
		HivePath:   hivePath,
		Ctx:        ctx,
		DiffMode:   diffMode,
		DiffStatus: diffStatus,
		OldNodeID:  oldNodeID,
		NewNodeID:  newNodeID,
		OldReader:  oldReader,
		NewReader:  newReader,
	}

	// Broadcast to all subscribers
	for _, ch := range b.subscribers {
		select {
		case ch <- event:
			// Sent successfully
		default:
			// Channel full, skip (subscriber is slow)
			// This prevents blocking the UI thread
		}
	}
}

// Close closes all subscriber channels.
// This should be called when the TUI is shutting down.
func (b *Bus) Close() {
	if b.cancel != nil {
		b.cancel()
	}
	for _, ch := range b.subscribers {
		close(ch)
	}
}
