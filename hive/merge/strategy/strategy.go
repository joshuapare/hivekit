// Package strategy defines write strategies for merge operations.
//
// Strategy Interface:
// All strategies must implement the Strategy interface, which provides
// methods for applying merge operations (EnsureKey, SetValue, DeleteValue, DeleteKey).
//
// Each strategy must properly track dirty ranges via DirtyTracker so that
// transaction commits can flush the correct pages.
//
// Available Strategies:
//   - InPlace: Mutate cells in-place when possible (may fragment over time)
//   - Append: Always allocate new cells, never free (safe, higher space usage)
//   - Hybrid: Heuristic-based selection between InPlace and Append
package strategy

import (
	"context"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/edit"
	"github.com/joshuapare/hivekit/hive/index"
)

// Strategy defines how merge operations are applied to a hive.
//
// All strategies must properly track dirty ranges via DirtyTracker to ensure
// transaction commits flush the correct pages.
//
// All methods accept a context for cancellation support. If the context is
// cancelled, operations should return the context error as soon as practical.
//
// Implementations:
//   - InPlace: Direct mutation (may reuse cells)
//   - Append: Append-only (never free cells)
//   - Hybrid: Heuristic selection
type Strategy interface {
	// EnsureKey ensures a key path exists, creating keys as needed.
	// Returns the NK cell reference, the count of keys created, and any error.
	// The context can be used to cancel the operation.
	EnsureKey(ctx context.Context, path []string) (nkRef uint32, keysCreated int, err error)

	// SetValue sets a value (creates or updates).
	// The path specifies the parent key, name is the value name.
	// The context can be used to cancel the operation.
	SetValue(ctx context.Context, path []string, name string, typ uint32, data []byte) error

	// DeleteValue removes a value from a key.
	// If the value doesn't exist, this is a no-op (idempotent).
	// The context can be used to cancel the operation.
	DeleteValue(ctx context.Context, path []string, name string) error

	// DeleteKey removes a key and optionally its subkeys (if recursive=true).
	// If the key doesn't exist, this is a no-op (idempotent).
	// The context can be used to cancel the operation.
	DeleteKey(ctx context.Context, path []string, recursive bool) error
}

// Base provides common functionality shared by all strategies.
//
// Base contains references to:
//   - Hive (h)
//   - FastAllocator (alloc)
//   - DirtyTracker (dt)
//   - ReadWriteIndex (idx)
//   - KeyEditor and ValueEditor (keyEditor, valEditor)
//   - Root NK cell offset (rootRef)
//
// All strategies should embed Base and delegate to the editors.
type Base struct {
	h         *hive.Hive
	alloc     alloc.Allocator
	dt        *dirty.Tracker
	idx       index.Index
	keyEditor edit.KeyEditor
	valEditor edit.ValueEditor
	rootRef   uint32
}

// NewBase creates the common base for all strategies.
//
// Parameters:
//   - h: The hive to modify
//   - a: The allocator for cell allocation
//   - dt: The dirty tracker for tracking modified ranges
//   - idx: The index for fast lookups
//
// Returns a Base struct with initialized editors.
func NewBase(h *hive.Hive, a alloc.Allocator, dt *dirty.Tracker, idx index.Index) *Base {
	return &Base{
		h:         h,
		alloc:     a,
		dt:        dt,
		idx:       idx,
		keyEditor: edit.NewKeyEditor(h, a, idx, dt),
		valEditor: edit.NewValueEditor(h, a, idx, dt),
		rootRef:   h.RootCellOffset(),
	}
}

// EnableDeferredMode enables deferred subkey list building for bulk operations.
// This dramatically improves performance by eliminating expensive read-modify-write cycles.
// Must be followed by FlushDeferredSubkeys before committing.
func (b *Base) EnableDeferredMode() {
	b.keyEditor.EnableDeferredMode()
}

// DisableDeferredMode disables deferred subkey list building.
// Returns an error if there are pending deferred updates.
func (b *Base) DisableDeferredMode() error {
	return b.keyEditor.DisableDeferredMode()
}

// FlushDeferredSubkeys writes all accumulated deferred children to disk.
// Returns the number of parents flushed and any error encountered.
func (b *Base) FlushDeferredSubkeys() (int, error) {
	return b.keyEditor.FlushDeferredSubkeys()
}
