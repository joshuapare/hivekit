// Package verify provides validation functions for Windows Registry hive structures.
// These helpers are used in tests to ensure hive invariants are maintained.
package verify

import (
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// Error types for different validation failures.
type ValidationError struct {
	Type    string
	Message string
	Offset  int
	Details map[string]interface{}
}

func (e *ValidationError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("%s at offset 0x%X: %s", e.Type, e.Offset, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

// AllInvariants validates all hive invariants in one call.
// Returns the first error encountered, or nil if all checks pass.
func AllInvariants(data []byte) error {
	if err := REGFHeader(data); err != nil {
		return err
	}
	if err := HBINStructure(data); err != nil {
		return err
	}
	if err := FileSize(data); err != nil {
		return err
	}
	if err := NKRefIntegrity(data); err != nil {
		return err
	}
	return nil
}

// REGFHeader validates the REGF header structure and invariants.
func REGFHeader(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "REGFHeader",
			Message: fmt.Sprintf("file too small: %d bytes (need %d)", len(data), format.HeaderSize),
			Offset:  -1,
		}
	}

	// Check signature
	sig := string(data[format.REGFSignatureOffset : format.REGFSignatureOffset+4])
	if sig != string(format.REGFSignature) {
		return &ValidationError{
			Type:    "REGFHeader",
			Message: fmt.Sprintf("invalid signature: got %q, expected %q", sig, format.REGFSignature),
			Offset:  format.REGFSignatureOffset,
		}
	}

	// Check version (major should be 1, minor typically 3, 4, or 5)
	major := format.ReadU32(data, format.REGFMajorVersionOffset)
	minor := format.ReadU32(data, format.REGFMinorVersionOffset)
	if major != 1 {
		return &ValidationError{
			Type:    "REGFHeader",
			Message: fmt.Sprintf("unexpected major version: %d (expected 1)", major),
			Offset:  format.REGFMajorVersionOffset,
		}
	}
	if minor < 3 || minor > 6 {
		return &ValidationError{
			Type:    "REGFHeader",
			Message: fmt.Sprintf("unusual minor version: %d (typically 3-6)", minor),
			Offset:  format.REGFMinorVersionOffset,
		}
	}

	// Check data size is 4KB-aligned
	dataSize := format.ReadU32(data, format.REGFDataSizeOffset)
	if dataSize%format.HBINAlignment != 0 {
		return &ValidationError{
			Type:    "REGFHeader",
			Message: fmt.Sprintf("data size not 4KB-aligned: 0x%X", dataSize),
			Offset:  format.REGFDataSizeOffset,
		}
	}

	return nil
}

// HBINStructure validates all HBIN blocks are valid and contiguous.
func HBINStructure(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "HBINStructure",
			Message: "file too small for HBIN data",
			Offset:  -1,
		}
	}

	pos := format.HeaderSize
	hbinCount := 0

	for pos < len(data) {
		// Check if there's room for an HBIN header
		if pos+format.HBINHeaderSize > len(data) {
			break
		}

		// Check HBIN signature
		sig := string(data[pos : pos+4])
		if sig != string(format.HBINSignature) {
			// No more HBINs - this is okay if we're at the end
			if pos == len(data) {
				break
			}
			return &ValidationError{
				Type:    "HBINStructure",
				Message: fmt.Sprintf("invalid HBIN signature: got %q, expected %q", sig, format.HBINSignature),
				Offset:  pos,
			}
		}

		// Check offset field matches position
		offsetField := int(format.ReadU32(data, pos+format.HBINFileOffsetField))
		expectedOffset := pos - format.HeaderSize
		if offsetField != expectedOffset {
			return &ValidationError{
				Type:    "HBINStructure",
				Message: fmt.Sprintf("HBIN offset mismatch: field=0x%X, expected=0x%X", offsetField, expectedOffset),
				Offset:  pos,
			}
		}

		// Check size is valid and aligned
		hbinSize := int(format.ReadU32(data, pos+format.HBINSizeOffset))
		if hbinSize <= 0 || hbinSize%format.HBINAlignment != 0 {
			return &ValidationError{
				Type:    "HBINStructure",
				Message: fmt.Sprintf("invalid HBIN size: 0x%X (must be positive and 4KB-aligned)", hbinSize),
				Offset:  pos,
			}
		}

		// Check HBIN doesn't exceed file size
		if pos+hbinSize > len(data) {
			return &ValidationError{
				Type:    "HBINStructure",
				Message: fmt.Sprintf("HBIN extends beyond file: size=0x%X, available=0x%X", hbinSize, len(data)-pos),
				Offset:  pos,
			}
		}

		// Validate cells within this HBIN
		if err := validateHBINCells(data, pos, hbinSize); err != nil {
			return err
		}

		pos += hbinSize
		hbinCount++
	}

	if hbinCount == 0 {
		return &ValidationError{
			Type:    "HBINStructure",
			Message: "no valid HBINs found",
			Offset:  -1,
		}
	}

	return nil
}

// validateHBINCells validates cells within a single HBIN don't cross boundaries.
func validateHBINCells(data []byte, hbinStart, hbinSize int) error {
	hbinEnd := hbinStart + hbinSize
	cellPos := hbinStart + format.HBINHeaderSize

	for cellPos < hbinEnd {
		// Check if there's room for a cell header
		if cellPos+format.CellHeaderSize > hbinEnd {
			break
		}

		// Read cell size (can be positive=free or negative=allocated)
		rawSize := format.ReadI32(data, cellPos)
		absSize := rawSize
		if absSize < 0 {
			absSize = -absSize
		}

		// Size of 0 or very small size indicates end of cells
		if absSize <= format.CellHeaderSize {
			break
		}

		// Check cell doesn't cross HBIN boundary
		cellEnd := cellPos + int(absSize)
		if cellEnd > hbinEnd {
			return &ValidationError{
				Type:    "HBINStructure",
				Message: fmt.Sprintf("cell crosses HBIN boundary: cell_end=0x%X, hbin_end=0x%X", cellEnd, hbinEnd),
				Offset:  cellPos,
			}
		}

		// Check cell size is 8-byte aligned
		if absSize%8 != 0 {
			return &ValidationError{
				Type:    "HBINStructure",
				Message: fmt.Sprintf("cell size not 8-byte aligned: %d bytes", absSize),
				Offset:  cellPos,
			}
		}

		cellPos += int(absSize)
	}

	return nil
}

// FileSize validates that the file size matches the header's data size field.
func FileSize(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "FileSize",
			Message: fmt.Sprintf("file too small: %d bytes", len(data)),
			Offset:  -1,
		}
	}

	dataSize := int(format.ReadU32(data, format.REGFDataSizeOffset))
	expectedFileSize := format.HeaderSize + dataSize
	actualFileSize := len(data)

	if actualFileSize != expectedFileSize {
		return &ValidationError{
			Type: "FileSize",
			Message: fmt.Sprintf(
				"file size mismatch: actual=0x%X, expected=0x%X (header+data)",
				actualFileSize,
				expectedFileSize,
			),
			Offset: -1,
			Details: map[string]interface{}{
				"actual":      actualFileSize,
				"expected":    expectedFileSize,
				"header_size": format.HeaderSize,
				"data_size":   dataSize,
			},
		}
	}

	return nil
}

// SequenceNumbers checks that sequence numbers are consistent (Seq1 == Seq2 for clean hive).
func SequenceNumbers(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "SequenceNumbers",
			Message: "file too small for header",
			Offset:  -1,
		}
	}

	seq1 := format.ReadU32(data, format.REGFPrimarySeqOffset)
	seq2 := format.ReadU32(data, format.REGFSecondarySeqOffset)

	if seq1 != seq2 {
		return &ValidationError{
			Type:    "SequenceNumbers",
			Message: fmt.Sprintf("sequences mismatch (dirty hive): Seq1=0x%X, Seq2=0x%X", seq1, seq2),
			Offset:  format.REGFPrimarySeqOffset,
			Details: map[string]interface{}{
				"primary":   seq1,
				"secondary": seq2,
			},
		}
	}

	return nil
}

// Checksum validates the REGF header checksum.
// The checksum is the XOR of all 508 dwords before the checksum field.
func Checksum(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "Checksum",
			Message: "file too small for header",
			Offset:  -1,
		}
	}

	// Calculate checksum (XOR of first 508 dwords, excluding checksum field at 0x1FC)
	var calculated uint32
	for i := 0; i < format.REGFCheckSumOffset; i += 4 {
		calculated ^= format.ReadU32(data, i)
	}

	stored := format.ReadU32(data, format.REGFCheckSumOffset)

	if calculated != stored {
		return &ValidationError{
			Type:    "Checksum",
			Message: fmt.Sprintf("checksum mismatch: calculated=0x%08X, stored=0x%08X", calculated, stored),
			Offset:  format.REGFCheckSumOffset,
			Details: map[string]interface{}{
				"calculated": calculated,
				"stored":     stored,
			},
		}
	}

	return nil
}

// NKRefIntegrity validates that all NKRefs in subkey lists point to valid allocated NK cells.
// This is a deep validation that walks the entire hive tree starting from the root.
//
// It detects:
//   - NKRefs pointing to freed cells (positive cell size)
//   - NKRefs pointing to non-NK cells (wrong signature)
//   - NKRefs pointing to invalid offsets (out of bounds)
//
// Returns nil if all NKRefs are valid.
func NKRefIntegrity(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "NKRefIntegrity",
			Message: "file too small for header",
			Offset:  -1,
		}
	}

	// Get root NK offset from REGF header
	rootOffset := format.ReadU32(data, format.REGFRootCellOffset)
	if rootOffset == format.InvalidOffset {
		return &ValidationError{
			Type:    "NKRefIntegrity",
			Message: "invalid root offset in header",
			Offset:  format.REGFRootCellOffset,
		}
	}

	// Use stack-based traversal to avoid recursion
	type stackEntry struct {
		nkOffset uint32
		parent   uint32
	}
	stack := []stackEntry{{nkOffset: rootOffset, parent: 0}}

	// Track visited NKs to detect cycles
	visited := make(map[uint32]bool)

	for len(stack) > 0 {
		// Pop from stack
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		nkOffset := entry.nkOffset

		// Skip if already visited (cycle detection)
		if visited[nkOffset] {
			continue
		}
		visited[nkOffset] = true

		// Validate the NK cell
		payload, err := resolveCellPayload(data, nkOffset)
		if err != nil {
			return &ValidationError{
				Type:    "NKRefIntegrity",
				Message: fmt.Sprintf("NKRef 0x%X from parent 0x%X: %v", nkOffset, entry.parent, err),
				Offset:  int(nkOffset),
				Details: map[string]interface{}{
					"parent_nk": entry.parent,
					"child_nk":  nkOffset,
				},
			}
		}

		// Check NK signature
		if len(payload) < 2 || payload[0] != 'n' || payload[1] != 'k' {
			return &ValidationError{
				Type:    "NKRefIntegrity",
				Message: fmt.Sprintf("NKRef 0x%X from parent 0x%X: not an NK cell (sig=%q)", nkOffset, entry.parent, string(payload[:min(2, len(payload))])),
				Offset:  int(nkOffset),
				Details: map[string]interface{}{
					"parent_nk":    entry.parent,
					"child_nk":     nkOffset,
					"actual_sig":   string(payload[:min(2, len(payload))]),
					"expected_sig": "nk",
				},
			}
		}

		// Check minimum NK size (signature + flags + timestamps, etc.)
		if len(payload) < format.NKMinSize {
			return &ValidationError{
				Type:    "NKRefIntegrity",
				Message: fmt.Sprintf("NKRef 0x%X from parent 0x%X: NK cell too small (%d bytes)", nkOffset, entry.parent, len(payload)),
				Offset:  int(nkOffset),
				Details: map[string]interface{}{
					"parent_nk":    entry.parent,
					"child_nk":     nkOffset,
					"payload_size": len(payload),
					"min_size":     format.NKMinSize,
				},
			}
		}

		// Get subkey list offset
		subkeyListOffset := format.ReadU32(payload, format.NKSubkeyListOffset)
		if subkeyListOffset == format.InvalidOffset {
			continue // No subkeys
		}

		// Read subkey offsets from the list
		childOffsets, err := readSubkeyListOffsets(data, subkeyListOffset)
		if err != nil {
			return &ValidationError{
				Type:    "NKRefIntegrity",
				Message: fmt.Sprintf("NK 0x%X: invalid subkey list at 0x%X: %v", nkOffset, subkeyListOffset, err),
				Offset:  int(subkeyListOffset),
				Details: map[string]interface{}{
					"nk_offset":   nkOffset,
					"list_offset": subkeyListOffset,
				},
			}
		}

		// Push children to stack
		for _, childOffset := range childOffsets {
			stack = append(stack, stackEntry{nkOffset: childOffset, parent: nkOffset})
		}
	}

	return nil
}

// resolveCellPayload resolves an offset to a cell payload, validating the cell is allocated.
func resolveCellPayload(data []byte, offset uint32) ([]byte, error) {
	absOffset := format.HeaderSize + int(offset)

	// Bounds check
	if absOffset < 0 || absOffset+4 > len(data) {
		return nil, fmt.Errorf("offset out of bounds")
	}

	// Read cell size (negative = allocated, positive = free)
	rawSize := int32(format.ReadU32(data, absOffset))
	if rawSize >= 0 {
		return nil, fmt.Errorf("points to free cell (size=%d)", rawSize)
	}

	size := int(-rawSize)
	if size < format.CellHeaderSize {
		return nil, fmt.Errorf("cell size too small (%d)", size)
	}

	endOffset := absOffset + size
	if endOffset > len(data) {
		return nil, fmt.Errorf("cell extends beyond file")
	}

	// Return payload (skip 4-byte header)
	return data[absOffset+4 : endOffset], nil
}

// readSubkeyListOffsets reads all NKRef offsets from a subkey list (LF/LH/LI/RI).
func readSubkeyListOffsets(data []byte, listOffset uint32) ([]uint32, error) {
	payload, err := resolveCellPayload(data, listOffset)
	if err != nil {
		return nil, fmt.Errorf("resolve list cell: %w", err)
	}

	if len(payload) < 4 {
		return nil, fmt.Errorf("list too short: %d bytes", len(payload))
	}

	sig := string(payload[0:2])
	count := int(format.ReadU16(payload, 2))

	switch sig {
	case "lf", "lh":
		// LF/LH: 8 bytes per entry (offset + hash)
		entrySize := 8
		minSize := 4 + count*entrySize
		if len(payload) < minSize {
			return nil, fmt.Errorf("LF/LH truncated: need %d, have %d", minSize, len(payload))
		}
		offsets := make([]uint32, count)
		for i := 0; i < count; i++ {
			offsets[i] = format.ReadU32(payload, 4+i*entrySize)
		}
		return offsets, nil

	case "li":
		// LI: 4 bytes per entry (offset only)
		entrySize := 4
		minSize := 4 + count*entrySize
		if len(payload) < minSize {
			return nil, fmt.Errorf("LI truncated: need %d, have %d", minSize, len(payload))
		}
		offsets := make([]uint32, count)
		for i := 0; i < count; i++ {
			offsets[i] = format.ReadU32(payload, 4+i*entrySize)
		}
		return offsets, nil

	case "ri":
		// RI: indirect list, contains offsets to other lists
		entrySize := 4
		minSize := 4 + count*entrySize
		if len(payload) < minSize {
			return nil, fmt.Errorf("RI truncated: need %d, have %d", minSize, len(payload))
		}
		var allOffsets []uint32
		for i := 0; i < count; i++ {
			subListOffset := format.ReadU32(payload, 4+i*entrySize)
			subOffsets, err := readSubkeyListOffsets(data, subListOffset)
			if err != nil {
				return nil, fmt.Errorf("RI sub-list %d at 0x%X: %w", i, subListOffset, err)
			}
			allOffsets = append(allOffsets, subOffsets...)
		}
		return allOffsets, nil

	default:
		return nil, fmt.Errorf("unknown list signature: %q", sig)
	}
}

// VKRefIntegrity validates that all VK references in value lists point to valid allocated VK cells.
// This walks the entire hive tree and checks each NK's value list.
//
// It detects:
//   - VKRefs pointing to freed cells (positive cell size)
//   - VKRefs pointing to non-VK cells (wrong signature)
//   - VKRefs pointing to invalid offsets (out of bounds)
//   - Value list cells that are freed
//
// Returns nil if all VKRefs are valid.
func VKRefIntegrity(data []byte) error {
	if len(data) < format.HeaderSize {
		return &ValidationError{
			Type:    "VKRefIntegrity",
			Message: "file too small for header",
			Offset:  -1,
		}
	}

	// Get root NK offset from REGF header
	rootOffset := format.ReadU32(data, format.REGFRootCellOffset)
	if rootOffset == format.InvalidOffset {
		return &ValidationError{
			Type:    "VKRefIntegrity",
			Message: "invalid root offset in header",
			Offset:  format.REGFRootCellOffset,
		}
	}

	// Use stack-based traversal to walk all NKs
	stack := []uint32{rootOffset}
	visited := make(map[uint32]bool)

	for len(stack) > 0 {
		// Pop from stack
		nkOffset := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Skip if already visited
		if visited[nkOffset] {
			continue
		}
		visited[nkOffset] = true

		// Get NK payload
		payload, err := resolveCellPayload(data, nkOffset)
		if err != nil {
			continue // Skip invalid NKs (NKRefIntegrity handles these)
		}

		// Check NK signature
		if len(payload) < format.NKMinSize || payload[0] != 'n' || payload[1] != 'k' {
			continue // Skip non-NK cells
		}

		// Get value count and value list offset
		valueCount := format.ReadU32(payload, format.NKValueCountOffset)
		valueListOffset := format.ReadU32(payload, format.NKValueListOffset)

		// Validate value list if present
		if valueCount > 0 && valueListOffset != format.InvalidOffset {
			if err := validateValueList(data, nkOffset, valueListOffset, valueCount); err != nil {
				return err
			}
		}

		// Get subkey list offset and push children to stack
		subkeyListOffset := format.ReadU32(payload, format.NKSubkeyListOffset)
		if subkeyListOffset != format.InvalidOffset {
			childOffsets, err := readSubkeyListOffsets(data, subkeyListOffset)
			if err == nil {
				stack = append(stack, childOffsets...)
			}
		}
	}

	return nil
}

// validateValueList validates all VK references in a value list.
func validateValueList(data []byte, nkOffset, listOffset, valueCount uint32) error {
	// Resolve value list cell
	listPayload, err := resolveCellPayload(data, listOffset)
	if err != nil {
		return &ValidationError{
			Type:    "VKRefIntegrity",
			Message: fmt.Sprintf("NK 0x%X: value list at 0x%X: %v", nkOffset, listOffset, err),
			Offset:  int(listOffset),
			Details: map[string]interface{}{
				"nk_offset":   nkOffset,
				"list_offset": listOffset,
			},
		}
	}

	// Check list has enough space for all VK offsets
	needed := int(valueCount) * 4
	if len(listPayload) < needed {
		return &ValidationError{
			Type:    "VKRefIntegrity",
			Message: fmt.Sprintf("NK 0x%X: value list too small: need %d bytes for %d values, have %d", nkOffset, needed, valueCount, len(listPayload)),
			Offset:  int(listOffset),
		}
	}

	// Validate each VK reference
	for i := uint32(0); i < valueCount; i++ {
		vkOffset := format.ReadU32(listPayload, int(i)*4)
		if vkOffset == 0 || vkOffset == format.InvalidOffset {
			continue // Skip empty slots
		}

		// Resolve VK cell
		vkPayload, err := resolveCellPayload(data, vkOffset)
		if err != nil {
			return &ValidationError{
				Type:    "VKRefIntegrity",
				Message: fmt.Sprintf("NK 0x%X value[%d]: VK at 0x%X: %v", nkOffset, i, vkOffset, err),
				Offset:  int(vkOffset),
				Details: map[string]interface{}{
					"nk_offset":    nkOffset,
					"value_index":  i,
					"vk_offset":    vkOffset,
					"list_offset":  listOffset,
					"value_count":  valueCount,
				},
			}
		}

		// Check VK signature
		if len(vkPayload) < 2 || vkPayload[0] != 'v' || vkPayload[1] != 'k' {
			return &ValidationError{
				Type:    "VKRefIntegrity",
				Message: fmt.Sprintf("NK 0x%X value[%d]: VK at 0x%X has wrong signature %q", nkOffset, i, vkOffset, string(vkPayload[:min(2, len(vkPayload))])),
				Offset:  int(vkOffset),
				Details: map[string]interface{}{
					"nk_offset":   nkOffset,
					"value_index": i,
					"vk_offset":   vkOffset,
					"actual_sig":  string(vkPayload[:min(2, len(vkPayload))]),
				},
			}
		}
	}

	return nil
}
