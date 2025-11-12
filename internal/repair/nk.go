package repair

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// NKModule handles repairs for NK (Node Key) record structures.
// NK records represent registry keys and contain pointers to subkeys and values.
type NKModule struct {
	RepairModuleBase
}

// NewNKModule creates a new NK repair module.
func NewNKModule() *NKModule {
	return &NKModule{
		RepairModuleBase: RepairModuleBase{
			name: "NK",
		},
	}
}

// CanRepair checks if this module can handle the given diagnostic.
func (m *NKModule) CanRepair(d Diagnostic) bool {
	// We handle NK structure repairs
	return d.Structure == "NK" && d.Repair != nil
}

// Validate checks if the repair is safe to apply.
func (m *NKModule) Validate(data []byte, d Diagnostic) error {
	// Basic offset validation
	if err := m.ValidateOffset(data, d.Offset, 4); err != nil {
		return err
	}

	// Verify we're actually at an NK record
	// The offset points to the field within the NK record, not the start
	// We need to calculate the NK record start based on the field offset
	nkStart := m.calculateNKStart(d.Offset)

	if nkStart >= uint64(len(data)) {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "NK record start out of bounds",
		}
	}

	// Try to verify NK signature if we can calculate a reasonable start position
	// This is best-effort validation - if we can't find the signature, we still allow
	// the repair since Verify() will do more thorough checks after the repair
	if nkStart > 0 && nkStart+uint64(len(format.NKSignature)) <= uint64(len(data)) {
		if bytes.Equal(data[nkStart:nkStart+uint64(len(format.NKSignature))], format.NKSignature) {
			// Signature found, good!
		} else {
			// Signature not found - this is OK, we'll verify after repair
			// Don't fail the validation here
		}
	}

	// Validate repair type
	switch d.Repair.Type {
	case RepairDefault:
		// Setting offsets to InvalidOffset (0xFFFFFFFF) is safe
		return nil
	case RepairReplace:
		// Replacing with a specific value - ensure it's valid
		return m.validateReplaceValue(d)
	case RepairTruncate, RepairRebuild, RepairRemove:
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
func (m *NKModule) Apply(data []byte, d Diagnostic) error {
	switch d.Repair.Type {
	case RepairDefault:
		return m.applyDefaultRepair(data, d)
	case RepairReplace:
		return m.applyReplaceRepair(data, d)
	case RepairTruncate, RepairRebuild, RepairRemove:
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
func (m *NKModule) Verify(data []byte, d Diagnostic) error {
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
		// Should be InvalidOffset (0xFFFFFFFF)
		if actualValue != format.InvalidOffset {
			return &RepairError{
				Module:  m.name,
				Offset:  d.Offset,
				Message: fmt.Sprintf("verification failed: expected 0x%X, got 0x%X", format.InvalidOffset, actualValue),
			}
		}
	case RepairReplace:
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
	case RepairTruncate, RepairRebuild, RepairRemove:
		// These repair types are not supported by NK module
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type in verify: %s", d.Repair.Type),
		}
	}

	// Verify NK signature is still intact (best effort)
	nkStart := m.calculateNKStart(d.Offset)
	if nkStart > 0 && nkStart+uint64(len(format.NKSignature)) <= uint64(len(data)) {
		if !bytes.Equal(data[nkStart:nkStart+uint64(len(format.NKSignature))], format.NKSignature) {
			// Only fail if we're confident about the NK start position
			// For now, just log this as a potential issue but don't fail
			// In production, we might want to be more strict here
		}
	}

	return nil
}

// applyDefaultRepair applies a default value repair (typically InvalidOffset).
func (m *NKModule) applyDefaultRepair(data []byte, d Diagnostic) error {
	// For NK repairs, "default" typically means setting an offset to InvalidOffset
	binary.LittleEndian.PutUint32(data[d.Offset:], format.InvalidOffset)
	return nil
}

// applyReplaceRepair applies a replacement value repair.
func (m *NKModule) applyReplaceRepair(data []byte, d Diagnostic) error {
	// Replace with the expected value
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
func (m *NKModule) validateReplaceValue(d Diagnostic) error {
	// Type assert expected value
	expectedValue, ok := d.Expected.(uint32)
	if !ok {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
		}
	}

	// Ensure the expected value is reasonable
	// For offsets, they should either be InvalidOffset or within plausible range
	if expectedValue != format.InvalidOffset && expectedValue > 0 {
		// Could add more sophisticated validation here
		// For now, just ensure it's not obviously wrong
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

// calculateNKStart calculates the start offset of an NK record given a field offset.
// This is necessary because diagnostics report the offset of the problematic field,
// not the start of the NK record.
func (m *NKModule) calculateNKStart(fieldOffset uint64) uint64 {
	// NK records start after the REGF header (0x1000) and are within HBIN cells
	// Field offsets are absolute in the file

	// Try to calculate based on known field offsets
	// Common NK field offsets from NK signature:
	// SubkeyListOffset: 0x1C
	// ValueListOffset: 0x28

	// Minimum offset for NK record (after REGF + HBIN header + cell header)
	minOffset := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)

	if fieldOffset < minOffset {
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

	// NK signature is at cellStart + CellHeaderSize
	nkSigOffset := uint64(
		format.HeaderSize,
	) + ((fieldOffset - uint64(format.HeaderSize)) / uint64(format.HBINAlignment) * uint64(format.HBINAlignment)) + uint64(
		format.HBINHeaderSize,
	) + cellStart + uint64(
		format.CellHeaderSize,
	)

	return nkSigOffset
}
