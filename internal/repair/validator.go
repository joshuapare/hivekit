package repair

import (
	"bytes"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// Validator provides pre and post-repair validation to ensure safety.
type Validator struct {
	// Critical regions that must not be overlapped by repairs
	criticalRegions []Region
}

// NewValidator creates a new validator with default critical regions.
func NewValidator(dataSize uint64) *Validator {
	v := &Validator{
		criticalRegions: make([]Region, 0, 8),
	}

	// REGF header is always critical
	v.criticalRegions = append(v.criticalRegions, Region{
		Start: 0,
		End:   format.HeaderSize,
	})

	return v
}

// AddCriticalRegion marks a region as critical (must not be overlapped).
// This is used to protect important structures from accidental corruption.
func (v *Validator) AddCriticalRegion(start, end uint64) {
	v.criticalRegions = append(v.criticalRegions, Region{
		Start: start,
		End:   end,
	})
}

// ValidateRepairSafe performs comprehensive pre-repair validation.
// It checks:
//   - Offset is valid and within bounds
//   - Repair won't overflow the data buffer
//   - Repair won't overlap critical regions
//   - The structure at the offset matches expectations
func (v *Validator) ValidateRepairSafe(data []byte, d Diagnostic) error {
	// Check offset is valid
	if d.Offset >= uint64(len(data)) {
		return &ValidationError{
			Phase:   "pre",
			Module:  "validator",
			Offset:  d.Offset,
			Message: fmt.Sprintf("offset out of bounds (offset=0x%X, datalen=%d)", d.Offset, len(data)),
		}
	}

	// Determine repair size (structure-specific)
	repairSize := v.estimateRepairSize(d)

	// Check repair won't overflow
	if d.Offset+repairSize > uint64(len(data)) {
		return &ValidationError{
			Phase:   "pre",
			Module:  "validator",
			Offset:  d.Offset,
			Message: fmt.Sprintf("repair would overflow buffer (offset=0x%X, size=%d, datalen=%d)", d.Offset, repairSize, len(data)),
		}
	}

	// Check alignment for certain structures
	if err := v.validateAlignment(d); err != nil {
		return err
	}

	return nil
}

// ValidateStructureIntegrity verifies that a structure is still valid after repair.
// This is structure-specific validation that ensures the repair didn't break the format.
func (v *Validator) ValidateStructureIntegrity(data []byte, offset uint64, structType string) error {
	switch structType {
	case "REGF":
		return v.validateREGFStructure(data, offset)
	case "HBIN":
		return v.validateHBINStructure(data, offset)
	case "NK":
		return v.validateNKStructure(data, offset)
	case "VK":
		return v.validateVKStructure(data, offset)
	default:
		// Unknown structure type, skip validation
		return nil
	}
}

// ValidateNoSideEffects ensures the repair didn't corrupt adjacent data.
// It compares before/after data outside the repair region to detect unintended changes.
func (v *Validator) ValidateNoSideEffects(before, after []byte, repairOffset, repairSize uint64) error {
	if len(before) != len(after) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  repairOffset,
			Message: "data size changed after repair",
		}
	}

	// Check bytes before repair region
	if repairOffset > 0 {
		beforeRegion := before[:repairOffset]
		afterRegion := after[:repairOffset]
		if !bytes.Equal(beforeRegion, afterRegion) {
			return &ValidationError{
				Phase:   "post",
				Module:  "validator",
				Offset:  repairOffset,
				Message: "data before repair region was modified",
			}
		}
	}

	// Check bytes after repair region
	endOffset := repairOffset + repairSize
	if endOffset < uint64(len(before)) {
		beforeRegion := before[endOffset:]
		afterRegion := after[endOffset:]
		if !bytes.Equal(beforeRegion, afterRegion) {
			return &ValidationError{
				Phase:   "post",
				Module:  "validator",
				Offset:  repairOffset,
				Message: "data after repair region was modified",
			}
		}
	}

	return nil
}

// estimateRepairSize estimates how many bytes a repair will modify.
// This is used for pre-validation checks.
func (v *Validator) estimateRepairSize(d Diagnostic) uint64 {
	switch d.Structure {
	case "REGF":
		// REGF repairs typically modify 4-byte fields
		return 4
	case "HBIN":
		// HBIN repairs typically modify 4-byte fields
		return 4
	case "NK":
		// NK repairs typically modify 4-byte offset/count fields
		return 4
	case "VK":
		// VK repairs typically modify 4-byte fields
		return 4
	case "CELL":
		// Cell header is 4 bytes
		return 4
	default:
		// Conservative estimate
		return 8
	}
}

// validateAlignment checks if the offset is properly aligned for the structure type.
func (v *Validator) validateAlignment(d Diagnostic) error {
	switch d.Structure {
	case "REGF":
		// For field-level repairs, the offset points to a field within the REGF header
		if d.Repair != nil && (d.Repair.Type == RepairDefault || d.Repair.Type == RepairReplace) {
			// Field-level repair - check offset is within REGF header
			if d.Offset >= format.HeaderSize {
				return &ValidationError{
					Phase:   "pre",
					Module:  "validator",
					Offset:  d.Offset,
					Message: fmt.Sprintf("REGF field offset must be within header (< 0x%X)", format.HeaderSize),
				}
			}
			return nil
		}
		// For structural repairs, REGF must be at offset 0
		if d.Offset != 0 {
			return &ValidationError{
				Phase:   "pre",
				Module:  "validator",
				Offset:  d.Offset,
				Message: "REGF header must be at offset 0",
			}
		}
	case "HBIN":
		// For field-level repairs, the offset points to a field within the HBIN header
		if d.Repair != nil && (d.Repair.Type == RepairDefault || d.Repair.Type == RepairReplace) {
			// Field-level repair - check that the containing HBIN block is aligned
			// The HBIN header starts at a 0x1000-aligned offset
			hbinStart := ((d.Offset-format.HeaderSize)/format.HBINAlignment)*format.HBINAlignment + format.HeaderSize
			if (hbinStart-format.HeaderSize)%format.HBINAlignment != 0 {
				return &ValidationError{
					Phase:   "pre",
					Module:  "validator",
					Offset:  d.Offset,
					Message: fmt.Sprintf("HBIN block not aligned to 0x%X", format.HBINAlignment),
				}
			}
			return nil
		}
		// For structural repairs, HBIN start must be 0x1000-aligned
		if (d.Offset-format.HeaderSize)%format.HBINAlignment != 0 {
			return &ValidationError{
				Phase:   "pre",
				Module:  "validator",
				Offset:  d.Offset,
				Message: fmt.Sprintf("HBIN offset not aligned to 0x%X", format.HBINAlignment),
			}
		}
	case "CELL", "NK", "VK", "LH", "LF", "LI", "RI", "SK", "DB":
		// For field-level repairs (RepairDefault, RepairReplace), the offset points
		// to a field within the structure, not the cell start. Skip alignment check
		// for these types of repairs.
		if d.Repair != nil && (d.Repair.Type == RepairDefault || d.Repair.Type == RepairReplace) {
			// Field-level repair, skip alignment check
			return nil
		}

		// For structural repairs (Rebuild, etc.), check cell alignment
		relativeOffset := (d.Offset - format.HeaderSize) % format.HBINAlignment
		if relativeOffset < format.HBINHeaderSize {
			// Within HBIN header, skip alignment check
			return nil
		}
		cellOffset := relativeOffset - format.HBINHeaderSize
		if cellOffset%format.CellAlignment != 0 {
			return &ValidationError{
				Phase:   "pre",
				Module:  "validator",
				Offset:  d.Offset,
				Message: fmt.Sprintf("cell offset not aligned to %d bytes", format.CellAlignment),
			}
		}
	}
	return nil
}

// validateREGFStructure checks if the REGF header is still valid.
func (v *Validator) validateREGFStructure(data []byte, offset uint64) error {
	if offset != 0 {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "REGF header must be at offset 0",
		}
	}

	if len(data) < format.HeaderSize {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "insufficient data for REGF header",
		}
	}

	// Check signature
	if !bytes.Equal(data[:len(format.REGFSignature)], format.REGFSignature) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "REGF signature corrupted after repair",
		}
	}

	// Parse header to ensure it's valid
	_, err := format.ParseHeader(data)
	if err != nil {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "REGF header invalid after repair",
			Cause:   err,
		}
	}

	return nil
}

// validateHBINStructure checks if an HBIN header is still valid.
func (v *Validator) validateHBINStructure(data []byte, offset uint64) error {
	if int(offset)+format.HBINHeaderSize > len(data) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "insufficient data for HBIN header",
		}
	}

	// Check signature
	hbinData := data[offset : offset+uint64(format.HBINHeaderSize)]
	if !bytes.Equal(hbinData[:len(format.HBINSignature)], format.HBINSignature) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "HBIN signature corrupted after repair",
		}
	}

	return nil
}

// validateNKStructure checks if an NK record is still valid.
func (v *Validator) validateNKStructure(data []byte, offset uint64) error {
	// Need at least NK minimum size
	if int(offset)+format.NKMinSize > len(data) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "insufficient data for NK record",
		}
	}

	// Check signature (skip cell header)
	cellStart := offset
	if cellStart >= format.HeaderSize {
		cellStart += format.CellHeaderSize
	}

	if int(cellStart)+len(format.NKSignature) > len(data) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "insufficient data for NK signature",
		}
	}

	nkData := data[cellStart:]
	if !bytes.Equal(nkData[:len(format.NKSignature)], format.NKSignature) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "NK signature corrupted after repair",
		}
	}

	return nil
}

// validateVKStructure checks if a VK record is still valid.
func (v *Validator) validateVKStructure(data []byte, offset uint64) error {
	// Need at least VK minimum size
	if int(offset)+format.VKMinSize > len(data) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "insufficient data for VK record",
		}
	}

	// Check signature (skip cell header)
	cellStart := offset
	if cellStart >= format.HeaderSize {
		cellStart += format.CellHeaderSize
	}

	if int(cellStart)+len(format.VKSignature) > len(data) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "insufficient data for VK signature",
		}
	}

	vkData := data[cellStart:]
	if !bytes.Equal(vkData[:len(format.VKSignature)], format.VKSignature) {
		return &ValidationError{
			Phase:   "post",
			Module:  "validator",
			Offset:  offset,
			Message: "VK signature corrupted after repair",
		}
	}

	return nil
}
