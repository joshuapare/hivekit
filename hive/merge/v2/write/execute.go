package write

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/merge/v2/plan"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// Execute walks the annotated trie and writes new cells via the allocator,
// returning a list of in-place updates to apply and write statistics.
//
// For new keys: allocates NK cell via bump allocator, writes NK bytes.
// For new values: allocates VK + data cells, writes bytes.
// For subkey list rebuilds: reads old list via subkeys.ReadRaw, merges with
// new entries, allocates new LH cell, queues in-place update to parent NK.
// For value operations on existing keys: similar pattern.
func Execute(
	h *hive.Hive,
	root *trie.Node,
	_ *plan.SpacePlan,
	fa alloc.Allocator,
) ([]InPlaceUpdate, WriteStats, error) {
	ex := &executor{
		h:               h,
		fa:              fa,
		updates:         make([]InPlaceUpdate, 0, 64),
		skRefIncrements: make(map[uint32]uint32),
	}

	if err := ex.processNode(root); err != nil {
		return nil, WriteStats{}, err
	}

	if err := ex.flushSKRefcounts(); err != nil {
		return nil, WriteStats{}, err
	}

	return ex.updates, ex.stats, nil
}

// executor holds mutable state during the write-phase walk.
type executor struct {
	h               *hive.Hive
	fa              alloc.Allocator
	updates         []InPlaceUpdate
	stats           WriteStats
	skRefIncrements map[uint32]uint32 // SK cell ref → number of new NKs inheriting it
}

// processNode recursively processes a trie node and its children.
func (ex *executor) processNode(node *trie.Node) error {
	// Count new children BEFORE processing them, because createNewKey sets
	// child.Exists = true. We need the original count to know whether the
	// parent's subkey list must be rebuilt.
	newChildCount := 0
	hasDeletedChild := false
	if node.Exists {
		for _, child := range node.Children {
			if !child.Exists {
				newChildCount++
			}
			if child.DeleteKey {
				hasDeletedChild = true
			}
		}
	}

	// Process children first (bottom-up for subkey list builds).
	// We need to process children before the parent so that child NK cell
	// references are available when building the parent's subkey list.
	for _, child := range node.Children {
		if err := ex.processChild(node, child); err != nil {
			return err
		}
	}

	// Handle value operations on this node (if it exists).
	if node.Exists && len(node.Values) > 0 {
		if err := ex.processValues(node); err != nil {
			return fmt.Errorf("values for %q: %w", node.Name, err)
		}
	}

	// Rebuild subkey list if this existing node has new or deleted children.
	if node.Exists && (newChildCount > 0 || hasDeletedChild) {
		if err := ex.rebuildSubkeyList(node); err != nil {
			return fmt.Errorf("subkey list for %q: %w", node.Name, err)
		}
	}

	return nil
}

// processChild handles a single child node. If the child is new, it allocates
// an NK cell and recursively processes the child's own children and values.
func (ex *executor) processChild(parent, child *trie.Node) error {
	if child.DeleteKey {
		// Mark for deletion. The actual deletion is deferred to the flush phase.
		ex.stats.KeysDeleted++
		return nil
	}

	if !child.Exists {
		// New key: allocate NK cell.
		if err := ex.createNewKey(parent, child); err != nil {
			return fmt.Errorf("create key %q: %w", child.Name, err)
		}
	}

	// Recurse into the child to process its own children and values.
	return ex.processNode(child)
}

// createNewKey allocates a new NK cell for a trie node that does not exist
// in the hive. After allocation, the node's CellIdx is updated so that
// parent subkey list rebuilds can reference it.
func (ex *executor) createNewKey(parent, node *trie.Node) error {
	// Determine parent ref for the NK cell.
	parentRef := parent.CellIdx

	// Determine SK ref: inherit from parent.
	skRef := parent.SKCellIdx

	// Payload size.
	payloadSize := NKPayloadSize(node.Name)
	totalSize := int32(payloadSize + format.CellHeaderSize)

	ref, buf, err := ex.fa.Alloc(totalSize, alloc.ClassNK)
	if err != nil {
		return fmt.Errorf("alloc NK: %w", err)
	}

	WriteNK(buf, node.Name, parentRef, skRef)

	// Update the trie node so downstream code can reference it.
	node.CellIdx = ref
	node.Exists = true // It now exists in the hive.
	node.SKCellIdx = skRef
	node.SubKeyListRef = format.InvalidOffset
	node.SubKeyCount = 0
	node.ValueListRef = format.InvalidOffset
	node.ValueCount = 0

	ex.stats.KeysCreated++

	// Track that this new NK inherits the parent's SK cell.
	if skRef != format.InvalidOffset {
		ex.skRefIncrements[skRef]++
	}

	// Note: values for this newly created key are handled by processNode
	// after processChild returns, since Exists is now true.

	return nil
}

// processValues handles value operations (set/delete) on a node.
// For set operations, it allocates VK and data cells.
// For delete operations, existing VK refs with the matching name are removed.
// For set operations on existing names (replacements), the old VK ref is
// replaced by the new one.
// For nodes with any value changes, it rebuilds the value list.
func (ex *executor) processValues(node *trie.Node) error {
	var newVKRefs []uint32
	for _, vop := range node.Values {
		if vop.Delete {
			ex.stats.ValuesDeleted++
			continue
		}

		// Allocate data cell if needed (external storage).
		var dataRef uint32
		if len(vop.Data) > format.DWORDSize {
			dataCellSize := int32(len(vop.Data) + format.CellHeaderSize)
			dRef, dBuf, err := ex.fa.Alloc(dataCellSize, alloc.ClassRD)
			if err != nil {
				return fmt.Errorf("alloc data cell for value %q: %w", vop.Name, err)
			}
			WriteDataCell(dBuf, vop.Data)
			dataRef = dRef
		}

		payloadSize := VKPayloadSize(vop.Name)
		totalSize := int32(payloadSize + format.CellHeaderSize)

		vkRef, vkBuf, err := ex.fa.Alloc(totalSize, alloc.ClassVK)
		if err != nil {
			return fmt.Errorf("alloc VK for value %q: %w", vop.Name, err)
		}

		WriteVK(vkBuf, vop.Name, vop.Type, vop.Data, dataRef)

		newVKRefs = append(newVKRefs, vkRef)
		ex.stats.ValuesSet++
	}

	// Fast path: no existing values
	if node.ValueCount == 0 || node.ValueListRef == format.InvalidOffset {
		return ex.writeValueList(node, nil, newVKRefs)
	}

	// Slow path: filter existing values by byte-level name comparison.
	modifiedNamesLower := make([]string, 0, len(node.Values))
	for _, vop := range node.Values {
		modifiedNamesLower = append(modifiedNamesLower, strings.ToLower(vop.Name))
	}

	existingRefs, err := ex.readExistingValueList(node)
	if err != nil {
		return fmt.Errorf("read value list: %w", err)
	}

	var filteredRefs []uint32
	for _, ref := range existingRefs {
		payload, resolveErr := ex.h.ResolveCellPayload(ref)
		if resolveErr != nil {
			return fmt.Errorf("resolve VK at 0x%X: %w", ref, resolveErr)
		}
		vk, parseErr := hive.ParseVK(payload)
		if parseErr != nil {
			return fmt.Errorf("parse VK at 0x%X: %w", ref, parseErr)
		}

		nameBytes := vk.Name()
		isModified := false
		if vk.NameCompressed() && nameBytes != nil {
			for _, target := range modifiedNamesLower {
				if subkeys.CompressedNameEqualsLower(nameBytes, target) {
					isModified = true
					break
				}
			}
		} else {
			name := ex.decodeVKName(vk)
			nameLower := strings.ToLower(name)
			for _, target := range modifiedNamesLower {
				if nameLower == target {
					isModified = true
					break
				}
			}
		}

		if isModified {
			continue
		}
		filteredRefs = append(filteredRefs, ref)
	}

	return ex.writeValueList(node, filteredRefs, newVKRefs)
}

// writeValueList allocates and writes a value list cell from filtered + new refs.
func (ex *executor) writeValueList(node *trie.Node, filteredRefs, newVKRefs []uint32) error {
	allRefs := append(filteredRefs, newVKRefs...)
	if len(allRefs) == 0 {
		ex.queueNKValueListUpdate(node, format.InvalidOffset, 0)
		return nil
	}
	vlistPayloadSize := ValueListPayloadSize(len(allRefs))
	vlistTotalSize := int32(vlistPayloadSize + format.CellHeaderSize)
	vlistRef, vlistBuf, err := ex.fa.Alloc(vlistTotalSize, alloc.ClassLH)
	if err != nil {
		return fmt.Errorf("alloc value list: %w", err)
	}
	WriteValueList(vlistBuf, allRefs)
	ex.queueNKValueListUpdate(node, vlistRef, uint32(len(allRefs)))
	return nil
}

// decodeVKName decodes a VK name from its raw bytes (UTF-16LE for non-compressed names).
func (ex *executor) decodeVKName(vk hive.VK) string {
	nameBytes := vk.Name()
	if nameBytes == nil {
		return ""
	}
	if len(nameBytes)%2 != 0 {
		return ""
	}
	u16s := make([]uint16, len(nameBytes)/2)
	for i := range u16s {
		u16s[i] = binary.LittleEndian.Uint16(nameBytes[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16s))
}

// rebuildSubkeyList reads the existing subkey list, merges in new children,
// and allocates a new LH list cell. Queues an in-place update to the parent NK.
func (ex *executor) rebuildSubkeyList(parent *trie.Node) error {
	oldListRef := parent.SubKeyListRef
	var oldRaw []subkeys.RawEntry
	if oldListRef != format.InvalidOffset && parent.SubKeyCount > 0 {
		raw, err := subkeys.ReadRaw(ex.h, oldListRef)
		if err != nil {
			return fmt.Errorf("read existing subkey list: %w", err)
		}
		oldRaw = raw
	}

	deletedRefs := make(map[uint32]bool)
	for _, child := range parent.Children {
		if child.DeleteKey && child.CellIdx != format.InvalidOffset {
			deletedRefs[child.CellIdx] = true
		}
	}

	merged := MergeRawWithInserts(oldRaw, parent.Children, deletedRefs)

	if len(merged) == 0 {
		ex.queueNKSubkeyListUpdate(parent, format.InvalidOffset, 0)
		return nil
	}

	listRef, err := subkeys.WriteRaw(ex.h, ex.fa, merged)
	if err != nil {
		return fmt.Errorf("write merged subkey list: %w", err)
	}

	ex.queueNKSubkeyListUpdate(parent, listRef, uint32(len(merged)))
	return nil
}

// categorySKRefcount matches flush.CategorySKRefcount. Defined here to avoid
// a circular import (write cannot import flush).
const categorySKRefcount = 1

// flushSKRefcounts reads the current refcount for each SK cell that gained
// references and queues an InPlaceUpdate to write the incremented value.
func (ex *executor) flushSKRefcounts() error {
	for skRef, increment := range ex.skRefIncrements {
		payload, err := ex.h.ResolveCellPayload(skRef)
		if err != nil {
			return fmt.Errorf("read SK cell at 0x%X: %w", skRef, err)
		}
		if len(payload) < format.SKReferenceCountOffset+4 {
			return fmt.Errorf("SK cell at 0x%X too small (%d bytes)", skRef, len(payload))
		}

		currentRefcount := binary.LittleEndian.Uint32(
			payload[format.SKReferenceCountOffset : format.SKReferenceCountOffset+4],
		)
		newRefcount := currentRefcount + increment

		absOffset := int64(format.HeaderSize) + int64(skRef) + int64(format.CellHeaderSize) + int64(format.SKReferenceCountOffset)
		if absOffset < 0 || absOffset > int64(math.MaxInt32) {
			return fmt.Errorf("SK cell at 0x%X: computed offset %d overflows int32", skRef, absOffset)
		}

		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], newRefcount)
		ex.updates = append(ex.updates, InPlaceUpdate{
			Offset:   int32(absOffset),
			Data:     buf[:],
			Category: categorySKRefcount,
		})
	}
	return nil
}

// readExistingValueList reads the current value list offsets from an NK cell.
func (ex *executor) readExistingValueList(node *trie.Node) ([]uint32, error) {
	if node.ValueListRef == format.InvalidOffset || node.ValueCount == 0 {
		return nil, nil
	}

	payload, err := ex.h.ResolveCellPayload(node.ValueListRef)
	if err != nil {
		return nil, err
	}

	count := int(node.ValueCount)
	refs := make([]uint32, 0, count)
	for i := 0; i < count; i++ {
		off := i * format.DWORDSize
		if off+4 > len(payload) {
			break
		}
		refs = append(refs, format.ReadU32(payload, off))
	}

	return refs, nil
}

// queueNKSubkeyListUpdate queues an in-place update to an existing NK cell's
// subkey count and subkey list offset fields.
func (ex *executor) queueNKSubkeyListUpdate(node *trie.Node, listRef uint32, count uint32) {
	absOffset := int32(format.HeaderSize) + int32(node.CellIdx) + int32(format.CellHeaderSize)

	// Subkey count at NKSubkeyCountOffset.
	var countBuf [4]byte
	format.PutU32(countBuf[:], 0, count)
	ex.updates = append(ex.updates, InPlaceUpdate{
		Offset: absOffset + int32(format.NKSubkeyCountOffset),
		Data:   append([]byte(nil), countBuf[:]...),
	})

	// Subkey list offset at NKSubkeyListOffset.
	var listBuf [4]byte
	format.PutU32(listBuf[:], 0, listRef)
	ex.updates = append(ex.updates, InPlaceUpdate{
		Offset: absOffset + int32(format.NKSubkeyListOffset),
		Data:   append([]byte(nil), listBuf[:]...),
	})

	// Update trie node's cached state.
	node.SubKeyCount = count
	node.SubKeyListRef = listRef
}

// queueNKValueListUpdate queues an in-place update to an existing NK cell's
// value count and value list offset fields.
func (ex *executor) queueNKValueListUpdate(node *trie.Node, listRef uint32, count uint32) {
	absOffset := int32(format.HeaderSize) + int32(node.CellIdx) + int32(format.CellHeaderSize)

	// Value count at NKValueCountOffset.
	var countBuf [4]byte
	format.PutU32(countBuf[:], 0, count)
	ex.updates = append(ex.updates, InPlaceUpdate{
		Offset: absOffset + int32(format.NKValueCountOffset),
		Data:   append([]byte(nil), countBuf[:]...),
	})

	// Value list offset at NKValueListOffset.
	var listBuf [4]byte
	format.PutU32(listBuf[:], 0, listRef)
	ex.updates = append(ex.updates, InPlaceUpdate{
		Offset: absOffset + int32(format.NKValueListOffset),
		Data:   append([]byte(nil), listBuf[:]...),
	})

	// Update trie node's cached state.
	node.ValueCount = count
	node.ValueListRef = listRef
}
