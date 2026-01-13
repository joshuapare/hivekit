package strategy

import (
	"context"
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
)

// InPlace implements a strategy that mutates cells in-place when possible.
//
// Characteristics:
//   - May reuse existing cells if they fit
//   - May fragment over time as values grow
//   - Best for: Small updates, workloads where space efficiency matters
//
// Dirty Tracking:
// Uses precise dirty tracking - the editors (KeyEditor, ValueEditor) track
// exactly which cells are modified. The allocator (FastAllocator.Grow)
// automatically tracks newly appended HBINs.
type InPlace struct {
	*Base
}

// NewInPlace creates an in-place strategy.
//
// Parameters:
//   - h: The hive to modify
//   - a: The allocator for cell allocation
//   - dt: The dirty tracker for tracking modified ranges
//   - idx: The index for fast lookups
//
// Returns a Strategy that mutates cells in-place when possible.
func NewInPlace(
	h *hive.Hive,
	a alloc.Allocator,
	dt *dirty.Tracker,
	idx index.Index,
) Strategy {
	return &InPlace{
		Base: NewBase(h, a, dt, idx),
	}
}

// EnsureKey ensures a key path exists, creating keys as needed.
//
// This delegates to KeyEditor.EnsureKeyPath. Dirty tracking is handled
// automatically by the KeyEditor:
//   - Parent NK cells (when subkey lists are modified)
//   - Newly created NK cells
//   - Subkey list structures (LF/LH/LI/RI)
//
// Returns the final NK reference and the count of keys created.
func (ip *InPlace) EnsureKey(ctx context.Context, path []string) (uint32, int, error) {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}

	// Empty path refers to root key (always exists)
	if len(path) == 0 {
		return ip.rootRef, 0, nil
	}

	// Delegate to KeyEditor - it handles precise dirty tracking
	nkRef, keysCreated, err := ip.keyEditor.EnsureKeyPath(ip.rootRef, path)
	if err != nil {
		return 0, 0, fmt.Errorf("EnsureKeyPath: %w", err)
	}

	return nkRef, keysCreated, nil
}

// SetValue sets a value (creates or updates).
//
// This delegates to ValueEditor.UpsertValue. Dirty tracking is handled
// automatically by the ValueEditor:
//   - NK cell (when value list is modified)
//   - Value list structures
//   - VK cells
//   - Value data (inline or external)
//   - DB/BL/RD structures (for large values > 16KB)
func (ip *InPlace) SetValue(ctx context.Context, path []string, name string, typ uint32, data []byte) error {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	var keyRef uint32
	var err error

	// Empty path refers to root key
	if len(path) == 0 {
		keyRef = ip.rootRef
	} else {
		// Ensure parent key exists
		keyRef, _, err = ip.keyEditor.EnsureKeyPath(ip.rootRef, path)
		if err != nil {
			return fmt.Errorf("EnsureKeyPath for SetValue: %w", err)
		}
	}

	// Set the value - ValueEditor handles precise dirty tracking
	err = ip.valEditor.UpsertValue(keyRef, name, typ, data)
	if err != nil {
		return fmt.Errorf("UpsertValue: %w", err)
	}

	return nil
}

// DeleteValue removes a value from a key.
//
// This delegates to ValueEditor.DeleteValue. Dirty tracking is handled
// automatically by the ValueEditor:
//   - NK cell (when value list is modified)
//   - Value list structures
//   - Freed VK cells (marked free in allocator)
//   - Freed value data cells
//
// If the value doesn't exist, this is a no-op (idempotent).
func (ip *InPlace) DeleteValue(ctx context.Context, path []string, name string) error {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	var keyRef uint32
	var err error

	// Empty path refers to root key
	if len(path) == 0 {
		keyRef = ip.rootRef
	} else {
		// Ensure parent key exists (idempotent if it doesn't)
		keyRef, _, err = ip.keyEditor.EnsureKeyPath(ip.rootRef, path)
		if err != nil {
			return fmt.Errorf("EnsureKeyPath for DeleteValue: %w", err)
		}
	}

	// Delete the value - ValueEditor handles precise dirty tracking
	err = ip.valEditor.DeleteValue(keyRef, name)
	if err != nil {
		return fmt.Errorf("DeleteValue: %w", err)
	}

	return nil
}

// DeleteKey removes a key and optionally its subkeys (if recursive=true).
//
// This delegates to KeyEditor.DeleteKey. Dirty tracking is handled
// automatically by the KeyEditor:
//   - Parent NK cell (when subkey list is modified)
//   - Subkey list structures
//   - Freed NK cells (marked free in allocator)
//   - Freed subkey structures
//   - Freed value structures (if recursive)
//
// If the key doesn't exist, this is a no-op (idempotent).
func (ip *InPlace) DeleteKey(ctx context.Context, path []string, recursive bool) error {
	// Check for cancellation
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(path) == 0 {
		return errors.New("DeleteKey: empty key path")
	}

	// Look up the key in the index to get its reference
	// Use index.WalkPath instead of EnsureKeyPath to avoid RE-CREATING the key
	keyRef, exists := index.WalkPath(ip.idx, ip.rootRef, path...)
	if !exists {
		// Key doesn't exist - this is a no-op (idempotent)
		return nil
	}

	// Delete the key - KeyEditor handles precise dirty tracking
	err := ip.keyEditor.DeleteKey(keyRef, recursive)
	if err != nil {
		return fmt.Errorf("DeleteKey: %w", err)
	}

	return nil
}
