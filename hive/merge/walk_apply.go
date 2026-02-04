// Package merge provides the single-pass walk-apply engine for plan application.
//
// The walk applier applies a sorted plan in a single tree traversal with subtree pruning.
// This is optimal for small-medium plans where full index build overhead dominates.
package merge

import (
	"cmp"
	"context"
	"fmt"
	"slices"
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
// It indexes nodes, applies operations, and prunes branches using childrenByParent.
type walkApplier struct {
	h     *hive.Hive
	alloc alloc.Allocator
	dt    *dirty.Tracker

	// Editors for applying operations
	keyEditor edit.KeyEditor
	valEditor edit.ValueEditor

	// Sorted ops and path lookup structures
	ops       []Op             // sorted by normalized path
	opsByPath map[string][]int // normalized path -> indices into ops

	// childrenByParent maps each normalized parent path to the set of
	// lowercase child names needed at that level. Replaces prefix scanning
	// with O(1) lookups per parent.
	childrenByParent map[string]map[string]struct{}

	visited map[string]uint32 // normalized path -> nkOffset (for key creation)

	// Index for lookup (used for key creation and deletion)
	idx index.Index

	// Root cell offset
	rootRef uint32

	// Reusable buffer for ReadOffsetsInto
	offsetBuf []uint32

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
		h:                h,
		alloc:            a,
		dt:               dt,
		idx:              idx,
		ops:              make([]Op, len(plan.Ops)),
		opsByPath:        make(map[string][]int),
		childrenByParent: make(map[string]map[string]struct{}),
		visited:          make(map[string]uint32),
		rootRef:          h.RootCellOffset(),
	}

	// Copy and normalize ops
	copy(wa.ops, plan.Ops)

	// Sort by path for efficient matching
	// Use stable sort to preserve original order for same-path ops
	slices.SortStableFunc(wa.ops, func(a, b Op) int {
		return cmp.Compare(normalizePath(a.KeyPath), normalizePath(b.KeyPath))
	})

	// Build path index and childrenByParent map
	for i, op := range wa.ops {
		pathKey := normalizePath(op.KeyPath)
		wa.opsByPath[pathKey] = append(wa.opsByPath[pathKey], i)

		// For each path segment, record the parent->child relationship
		for j := 0; j < len(op.KeyPath); j++ {
			parentKey := normalizePath(op.KeyPath[:j])
			childName := toLowerASCII(op.KeyPath[j])

			children := wa.childrenByParent[parentKey]
			if children == nil {
				children = make(map[string]struct{})
				wa.childrenByParent[parentKey] = children
			}
			children[childName] = struct{}{}
		}
	}

	// Create editors with index
	wa.keyEditor = edit.NewKeyEditor(h, a, idx, dt)
	wa.valEditor = edit.NewValueEditor(h, a, idx, dt)

	return wa
}

// Apply executes all ops in a single tree walk that indexes, applies, and prunes.
func (wa *walkApplier) Apply(ctx context.Context) (Applied, error) {
	// Single pass: walk, index, and apply ops to existing nodes
	if err := wa.walkAndApply(ctx, wa.rootRef, 0, nil); err != nil {
		return wa.result, err
	}

	// Phase 2: Create missing keys and apply remaining ops
	if err := wa.createMissingKeysAndApply(ctx); err != nil {
		return wa.result, err
	}

	return wa.result, nil
}

// walkAndApply recursively walks the tree, indexes nodes, and applies matching ops.
// It uses childrenByParent for O(1) pruning and ReadOffsetsInto + MatchNKsFromOffsets
// for targeted child selection (avoids decoding all sibling names).
func (wa *walkApplier) walkAndApply(ctx context.Context, nkOffset uint32, parentRef uint32, currentPath []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	pathKey := normalizePath(currentPath)

	// Parse NK (needed for value indexing and subkey access)
	payload, err := wa.h.ResolveCellPayload(nkOffset)
	if err != nil {
		return nil // Can't resolve, skip
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return nil // Can't parse, skip
	}

	// Index this node (root is already seeded in newWalkApplySession)
	if len(currentPath) > 0 {
		name := currentPath[len(currentPath)-1]
		wa.idx.AddNK(parentRef, toLowerASCII(name), nkOffset)
	}

	// Index values before applying ops (editors need value index)
	wa.indexValues(nkOffset, nk)

	// Record this node as visited
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

	// Check if any children are needed at this level
	neededChildren, hasNeeded := wa.childrenByParent[pathKey]
	if !hasNeeded || nk.SubkeyCount() == 0 {
		return nil
	}

	subkeyListRef := nk.SubkeyListOffsetRel()
	if subkeyListRef == format.InvalidOffset {
		return nil
	}

	// Read child offsets into reusable buffer
	wa.offsetBuf, err = subkeys.ReadOffsetsInto(wa.h, subkeyListRef, wa.offsetBuf)
	if err != nil {
		return nil
	}

	// Match only needed children using cheap name comparison
	matched, err := subkeys.MatchNKsFromOffsets(wa.h, wa.offsetBuf, neededChildren)
	if err != nil {
		return nil
	}

	// Recurse into matched children
	for _, entry := range matched {
		// CRITICAL: Make a copy of the slice to avoid sharing underlying array
		// between siblings. Using append with capacity limit forces allocation.
		childPath := append(currentPath[:len(currentPath):len(currentPath)], entry.NameLower)
		if err := wa.walkAndApply(ctx, entry.NKRef, nkOffset, childPath); err != nil {
			return err
		}
	}

	return nil
}

// indexValues indexes values for a key so editors can find them.
func (wa *walkApplier) indexValues(nkOffset uint32, nk hive.NK) {
	if nk.ValueCount() == 0 {
		return
	}

	_ = walker.WalkValues(wa.h, nkOffset, func(vk hive.VK, vkRef uint32) error {
		name := string(vk.Name())
		wa.idx.AddVK(nkOffset, strings.ToLower(name), vkRef)
		return nil
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
// Fuses join + lowercase in a single pass with one allocation.
func normalizePath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	if len(path) == 1 {
		return toLowerASCII(path[0])
	}

	// Pre-compute total length: sum of segments + (len-1) separators
	n := len(path) - 1 // separators
	for _, s := range path {
		n += len(s)
	}

	var b strings.Builder
	b.Grow(n)
	for i, s := range path {
		if i > 0 {
			b.WriteByte('\\')
		}
		for j := 0; j < len(s); j++ {
			c := s[j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

// toLowerASCII lowercases an ASCII string in a single allocation.
func toLowerASCII(s string) string {
	// Fast check: if already lowercase, return as-is (zero alloc).
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}

	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
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
// The walk indexes nodes, applies ops, and prunes in a single traversal.
func (was *walkApplySession) ApplyPlan(ctx context.Context, plan *Plan) (Applied, error) {
	wa := newWalkApplier(was.h, was.alloc, was.dt, was.idx, plan)
	return wa.Apply(ctx)
}
