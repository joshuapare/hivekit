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

	// hashTargetsByParent maps each normalized parent path to the
	// hash-keyed target map used by MatchByHash. Pre-computed once during
	// initialization to avoid per-node hashing overhead.
	hashTargetsByParent map[string]map[uint32]string

	visited map[string]uint32  // normalized path -> nkOffset (for key creation)
	deleted map[string]struct{} // normalized paths deleted during walk (skip in Phase 2)

	// Index for lookup (used for key creation and deletion)
	idx index.Index

	// Root cell offset
	rootRef uint32

	// Cursor stack caches parsed NK/subkey-list data at each tree level.
	// Avoids redundant hive reads when ascending from one child to a sibling.
	cursor *cursorStack

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
		h:                   h,
		alloc:               a,
		dt:                  dt,
		idx:                 idx,
		ops:                 make([]Op, len(plan.Ops)),
		opsByPath:           make(map[string][]int),
		childrenByParent:    make(map[string]map[string]struct{}),
		hashTargetsByParent: make(map[string]map[uint32]string),
		visited:             make(map[string]uint32),
		deleted:             make(map[string]struct{}),
		rootRef:             h.RootCellOffset(),
		cursor:              newCursorStack(16),
	}

	// Copy and normalize ops
	copy(wa.ops, plan.Ops)

	// Ensure every op has NormalizedPath populated.
	// Plan.Add* methods set it, but ops constructed directly (e.g. in tests
	// or convertEditOpToMergeOp) may not.
	for i := range wa.ops {
		if wa.ops[i].NormalizedPath == "" && len(wa.ops[i].KeyPath) > 0 {
			wa.ops[i].NormalizedPath = normalizePath(wa.ops[i].KeyPath)
		}
	}

	// Sort by path for efficient matching
	// Use stable sort to preserve original order for same-path ops
	slices.SortStableFunc(wa.ops, func(a, b Op) int {
		return cmp.Compare(a.NormalizedPath, b.NormalizedPath)
	})

	// Build path index and childrenByParent map.
	// Use pre-computed NormalizedPath for the full op path (zero extra allocs).
	// For parent sub-paths, incrementally build the key by truncating the
	// normalized path at each backslash boundary.
	for i, op := range wa.ops {
		wa.opsByPath[op.NormalizedPath] = append(wa.opsByPath[op.NormalizedPath], i)

		// For each path segment, record the parent->child relationship.
		// We derive parent keys from the pre-computed NormalizedPath by
		// finding backslash positions, avoiding normalizePath calls entirely.
		np := op.NormalizedPath
		pos := 0 // byte position in np after the current segment
		parentKey := ""
		for j := 0; j < len(op.KeyPath); j++ {
			if j > 0 {
				// Skip past the backslash separator
				pos++ // for '\\'
			}
			childName := toLowerASCII(op.KeyPath[j])

			children := wa.childrenByParent[parentKey]
			if children == nil {
				children = make(map[string]struct{})
				wa.childrenByParent[parentKey] = children
			}
			children[childName] = struct{}{}

			// Advance parentKey to include this segment for next iteration
			pos += len(op.KeyPath[j])
			parentKey = np[:pos]
		}
	}

	// Pre-compute hash target maps for MatchByHash.
	// Each parent path gets a map of LH-hash -> lowercase child name.
	for parentKey, children := range wa.childrenByParent {
		targets := make(map[uint32]string, len(children))
		for childName := range children {
			targets[subkeys.Hash(childName)] = childName
		}
		wa.hashTargetsByParent[parentKey] = targets
	}

	// Create editors with index
	wa.keyEditor = edit.NewKeyEditor(h, a, idx, dt)
	wa.valEditor = edit.NewValueEditor(h, a, idx, dt)

	return wa
}

// Apply executes all ops in a single tree walk that indexes, applies, and prunes.
func (wa *walkApplier) Apply(ctx context.Context) (Applied, error) {
	// Defer subkey list writes: Phase 1 only reads subkey lists, never inserts
	// new children. Phase 2 creates keys via EnsureKeyPath which uses the index,
	// not on-disk subkey lists. Flushing once at the end eliminates expensive
	// read-modify-write cycles during bulk inserts.
	wa.keyEditor.EnableDeferredMode()

	// Single pass: walk, index, and apply ops to existing nodes
	if err := wa.walkAndApply(ctx, wa.rootRef, 0, nil); err != nil {
		return wa.result, err
	}

	// Phase 2: Create missing keys and apply remaining ops
	if err := wa.createMissingKeysAndApply(ctx); err != nil {
		return wa.result, err
	}

	// Flush accumulated subkey lists to disk
	if _, err := wa.keyEditor.FlushDeferredSubkeys(); err != nil {
		return wa.result, fmt.Errorf("flush deferred subkeys: %w", err)
	}

	return wa.result, nil
}

// walkAndApply recursively walks the tree, indexes nodes, and applies matching ops.
// It uses childrenByParent for O(1) pruning and MatchByHash for hash-first child
// selection (avoids dereferencing all sibling NK cells). The cursor stack caches
// parsed NK data at each level so sibling descents skip redundant hive reads.
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

	// Push cursor entry for this level
	wa.cursor.push(cursorEntry{
		nkCellIdx:   nkOffset,
		subkeyCount: nk.SubkeyCount(),
	})
	defer wa.cursor.pop()

	// Index this node (root is already seeded in newWalkApplySession)
	if len(currentPath) > 0 {
		name := currentPath[len(currentPath)-1]
		wa.idx.AddNK(parentRef, toLowerASCII(name), nkOffset)
	}

	// Index values before applying ops (editors need value index)
	wa.indexValues(nkOffset, nk)

	// Record this node as visited
	wa.visited[pathKey] = nkOffset

	// Apply all ops that target this exact path.
	// Value ops (SetValue, DeleteValue) are batched into a single UpsertValues call
	// to reduce redundant value list reads/rebuilds.
	keyDeleted := false
	if indices, ok := wa.opsByPath[pathKey]; ok {
		var valueOps []edit.ValueOp
		for _, idx := range indices {
			op := &wa.ops[idx]
			switch op.Type {
			case OpSetValue:
				valueOps = append(valueOps, edit.ValueOp{
					Name: op.ValueName,
					Type: op.ValueType,
					Data: op.Data,
				})
			case OpDeleteValue:
				valueOps = append(valueOps, edit.ValueOp{
					Name:   op.ValueName,
					Delete: true,
				})
			default:
				// Flush any pending value ops before applying a non-value op
				if len(valueOps) > 0 {
					if err := wa.applyValueOps(ctx, nkOffset, valueOps); err != nil {
						return err
					}
					wa.result.ValuesSet += countSetOps(valueOps)
					wa.result.ValuesDeleted += countDeleteOps(valueOps)
					valueOps = valueOps[:0]
				}
				if err := wa.applyOpAtNode(ctx, nkOffset, op); err != nil {
					return err
				}
				if op.Type == OpDeleteKey {
					keyDeleted = true
				}
			}
		}
		// Flush remaining value ops
		if len(valueOps) > 0 && !keyDeleted {
			if err := wa.applyValueOps(ctx, nkOffset, valueOps); err != nil {
				return err
			}
			wa.result.ValuesSet += countSetOps(valueOps)
			wa.result.ValuesDeleted += countDeleteOps(valueOps)
		}
	}

	// If this key was deleted, do NOT walk children — the NK cell has been freed
	// and its subkey list is gone. Walking children would read stale/freed memory.
	// Record the deletion so Phase 2 skips ops targeting this path or descendants.
	if keyDeleted {
		wa.deleted[pathKey] = struct{}{}
		return nil
	}

	// Re-resolve NK after ops which may have triggered hive growth
	// (e.g., UpsertValue → Alloc → Grow → Append invalidates the old nk slice).
	payload, err = wa.h.ResolveCellPayload(nkOffset)
	if err != nil {
		return nil
	}
	nk, err = hive.ParseNK(payload)
	if err != nil {
		return nil
	}

	// Check if any children are needed at this level
	if _, hasNeeded := wa.childrenByParent[pathKey]; !hasNeeded || nk.SubkeyCount() == 0 {
		return nil
	}

	subkeyListRef := nk.SubkeyListOffsetRel()
	if subkeyListRef == format.InvalidOffset {
		return nil
	}

	// Cache the subkey list ref in the cursor for potential sibling reuse
	if ce := wa.cursor.peekPtr(); ce != nil {
		ce.lhListRef = subkeyListRef
		ce.subkeyCount = nk.SubkeyCount()
	}

	// Use hash-first matching: scan LH list entries by hash and only
	// dereference NK cells on hash match. For a parent with 200 children
	// and 3 targets, this reduces ~200 random reads to ~3.
	targets := wa.hashTargetsByParent[pathKey]
	matched, err := subkeys.MatchByHash(wa.h, subkeyListRef, targets)
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
		name := strings.ToLower(string(vk.Name()))
		wa.idx.AddVKLower(nkOffset, name, vkRef)
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

// applyValueOps applies a batch of value operations to a single key.
func (wa *walkApplier) applyValueOps(ctx context.Context, nkOffset uint32, ops []edit.ValueOp) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return wa.valEditor.UpsertValues(nkOffset, ops)
}

// countSetOps returns the number of non-delete ops in a ValueOp slice.
func countSetOps(ops []edit.ValueOp) int {
	n := 0
	for i := range ops {
		if !ops[i].Delete {
			n++
		}
	}
	return n
}

// countDeleteOps returns the number of delete ops in a ValueOp slice.
func countDeleteOps(ops []edit.ValueOp) int {
	n := 0
	for i := range ops {
		if ops[i].Delete {
			n++
		}
	}
	return n
}

// createMissingKeysAndApply creates keys that don't exist and applies ops to them.
// This handles the case where EnsureKey or SetValue targets a non-existent path.
// SetValue ops for the same path are batched into a single UpsertValues call.
func (wa *walkApplier) createMissingKeysAndApply(ctx context.Context) error {
	// Group ops by path for batching. Ops are already sorted by NormalizedPath,
	// so we can process consecutive runs of same-path SetValue ops together.
	i := 0
	for i < len(wa.ops) {
		if err := ctx.Err(); err != nil {
			return err
		}

		op := wa.ops[i]

		// Only EnsureKey and SetValue can create keys
		if op.Type != OpEnsureKey && op.Type != OpSetValue {
			i++
			continue
		}

		pathKey := op.NormalizedPath

		// Skip ops targeting a deleted key or any descendant of a deleted key.
		if wa.isUnderDeletedPath(op) {
			i++
			continue
		}

		// Ensure the key exists (create if missing)
		nkRef := wa.ensureKeyForPhase2(op, pathKey)

		// Collect all consecutive SetValue ops for this same path
		var valueOps []edit.ValueOp
		for i < len(wa.ops) {
			cur := wa.ops[i]
			if cur.NormalizedPath != pathKey {
				break
			}
			if cur.Type == OpSetValue {
				valueOps = append(valueOps, edit.ValueOp{
					Name: cur.ValueName,
					Type: cur.ValueType,
					Data: cur.Data,
				})
			}
			i++
		}

		// Apply batched value ops
		if len(valueOps) > 0 && nkRef != 0 {
			if err := wa.valEditor.UpsertValues(nkRef, valueOps); err != nil {
				return fmt.Errorf("set values at %v: %w", op.KeyPath, err)
			}
			wa.result.ValuesSet += len(valueOps)
		}
	}

	return nil
}

// ensureKeyForPhase2 ensures a key exists for phase 2, creating it if necessary.
// Returns the NK reference for the key.
func (wa *walkApplier) ensureKeyForPhase2(op Op, pathKey string) uint32 {
	// Check if key was already visited (exists)
	nkRef, keyExists := wa.visited[pathKey]

	if !keyExists {
		// Find the deepest existing ancestor by walking prefix substrings
		// of the pre-computed NormalizedPath.
		var ancestorPath []string
		var ancestorRef uint32 = wa.rootRef

		pos := 0
		for i := 1; i <= len(op.KeyPath); i++ {
			if i > 1 {
				pos++ // skip '\\'
			}
			pos += len(op.KeyPath[i-1])
			partialPath := pathKey[:pos]
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
				// Can't return error from here; caller checks nkRef == 0
				return 0
			}
			wa.result.KeysCreated += keysCreated

			// Mark the newly created path as visited
			wa.visited[pathKey] = nkRef

			// Also mark intermediate keys as visited.
			// Derive sub-path keys from the pre-computed NormalizedPath.
			tempRef := ancestorRef
			ancestorEnd := 0
			if len(ancestorPath) > 0 {
				for _, seg := range ancestorPath {
					if ancestorEnd > 0 {
						ancestorEnd++ // '\\'
					}
					ancestorEnd += len(seg)
				}
			}
			for i := 1; i < len(remainingPath); i++ {
				if ancestorEnd > 0 || i > 1 {
					ancestorEnd++ // '\\'
				}
				ancestorEnd += len(remainingPath[i-1])
				intermediatePath := pathKey[:ancestorEnd]
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

	if nkRef == 0 {
		// Should not happen, but get it from index as fallback
		if ref, ok := index.WalkPath(wa.idx, wa.rootRef, op.KeyPath...); ok {
			nkRef = ref
		}
	}

	return nkRef
}

// isUnderDeletedPath returns true if the given op's path, or any ancestor of it,
// was deleted during the walk phase. Uses the pre-computed NormalizedPath to
// derive prefix substrings without allocating.
func (wa *walkApplier) isUnderDeletedPath(op Op) bool {
	np := op.NormalizedPath
	pos := 0
	for i := 1; i <= len(op.KeyPath); i++ {
		if i > 1 {
			pos++ // skip '\\'
		}
		pos += len(op.KeyPath[i-1])
		if _, ok := wa.deleted[np[:pos]]; ok {
			return true
		}
	}
	return false
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
	// Create a minimal in-memory index for tracking created keys (pooled)
	idx := index.AcquireNumericIndex(1000, 1000)

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
