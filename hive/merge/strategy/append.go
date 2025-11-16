package strategy

import (
	"errors"
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/internal/format"
)

// Append is an append-only strategy that never frees cells.
// All updates allocate new cells and leave old cells orphaned.
//
// Use cases:
//   - Append-only workloads (logs, audit trails)
//   - Memory-mapped hives where fragmentation is acceptable
//   - Scenarios where avoiding Free() overhead is critical
//   - Crash recovery scenarios (orphaned cells are safe)
//
// Trade-offs:
//   - Fastest: No Free() overhead
//   - Safest: No risk of use-after-free bugs
//   - Wastes space: Orphaned cells are never reclaimed
//   - Hive grows monotonically (no compaction)
type Append struct {
	*Base
}

// NewAppend creates an append-only strategy.
// Wraps the allocator to disable Free() calls.
func NewAppend(
	h *hive.Hive,
	a alloc.Allocator,
	dt *dirty.Tracker,
	idx index.Index,
) Strategy {
	// Wrap allocator to no-op on Free()
	noFreeAlloc := &noFreeAllocator{Allocator: a}

	// Use NewBase to create common base with wrapped allocator
	base := NewBase(h, noFreeAlloc, dt, idx)

	return &Append{
		Base: base,
	}
}

// EnsureKey implements Strategy.
// Creates keys using normal editor logic (allocates new cells as needed).
// If the key already exists, returns the existing ref.
// Returns the final NK reference and the count of keys created.
func (ap *Append) EnsureKey(path []string) (uint32, int, error) {
	// Empty path refers to root key (always exists)
	if len(path) == 0 {
		return ap.rootRef, 0, nil
	}

	// Delegate to KeyEditor
	nkRef, keysCreated, err := ap.keyEditor.EnsureKeyPath(ap.rootRef, path)
	if err != nil {
		return 0, 0, fmt.Errorf("EnsureKeyPath: %w", err)
	}

	// Track dirty ranges (heuristic)
	// For append strategy, we track conservatively
	if keysCreated > 0 {
		ap.dt.Add(format.HeaderSize, format.HBINAlignment) // Conservative heuristic
	}

	return nkRef, keysCreated, nil
}

// SetValue implements Strategy.
// Creates/updates values using normal editor logic.
// For updates, allocates new VK cell and leaves old one orphaned (never freed).
//
// Note: The editor's Free() calls are skipped by temporarily swapping the allocator.
func (ap *Append) SetValue(path []string, name string, typ uint32, data []byte) error {
	var keyRef uint32
	var err error

	// Empty path refers to root key
	if len(path) == 0 {
		keyRef = ap.rootRef
	} else {
		// Ensure parent key exists
		keyRef, _, err = ap.keyEditor.EnsureKeyPath(ap.rootRef, path)
		if err != nil {
			return fmt.Errorf("EnsureKeyPath for SetValue: %w", err)
		}
	}

	// Set the value
	// Note: The editor may call Free() on old cells during update,
	// but for append strategy we want to skip those calls.
	// Since we can't easily intercept editor calls, we accept that
	// the editor will attempt to free (but those cells remain allocated).
	err = ap.valEditor.UpsertValue(keyRef, name, typ, data)
	if err != nil {
		return fmt.Errorf("UpsertValue: %w", err)
	}

	// Track dirty ranges (heuristic)
	ap.dt.Add(format.HeaderSize, format.HBINAlignment) // Conservative heuristic

	return nil
}

// DeleteValue implements Strategy.
// Removes value from index and parent's value list, but DOES NOT free the VK cell.
// The orphaned VK cell remains in the hive (append-only semantics).
//
// Note: The editor will attempt to call Free(), but those cells remain allocated.
func (ap *Append) DeleteValue(path []string, name string) error {
	var keyRef uint32
	var err error

	// Empty path refers to root key
	if len(path) == 0 {
		keyRef = ap.rootRef
	} else {
		// Ensure parent key exists (idempotent if it doesn't)
		keyRef, _, err = ap.keyEditor.EnsureKeyPath(ap.rootRef, path)
		if err != nil {
			return fmt.Errorf("EnsureKeyPath for DeleteValue: %w", err)
		}
	}

	// Delete the value (idempotent if it doesn't exist)
	// Note: The editor will call Free(), but cells remain allocated
	err = ap.valEditor.DeleteValue(keyRef, name)
	if err != nil {
		return fmt.Errorf("DeleteValue: %w", err)
	}

	// Track dirty ranges (heuristic)
	ap.dt.Add(format.HeaderSize, format.HBINAlignment) // Conservative heuristic

	return nil
}

// DeleteKey implements Strategy.
// Removes key from index and parent's subkey list, but DOES NOT free the NK cell.
// The orphaned NK cell remains in the hive (append-only semantics).
//
// Note: The editor will attempt to call Free(), but those cells remain allocated.
func (ap *Append) DeleteKey(path []string, recursive bool) error {
	if len(path) == 0 {
		return errors.New("DeleteKey: empty key path")
	}

	// Look up the key in the index to get its reference
	// Use index.WalkPath instead of EnsureKeyPath to avoid RE-CREATING the key
	keyRef, exists := index.WalkPath(ap.idx, ap.rootRef, path...)
	if !exists {
		// Key doesn't exist - this is a no-op (idempotent)
		return nil
	}

	// Delete the key (idempotent if it doesn't exist)
	// Note: The editor will call Free(), but cells remain allocated
	err := ap.keyEditor.DeleteKey(keyRef, recursive)
	if err != nil {
		return fmt.Errorf("DeleteKey: %w", err)
	}

	// Track dirty ranges (heuristic)
	ap.dt.Add(format.HeaderSize, format.HBINAlignment) // Conservative heuristic

	return nil
}

// noFreeAllocator wraps an Allocator and makes Free() a no-op.
// This enables append-only semantics where cells are never reclaimed.
type noFreeAllocator struct {
	alloc.Allocator
}

// Alloc delegates to the wrapped Allocator.
func (nfa *noFreeAllocator) Alloc(need int32, cls alloc.Class) (alloc.CellRef, []byte, error) {
	return nfa.Allocator.Alloc(need, cls)
}

// Free is a no-op in append-only mode.
// Returns nil to indicate "success" (even though nothing was freed).
func (nfa *noFreeAllocator) Free(_ uint32) error {
	// No-op: Never free cells in append-only mode
	return nil
}

// Grow delegates to the wrapped Allocator.
func (nfa *noFreeAllocator) Grow(need int32) error {
	// Convert bytes to pages (round up)
	numPages := (int(need) + 4095) / 4096
	return nfa.Allocator.GrowByPages(numPages)
}

// GrowByPages delegates to the wrapped Allocator.
func (nfa *noFreeAllocator) GrowByPages(numPages int) error {
	return nfa.Allocator.GrowByPages(numPages)
}

// TruncatePages delegates to the wrapped Allocator.
func (nfa *noFreeAllocator) TruncatePages(numPages int) error {
	return nfa.Allocator.TruncatePages(numPages)
}
