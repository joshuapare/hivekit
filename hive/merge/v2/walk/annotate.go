package walk

import (
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// Annotate performs a read-only walk of the hive guided by the trie structure.
// For each trie node it attempts to locate the corresponding NK cell in the hive
// and populates the node's walk-phase fields:
//
//   - CellIdx, Exists, SKCellIdx, SubKeyListRef, SubKeyCount, ValueListRef, ValueCount
//
// Nodes whose key does not exist in the hive are marked Exists=false with
// CellIdx=InvalidOffset. If a node is not found, all of its descendants are
// also marked as non-existent via markSubtreeAsNew.
//
// The hive is never modified.
func Annotate(h *hive.Hive, root *trie.Node) error {
	if len(root.Children) == 0 {
		return nil
	}

	// Resolve the root NK cell.
	rootOff := h.RootCellOffset()
	rootPayload, err := h.ResolveCellPayload(rootOff)
	if err != nil {
		return fmt.Errorf("walk: resolve root NK: %w", err)
	}

	rootNK, err := hive.ParseNK(rootPayload)
	if err != nil {
		return fmt.Errorf("walk: parse root NK: %w", err)
	}

	// Annotate the virtual root trie node with the hive root's data.
	root.CellIdx = rootOff
	root.Exists = true
	root.SKCellIdx = rootNK.SecurityOffsetRel()
	root.SubKeyListRef = rootNK.SubkeyListOffsetRel()
	root.SubKeyCount = rootNK.SubkeyCount()
	root.ValueListRef = rootNK.ValueListOffsetRel()
	root.ValueCount = rootNK.ValueCount()

	// Walk the trie level by level using a cursor stack.
	stack := NewCursorStack(16)
	stack.Push(CursorEntry{
		NKCellIdx:   rootOff,
		NKPayload:   rootPayload,
		ListRef:     root.SubKeyListRef,
		SubkeyCount: root.SubKeyCount,
		SKCellIdx:   root.SKCellIdx,
	})

	return annotateChildren(h, root, stack)
}

// annotateChildren resolves children of the given parent trie node by scanning
// the parent's subkey list. Found children are annotated; missing children (and
// their subtrees) are marked as new. The function recurses into found children.
func annotateChildren(h *hive.Hive, parent *trie.Node, stack *CursorStack) error {
	cur := stack.Peek()
	if cur == nil {
		return nil
	}

	children := parent.Children
	if len(children) == 0 {
		return nil
	}

	// If the parent has no subkeys, all trie children must be new.
	if cur.SubkeyCount == 0 || cur.ListRef == format.InvalidOffset {
		for _, child := range children {
			markSubtreeAsNew(child)
		}
		return nil
	}

	// Build hash targets from sibling names.
	targets := make(map[uint32][]string, len(children))
	for _, child := range children {
		subkeys.AddHashTarget(targets, child.Name)
	}

	// Single scan of the subkey list to find all children at this level.
	matched, err := subkeys.MatchByHash(h, cur.ListRef, targets)
	if err != nil {
		return fmt.Errorf("walk: match children: %w", err)
	}

	// Index matched entries by lowercase name for O(1) lookup.
	matchMap := make(map[string]*subkeys.MatchedEntry, len(matched))
	for i := range matched {
		matchMap[matched[i].NameLower] = &matched[i]
	}

	// Annotate each child.
	for _, child := range children {
		m, found := matchMap[child.NameLower]
		if !found {
			markSubtreeAsNew(child)
			continue
		}

		// Resolve the matched NK cell to read its metadata.
		nkPayload, resolveErr := h.ResolveCellPayload(m.NKRef)
		if resolveErr != nil {
			return fmt.Errorf("walk: resolve NK for %q: %w", child.Name, resolveErr)
		}

		nk, parseErr := hive.ParseNK(nkPayload)
		if parseErr != nil {
			return fmt.Errorf("walk: parse NK for %q: %w", child.Name, parseErr)
		}

		child.CellIdx = m.NKRef
		child.Exists = true
		child.SKCellIdx = nk.SecurityOffsetRel()
		child.SubKeyListRef = nk.SubkeyListOffsetRel()
		child.SubKeyCount = nk.SubkeyCount()
		child.ValueListRef = nk.ValueListOffsetRel()
		child.ValueCount = nk.ValueCount()

		// Recurse into this child's children if any exist in the trie.
		if len(child.Children) > 0 {
			stack.Push(CursorEntry{
				NKCellIdx:   m.NKRef,
				NKPayload:   nkPayload,
				ListRef:     child.SubKeyListRef,
				SubkeyCount: child.SubKeyCount,
				SKCellIdx:   child.SKCellIdx,
			})

			if recurseErr := annotateChildren(h, child, stack); recurseErr != nil {
				return recurseErr
			}

			stack.Pop()
		}
	}

	return nil
}

// markSubtreeAsNew marks a node and all of its descendants as non-existent.
func markSubtreeAsNew(n *trie.Node) {
	n.Exists = false
	n.CellIdx = format.InvalidOffset
	n.SKCellIdx = format.InvalidOffset
	n.SubKeyListRef = format.InvalidOffset
	n.SubKeyCount = 0
	n.ValueListRef = format.InvalidOffset
	n.ValueCount = 0

	for _, child := range n.Children {
		markSubtreeAsNew(child)
	}
}
