package keyselection

import (
	"context"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// Event represents a key selection event when the user selects a different registry key.
// It includes the NodeID for efficient value/metadata lookups, the hive path for
// loading from the correct hive, and a context for cancellation.
type Event struct {
	NodeID   hive.NodeID
	Path     string          // Key path (e.g., "SOFTWARE\Microsoft")
	HivePath string          // Path to hive file
	Ctx      context.Context // For cancelling in-progress loads
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
func (b *Bus) Notify(nodeID hive.NodeID, path string, hivePath string) {
	// Cancel previous load if any
	if b.cancel != nil {
		b.cancel()
	}

	// Create new context for this selection
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel

	event := Event{
		NodeID:   nodeID,
		Path:     path,
		HivePath: hivePath,
		Ctx:      ctx,
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
