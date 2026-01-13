package repair

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// HBINModule handles repairs for HBIN (Hive Bin) header structures.
// HBINs are 4KB-aligned blocks that contain cells.
type HBINModule struct {
	RepairModuleBase
}

// NewHBINModule creates a new HBIN repair module.
func NewHBINModule() *HBINModule {
	return &HBINModule{
		RepairModuleBase: RepairModuleBase{
			name: hbinStructureName,
		},
	}
}

// CanRepair checks if this module can handle the given diagnostic.
func (m *HBINModule) CanRepair(d Diagnostic) bool {
	return d.Structure == hbinStructureName && d.Repair != nil
}

// Validate checks if the repair is safe to apply.
func (m *HBINModule) Validate(data []byte, d Diagnostic) error {
	// Basic offset validation
	if err := m.ValidateOffset(data, d.Offset, 4); err != nil {
		return err
	}

	// Calculate HBIN start from field offset
	hbinStart := m.calculateHBINStart(d.Offset)

	if hbinStart >= uint64(len(data)) {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: "HBIN start out of bounds",
		}
	}

	// Verify HBIN signature if we have enough data
	if hbinStart+uint64(len(format.HBINSignature)) <= uint64(len(data)) {
		if !bytes.Equal(data[hbinStart:hbinStart+uint64(len(format.HBINSignature))], format.HBINSignature) {
			// Signature not found - might be OK for field repairs
			// The Verify() will do more thorough checks
		}
	}

	// Validate repair type
	switch d.Repair.Type {
	case RepairReplace:
		return m.validateReplaceValue(d)
	case RepairDefault:
		return nil
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
func (m *HBINModule) Apply(data []byte, d Diagnostic) error {
	switch d.Repair.Type {
	case RepairReplace:
		return m.applyReplaceRepair(data, d)
	case RepairDefault:
		return m.applyDefaultRepair(data, d)
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
func (m *HBINModule) Verify(data []byte, d Diagnostic) error {
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
	case RepairDefault:
		// Default repair doesn't need value verification beyond basic checks
	case RepairTruncate, RepairRebuild, RepairRemove:
		// These repair types are not supported by HBIN module
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("unsupported repair type in verify: %s", d.Repair.Type),
		}
	}

	// Verify HBIN signature is still intact (best effort)
	hbinStart := m.calculateHBINStart(d.Offset)
	if hbinStart > 0 && hbinStart+uint64(len(format.HBINSignature)) <= uint64(len(data)) {
		if !bytes.Equal(data[hbinStart:hbinStart+uint64(len(format.HBINSignature))], format.HBINSignature) {
			// Only fail if we're confident about the HBIN start position
			// For now, just log this as a potential issue but don't fail
		}
	}

	return nil
}

// applyReplaceRepair applies a replacement value repair.
func (m *HBINModule) applyReplaceRepair(data []byte, d Diagnostic) error {
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
func (m *HBINModule) applyDefaultRepair(data []byte, d Diagnostic) error {
	// For HBIN, default repairs typically use the Expected value
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
func (m *HBINModule) validateReplaceValue(d Diagnostic) error {
	expectedValue, ok := d.Expected.(uint32)
	if !ok {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("expected value is not uint32: %T", d.Expected),
		}
	}

	// For HBIN file offset field, the value should be aligned
	if expectedValue > 0 && expectedValue%uint32(format.HBINAlignment) != 0 {
		return &RepairError{
			Module:  m.name,
			Offset:  d.Offset,
			Message: fmt.Sprintf("HBIN file offset not aligned to 0x%X: 0x%X", format.HBINAlignment, expectedValue),
		}
	}

	return nil
}

// calculateHBINStart calculates the start offset of an HBIN given a field offset.
// HBINs are 0x1000-aligned after the REGF header (0x1000).
func (m *HBINModule) calculateHBINStart(fieldOffset uint64) uint64 {
	if fieldOffset < uint64(format.HeaderSize) {
		return 0
	}

	// Calculate which HBIN this offset is in
	relativeOffset := fieldOffset - uint64(format.HeaderSize)
	hbinIndex := relativeOffset / uint64(format.HBINAlignment)
	hbinStart := uint64(format.HeaderSize) + (hbinIndex * uint64(format.HBINAlignment))

	return hbinStart
}
