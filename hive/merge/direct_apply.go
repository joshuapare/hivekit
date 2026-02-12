// Package merge provides the direct-apply engine for append-only plan application.
//
// The direct applier matches hivex's approach: direct path lookups without
// building an in-memory index or walking the entire tree. This is optimal
// for append-only operations where we don't need to track existing cells.
package merge

import (
	"context"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/subkeys"
	"github.com/joshuapare/hivekit/internal/format"
)

// directApplier applies ops using direct path lookups without building an index.
// This matches hivex's approach and is optimal for append-only operations.
type directApplier struct {
	h     *hive.Hive
	alloc alloc.Allocator
	dt    *dirty.Tracker

	// Root cell offset
	rootRef uint32

	// Cache for resolved paths (path string -> nkOffset)
	// This is a per-merge cache, not a persistent index
	pathCache map[string]uint32

	// Reusable buffers
	offsetBuf []uint32

	// Results
	result Applied
}

// newDirectApplier creates an applier for direct path-based operations.
func newDirectApplier(h *hive.Hive, a alloc.Allocator, dt *dirty.Tracker) *directApplier {
	return &directApplier{
		h:         h,
		alloc:     a,
		dt:        dt,
		rootRef:   h.RootCellOffset(),
		pathCache: make(map[string]uint32, 256),
	}
}

// Apply executes all ops using direct path lookups.
func (da *directApplier) Apply(ctx context.Context, plan *Plan) (Applied, error) {
	for i := range plan.Ops {
		if err := ctx.Err(); err != nil {
			return da.result, err
		}

		op := &plan.Ops[i]
		if err := da.applyOp(ctx, op); err != nil {
			return da.result, fmt.Errorf("op %d (%s): %w", i, op.Type, err)
		}
	}

	return da.result, nil
}

// applyOp applies a single operation.
func (da *directApplier) applyOp(ctx context.Context, op *Op) error {
	switch op.Type {
	case OpEnsureKey:
		_, err := da.ensureKeyPath(op.KeyPath)
		if err == nil {
			da.result.KeysCreated++
		}
		return err

	case OpSetValue:
		nkRef, err := da.ensureKeyPath(op.KeyPath)
		if err != nil {
			return err
		}
		return da.setValue(nkRef, op.ValueName, op.ValueType, op.Data)

	case OpDeleteValue:
		nkRef, err := da.resolvePath(op.KeyPath)
		if err != nil {
			return nil // Key doesn't exist, value can't exist
		}
		return da.deleteValue(nkRef, op.ValueName)

	case OpDeleteKey:
		nkRef, err := da.resolvePath(op.KeyPath)
		if err != nil {
			return nil // Key doesn't exist, nothing to delete
		}
		return da.deleteKey(nkRef, op.KeyPath)
	}

	return nil
}

// resolvePath looks up a path and returns the NK offset.
// Uses cache for repeated lookups.
func (da *directApplier) resolvePath(path []string) (uint32, error) {
	if len(path) == 0 {
		return da.rootRef, nil
	}

	// Build full cache key first to check cache
	var cacheKeyBuilder strings.Builder
	cacheKeyBuilder.Grow(len(path) * 16)
	for i, segment := range path {
		if i > 0 {
			cacheKeyBuilder.WriteByte('\\')
		}
		for _, c := range segment {
			if c >= 'A' && c <= 'Z' {
				cacheKeyBuilder.WriteByte(byte(c + ('a' - 'A')))
			} else {
				cacheKeyBuilder.WriteRune(c)
			}
		}
	}
	fullCacheKey := cacheKeyBuilder.String()

	if ref, ok := da.pathCache[fullCacheKey]; ok {
		return ref, nil
	}

	// Walk from root to target, caching intermediate paths
	cacheKeyBuilder.Reset()
	currentRef := da.rootRef
	for i, segment := range path {
		if i > 0 {
			cacheKeyBuilder.WriteByte('\\')
		}
		for _, c := range segment {
			if c >= 'A' && c <= 'Z' {
				cacheKeyBuilder.WriteByte(byte(c + ('a' - 'A')))
			} else {
				cacheKeyBuilder.WriteRune(c)
			}
		}

		childRef, err := da.findChild(currentRef, segment)
		if err != nil {
			return 0, fmt.Errorf("segment %d (%s): %w", i, segment, err)
		}
		currentRef = childRef

		// Cache intermediate paths too
		da.pathCache[cacheKeyBuilder.String()] = currentRef
	}

	return currentRef, nil
}

// ensureKeyPath ensures all segments of a path exist, creating missing ones.
func (da *directApplier) ensureKeyPath(path []string) (uint32, error) {
	if len(path) == 0 {
		return da.rootRef, nil
	}

	// Build cache key incrementally to avoid O(n²) string allocations
	var cacheKeyBuilder strings.Builder
	cacheKeyBuilder.Grow(len(path) * 16) // Estimate avg segment length

	currentRef := da.rootRef
	for i, segment := range path {
		// Build cache key incrementally
		if i > 0 {
			cacheKeyBuilder.WriteByte('\\')
		}
		for _, c := range segment {
			if c >= 'A' && c <= 'Z' {
				cacheKeyBuilder.WriteByte(byte(c + ('a' - 'A')))
			} else {
				cacheKeyBuilder.WriteRune(c)
			}
		}
		cacheKey := cacheKeyBuilder.String()

		// Check cache first
		if ref, ok := da.pathCache[cacheKey]; ok {
			currentRef = ref
			continue
		}

		// Try to find existing child
		childRef, err := da.findChild(currentRef, segment)
		if err == nil {
			da.pathCache[cacheKey] = childRef
			currentRef = childRef
			continue
		}

		// Child doesn't exist - create it
		newRef, err := da.createKey(currentRef, segment)
		if err != nil {
			return 0, fmt.Errorf("create key %s: %w", segment, err)
		}
		da.pathCache[cacheKey] = newRef
		currentRef = newRef
	}

	return currentRef, nil
}

// findChild finds a child NK by name under the given parent.
func (da *directApplier) findChild(parentRef uint32, name string) (uint32, error) {
	payload, err := da.h.ResolveCellPayload(parentRef)
	if err != nil {
		return 0, err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return 0, err
	}

	if nk.SubkeyCount() == 0 {
		return 0, fmt.Errorf("key has no children")
	}

	subkeyListRef := nk.SubkeyListOffsetRel()
	if subkeyListRef == format.InvalidOffset {
		return 0, fmt.Errorf("invalid subkey list")
	}

	// Read all child offsets
	da.offsetBuf, err = subkeys.ReadOffsetsInto(da.h, subkeyListRef, da.offsetBuf)
	if err != nil {
		return 0, err
	}

	// Find matching child by name (case-insensitive, allocation-free)
	nameBytes := []byte(name)
	for _, childRef := range da.offsetBuf {
		childPayload, err := da.h.ResolveCellPayload(childRef)
		if err != nil {
			continue
		}

		childNK, err := hive.ParseNK(childPayload)
		if err != nil {
			continue
		}

		childName := childNK.Name()
		if equalFoldBytes(childName, nameBytes) {
			return childRef, nil
		}
	}

	return 0, fmt.Errorf("child not found: %s", name)
}

// equalFoldBytes compares two byte slices case-insensitively without allocation.
func equalFoldBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		// Fast path for matching bytes
		if ca == cb {
			continue
		}
		// Convert to lowercase for comparison (ASCII only for registry names)
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// createKey creates a new NK cell under the given parent.
func (da *directApplier) createKey(parentRef uint32, name string) (uint32, error) {
	// Allocate NK cell (payload + cell header)
	payloadSize := format.NKFixedHeaderSize + len(name)
	payloadSize = (payloadSize + 7) &^ 7 // 8-byte align

	nkRef, payload, err := da.alloc.Alloc(int32(payloadSize+format.CellHeaderSize), alloc.ClassNK)
	if err != nil {
		return 0, fmt.Errorf("alloc NK: %w", err)
	}

	// Initialize NK cell - payload starts after cell header (4 bytes)
	// Write NK signature "nk"
	copy(payload[format.NKSignatureOffset:], format.NKSignature)

	// Flags: KEY_COMP_NAME (0x0020) for compressed ASCII name
	format.PutU16(payload, format.NKFlagsOffset, format.NKFlagCompressedName)

	// Timestamp (zero for now)
	format.PutU64(payload, format.NKLastWriteOffset, 0)

	// Access bits (zero)
	format.PutU32(payload, format.NKAccessBitsOffset, 0)

	// Parent offset
	format.PutU32(payload, format.NKParentOffset, parentRef)

	// Subkey counts (0)
	format.PutU32(payload, format.NKSubkeyCountOffset, 0)    // stable subkeys
	format.PutU32(payload, format.NKVolSubkeyCountOffset, 0) // volatile subkeys

	// Subkey list offsets (invalid)
	format.PutU32(payload, format.NKSubkeyListOffset, format.InvalidOffset)    // stable list
	format.PutU32(payload, format.NKVolSubkeyListOffset, format.InvalidOffset) // volatile list

	// Value count (0)
	format.PutU32(payload, format.NKValueCountOffset, 0)

	// Value list offset (invalid)
	format.PutU32(payload, format.NKValueListOffset, format.InvalidOffset)

	// Security descriptor (invalid for now)
	format.PutU32(payload, format.NKSecurityOffset, format.InvalidOffset)

	// Class name (invalid)
	format.PutU32(payload, format.NKClassNameOffset, format.InvalidOffset)

	// Max name/class lengths (0)
	format.PutU32(payload, format.NKMaxNameLenOffset, 0)
	format.PutU32(payload, format.NKMaxClassLenOffset, 0)
	format.PutU32(payload, format.NKMaxValueNameOffset, 0)
	format.PutU32(payload, format.NKMaxValueDataOffset, 0)
	format.PutU32(payload, format.NKWorkVarOffset, 0)

	// Name length
	format.PutU16(payload, format.NKNameLenOffset, uint16(len(name)))

	// Class name length (0)
	format.PutU16(payload, format.NKClassLenOffset, 0)

	// Name (compressed ASCII)
	copy(payload[format.NKNameOffset:], name)

	// Mark dirty - calculate absolute offset
	offset := format.HeaderSize + int(nkRef)
	da.dt.Add(int(offset), int(payloadSize+format.CellHeaderSize))

	// Add to parent's subkey list
	if err := da.addSubkey(parentRef, nkRef, name); err != nil {
		return 0, fmt.Errorf("add to subkey list: %w", err)
	}

	return nkRef, nil
}

// addSubkey adds a subkey reference to the parent's subkey list.
func (da *directApplier) addSubkey(parentRef, childRef uint32, name string) error {
	// Read parent NK
	payload, err := da.h.ResolveCellPayload(parentRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	currentCount := nk.SubkeyCount()
	subkeyListRef := nk.SubkeyListOffsetRel()

	// If no existing list, create a new LF list
	if currentCount == 0 || subkeyListRef == format.InvalidOffset {
		return da.createSubkeyList(parentRef, childRef, name)
	}

	// Append to existing list
	return da.appendToSubkeyList(parentRef, subkeyListRef, childRef, name, currentCount)
}

// createSubkeyList creates a new LF subkey list with one entry.
func (da *directApplier) createSubkeyList(parentRef, childRef uint32, name string) error {
	// LF list: 4-byte header + 8 bytes per entry (4-byte offset + 4-byte hash hint)
	listPayloadSize := format.ListHeaderSize + format.LFEntrySize
	listPayloadSize = (listPayloadSize + 7) &^ 7

	listRef, payload, err := da.alloc.Alloc(int32(listPayloadSize+format.CellHeaderSize), alloc.ClassLF)
	if err != nil {
		return err
	}

	// Write LF signature "lf"
	copy(payload[format.IdxSignatureOffset:], format.LFSignature)

	// Count = 1 (at offset 2 in payload, after signature)
	format.PutU16(payload, format.IdxCountOffset, 1)

	// Entry: offset + hash hint (first 4 chars of name)
	format.PutU32(payload, format.IdxListOffset, childRef)

	// Hash hint: first 4 characters of name
	for i := 0; i < 4 && i < len(name); i++ {
		payload[format.IdxListOffset+4+i] = name[i]
	}

	offset := format.HeaderSize + int(listRef)
	da.dt.Add(int(offset), int(listPayloadSize+format.CellHeaderSize))

	// Update parent NK to point to new list
	return da.updateParentSubkeyList(parentRef, listRef, 1)
}

// appendToSubkeyList appends a new entry to an existing subkey list.
func (da *directApplier) appendToSubkeyList(parentRef, listRef, childRef uint32, name string, currentCount uint32) error {
	// Get old list payload BEFORE any allocation (allocation may invalidate payloads)
	oldPayload, err := da.h.ResolveCellPayload(listRef)
	if err != nil {
		return err
	}

	// Check old list type
	oldSig := string(oldPayload[0:2])

	// Handle RI lists specially - they have nested structure
	if oldSig == "ri" {
		return da.appendToRIList(parentRef, listRef, childRef, name, oldPayload)
	}

	// For lf/lh/li: allocate new larger list and copy
	newCount := currentCount + 1
	newListPayloadSize := format.ListHeaderSize + int(newCount)*format.LFEntrySize
	newListPayloadSize = (newListPayloadSize + 7) &^ 7

	// For lf/lh, we can bulk copy the existing entries
	// Cache the old entries data before allocation
	oldEntriesSize := int(currentCount) * format.LFEntrySize
	var oldEntries []byte
	if oldSig == "lf" || oldSig == "lh" {
		// Make a copy of entries since allocation may invalidate oldPayload
		oldEntries = make([]byte, oldEntriesSize)
		copy(oldEntries, oldPayload[format.IdxListOffset:format.IdxListOffset+oldEntriesSize])
	}

	newListRef, newPayload, err := da.alloc.Alloc(int32(newListPayloadSize+format.CellHeaderSize), alloc.ClassLF)
	if err != nil {
		return err
	}

	// Write LF signature for new list
	copy(newPayload[format.IdxSignatureOffset:], format.LFSignature)

	// New count
	format.PutU16(newPayload, format.IdxCountOffset, uint16(newCount))

	// Copy existing entries based on old list type
	switch oldSig {
	case "lf", "lh":
		// Bulk copy of entries from cached data
		copy(newPayload[format.IdxListOffset:], oldEntries)
	case "li":
		// LI has 4-byte entries (just offsets), convert to LF
		// Need to re-resolve oldPayload after allocation
		oldPayload, err = da.h.ResolveCellPayload(listRef)
		if err != nil {
			return err
		}
		for i := uint32(0); i < currentCount; i++ {
			srcOff := format.IdxListOffset + int(i)*format.LIEntrySize
			dstOff := format.IdxListOffset + int(i)*format.LFEntrySize

			entryRef := format.ReadU32(oldPayload, srcOff)
			format.PutU32(newPayload, dstOff, entryRef)

			// Get name hint from child
			childPayload, err := da.h.ResolveCellPayload(entryRef)
			if err == nil {
				childNK, err := hive.ParseNK(childPayload)
				if err == nil {
					childName := childNK.Name()
					for j := 0; j < 4 && j < len(childName); j++ {
						newPayload[dstOff+4+j] = childName[j]
					}
				}
			}
		}
	}

	// Add new entry
	newEntryOff := format.IdxListOffset + int(currentCount)*format.LFEntrySize
	format.PutU32(newPayload, newEntryOff, childRef)

	// Hash hint
	for i := 0; i < 4 && i < len(name); i++ {
		newPayload[newEntryOff+4+i] = name[i]
	}

	newOffset := format.HeaderSize + int(newListRef)
	da.dt.Add(int(newOffset), int(newListPayloadSize+format.CellHeaderSize))

	// Free old list (in append mode this is typically a no-op)
	_ = da.alloc.Free(listRef)

	// Update parent
	return da.updateParentSubkeyList(parentRef, newListRef, newCount)
}

// appendToRIList handles appending to an RI (index of indices) list.
// Following hivex's approach: find the last child LF/LH, reallocate it
// with the new entry, then update the RI pointer in-place.
func (da *directApplier) appendToRIList(parentRef, riRef, childRef uint32, name string, riPayload []byte) error {
	// RI structure: signature(2) + count(2) + offsets[](4 each)
	riCount := format.ReadU16(riPayload, format.IdxCountOffset)
	if riCount == 0 {
		return fmt.Errorf("empty RI list")
	}

	// Get the last child list reference (append to the last chunk)
	lastChildIdx := int(riCount) - 1
	lastChildOff := format.IdxListOffset + lastChildIdx*4
	lastChildRef := format.ReadU32(riPayload, lastChildOff)

	// Read the last child list BEFORE any allocation
	childPayload, err := da.h.ResolveCellPayload(lastChildRef)
	if err != nil {
		return fmt.Errorf("read RI child: %w", err)
	}

	childSig := string(childPayload[0:2])
	childCount := int(format.ReadU16(childPayload, format.IdxCountOffset))

	// Cache existing entries before allocation (allocation may invalidate childPayload)
	oldEntriesSize := childCount * format.LFEntrySize
	var oldEntries []byte
	if childSig == "lf" || childSig == "lh" {
		oldEntries = make([]byte, oldEntriesSize)
		copy(oldEntries, childPayload[format.IdxListOffset:format.IdxListOffset+oldEntriesSize])
	}

	// Create a new larger child list with the new entry
	newChildCount := childCount + 1
	newChildSize := format.ListHeaderSize + newChildCount*format.LFEntrySize
	newChildSize = (newChildSize + 7) &^ 7

	newChildRef, newChildPayload, err := da.alloc.Alloc(int32(newChildSize+format.CellHeaderSize), alloc.ClassLF)
	if err != nil {
		return err
	}

	// Write LF signature
	copy(newChildPayload[format.IdxSignatureOffset:], format.LFSignature)

	// Write new count
	format.PutU16(newChildPayload, format.IdxCountOffset, uint16(newChildCount))

	// Copy existing entries from cached data or convert from LI
	switch childSig {
	case "lf", "lh":
		// Bulk copy from cached entries
		copy(newChildPayload[format.IdxListOffset:], oldEntries)
	case "li":
		// LI has 4-byte entries, convert to LF
		// Re-resolve childPayload since allocation may have invalidated it
		childPayload, err = da.h.ResolveCellPayload(lastChildRef)
		if err != nil {
			return fmt.Errorf("re-read RI child: %w", err)
		}
		for i := 0; i < childCount; i++ {
			srcOff := format.IdxListOffset + i*format.LIEntrySize
			dstOff := format.IdxListOffset + i*format.LFEntrySize

			entryRef := format.ReadU32(childPayload, srcOff)
			format.PutU32(newChildPayload, dstOff, entryRef)

			// Get name hint from the entry's NK
			entryPayload, err := da.h.ResolveCellPayload(entryRef)
			if err == nil {
				entryNK, err := hive.ParseNK(entryPayload)
				if err == nil {
					entryName := entryNK.Name()
					for j := 0; j < 4 && j < len(entryName); j++ {
						newChildPayload[dstOff+4+j] = entryName[j]
					}
				}
			}
		}
	default:
		return fmt.Errorf("unexpected RI child type: %s", childSig)
	}

	// Add new entry at the end
	newEntryOff := format.IdxListOffset + childCount*format.LFEntrySize
	format.PutU32(newChildPayload, newEntryOff, childRef)
	for i := 0; i < 4 && i < len(name); i++ {
		newChildPayload[newEntryOff+4+i] = name[i]
	}

	// Mark new child dirty
	newChildOffset := format.HeaderSize + int(newChildRef)
	da.dt.Add(newChildOffset, newChildSize+format.CellHeaderSize)

	// Free old child (no-op in append mode)
	_ = da.alloc.Free(lastChildRef)

	// Update RI pointer in-place to point to new child
	format.PutU32(riPayload, lastChildOff, newChildRef)

	// Mark RI dirty (just the modified offset)
	riOffset := format.HeaderSize + int(riRef) + lastChildOff
	da.dt.Add(riOffset, 4)

	// Update parent NK subkey count (RI reference stays the same)
	nkPayload, err := da.h.ResolveCellPayload(parentRef)
	if err != nil {
		return err
	}
	currentCount := format.ReadU32(nkPayload, format.NKSubkeyCountOffset)
	format.PutU32(nkPayload, format.NKSubkeyCountOffset, currentCount+1)

	nkOffset := format.HeaderSize + int(parentRef)
	da.dt.Add(nkOffset+format.NKSubkeyCountOffset, 4)

	return nil
}

// updateParentSubkeyList updates the parent NK's subkey list reference and count.
func (da *directApplier) updateParentSubkeyList(parentRef, listRef uint32, count uint32) error {
	// Get parent cell payload directly
	payload, err := da.h.ResolveCellPayload(parentRef)
	if err != nil {
		return err
	}

	// Update stable subkey count
	format.PutU32(payload, format.NKSubkeyCountOffset, count)

	// Update stable subkey list offset
	format.PutU32(payload, format.NKSubkeyListOffset, listRef)

	// Mark parent dirty
	offset := format.HeaderSize + int(parentRef)
	da.dt.Add(int(offset), int(format.NKFixedHeaderSize+format.CellHeaderSize))

	return nil
}

// setValue sets a value on the given NK.
// In append-only mode, we always create a new VK cell.
func (da *directApplier) setValue(nkRef uint32, name string, valType uint32, data []byte) error {
	// Find existing value (to update the reference in value list)
	existingVK, existingIdx := da.findValue(nkRef, name)

	// Create new VK cell
	vkRef, err := da.createVK(name, valType, data)
	if err != nil {
		return err
	}

	if existingVK != 0 {
		// Update existing entry in value list
		return da.updateValueListEntry(nkRef, existingIdx, vkRef)
	}

	// Add new entry to value list
	err = da.addToValueList(nkRef, vkRef)
	if err == nil {
		da.result.ValuesSet++
	}
	return err
}

// findValue looks for a value by name, returns VK ref and index in value list.
func (da *directApplier) findValue(nkRef uint32, name string) (uint32, int) {
	payload, err := da.h.ResolveCellPayload(nkRef)
	if err != nil {
		return 0, -1
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return 0, -1
	}

	valueCount := nk.ValueCount()
	if valueCount == 0 {
		return 0, -1
	}

	valueListRef := nk.ValueListOffsetRel()
	if valueListRef == format.InvalidOffset {
		return 0, -1
	}

	listPayload, err := da.h.ResolveCellPayload(valueListRef)
	if err != nil {
		return 0, -1
	}

	// Value list is just an array of VK offsets (4 bytes each)
	nameLower := strings.ToLower(name)
	for i := uint32(0); i < valueCount; i++ {
		vkRef := format.ReadU32(listPayload, int(i)*4)

		vkPayload, err := da.h.ResolveCellPayload(vkRef)
		if err != nil {
			continue
		}

		vk, err := hive.ParseVK(vkPayload)
		if err != nil {
			continue
		}

		vkName := string(vk.Name())
		if strings.EqualFold(vkName, nameLower) {
			return vkRef, int(i)
		}
	}

	return 0, -1
}

// createVK creates a new VK cell with the given data.
func (da *directApplier) createVK(name string, valType uint32, valData []byte) (uint32, error) {
	// Determine if data is inline or external
	isInline := len(valData) <= 4

	// Allocate external data cell FIRST (before VK) to avoid re-resolve
	var dataRef uint32
	if !isInline {
		var err error
		dataRef, err = da.allocDataCell(valData)
		if err != nil {
			return 0, err
		}
	}

	// VK size: fixed header + name
	vkSize := format.VKFixedHeaderSize + len(name)
	vkSize = (vkSize + 7) &^ 7

	vkRef, payload, err := da.alloc.Alloc(int32(vkSize+format.CellHeaderSize), alloc.ClassVK)
	if err != nil {
		return 0, err
	}

	// Write VK signature "vk"
	copy(payload[format.VKSignatureOffset:], format.VKSignature)

	// Name length
	format.PutU16(payload, format.VKNameLenOffset, uint16(len(name)))

	// Data length + inline flag
	dataLen := uint32(len(valData))
	if isInline {
		dataLen |= format.VKDataInlineBit // Set high bit for inline
	}
	format.PutU32(payload, format.VKDataLenOffset, dataLen)

	// Data offset or inline data
	if isInline {
		// Store data inline at data offset field
		copy(payload[format.VKDataOffOffset:], valData)
		for i := len(valData); i < 4; i++ {
			payload[format.VKDataOffOffset+i] = 0
		}
	} else {
		format.PutU32(payload, format.VKDataOffOffset, dataRef)
	}

	// Value type
	format.PutU32(payload, format.VKTypeOffset, valType)

	// Flags: 1 = compressed name (ASCII)
	format.PutU16(payload, format.VKFlagsOffset, format.VKFlagNameCompressed)

	// Spare/padding
	format.PutU16(payload, format.VKSpareOffset, 0)

	// Name
	copy(payload[format.VKNameOffset:], name)

	offset := format.HeaderSize + int(vkRef)
	da.dt.Add(int(offset), int(vkSize+format.CellHeaderSize))

	return vkRef, nil
}

// allocDataCell allocates a data cell for external value data.
func (da *directApplier) allocDataCell(data []byte) (uint32, error) {
	cellSize := len(data)
	cellSize = (cellSize + 7) &^ 7

	// Use ClassDB for raw data (no signature)
	ref, payload, err := da.alloc.Alloc(int32(cellSize+format.CellHeaderSize), alloc.ClassRD)
	if err != nil {
		return 0, err
	}

	copy(payload, data)

	offset := format.HeaderSize + int(ref)
	da.dt.Add(int(offset), int(cellSize+format.CellHeaderSize))

	return ref, nil
}

// updateValueListEntry updates an existing entry in the value list.
func (da *directApplier) updateValueListEntry(nkRef uint32, entryIdx int, newVKRef uint32) error {
	payload, err := da.h.ResolveCellPayload(nkRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	valueListRef := nk.ValueListOffsetRel()
	listPayload, err := da.h.ResolveCellPayload(valueListRef)
	if err != nil {
		return err
	}

	// Update entry
	format.PutU32(listPayload, entryIdx*4, newVKRef)

	listOffset := format.HeaderSize + int(valueListRef)
	da.dt.Add(listOffset+entryIdx*4, 4)

	return nil
}

// addToValueList adds a new VK reference to the NK's value list.
func (da *directApplier) addToValueList(nkRef uint32, vkRef uint32) error {
	payload, err := da.h.ResolveCellPayload(nkRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	currentCount := nk.ValueCount()
	valueListRef := nk.ValueListOffsetRel()

	if currentCount == 0 || valueListRef == format.InvalidOffset {
		// Create new value list
		return da.createValueList(nkRef, vkRef)
	}

	// Append to existing list
	return da.appendToValueList(nkRef, valueListRef, vkRef, currentCount)
}

// createValueList creates a new value list with one entry.
func (da *directApplier) createValueList(nkRef, vkRef uint32) error {
	// Value list: just an array of VK offsets (4 bytes each)
	listSize := 4
	listSize = (listSize + 7) &^ 7

	listRef, payload, err := da.alloc.Alloc(int32(listSize+format.CellHeaderSize), alloc.ClassLI) // Use LI for simple list
	if err != nil {
		return err
	}

	// First entry
	format.PutU32(payload, 0, vkRef)

	listOffset := format.HeaderSize + int(listRef)
	da.dt.Add(int(listOffset), int(listSize+format.CellHeaderSize))

	// Update NK
	nkPayload, err := da.h.ResolveCellPayload(nkRef)
	if err != nil {
		return err
	}

	format.PutU32(nkPayload, format.NKValueCountOffset, 1)
	format.PutU32(nkPayload, format.NKValueListOffset, listRef)

	nkOffset := format.HeaderSize + int(nkRef)
	da.dt.Add(nkOffset+format.NKValueCountOffset, 8)

	return nil
}

// appendToValueList appends a VK ref to an existing value list.
func (da *directApplier) appendToValueList(nkRef, listRef, vkRef uint32, currentCount uint32) error {
	// Allocate new larger list
	newCount := currentCount + 1
	newListSize := int(newCount) * 4
	newListSize = (newListSize + 7) &^ 7

	// Add cell header size - Alloc expects total cell size, not just payload
	newListRef, newPayload, err := da.alloc.Alloc(int32(newListSize+format.CellHeaderSize), alloc.ClassLI)
	if err != nil {
		return err
	}

	oldPayload, err := da.h.ResolveCellPayload(listRef)
	if err != nil {
		return err
	}

	// Copy existing entries
	for i := uint32(0); i < currentCount; i++ {
		entry := format.ReadU32(oldPayload, int(i)*4)
		format.PutU32(newPayload, int(i)*4, entry)
	}

	// Add new entry
	format.PutU32(newPayload, int(currentCount)*4, vkRef)

	newOffset := format.HeaderSize + int(newListRef)
	da.dt.Add(int(newOffset), int(newListSize+format.CellHeaderSize))

	// Free old list (no-op in append mode)
	_ = da.alloc.Free(listRef)

	// Update NK
	nkPayload, err := da.h.ResolveCellPayload(nkRef)
	if err != nil {
		return err
	}

	format.PutU32(nkPayload, format.NKValueCountOffset, newCount)
	format.PutU32(nkPayload, format.NKValueListOffset, newListRef)

	nkOffset := format.HeaderSize + int(nkRef)
	da.dt.Add(nkOffset+format.NKValueCountOffset, 8)

	return nil
}

// deleteValue marks a value as deleted.
func (da *directApplier) deleteValue(nkRef uint32, name string) error {
	vkRef, idx := da.findValue(nkRef, name)
	if vkRef == 0 {
		return nil // Value doesn't exist
	}

	// Get NK payload
	nkPayload, err := da.h.ResolveCellPayload(nkRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		return err
	}

	currentCount := nk.ValueCount()
	if currentCount == 1 {
		// Last value - just clear the value list
		format.PutU32(nkPayload, format.NKValueCountOffset, 0)
		format.PutU32(nkPayload, format.NKValueListOffset, format.InvalidOffset)

		nkOffset := format.HeaderSize + int(nkRef)
		da.dt.Add(nkOffset+format.NKValueCountOffset, 8)
		da.result.ValuesDeleted++
		return nil
	}

	// Create new list without this entry
	valueListRef := nk.ValueListOffsetRel()
	newCount := currentCount - 1
	newListSize := int(newCount) * 4
	newListSize = (newListSize + 7) &^ 7

	newListRef, newPayload, err := da.alloc.Alloc(int32(newListSize+format.CellHeaderSize), alloc.ClassLI)
	if err != nil {
		return err
	}

	oldPayload, err := da.h.ResolveCellPayload(valueListRef)
	if err != nil {
		return err
	}

	// Copy entries except the deleted one
	newIdx := 0
	for i := uint32(0); i < currentCount; i++ {
		if int(i) == idx {
			continue
		}
		entry := format.ReadU32(oldPayload, int(i)*4)
		format.PutU32(newPayload, newIdx*4, entry)
		newIdx++
	}

	newOffset := format.HeaderSize + int(newListRef)
	da.dt.Add(int(newOffset), int(newListSize+format.CellHeaderSize))

	// Update NK
	format.PutU32(nkPayload, format.NKValueCountOffset, newCount)
	format.PutU32(nkPayload, format.NKValueListOffset, newListRef)

	nkOffset := format.HeaderSize + int(nkRef)
	da.dt.Add(nkOffset+format.NKValueCountOffset, 8)

	da.result.ValuesDeleted++
	return nil
}

// deleteKey marks a key and its subtree as deleted.
func (da *directApplier) deleteKey(nkRef uint32, path []string) error {
	// Get parent
	if len(path) == 0 {
		return fmt.Errorf("cannot delete root")
	}

	parentPath := path[:len(path)-1]

	parentRef, err := da.resolvePath(parentPath)
	if err != nil {
		return nil // Parent doesn't exist
	}

	// Remove from parent's subkey list
	return da.removeFromSubkeyList(parentRef, nkRef)
}

// removeFromSubkeyList removes a subkey from the parent's subkey list.
func (da *directApplier) removeFromSubkeyList(parentRef, childRef uint32) error {
	payload, err := da.h.ResolveCellPayload(parentRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(payload)
	if err != nil {
		return err
	}

	currentCount := nk.SubkeyCount()
	if currentCount == 0 {
		return nil
	}

	subkeyListRef := nk.SubkeyListOffsetRel()
	if subkeyListRef == format.InvalidOffset {
		return nil
	}

	// Read existing offsets
	da.offsetBuf, err = subkeys.ReadOffsetsInto(da.h, subkeyListRef, da.offsetBuf)
	if err != nil {
		return err
	}

	// Find the entry to remove
	removeIdx := -1
	for i, ref := range da.offsetBuf {
		if ref == childRef {
			removeIdx = i
			break
		}
	}

	if removeIdx == -1 {
		return nil // Not found
	}

	if currentCount == 1 {
		// Last subkey - clear the list
		format.PutU32(payload, format.NKSubkeyCountOffset, 0)
		format.PutU32(payload, format.NKSubkeyListOffset, format.InvalidOffset)

		nkOffset := format.HeaderSize + int(parentRef)
		da.dt.Add(nkOffset+format.NKSubkeyCountOffset, 16)
		da.result.KeysDeleted++
		return nil
	}

	// Create new list without this entry
	newCount := currentCount - 1
	newListSize := format.ListHeaderSize + int(newCount)*format.LFEntrySize
	newListSize = (newListSize + 7) &^ 7

	newListRef, newPayload, err := da.alloc.Alloc(int32(newListSize+format.CellHeaderSize), alloc.ClassLF)
	if err != nil {
		return err
	}

	// Write LF signature
	copy(newPayload[format.IdxSignatureOffset:], format.LFSignature)

	// Count
	format.PutU16(newPayload, format.IdxCountOffset, uint16(newCount))

	// Copy entries except removed one
	newIdx := 0
	for i, ref := range da.offsetBuf {
		if i == removeIdx {
			continue
		}

		entryOff := format.IdxListOffset + newIdx*format.LFEntrySize
		format.PutU32(newPayload, entryOff, ref)

		// Get name hint
		childPayload, _ := da.h.ResolveCellPayload(ref)
		if childPayload != nil {
			childNK, _ := hive.ParseNK(childPayload)
			if err == nil {
				childName := childNK.Name()
				for j := 0; j < 4 && j < len(childName); j++ {
					newPayload[entryOff+4+j] = childName[j]
				}
			}
		}
		newIdx++
	}

	newOffset := format.HeaderSize + int(newListRef)
	da.dt.Add(int(newOffset), int(newListSize+format.CellHeaderSize))

	// Re-resolve parent payload after allocation (may have moved due to buffer realloc)
	payload, err = da.h.ResolveCellPayload(parentRef)
	if err != nil {
		return err
	}

	// Update parent
	format.PutU32(payload, format.NKSubkeyCountOffset, newCount)
	format.PutU32(payload, format.NKSubkeyListOffset, newListRef)

	nkOffset := format.HeaderSize + int(parentRef)
	da.dt.Add(nkOffset+format.NKSubkeyCountOffset, 16)

	da.result.KeysDeleted++
	return nil
}

// directApplySession wraps directApplier for session-level operations.
type directApplySession struct {
	h     *hive.Hive
	alloc alloc.Allocator
	dt    *dirty.Tracker
}

// newDirectApplySession creates a session for direct-apply operations.
func newDirectApplySession(h *hive.Hive, a alloc.Allocator, dt *dirty.Tracker) *directApplySession {
	return &directApplySession{
		h:     h,
		alloc: a,
		dt:    dt,
	}
}

// ApplyPlan applies a plan using direct path lookups.
func (das *directApplySession) ApplyPlan(ctx context.Context, plan *Plan) (Applied, error) {
	da := newDirectApplier(das.h, das.alloc, das.dt)
	return da.Apply(ctx, plan)
}
