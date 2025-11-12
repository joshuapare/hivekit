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
