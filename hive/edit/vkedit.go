package edit

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/hive/bigdata"
	"github.com/joshuapare/hivekit/hive/dirty"
	"github.com/joshuapare/hivekit/hive/index"
	"github.com/joshuapare/hivekit/hive/values"
	"github.com/joshuapare/hivekit/internal/format"
)

// valueEditor implements ValueEditor interface.
type valueEditor struct {
	h       *hive.Hive
	alloc   alloc.Allocator
	index   index.Index
	bigdata *bigdata.Writer
	dt      dirty.DirtyTracker
}

// NewValueEditor creates a new value editor.
func NewValueEditor(
	h *hive.Hive,
	allocator alloc.Allocator,
	idx index.Index,
	dt dirty.DirtyTracker,
) ValueEditor {
	return &valueEditor{
		h:       h,
		alloc:   allocator,
		index:   idx,
		bigdata: bigdata.NewWriter(h, allocator, dt),
		dt:      dt,
	}
}

// UpsertValue creates or updates a value under the given NK (case-insensitive name).
// Automatically chooses inline/external/DB storage based on data size.
func (ve *valueEditor) UpsertValue(nk NKRef, name string, typ ValueType, data []byte) error {
	if nk == 0 {
		return ErrInvalidRef
	}

	// Check if value already exists (index handles case-insensitivity)
	existingRef, exists := ve.index.GetVK(nk, name)
	if exists {
		// Value exists - check if we need to update it
		needsUpdate, err := ve.needsUpdate(existingRef, typ, data)
		if err != nil {
			return fmt.Errorf("check existing value: %w", err)
		}
		if !needsUpdate {
			// Value is unchanged, no-op
			return nil
		}

		// Update existing value
		return ve.updateValue(nk, existingRef, name, typ, data)
	}

	// Value doesn't exist - create new one
	return ve.createValue(nk, name, typ, data)
}

// needsUpdate checks if an existing value needs to be updated.
func (ve *valueEditor) needsUpdate(vkRef VKRef, newType ValueType, newData []byte) (bool, error) {
	vkPayload, err := ve.resolveCell(vkRef)
	if err != nil {
		return true, err // If we can't read it, assume update needed
	}

	vk, err := hive.ParseVK(vkPayload)
	if err != nil {
		return true, err
	}

	// Check type
	if vk.Type() != newType {
		return true, nil
	}

	// Check data length
	if vk.DataLen() != len(newData) {
		return true, nil
	}

	// Check data content
	existingData, err := vk.Data(ve.h.Bytes())
	if err != nil {
		return true, err
	}

	return !bytes.Equal(existingData, newData), nil
}

// createValue creates a new VK and adds it to the parent NK's value list.
func (ve *valueEditor) createValue(nkRef NKRef, name string, typ ValueType, data []byte) error {
	// Allocate VK cell
	vkRef, err := ve.allocateVK(name, typ, data)
	if err != nil {
		return fmt.Errorf("allocate VK: %w", err)
	}

	// Add to NK's value list
	if addErr := ve.addToValueList(nkRef, vkRef); addErr != nil {
		return fmt.Errorf("add to value list: %w", addErr)
	}

	// Register in index (index handles case-insensitivity)
	ve.index.AddVK(nkRef, name, vkRef)

	return nil
}

// updateValue updates an existing VK.
func (ve *valueEditor) updateValue(
	nkRef NKRef,
	vkRef VKRef,
	name string,
	typ ValueType,
	data []byte,
) error {
	// For simplicity in phase 1, we'll reallocate the VK rather than trying to reuse
	// This is safe but less efficient - could be optimized later

	// Allocate new VK
	newVKRef, err := ve.allocateVK(name, typ, data)
	if err != nil {
		return fmt.Errorf("allocate new VK: %w", err)
	}

	// Update the value list to point to new VK
	if replaceErr := ve.replaceInValueList(nkRef, vkRef, newVKRef); replaceErr != nil {
		return fmt.Errorf("replace in value list: %w", replaceErr)
	}

	// Update index (index handles case-insensitivity)
	ve.index.AddVK(nkRef, name, newVKRef)

	// CRITICAL: Free the old VK cell and its data to prevent memory leak
	if freeErr := ve.freeVK(vkRef); freeErr != nil {
		return fmt.Errorf("free old VK: %w", freeErr)
	}

	return nil
}

// freeVK frees a VK cell and its associated data (if external/bigdata).
// This is critical to prevent memory leaks when updating values.
func (ve *valueEditor) freeVK(vkRef VKRef) error {
	if vkRef == 0 {
		return nil // Nothing to free
	}

	// Get the VK cell buffer to determine storage type
	vkBuf, err := ve.resolveCell(vkRef)
	if err != nil {
		return fmt.Errorf("resolve VK cell: %w", err)
	}

	// Parse VK to determine storage type
	vk, err := hive.ParseVK(vkBuf)
	if err != nil {
		return fmt.Errorf("parse VK: %w", err)
	}

	// Free external data if present
	if !vk.IsSmallData() {
		dataRef := vk.DataOffsetRel()
		if dataRef != 0 {
			dataLen := vk.DataLen()

			// Determine if it's regular external data or big data (DB format)
			if dataLen <= MaxExternalValueBytes {
				// Regular external data cell - free it
				if freeErr := ve.alloc.Free(dataRef); freeErr != nil {
					return fmt.Errorf("free data cell: %w", freeErr)
				}
			} else {
				// Big data (DB format) - TODO: implement bigdata.Free()
				// For now, skip freeing big data structures
				// This is a known limitation that should be addressed in the future
			}
		}
	}

	// Free the VK cell itself
	if freeErr := ve.alloc.Free(vkRef); freeErr != nil {
		return fmt.Errorf("free VK cell: %w", freeErr)
	}

	return nil
}

// allocateVK allocates and initializes a new VK cell.
func (ve *valueEditor) allocateVK(name string, typ ValueType, data []byte) (VKRef, error) {
	dataLen := len(data)

	// Validate data size
	if dataLen > 1024*1024*1024 { // 1GB sanity limit
		return 0, ErrDataTooLarge
	}

	// Encode name (ASCII for simplicity)
	nameBytes := []byte(name)
	nameLen := len(nameBytes)

	// CRITICAL: Allocate data storage FIRST, before allocating VK cell.
	// If we allocate VK first, then bigdata.Store() or allocateDataCell() may grow the hive,
	// invalidating the vkBuf slice pointer!
	var dataOffsetRel uint32
	var rawDataLen uint32

	switch {
	case dataLen <= MaxInlineValueBytes:
		// Inline storage: data goes in the DataOff field, high bit set in DataLen
		rawDataLen = uint32(dataLen) | format.VKSmallDataMask
		dataOffsetRel = 0 // Not used for inline, but set to 0
	case dataLen <= MaxExternalValueBytes:
		// External storage: single data cell
		dataRef, err := ve.allocateDataCell(data)
		if err != nil {
			return 0, fmt.Errorf("allocate data cell: %w", err)
		}
		rawDataLen = uint32(dataLen)
		dataOffsetRel = dataRef
	default:
		// Big-data storage: use DB format
		dbRef, err := ve.bigdata.Store(data)
		if err != nil {
			return 0, fmt.Errorf("store bigdata: %w", err)
		}
		rawDataLen = uint32(dataLen)
		dataOffsetRel = dbRef
	}

	// NOW allocate VK cell (after data storage is complete)
	// Calculate VK payload size: fixed header + name
	vkPayloadSize := format.VKFixedHeaderSize + nameLen
	vkTotalSize := int32(vkPayloadSize + format.CellHeaderSize)

	vkRef, vkBuf, err := ve.alloc.Alloc(vkTotalSize, alloc.ClassVK)
	if err != nil {
		return 0, fmt.Errorf("alloc VK cell: %w", err)
	}

	// Write VK signature
	vkBuf[0] = 'v'
	vkBuf[1] = 'k'

	// Name length
	format.PutU16(vkBuf, format.VKNameLenOffset, uint16(nameLen))

	// Flags: compressed name
	flags := uint16(format.VKFlagNameCompressed)
	format.PutU16(vkBuf, format.VKFlagsOffset, flags)

	// Spare: 0
	format.PutU16(vkBuf, format.VKSpareOffset, 0)

	// Type
	format.PutU32(vkBuf, format.VKTypeOffset, typ)

	// Write data length and offset (computed above)
	format.PutU32(vkBuf, format.VKDataLenOffset, rawDataLen)
	format.PutU32(vkBuf, format.VKDataOffOffset, dataOffsetRel)

	// Handle inline data copy (after VK is allocated)
	if dataLen <= MaxInlineValueBytes && dataLen > 0 {
		copy(vkBuf[format.VKDataOffOffset:format.VKDataOffOffset+4], data)
	}

	// Copy name
	copy(vkBuf[format.VKNameOffset:], nameBytes)

	// Mark VK cell as dirty so it's flushed to disk
	ve.markCellDirty(vkRef)

	return vkRef, nil
}

// allocateDataCell allocates a single data cell for external storage.
func (ve *valueEditor) allocateDataCell(data []byte) (uint32, error) {
	dataLen := len(data)
	totalSize := int32(dataLen + format.CellHeaderSize)

	ref, buf, err := ve.alloc.Alloc(totalSize, alloc.ClassRD)
	if err != nil {
		return 0, fmt.Errorf("alloc data cell: %w", err)
	}

	// Copy data
	copy(buf, data)

	// Mark data cell as dirty so it's flushed to disk
	ve.markCellDirty(ref)

	return ref, nil
}

// addToValueList adds a new VK to the NK's value list.
func (ve *valueEditor) addToValueList(nkRef NKRef, vkRef VKRef) error {
	nkPayload, err := ve.resolveCell(nkRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		return err
	}

	// Read existing value list (if any)
	var existingList *values.List
	valueCount := nk.ValueCount()

	if valueCount > 0 {
		listRef := nk.ValueListOffsetRel()
		if listRef != format.InvalidOffset {
			existingList, err = values.Read(ve.h, nk)
			if err != nil {
				// If read fails, start with empty list
				existingList = &values.List{VKRefs: []uint32{}}
			}
		} else {
			existingList = &values.List{VKRefs: []uint32{}}
		}
	} else {
		existingList = &values.List{VKRefs: []uint32{}}
	}

	// Append new VK
	newList := existingList.Append(vkRef)

	// Write updated value list
	newListRef, err := values.Write(ve.h, ve.alloc, newList)
	if err != nil {
		return fmt.Errorf("write value list: %w", err)
	}

	// CRITICAL: Re-resolve NK cell after values.Write() which may have grown the hive,
	// invalidating the nkPayload buffer we obtained earlier!
	nkPayload, err = ve.resolveCell(nkRef)
	if err != nil {
		return err
	}

	// Update NK's value count and list reference
	format.PutU32(nkPayload, format.NKValueCountOffset, uint32(newList.Len()))
	format.PutU32(nkPayload, format.NKValueListOffset, newListRef)

	// Mark NK cell as dirty so changes are flushed to disk
	ve.markCellDirty(nkRef)

	return nil
}

// replaceInValueList replaces an old VK ref with a new one in the NK's value list.
func (ve *valueEditor) replaceInValueList(nkRef NKRef, oldVKRef VKRef, newVKRef VKRef) error {
	nkPayload, err := ve.resolveCell(nkRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		return err
	}

	// Read value list
	existingList, err := values.Read(ve.h, nk)
	if err != nil {
		return fmt.Errorf("read value list: %w", err)
	}

	// Find and replace the old ref
	found := false
	for i, ref := range existingList.VKRefs {
		if ref == oldVKRef {
			existingList.VKRefs[i] = newVKRef
			found = true
			break
		}
	}

	if !found {
		return ErrValueNotFound
	}

	// Write updated value list
	newListRef, err := values.Write(ve.h, ve.alloc, existingList)
	if err != nil {
		return fmt.Errorf("write value list: %w", err)
	}

	// CRITICAL: Re-resolve NK cell after values.Write() which may have grown the hive
	nkPayload, err = ve.resolveCell(nkRef)
	if err != nil {
		return err
	}

	// Update NK's list reference (count stays the same)
	format.PutU32(nkPayload, format.NKValueListOffset, newListRef)

	// Mark NK cell as dirty so changes are flushed to disk
	ve.markCellDirty(nkRef)

	return nil
}

// DeleteValue removes a value by name; idempotent if missing.
func (ve *valueEditor) DeleteValue(nk NKRef, name string) error {
	if nk == 0 {
		return ErrInvalidRef
	}

	// Check if value exists (index handles case-insensitivity)
	vkRef, exists := ve.index.GetVK(nk, name)
	if !exists {
		// Value doesn't exist - idempotent, return success
		return nil
	}

	// Parse VK to extract name and data info before removing
	vkPayload, err := ve.resolveCell(vkRef)
	if err != nil {
		return fmt.Errorf("resolve VK for cleanup: %w", err)
	}

	vk, err := hive.ParseVK(vkPayload)
	if err != nil {
		return fmt.Errorf("parse VK for cleanup: %w", err)
	}

	// Extract value name for index removal
	vkName := decodeName(vk.Name(), vk.NameCompressed())

	// Remove from value list
	if removeErr := ve.removeFromValueList(nk, vkRef); removeErr != nil {
		return fmt.Errorf("remove from value list: %w", removeErr)
	}

	// Free data cells if external (not inline in VK)
	dataLen := vk.DataLen()
	if dataLen > format.DWORDSize {
		// Data is stored externally (not inline in VK)
		dataRef := vk.DataOffsetRel()
		if dataRef != 0 && dataRef != format.InvalidOffset {
			// Check if it's big-data (DB format)
			if freeErr := ve.freeBigDataIfNeeded(dataRef); freeErr != nil {
				// If it's not DB format, just free as single cell
				ve.markCellDirty(dataRef)
				_ = ve.alloc.Free(dataRef)
			}
		}
	}

	// Remove from index to maintain consistency
	ve.index.RemoveVK(nk, vkName)

	// Mark VK cell as dirty before freeing (size field changes to positive)
	ve.markCellDirty(vkRef)

	// Free the VK cell itself
	if freeErr := ve.alloc.Free(vkRef); freeErr != nil {
		return fmt.Errorf("free VK cell: %w", freeErr)
	}

	return nil
}

// removeFromValueList removes a VK from the NK's value list.
func (ve *valueEditor) removeFromValueList(nkRef NKRef, vkRef VKRef) error {
	nkPayload, err := ve.resolveCell(nkRef)
	if err != nil {
		return err
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		return err
	}

	// Read value list
	existingList, err := values.Read(ve.h, nk)
	if err != nil {
		return fmt.Errorf("read value list: %w", err)
	}

	// Remove the VK
	newList := existingList.Remove(vkRef)

	// Write updated value list (or clear if empty)
	var newListRef uint32 = 0xFFFFFFFF
	if newList.Len() > 0 {
		newListRef, err = values.Write(ve.h, ve.alloc, newList)
		if err != nil {
			return fmt.Errorf("write value list: %w", err)
		}
	}

	// CRITICAL: Re-resolve NK cell after values.Write() which may have grown the hive
	nkPayload, err = ve.resolveCell(nkRef)
	if err != nil {
		return err
	}

	// Update NK's value count and list reference
	format.PutU32(nkPayload, format.NKValueCountOffset, uint32(newList.Len()))
	format.PutU32(nkPayload, format.NKValueListOffset, newListRef)

	// Mark NK cell as dirty so changes are flushed to disk
	ve.markCellDirty(nkRef)

	return nil
}

// resolveCell resolves an HCELL_INDEX to its payload.
func (ve *valueEditor) resolveCell(ref uint32) ([]byte, error) {
	if ref == 0 || ref == format.InvalidOffset {
		return nil, ErrInvalidRef
	}

	data := ve.h.Bytes()
	offset := format.HeaderSize + int(ref)

	if offset+format.CellHeaderSize > len(data) {
		return nil, fmt.Errorf("ref 0x%X beyond hive bounds", ref)
	}

	// Read cell size
	cellSize := int32(format.ReadU32(data, offset))
	if cellSize >= 0 {
		return nil, fmt.Errorf("ref 0x%X points to free cell", ref)
	}

	size := int(-cellSize)
	payloadSize := size - format.CellHeaderSize

	payloadOffset := offset + format.CellHeaderSize
	if payloadOffset+payloadSize > len(data) {
		return nil, fmt.Errorf("ref 0x%X payload truncated", ref)
	}

	return data[payloadOffset : payloadOffset+payloadSize], nil
}

// freeBigDataIfNeeded frees a DB structure if the ref points to one.
func (ve *valueEditor) freeBigDataIfNeeded(ref uint32) error {
	return freeBigDataIfNeeded(ve, ve.alloc, ref)
}

// markCellDirty marks a cell as dirty in the dirty tracker.
func (ve *valueEditor) markCellDirty(ref uint32) {
	data := ve.h.Bytes()
	offset := format.HeaderSize + int(ref)

	// Read cell size (including header)
	cellSize := int32(format.ReadU32(data, offset))
	if cellSize < 0 {
		cellSize = -cellSize
	}

	// Mark the entire cell (including header) as dirty
	ve.dt.Add(offset, int(cellSize))
}
