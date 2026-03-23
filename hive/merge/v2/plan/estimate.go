package plan

import (
	"fmt"
	"math"

	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/internal/format"
)

// nkFixedPayload is the number of bytes in an NK cell before the variable-length
// name: 4-byte cell header + 76-byte fixed NK struct (NKNameOffset == 0x4C == 76).
const nkFixedPayload = format.CellHeaderSize + format.NKFixedHeaderSize // 4 + 76 = 80

// vkFixedPayload is the number of bytes in a VK cell before the variable-length
// name: 4-byte cell header + 20-byte fixed VK struct (VKNameOffset == 0x14 == 20).
const vkFixedPayload = format.CellHeaderSize + format.VKFixedHeaderSize // 4 + 20 = 24

// lhEntrySize is the size of a single entry in an LH subkey list: 4-byte cell
// offset + 4-byte hash = 8 bytes.
const lhEntrySize = format.LFFHEntrySize // 8

// listHeaderSize is the 4-byte header common to all list cell types (2-byte
// signature + 2-byte count).
const listHeaderSize = format.ListHeaderSize // 4

// valueListEntrySize is the size of each entry in a value list: one uint32
// cell offset.
const valueListEntrySize = format.OffsetFieldSize // 4

// Estimate walks the annotated trie rooted at root and computes exactly how
// many bytes of new cells the write phase will need to allocate. It returns a
// SpacePlan containing the total byte count, per-category counters, and a
// detailed allocation manifest.
//
// The function performs pure arithmetic — it never accesses the hive.
//
// Rules applied during the walk:
//
//  1. For every node where !Exists and the node has operations (EnsureKey,
//     values, or children that need creation), allocate an NK cell:
//     align8(nkFixedPayload + len(name))
//
//  2. For every non-delete ValueOp on any node (new or existing):
//     - VK cell: align8(vkFixedPayload + len(valueName))
//     - Data cell if len(data) > 4: align8(CellHeaderSize + len(data))
//
//  3. For every existing parent that has at least one new (!Exists) child:
//     - Subkey list rebuild: align8(CellHeaderSize + listHeaderSize + lhEntrySize*(existingCount+newCount))
//
//  4. For every existing node that gains at least one new value:
//     - Value list rebuild: align8(CellHeaderSize + valueListEntrySize*(existingValueCount+newValueCount))
//
// The accumulated total is validated to fit within int32 before being returned.
func Estimate(root *trie.Node) (*SpacePlan, error) {
	var (
		total    int64
		sp       SpacePlan
	)

	addCell := func(node *trie.Node, kind CellKind, size int) {
		aligned := format.Align8(size)
		total += int64(aligned)
		sp.Manifest = append(sp.Manifest, AllocEntry{
			Node: node,
			Kind: kind,
			Size: int32(aligned),
		})
	}

	err := trie.Walk(root, func(node *trie.Node, _ int) error {
		hasOps := node.EnsureKey || node.DeleteKey || len(node.Values) > 0 || len(node.Children) > 0

		// ── NK cell for new keys ─────────────────────────────────────────────
		if !node.Exists && hasOps {
			size := nkFixedPayload + len(node.Name)
			addCell(node, CellNK, size)
			sp.NewNKCount++
		}

		// ── VK and data cells for set-value operations ───────────────────────
		newValueCount := 0
		for _, vop := range node.Values {
			if vop.Delete {
				continue
			}

			// VK cell.
			vkSize := vkFixedPayload + len(vop.Name)
			addCell(node, CellVK, vkSize)
			sp.NewVKCount++
			newValueCount++

			// Data cell (omit for inline DWORDs: data <= 4 bytes).
			if len(vop.Data) > format.DWORDSize {
				dataSize := format.CellHeaderSize + len(vop.Data)
				addCell(node, CellData, dataSize)
				sp.NewDataCount++
			}
		}

		// ── Subkey list rebuild when an existing parent gains new children ───
		if node.Exists {
			newChildCount := 0
			for _, child := range node.Children {
				if !child.Exists {
					newChildCount++
				}
			}
			if newChildCount > 0 {
				totalEntries := int(node.SubKeyCount) + newChildCount
				listSize := format.CellHeaderSize + listHeaderSize + lhEntrySize*totalEntries
				addCell(node, CellSubkeyList, listSize)
				sp.ListRebuilds++
			}

			// ── Value list rebuild when an existing node gains new values ────
			if newValueCount > 0 {
				totalValues := int(node.ValueCount) + newValueCount
				vlistSize := format.CellHeaderSize + valueListEntrySize*totalValues
				addCell(node, CellValueList, vlistSize)
				sp.ListRebuilds++
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("plan.Estimate: walk failed: %w", err)
	}

	if total > math.MaxInt32 {
		return nil, fmt.Errorf("plan.Estimate: required space %d bytes exceeds int32 max (%d)",
			total, math.MaxInt32)
	}
	sp.TotalNewBytes = int32(total)
	return &sp, nil
}
