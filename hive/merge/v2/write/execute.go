package write

import (
	"encoding/binary"
	"fmt"
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
		h:       h,
		fa:      fa,
		updates: make([]InPlaceUpdate, 0, 64),
	}

	if err := ex.processNode(root); err != nil {
		return nil, WriteStats{}, err
	}

	return ex.updates, ex.stats, nil
}

// executor holds mutable state during the write-phase walk.
type executor struct {
	h       *hive.Hive
	fa      alloc.Allocator
	updates []InPlaceUpdate
	stats   WriteStats
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
	// Build a set of value names being modified (set or deleted).
	// These names will be removed from the existing value list to avoid
	// duplicates (for replacements) or stale entries (for deletes).
	modifiedNames := make(map[string]bool, len(node.Values))
	for _, vop := range node.Values {
		modifiedNames[strings.ToLower(vop.Name)] = true
	}

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

		// Allocate VK cell.
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

	// Read existing value list.
	existingRefs, err := ex.readExistingValueList(node)
	if err != nil {
		return fmt.Errorf("read value list: %w", err)
	}

	// Filter existing refs: remove any whose name matches a deleted or
	// replaced value so we don't produce duplicates or keep stale entries.
	var filteredRefs []uint32
	for _, ref := range existingRefs {
		name, resolveErr := ex.resolveVKName(ref)
		if resolveErr != nil {
			return fmt.Errorf("resolve VK name for ref 0x%X: %w", ref, resolveErr)
		}
		if modifiedNames[strings.ToLower(name)] {
			continue // skip: this value is being replaced or deleted
		}
		filteredRefs = append(filteredRefs, ref)
	}

	allRefs := append(filteredRefs, newVKRefs...)

	if len(allRefs) == 0 {
		// All values were deleted; clear the value list.
		ex.queueNKValueListUpdate(node, format.InvalidOffset, 0)
		return nil
	}

	// Allocate and write new value list cell.
	vlistPayloadSize := ValueListPayloadSize(len(allRefs))
	vlistTotalSize := int32(vlistPayloadSize + format.CellHeaderSize)

	vlistRef, vlistBuf, err := ex.fa.Alloc(vlistTotalSize, alloc.ClassLH) // value list reuses LH class
	if err != nil {
		return fmt.Errorf("alloc value list: %w", err)
	}

	WriteValueList(vlistBuf, allRefs)

	// Queue in-place update to the parent NK to point to the new value list.
	ex.queueNKValueListUpdate(node, vlistRef, uint32(len(allRefs)))

	return nil
}

// rebuildSubkeyList reads the existing subkey list, merges in new children,
// and allocates a new LH list cell. Queues an in-place update to the parent NK.
func (ex *executor) rebuildSubkeyList(parent *trie.Node) error {
	// Read existing entries.
	var oldEntries []subkeys.Entry
	if parent.SubKeyListRef != format.InvalidOffset && parent.SubKeyCount > 0 {
		raw, err := subkeys.ReadRaw(ex.h, parent.SubKeyListRef)
		if err != nil {
			return fmt.Errorf("read existing subkey list: %w", err)
		}
		// Convert RawEntry to Entry for merge (we need NameLower for sorting).
		// For existing entries, resolve each NK to get the name.
		oldEntries = make([]subkeys.Entry, 0, len(raw))
		for _, r := range raw {
			name, nameErr := ex.resolveNKName(r.NKRef)
			if nameErr != nil {
				return fmt.Errorf("resolve NK name for ref 0x%X: %w", r.NKRef, nameErr)
			}
			oldEntries = append(oldEntries, subkeys.Entry{
				NameLower: strings.ToLower(name),
				NKRef:     r.NKRef,
				Hash:      r.Hash,
			})
		}
	}

	// Build a set of old NKRefs for O(1) lookup.
	oldNKRefs := make(map[uint32]bool, len(oldEntries))
	for _, old := range oldEntries {
		oldNKRefs[old.NKRef] = true
	}

	// Build new entries from children that are new.
	var newEntries []subkeys.Entry
	deletedRefs := make(map[uint32]bool)
	for _, child := range parent.Children {
		if child.DeleteKey {
			if child.CellIdx != format.InvalidOffset {
				deletedRefs[child.CellIdx] = true
			}
			continue
		}
		if child.CellIdx == format.InvalidOffset {
			continue
		}
		// Only add children that were newly created in this pass.
		// Existing children are already in the old list.
		if !oldNKRefs[child.CellIdx] {
			hash := child.Hash
			if hash == 0 {
				hash = subkeys.Hash(child.Name)
			}
			newEntries = append(newEntries, subkeys.Entry{
				NameLower: child.NameLower,
				NKRef:     child.CellIdx,
				Hash:      hash,
			})
		}
	}

	// Merge.
	merged := MergeSortedEntries(oldEntries, newEntries, deletedRefs)

	if len(merged) == 0 {
		// All children were deleted; set subkey list to InvalidOffset.
		ex.queueNKSubkeyListUpdate(parent, format.InvalidOffset, 0)
		return nil
	}

	// Convert to RawEntry for writing.
	rawMerged := make([]subkeys.RawEntry, len(merged))
	for i, e := range merged {
		hash := e.Hash
		if hash == 0 {
			hash = subkeys.Hash(e.NameLower)
		}
		rawMerged[i] = subkeys.RawEntry{NKRef: e.NKRef, Hash: hash}
	}

	// Allocate and write the new LH list using the subkeys package writer.
	listRef, err := subkeys.WriteRaw(ex.h, ex.fa, rawMerged)
	if err != nil {
		return fmt.Errorf("write merged subkey list: %w", err)
	}

	// Queue in-place update to the parent NK.
	ex.queueNKSubkeyListUpdate(parent, listRef, uint32(len(merged)))

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

// resolveNKName reads the name from an NK cell at the given offset.
func (ex *executor) resolveNKName(nkRef uint32) (string, error) {
	payload, err := ex.h.ResolveCellPayload(nkRef)
	if err != nil {
		return "", err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return "", err
	}

	nameBytes := nk.Name()
	if nameBytes == nil {
		return "", nil
	}

	// Compressed (ASCII) names are the common case.
	if nk.IsCompressedName() {
		return string(nameBytes), nil
	}

	// UTF-16LE: decode to Go string.
	if len(nameBytes)%2 != 0 {
		return "", fmt.Errorf("UTF-16LE name has odd byte count: %d", len(nameBytes))
	}
	u16s := make([]uint16, len(nameBytes)/2)
	for i := range u16s {
		u16s[i] = binary.LittleEndian.Uint16(nameBytes[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16s)), nil
}

// resolveVKName reads the name from a VK cell at the given offset.
func (ex *executor) resolveVKName(vkRef uint32) (string, error) {
	payload, err := ex.h.ResolveCellPayload(vkRef)
	if err != nil {
		return "", err
	}

	vk, err := hive.ParseVK(payload)
	if err != nil {
		return "", err
	}

	nameBytes := vk.Name()
	if nameBytes == nil {
		return "", nil // default (unnamed) value
	}

	if vk.NameCompressed() {
		return string(nameBytes), nil
	}

	// UTF-16LE: decode to Go string.
	if len(nameBytes)%2 != 0 {
		return "", fmt.Errorf("UTF-16LE VK name has odd byte count: %d", len(nameBytes))
	}
	u16s := make([]uint16, len(nameBytes)/2)
	for i := range u16s {
		u16s[i] = binary.LittleEndian.Uint16(nameBytes[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16s)), nil
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
