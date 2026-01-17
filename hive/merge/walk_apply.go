// Package merge provides the single-pass walk-apply engine for plan application.
//
// The walk applier applies a sorted plan in a single tree traversal with subtree pruning.
// This is optimal for small-medium plans where full index build overhead dominates.
package merge

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/edit"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/hive/walker"
	"github.com/joshuapare/hivekit/internal/format"
)

// walkApplier applies a sorted plan in a single tree traversal.
// It uses subtree pruning to skip branches with no pending operations.
type walkApplier struct {
	h     *hive.Hive
	alloc alloc.Allocator
	dt    *dirty.Tracker

	// Editors for applying operations
	keyEditor edit.KeyEditor
	valEditor edit.ValueEditor

	// Sorted ops and path lookup structures
	ops       []Op                 // sorted by normalized path
	opsByPath map[string][]int     // normalized path -> indices into ops
	prefixSet map[string]struct{}  // all path prefixes for pruning
	visited   map[string]uint32    // normalized path -> nkOffset (for key creation)

	// Index for lookup (used for key creation and deletion)
	idx index.Index

	// Root cell offset
	rootRef uint32

	// Results
	result Applied
}

// newWalkApplier creates an applier for the given plan.
func newWalkApplier(
	h *hive.Hive,
	a alloc.Allocator,
	dt *dirty.Tracker,
	idx index.Index,
	plan *Plan,
) *walkApplier {
	wa := &walkApplier{
		h:         h,
		alloc:     a,
		dt:        dt,
		idx:       idx,
		ops:       make([]Op, len(plan.Ops)),
		opsByPath: make(map[string][]int),
		prefixSet: make(map[string]struct{}),
		visited:   make(map[string]uint32),
		rootRef:   h.RootCellOffset(),
	}

	// Copy and normalize ops
	copy(wa.ops, plan.Ops)

	// Sort by path for efficient matching
	// Use stable sort to preserve original order for same-path ops
	sort.SliceStable(wa.ops, func(i, j int) bool {
		return pathLess(wa.ops[i].KeyPath, wa.ops[j].KeyPath)
	})

	// Build path index and prefix set
	for i, op := range wa.ops {
		pathKey := normalizePath(op.KeyPath)
		wa.opsByPath[pathKey] = append(wa.opsByPath[pathKey], i)

		// Add all prefixes for pruning
		for j := 1; j <= len(op.KeyPath); j++ {
			prefix := normalizePath(op.KeyPath[:j])
			wa.prefixSet[prefix] = struct{}{}
		}
	}

	// Create editors with index
	wa.keyEditor = edit.NewKeyEditor(h, a, idx, dt)
	wa.valEditor = edit.NewValueEditor(h, a, idx, dt)

	return wa
}

// Apply executes all ops in a single tree walk.
func (wa *walkApplier) Apply(ctx context.Context) (Applied, error) {
	// Phase 1: Walk existing tree and apply ops to existing nodes
	if err := wa.walkAndApply(ctx, wa.rootRef, nil); err != nil {
		return wa.result, err
	}

	// Phase 2: Create missing keys and apply remaining ops
	if err := wa.createMissingKeysAndApply(ctx); err != nil {
		return wa.result, err
	}

	return wa.result, nil
}

// walkAndApply recursively walks the tree and applies matching ops.
func (wa *walkApplier) walkAndApply(ctx context.Context, nkOffset uint32, currentPath []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Record this node as visited
	pathKey := normalizePath(currentPath)
	wa.visited[pathKey] = nkOffset

	// Apply all ops that target this exact path
	if indices, ok := wa.opsByPath[pathKey]; ok {
		for _, idx := range indices {
			op := &wa.ops[idx]
			if err := wa.applyOpAtNode(ctx, nkOffset, op); err != nil {
				return err
			}
		}
	}

	// Check if this node has subkeys before trying to walk them
	payload, err := wa.h.ResolveCellPayload(nkOffset)
	if err != nil {
		return nil // Can't resolve, skip walking children
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return nil // Can't parse, skip walking children
	}

	// If no subkeys, nothing to walk
	if nk.SubkeyCount() == 0 {
		return nil
	}

	// Walk children, but only those with pending ops (pruning)
	return walker.WalkSubkeysCtx(ctx, wa.h, nkOffset, func(childNK hive.NK, childRef uint32) error {
		childName := string(childNK.Name())
		// CRITICAL: Make a copy of the slice to avoid sharing underlying array
		// between siblings. Using append with capacity limit forces allocation.
		childPath := append(currentPath[:len(currentPath):len(currentPath)], childName)
		childPathKey := normalizePath(childPath)

		// PRUNING: Skip subtree if no ops target it or its descendants
		if _, hasOps := wa.prefixSet[childPathKey]; !hasOps {
			return nil // Skip entire subtree
		}

		return wa.walkAndApply(ctx, childRef, childPath)
	})
}

// applyOpAtNode applies a single operation at the given NK offset.
// This is called for ops where the key already exists.
func (wa *walkApplier) applyOpAtNode(ctx context.Context, nkOffset uint32, op *Op) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	switch op.Type {
	case OpEnsureKey:
		// Key already exists if we reached here via walk
		// Nothing to do - key is already ensured
		return nil

	case OpSetValue:
		err := wa.valEditor.UpsertValue(nkOffset, op.ValueName, op.ValueType, op.Data)
		if err == nil {
			wa.result.ValuesSet++
		}
		return err

	case OpDeleteValue:
		err := wa.valEditor.DeleteValue(nkOffset, op.ValueName)
		if err == nil {
			wa.result.ValuesDeleted++
		}
		return err

	case OpDeleteKey:
		err := wa.keyEditor.DeleteKey(nkOffset, true)
		if err == nil {
			wa.result.KeysDeleted++
		}
		return err
	}
	return nil
}

// createMissingKeysAndApply creates keys that don't exist and applies ops to them.
// This handles the case where EnsureKey or SetValue targets a non-existent path.
func (wa *walkApplier) createMissingKeysAndApply(ctx context.Context) error {
	for _, op := range wa.ops {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Only EnsureKey and SetValue can create keys
		if op.Type != OpEnsureKey && op.Type != OpSetValue {
			continue
		}

		pathKey := normalizePath(op.KeyPath)

		// Check if key was already visited (exists)
		nkRef, keyExists := wa.visited[pathKey]

		// If key doesn't exist, create it
		if !keyExists {
			// Find the deepest existing ancestor
			var ancestorPath []string
			var ancestorRef uint32 = wa.rootRef

			for i := 1; i <= len(op.KeyPath); i++ {
				partialPath := normalizePath(op.KeyPath[:i])
				if ref, ok := wa.visited[partialPath]; ok {
					ancestorPath = op.KeyPath[:i]
					ancestorRef = ref
				} else {
					break
				}
			}

			// Create the remaining path segments
			remainingPath := op.KeyPath[len(ancestorPath):]
			if len(remainingPath) > 0 {
				var keysCreated int
				var err error
				nkRef, keysCreated, err = wa.keyEditor.EnsureKeyPath(ancestorRef, remainingPath)
				if err != nil {
					return fmt.Errorf("create key path %v: %w", op.KeyPath, err)
				}
				wa.result.KeysCreated += keysCreated

				// Mark the newly created path as visited
				wa.visited[pathKey] = nkRef

				// Also mark intermediate keys as visited
				tempRef := ancestorRef
				for i := 1; i < len(remainingPath); i++ {
					intermediatePath := normalizePath(append(ancestorPath, remainingPath[:i]...))
					// Get the intermediate ref by walking up to it
					if interRef, ok := wa.idx.GetNK(tempRef, remainingPath[i-1]); ok {
						wa.visited[intermediatePath] = interRef
						tempRef = interRef
					}
				}
			} else {
				// Key already exists at ancestor level
				nkRef = ancestorRef
				wa.visited[pathKey] = nkRef
			}
		}

		// Now apply the op if it's SetValue (EnsureKey is done)
		if op.Type == OpSetValue {
			if nkRef == 0 {
				// Should not happen, but get it from index as fallback
				if ref, ok := index.WalkPath(wa.idx, wa.rootRef, op.KeyPath...); ok {
					nkRef = ref
				}
			}
			if nkRef != 0 {
				err := wa.valEditor.UpsertValue(nkRef, op.ValueName, op.ValueType, op.Data)
				if err != nil {
					return fmt.Errorf("set value at %v: %w", op.KeyPath, err)
				}
				wa.result.ValuesSet++
			}
		}
	}

	return nil
}

// normalizePath converts a path slice to a lowercase string key for map lookups.
func normalizePath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return strings.ToLower(strings.Join(path, "\\"))
}

// pathLess compares two paths lexicographically (case-insensitive).
func pathLess(a, b []string) bool {
	return normalizePath(a) < normalizePath(b)
}

// noIndexKeyEditor wraps KeyEditor to work without full index during walk-apply.
// It creates keys by walking the tree directly rather than using index lookups.
type noIndexKeyEditor struct {
	h     *hive.Hive
	alloc alloc.Allocator
	dt    *dirty.Tracker
	idx   index.Index
	inner edit.KeyEditor
}

// newNoIndexKeyEditor creates a key editor that uses an in-memory index.
func newNoIndexKeyEditor(h *hive.Hive, a alloc.Allocator, dt *dirty.Tracker) *noIndexKeyEditor {
	// Create a minimal in-memory index for tracking created keys
	idx := index.NewIndex(index.IndexNumeric, 100, 100)

	return &noIndexKeyEditor{
		h:     h,
		alloc: a,
		dt:    dt,
		idx:   idx,
		inner: edit.NewKeyEditor(h, a, idx, dt),
	}
}

// EnsureKeyPath creates intermediate keys as needed.
func (nke *noIndexKeyEditor) EnsureKeyPath(root edit.NKRef, segments []string) (edit.NKRef, int, error) {
	return nke.inner.EnsureKeyPath(root, segments)
}

// DeleteKey removes a key and optionally its subkeys.
func (nke *noIndexKeyEditor) DeleteKey(nk edit.NKRef, recursive bool) error {
	return nke.inner.DeleteKey(nk, recursive)
}

// EnableDeferredMode enables deferred subkey list building.
func (nke *noIndexKeyEditor) EnableDeferredMode() {
	nke.inner.EnableDeferredMode()
}

// DisableDeferredMode disables deferred subkey list building.
func (nke *noIndexKeyEditor) DisableDeferredMode() error {
	return nke.inner.DisableDeferredMode()
}

// FlushDeferredSubkeys writes all accumulated deferred children to disk.
func (nke *noIndexKeyEditor) FlushDeferredSubkeys() (int, error) {
	return nke.inner.FlushDeferredSubkeys()
}

// Index returns the internal index used by this editor.
func (nke *noIndexKeyEditor) Index() index.Index {
	return nke.idx
}

// noIndexValueEditor wraps ValueEditor to work without full index during walk-apply.
type noIndexValueEditor struct {
	h     *hive.Hive
	alloc alloc.Allocator
	dt    *dirty.Tracker
	idx   index.Index
	inner edit.ValueEditor
}

// newNoIndexValueEditor creates a value editor that uses an in-memory index.
func newNoIndexValueEditor(h *hive.Hive, a alloc.Allocator, dt *dirty.Tracker, idx index.Index) *noIndexValueEditor {
	return &noIndexValueEditor{
		h:     h,
		alloc: a,
		dt:    dt,
		idx:   idx,
		inner: edit.NewValueEditor(h, a, idx, dt),
	}
}

// UpsertValue creates or updates a value under the given NK.
func (nve *noIndexValueEditor) UpsertValue(nk edit.NKRef, name string, typ edit.ValueType, data []byte) error {
	return nve.inner.UpsertValue(nk, name, typ, data)
}

// DeleteValue removes a value by name.
func (nve *noIndexValueEditor) DeleteValue(nk edit.NKRef, name string) error {
	return nve.inner.DeleteValue(nk, name)
}

// walkApplySession provides session-level operations for single-pass mode.
type walkApplySession struct {
	h       *hive.Hive
	alloc   alloc.Allocator
	dt      *dirty.Tracker
	idx     index.Index
	rootRef uint32
}

// newWalkApplySession creates a session for single-pass walk-apply operations.
func newWalkApplySession(h *hive.Hive, a alloc.Allocator, dt *dirty.Tracker) *walkApplySession {
	// Create a minimal in-memory index for tracking created keys
	idx := index.NewIndex(index.IndexNumeric, 1000, 1000)

	// Seed the index with root key
	rootRef := h.RootCellOffset()
	rootPayload, err := h.ResolveCellPayload(rootRef)
	if err == nil {
		nk, parseErr := hive.ParseNK(rootPayload)
		if parseErr == nil {
			idx.AddNK(0, strings.ToLower(string(nk.Name())), rootRef)
		}
	}

	return &walkApplySession{
		h:       h,
		alloc:   a,
		dt:      dt,
		idx:     idx,
		rootRef: rootRef,
	}
}

// ApplyPlan applies a plan using single-pass walk-apply.
func (was *walkApplySession) ApplyPlan(ctx context.Context, plan *Plan) (Applied, error) {
	// Build index for paths that need it
	// For single-pass, we build a minimal index as we walk
	if err := was.buildMinimalIndex(ctx, plan); err != nil {
		return Applied{}, fmt.Errorf("build minimal index: %w", err)
	}

	// Create walk applier and apply
	wa := newWalkApplier(was.h, was.alloc, was.dt, was.idx, plan)
	return wa.Apply(ctx)
}

// buildMinimalIndex builds an index containing only the paths needed by the plan.
func (was *walkApplySession) buildMinimalIndex(ctx context.Context, plan *Plan) error {
	// Collect unique paths we need to index
	pathsToIndex := make(map[string]struct{})
	for _, op := range plan.Ops {
		pathKey := normalizePath(op.KeyPath)
		pathsToIndex[pathKey] = struct{}{}

		// Also add all ancestors
		for i := 1; i < len(op.KeyPath); i++ {
			ancestorKey := normalizePath(op.KeyPath[:i])
			pathsToIndex[ancestorKey] = struct{}{}
		}
	}

	// Walk the tree and build index for paths we need
	return was.walkAndIndex(ctx, was.rootRef, nil, pathsToIndex)
}

// walkAndIndex walks the tree and indexes nodes that match needed paths.
func (was *walkApplySession) walkAndIndex(ctx context.Context, nkOffset uint32, currentPath []string, neededPaths map[string]struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Get NK name
	payload, err := was.h.ResolveCellPayload(nkOffset)
	if err != nil {
		return nil // Skip invalid nodes
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return nil // Skip invalid nodes
	}

	name := string(nk.Name())
	var thisPath []string
	if currentPath == nil {
		thisPath = []string{name}
	} else {
		// CRITICAL: Make a copy of the slice to avoid sharing underlying array
		// between siblings. Using append with capacity limit forces allocation.
		thisPath = append(currentPath[:len(currentPath):len(currentPath)], name)
	}

	pathKey := normalizePath(thisPath)

	// Check if this path or any descendant is needed
	isNeeded := false
	if _, ok := neededPaths[pathKey]; ok {
		isNeeded = true
	} else {
		// Check if any needed path has this as prefix
		for neededPath := range neededPaths {
			if strings.HasPrefix(neededPath, pathKey+"\\") || neededPath == pathKey {
				isNeeded = true
				break
			}
		}
	}

	if !isNeeded {
		return nil // Prune this subtree
	}

	// Add to index
	var parentRef uint32
	if len(currentPath) > 0 {
		// Find parent in index by walking up
		parentRef = was.findParentRef(currentPath)
	}
	was.idx.AddNK(parentRef, strings.ToLower(name), nkOffset)

	// Also index values for this key if we have ops targeting them
	if err := was.indexValues(nkOffset, nk); err != nil {
		// Non-fatal, continue
	}

	// Walk subkeys only if this key has any
	if nk.SubkeyCount() == 0 {
		return nil
	}

	subkeyListRef := nk.SubkeyListOffsetRel()
	if subkeyListRef == format.InvalidOffset {
		return nil
	}

	// Read subkey list
	subkeyList, err := subkeys.Read(was.h, subkeyListRef)
	if err != nil {
		return nil
	}

	for _, entry := range subkeyList.Entries {
		if err := was.walkAndIndex(ctx, entry.NKRef, thisPath, neededPaths); err != nil {
			return err
		}
	}

	return nil
}

// findParentRef finds the parent NK ref for a path by walking up the path.
func (was *walkApplySession) findParentRef(path []string) uint32 {
	if len(path) == 0 {
		return 0
	}

	current := was.rootRef
	for i := 0; i < len(path); i++ {
		next, ok := was.idx.GetNK(current, strings.ToLower(path[i]))
		if !ok {
			return current
		}
		current = next
	}
	return current
}

// indexValues indexes values for a key.
func (was *walkApplySession) indexValues(nkOffset uint32, nk hive.NK) error {
	if nk.ValueCount() == 0 {
		return nil
	}

	return walker.WalkValues(was.h, nkOffset, func(vk hive.VK, vkRef uint32) error {
		name := string(vk.Name())
		was.idx.AddVK(nkOffset, strings.ToLower(name), vkRef)
		return nil
	})
}
