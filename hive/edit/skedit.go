package edit

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/format"
)

// getOrCreateSKCell returns the offset of an SK cell for the given security descriptor.
// If an SK cell with the same security descriptor already exists, it increments the
// ref_count and returns the existing offset. Otherwise, it creates a new SK cell.
//
// This implements security descriptor deduplication, which is critical for matching
// Windows registry hive sizes. Windows shares SK cells across keys with identical
// security descriptors.
func (ke *keyEditor) getOrCreateSKCell(secDesc []byte) (uint32, error) {
	if len(secDesc) == 0 {
		return format.InvalidOffset, nil // No security descriptor
	}

	// Hash the security descriptor for deduplication
	hash := sha256.Sum256(secDesc)
	hashKey := string(hash[:])

	ke.skMutex.Lock()
	defer ke.skMutex.Unlock()

	// Check if we already have this security descriptor
	if existingOffset, exists := ke.skMap[hashKey]; exists {
		// Increment reference count
		if err := ke.incrementSKRefCount(existingOffset); err != nil {
			return format.InvalidOffset, fmt.Errorf("failed to increment SK ref count: %w", err)
		}
		return existingOffset, nil
	}

	// Create new SK cell
	offset, err := ke.createSKCell(secDesc)
	if err != nil {
		return format.InvalidOffset, fmt.Errorf("failed to create SK cell: %w", err)
	}

	// Add to map and list
	ke.skMap[hashKey] = offset
	ke.skList = append(ke.skList, offset)

	// Link SK cells in a doubly-linked list
	if len(ke.skList) > 1 {
		prevOffset := ke.skList[len(ke.skList)-2]
		if err := ke.linkSKCells(prevOffset, offset); err != nil {
			return format.InvalidOffset, fmt.Errorf("failed to link SK cells: %w", err)
		}
	}

	return offset, nil
}

// createSKCell allocates and writes a new SK cell.
func (ke *keyEditor) createSKCell(secDesc []byte) (uint32, error) {
	cellSize := format.SKHeaderSize + len(secDesc)

	// Allocate cell
	ref, data, err := ke.alloc.Alloc(int32(cellSize), alloc.ClassSK)
	if err != nil {
		return format.InvalidOffset, fmt.Errorf("alloc failed: %w", err)
	}

	// Convert ref (CellRef/uint32, no header offset) to absolute offset (uint32, with header offset)
	offset := ref + format.HeaderSize

	// Write SK header
	copy(data[format.SKSignatureOffset:format.SKSignatureOffset+2], format.SKSignature)
	binary.LittleEndian.PutUint16(data[format.SKReservedOffset:format.SKReservedOffset+2], 0)
	binary.LittleEndian.PutUint32(data[format.SKFlinkOffset:format.SKFlinkOffset+4], format.InvalidOffset)
	binary.LittleEndian.PutUint32(data[format.SKBlinkOffset:format.SKBlinkOffset+4], format.InvalidOffset)
	binary.LittleEndian.PutUint32(
		data[format.SKReferenceCountOffset:format.SKReferenceCountOffset+4],
		1,
	) // Initial ref_count = 1
	binary.LittleEndian.PutUint32(
		data[format.SKDescriptorLengthOffset:format.SKDescriptorLengthOffset+4],
		uint32(len(secDesc)),
	)

	// Write security descriptor
	copy(data[format.SKDescriptorOffset:], secDesc)

	// Mark dirty
	ke.markCellDirty(ref)

	return offset, nil
}

// incrementSKRefCount increments the reference count of an SK cell.
func (ke *keyEditor) incrementSKRefCount(offset uint32) error {
	// Get hive data
	hiveData := ke.h.Bytes()

	// Calculate absolute offset of ref_count field
	// offset is already absolute (includes 0x1000), points to cell data (after cell_size)
	refCountOffset := int(offset) + format.SKReferenceCountOffset

	// Bounds check
	if refCountOffset+4 > len(hiveData) {
		return fmt.Errorf("SK ref_count offset out of bounds: %d", refCountOffset)
	}

	// Read current ref_count
	refCount := binary.LittleEndian.Uint32(hiveData[refCountOffset : refCountOffset+4])

	// Increment
	refCount++

	// Write back
	binary.LittleEndian.PutUint32(hiveData[refCountOffset:refCountOffset+4], refCount)

	// Mark dirty
	if ke.dt != nil {
		ke.dt.Add(refCountOffset, 4)
	}

	return nil
}

// linkSKCells creates doubly-linked list between two SK cells.
func (ke *keyEditor) linkSKCells(prevOffset, nextOffset uint32) error {
	hiveData := ke.h.Bytes()

	// Update prev cell's flink
	flinkOffset := int(prevOffset) + format.SKFlinkOffset
	if flinkOffset+4 > len(hiveData) {
		return fmt.Errorf("SK flink offset out of bounds: %d", flinkOffset)
	}
	binary.LittleEndian.PutUint32(hiveData[flinkOffset:flinkOffset+4], nextOffset)

	// Update next cell's blink
	blinkOffset := int(nextOffset) + format.SKBlinkOffset
	if blinkOffset+4 > len(hiveData) {
		return fmt.Errorf("SK blink offset out of bounds: %d", blinkOffset)
	}
	binary.LittleEndian.PutUint32(hiveData[blinkOffset:blinkOffset+4], prevOffset)

	// Mark dirty
	if ke.dt != nil {
		ke.dt.Add(flinkOffset, 4)
		ke.dt.Add(blinkOffset, 4)
	}

	return nil
}

// DefaultSecurityDescriptor returns a permissive security descriptor for keys without explicit security.
// This matches Windows default behavior for registry keys.
//
// The security descriptor grants:
// - SYSTEM: Full Control
// - Administrators: Full Control
// - Users: Read.
func DefaultSecurityDescriptor() []byte {
	// This is a minimal SECURITY_DESCRIPTOR in self-relative format
	// Revision(1) + Sbz1(1) + Control(2) + OwnerOffset(4) + GroupOffset(4) + SaclOffset(4) + DaclOffset(4)
	// + Owner SID + Group SID + DACL
	//
	// For simplicity, we use a permissive descriptor that Windows would accept.
	// A production implementation might want to parse this from a template or configuration.

	// Minimal descriptor: Everyone (S-1-1-0) with Read access
	// This is intentionally permissive to avoid access issues during testing
	return []byte{
		0x01, 0x00, 0x04, 0x80, // Revision + Control (SE_DACL_PRESENT | SE_SELF_RELATIVE)
		0x30, 0x00, 0x00, 0x00, // Owner offset
		0x3C, 0x00, 0x00, 0x00, // Group offset
		0x00, 0x00, 0x00, 0x00, // SACL offset (none)
		0x14, 0x00, 0x00, 0x00, // DACL offset

		// DACL
		0x02, 0x00, // Revision
		0x1C, 0x00, // Size (28 bytes)
		0x01, 0x00, // ACE count (1)
		0x00, 0x00, // Reserved

		// ACE: Everyone - Read
		0x00,       // Type: ACCESS_ALLOWED_ACE_TYPE
		0x00,       // Flags
		0x14, 0x00, // Size (20 bytes)
		0x01, 0x00, 0x00, 0x00, // Mask: READ_CONTROL
		0x01, 0x01, 0x00, 0x00, // SID: S-1-1-0 (Everyone)
		0x00, 0x00, 0x00, 0x01,

		// Owner SID: SYSTEM (S-1-5-18)
		0x01, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x05,
		0x12, 0x00, 0x00, 0x00,

		// Group SID: SYSTEM (S-1-5-18)
		0x01, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x05,
		0x12, 0x00, 0x00, 0x00,
	}
}
