package repair

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// REGFModule handles repairs for REGF (Registry File) header structures.
// The REGF header is the main header at offset 0 of a hive file.
type REGFModule struct {
	RepairModuleBase
}

// NewREGFModule creates a new REGF repair module.
func NewREGFModule() *REGFModule {
	return &REGFModule{
		RepairModuleBase: RepairModuleBase{
			name: "REGF",
		},
	}
}

// CanRepair checks if this module can handle the given diagnostic.
func (m *REGFModule) CanRepair(d Diagnostic) bool {
	return d.Structure == "REGF" && d.Repair != nil
}

// Validate checks if the repair is safe to apply.
func (m *REGFModule) Validate(data []byte, d Diagnostic) error {
	// REGF header must be at offset 0
	if d.Offset >= format.HeaderSize {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "REGF field offset must be within header",
		}
	}

	// Basic offset validation
	if err := m.ValidateOffset(data, d.Offset, 4); err != nil {
		return err
	}

	// Verify REGF signature is intact
	if !bytes.Equal(data[:len(format.REGFSignature)], format.REGFSignature) {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "REGF signature corrupted - cannot repair",
		}
	}

	// Validate repair type
	switch d.Repair.Type {
	case RepairReplace:
		return m.validateReplaceValue(d)
	case RepairDefault:
		// Setting fields to default values
		return nil
	default:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	}
}

// Apply performs the actual repair on the data.
func (m *REGFModule) Apply(data []byte, d Diagnostic) error {
	switch d.Repair.Type {
	case RepairReplace:
		return m.applyReplaceRepair(data, d)
	case RepairDefault:
		return m.applyDefaultRepair(data, d)
	default:
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type: %s", d.Repair.Type),
		}
	}
}

// Verify confirms the repair was successful.
func (m *REGFModule) Verify(data []byte, d Diagnostic) error {
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
	case RepairReplace:
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
	}

	// Verify REGF signature is still intact
	if !bytes.Equal(data[:len(format.REGFSignature)], format.REGFSignature) {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "REGF signature corrupted after repair",
		}
	}

	return nil
}

// applyReplaceRepair applies a replacement value repair.
func (m *REGFModule) applyReplaceRepair(data []byte, d Diagnostic) error {
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

// applyDefaultRepair applies a default value repair.
func (m *REGFModule) applyDefaultRepair(data []byte, d Diagnostic) error {
	// For REGF, default repairs might set sequences to match, etc.
	// The specific value depends on the field being repaired
	// For now, we'll rely on the Expected value being provided
	if d.Expected == nil {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "default repair requires Expected value",
		}
	}
	return m.applyReplaceRepair(data, d)
}

// validateReplaceValue validates a replace repair's value.
func (m *REGFModule) validateReplaceValue(d Diagnostic) error {
	expectedValue, ok := d.Expected.(uint32)
	if !ok {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
		}
	}

	// Additional validation based on the field being repaired
	// Check reasonable ranges for common REGF fields
	if d.Offset == uint64(format.REGFPrimarySeqOffset) || d.Offset == uint64(format.REGFSecondarySeqOffset) {
		// Sequence numbers can be any value, no validation needed
		return nil
	}

	if d.Offset == uint64(format.REGFRootCellOffset) {
		// Root cell offset should be within reasonable range
		if expectedValue > 0 && expectedValue < 0x1000 {
			return &RepairError{
				Module:  m.name,
				Offset:  d.Offset,
				Message: fmt.Sprintf("suspicious root cell offset: 0x%X", expectedValue),
			}
		}
	}

	return nil
}
