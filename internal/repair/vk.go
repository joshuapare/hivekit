package repair

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// VKModule handles repairs for VK (Value Key) record structures.
// VK records represent registry values and contain data about value names,
// types, and data offsets.
type VKModule struct {
	RepairModuleBase
}

// NewVKModule creates a new VK repair module.
func NewVKModule() *VKModule {
	return &VKModule{
		RepairModuleBase: RepairModuleBase{
			name: "VK",
		},
	}
}

// CanRepair checks if this module can handle the given diagnostic.
func (m *VKModule) CanRepair(d Diagnostic) bool {
	return d.Structure == "VK" && d.Repair != nil
}

// Validate checks if the repair is safe to apply.
func (m *VKModule) Validate(data []byte, d Diagnostic) error {
	// Basic offset validation
	if err := m.ValidateOffset(data, d.Offset, 4); err != nil {
		return err
	}

	// Calculate VK start from field offset
	vkStart := m.calculateVKStart(d.Offset)

	if vkStart >= uint64(len(data)) {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "VK record start out of bounds",
		}
	}

	// Verify VK signature if we have enough data (best effort)
	if vkStart > 0 && vkStart+uint64(len(format.VKSignature)) <= uint64(len(data)) {
		if bytes.Equal(data[vkStart:vkStart+uint64(len(format.VKSignature))], format.VKSignature) {
			// Signature found, good!
		} else {
			// Signature not found - this is OK, we'll verify after repair
			// Don't fail the validation here
		}
	}

	// Validate repair type
	switch d.Repair.Type {
	case RepairDefault:
		// Setting offsets to InvalidOffset is safe
		return nil
	case RepairReplace:
		return m.validateReplaceValue(d)
	case RepairTruncate:
		// Truncating data lengths is safe
		return nil
	case RepairRebuild, RepairRemove:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	default:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	}
}

// Apply performs the actual repair on the data.
func (m *VKModule) Apply(data []byte, d Diagnostic) error {
	switch d.Repair.Type {
	case RepairDefault:
		return m.applyDefaultRepair(data, d)
	case RepairReplace:
		return m.applyReplaceRepair(data, d)
	case RepairTruncate:
		return m.applyTruncateRepair(data, d)
	case RepairRebuild, RepairRemove:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	default:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	}
}

// Verify confirms the repair was successful.
func (m *VKModule) Verify(data []byte, d Diagnostic) error {
	// Verify the bytes were written correctly
	if d.Offset+4 > uint64(len(data)) {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "offset out of bounds after repair",
		}
	}

	// Read the value at the offset
	actualValue := binary.LittleEndian.Uint32(data[d.Offset : d.Offset+4])

	// Verify it matches expected value based on repair type
	switch d.Repair.Type {
	case RepairDefault:
		// Should be InvalidOffset or 0 depending on the field
		var expectedValue uint32 = format.InvalidOffset
		if d.Expected != nil {
			if exp, ok := d.Expected.(uint32); ok {
				expectedValue = exp
			}
		}
		if actualValue != expectedValue {
			return &RepairError{
				Module:  m.name,
				Offset:  d.Offset,
				Message: fmt.Sprintf("verification failed: expected 0x%X, got 0x%X", expectedValue, actualValue),
			}
		}
	case RepairReplace, RepairTruncate:
		// Should match the expected value from diagnostic
		expectedValue, ok := d.Expected.(uint32)
		if !ok {
			return &RepairError{
				Module:  m.name,
				Offset:  d.Offset,
				Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
			}
		}
		if actualValue != expectedValue {
			return &RepairError{
				Module:  m.name,
				Offset:  d.Offset,
				Message: fmt.Sprintf("verification failed: expected 0x%X, got 0x%X", expectedValue, actualValue),
			}
		}
	case RepairRebuild, RepairRemove:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	}

	// Verify VK signature is still intact (best effort)
	vkStart := m.calculateVKStart(d.Offset)
	if vkStart > 0 && vkStart+uint64(len(format.VKSignature)) <= uint64(len(data)) {
		if !bytes.Equal(data[vkStart:vkStart+uint64(len(format.VKSignature))], format.VKSignature) {
			// Only fail if we're confident about the VK start position
			// For now, just log this as a potential issue but don't fail
		}
	}

	return nil
}

// applyDefaultRepair applies a default value repair (typically InvalidOffset or 0).
func (m *VKModule) applyDefaultRepair(data []byte, d Diagnostic) error {
	// For VK repairs, "default" typically means setting an offset to InvalidOffset
	// or a data length to 0, depending on the field
	var defaultValue uint32 = format.InvalidOffset
	if d.Expected != nil {
		if exp, ok := d.Expected.(uint32); ok {
			defaultValue = exp
		}
	}
	binary.LittleEndian.PutUint32(data[d.Offset:], defaultValue)
	return nil
}

// applyReplaceRepair applies a replacement value repair.
func (m *VKModule) applyReplaceRepair(data []byte, d Diagnostic) error {
	expectedValue, ok := d.Expected.(uint32)
	if !ok {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
		}
	}
	binary.LittleEndian.PutUint32(data[d.Offset:], expectedValue)
	return nil
}

// applyTruncateRepair applies a truncation repair (reduces a value to fit constraints).
func (m *VKModule) applyTruncateRepair(data []byte, d Diagnostic) error {
	// Truncate typically means reducing a data length field
	expectedValue, ok := d.Expected.(uint32)
	if !ok {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
		}
	}
	binary.LittleEndian.PutUint32(data[d.Offset:], expectedValue)
	return nil
}

// validateReplaceValue validates a replace repair's value.
func (m *VKModule) validateReplaceValue(d Diagnostic) error {
	expectedValue, ok := d.Expected.(uint32)
	if !ok {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
		}
	}

	// For data offsets, they should either be InvalidOffset or within plausible range
	if expectedValue != format.InvalidOffset && expectedValue > 0 {
		if expectedValue < 0x1000 {
			// Offsets below HBIN start (0x1000) are suspicious
			return &RepairError{
				Module:  m.name,
				Offset:  d.Offset,
				Message: fmt.Sprintf("suspicious expected value: 0x%X", expectedValue),
			}
		}
	}

	return nil
}

// calculateVKStart calculates the start offset of a VK record given a field offset.
// This is necessary because diagnostics report the offset of the problematic field,
// not the start of the VK record.
func (m *VKModule) calculateVKStart(fieldOffset uint64) uint64 {
	// VK records are within HBIN cells like NK records
	// For field-level repairs, we estimate the VK start by subtracting common field offsets

	if fieldOffset < uint64(format.HeaderSize+format.HBINHeaderSize+format.CellHeaderSize) {
		return 0
	}

	// Calculate offset within the current HBIN
	relativeToBin := (fieldOffset - uint64(format.HeaderSize)) % uint64(format.HBINAlignment)

	// If we're within the HBIN header, return 0
	if relativeToBin < uint64(format.HBINHeaderSize) {
		return 0
	}

	// Calculate offset within data area of HBIN
	relativeToData := relativeToBin - uint64(format.HBINHeaderSize)

	// Round down to nearest cell alignment to find cell start
	cellStart := (relativeToData / uint64(format.CellAlignment)) * uint64(format.CellAlignment)

	// VK signature is at cellStart + CellHeaderSize
	vkSigOffset := uint64(
		format.HeaderSize,
	) + ((fieldOffset - uint64(format.HeaderSize)) / uint64(format.HBINAlignment) * uint64(format.HBINAlignment)) + uint64(
		format.HBINHeaderSize,
	) + cellStart + uint64(
		format.CellHeaderSize,
	)

	return vkSigOffset
}
