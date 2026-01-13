package repair

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// Helper function to create a minimal NK record for testing.
func createTestNKRecord(_ uint64) []byte {
	// Create a buffer with REGF header + HBIN header + NK record
	// This ensures offsets are realistic
	data := make([]byte, format.HeaderSize+format.HBINHeaderSize+format.NKMinSize+100)

	// Add REGF signature
	copy(data, format.REGFSignature)

	// Add HBIN signature at offset 0x1000
	hbinStart := format.HeaderSize
	copy(data[hbinStart:], format.HBINSignature)

	// Add NK record at specified offset (should be after HBIN header)
	// NK records are within cells, which have a 4-byte size header
	nkCellStart := hbinStart + format.HBINHeaderSize

	// Cell size (negative for allocated)
	binary.LittleEndian.PutUint32(data[nkCellStart:], 0xFFFFFF00) // -256 bytes

	// NK signature at cell + 4
	nkStart := nkCellStart + format.CellHeaderSize
	copy(data[nkStart:], format.NKSignature)

	// Set some reasonable NK fields
	binary.LittleEndian.PutUint16(data[nkStart+format.NKFlagsOffset:], 0x0020) // Compressed name flag
	binary.LittleEndian.PutUint32(data[nkStart+format.NKParentOffset:], format.InvalidOffset)
	binary.LittleEndian.PutUint32(data[nkStart+format.NKSubkeyCountOffset:], 0)
	binary.LittleEndian.PutUint32(data[nkStart+format.NKSubkeyListOffset:], 0xDEADBEEF) // Bogus offset
	binary.LittleEndian.PutUint32(data[nkStart+format.NKValueCountOffset:], 0)
	binary.LittleEndian.PutUint32(data[nkStart+format.NKValueListOffset:], 0xCAFEBABE) // Bogus offset

	return data
}

func TestNKModule_CanRepair(t *testing.T) {
	module := NewNKModule()

	tests := []struct {
		name     string
		diag     Diagnostic
		expected bool
	}{
		{
			name: "NK repair with action",
			diag: Diagnostic{
				Structure: "NK",
				Repair: &RepairAction{
					Type: RepairDefault,
				},
			},
			expected: true,
		},
		{
			name: "NK without repair action",
			diag: Diagnostic{
				Structure: "NK",
				Repair:    nil,
			},
			expected: false,
		},
		{
			name: "VK structure",
			diag: Diagnostic{
				Structure: "VK",
				Repair: &RepairAction{
					Type: RepairDefault,
				},
			},
			expected: false,
		},
		{
			name: "REGF structure",
			diag: Diagnostic{
				Structure: "REGF",
				Repair: &RepairAction{
					Type: RepairDefault,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := module.CanRepair(tt.diag)
			if result != tt.expected {
				t.Errorf("CanRepair() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNKModule_ValidateRepairDefault(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	// Offset to SubkeyListOffset field in NK record
	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	err := module.Validate(data, diag)
	if err != nil {
		t.Errorf("Validate() failed: %v", err)
	}
}

func TestNKModule_ValidateInvalidOffset(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	// Offset beyond data bounds
	diag := Diagnostic{
		Structure: "NK",
		Offset:    uint64(len(data) + 100),
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	err := module.Validate(data, diag)
	if err == nil {
		t.Error("Validate() should fail for out-of-bounds offset")
	}
}

func TestNKModule_ApplyDefaultRepair(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	// Verify initial value is bogus
	initialValue := binary.LittleEndian.Uint32(data[subkeyListOffset:])
	if initialValue != 0xDEADBEEF {
		t.Fatalf("initial value should be 0xDEADBEEF, got 0x%X", initialValue)
	}

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	// Apply repair
	err := module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Verify value is now InvalidOffset
	repairedValue := binary.LittleEndian.Uint32(data[subkeyListOffset:])
	if repairedValue != format.InvalidOffset {
		t.Errorf("expected InvalidOffset (0x%X), got 0x%X", format.InvalidOffset, repairedValue)
	}
}

func TestNKModule_ApplyReplaceRepair(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	expectedValue := uint32(0x2000)

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairReplace,
		},
	}

	// Validate
	err := module.Validate(data, diag)
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	// Apply repair
	err = module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Verify value
	repairedValue := binary.LittleEndian.Uint32(data[subkeyListOffset:])
	if repairedValue != expectedValue {
		t.Errorf("expected 0x%X, got 0x%X", expectedValue, repairedValue)
	}
}

func TestNKModule_Verify(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	// Apply repair
	module.Apply(data, diag)

	// Verify should pass
	err := module.Verify(data, diag)
	if err != nil {
		t.Errorf("Verify() failed after successful repair: %v", err)
	}

	// Verify NK signature is still intact
	nkSigOffset := nkStart
	if !bytes.Equal(data[nkSigOffset:nkSigOffset+uint64(len(format.NKSignature))], format.NKSignature) {
		t.Error("NK signature corrupted after repair")
	}
}

func TestNKModule_VerifyFailsOnWrongValue(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	// Apply repair
	module.Apply(data, diag)

	// Manually corrupt the value
	binary.LittleEndian.PutUint32(data[subkeyListOffset:], 0xBAD)

	// Verify should fail
	err := module.Verify(data, diag)
	if err == nil {
		t.Error("Verify() should fail when value doesn't match expected")
	}
}

func TestNKModule_VerifyWithCorruptSignature(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	// Apply repair
	module.Apply(data, diag)

	// Corrupt NK signature
	nkSigOffset := nkStart
	data[nkSigOffset] = 'X'

	// Verify should pass (signature check is lenient/best-effort)
	// In production, the diagnostic system would catch the corrupt signature separately
	err := module.Verify(data, diag)
	if err != nil {
		t.Errorf("Verify() with lenient signature check should pass, got error: %v", err)
	}
}

func TestNKModule_UnsupportedRepairType(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Repair: &RepairAction{
			Type: RepairTruncate, // Unsupported for NK
		},
	}

	// Validate should fail
	err := module.Validate(data, diag)
	if err == nil {
		t.Error("Validate() should fail for unsupported repair type")
	}

	// Apply should fail
	err = module.Apply(data, diag)
	if err == nil {
		t.Error("Apply() should fail for unsupported repair type")
	}
}

func TestNKModule_ValidateReplaceSuspiciousValue(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	subkeyListOffset := nkStart + uint64(format.NKSubkeyListOffset)

	// Try to replace with suspicious low offset
	diag := Diagnostic{
		Structure: "NK",
		Offset:    subkeyListOffset,
		Expected:  0x100, // Below HBIN start (0x1000)
		Repair: &RepairAction{
			Type: RepairReplace,
		},
	}

	err := module.Validate(data, diag)
	if err == nil {
		t.Error("Validate() should fail for suspicious replacement value")
	}
}

func TestNKModule_Name(t *testing.T) {
	module := NewNKModule()

	if module.Name() != "NK" {
		t.Errorf("expected name 'NK', got '%s'", module.Name())
	}
}

func TestNKModule_DanglingValueListOffset(t *testing.T) {
	module := NewNKModule()
	data := createTestNKRecord(0)

	nkStart := uint64(format.HeaderSize + format.HBINHeaderSize + format.CellHeaderSize)
	valueListOffset := nkStart + uint64(format.NKValueListOffset)

	// Verify initial value is bogus
	initialValue := binary.LittleEndian.Uint32(data[valueListOffset:])
	if initialValue != 0xCAFEBABE {
		t.Fatalf("initial value should be 0xCAFEBABE, got 0x%X", initialValue)
	}

	diag := Diagnostic{
		Structure: "NK",
		Offset:    valueListOffset,
		Issue:     "Value count is 0 but list offset is set",
		Repair: &RepairAction{
			Type:        RepairDefault,
			Description: "Set value list offset to InvalidOffset (0xFFFFFFFF)",
		},
	}

	// Validate
	err := module.Validate(data, diag)
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	// Apply repair
	err = module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Verify value is now InvalidOffset
	repairedValue := binary.LittleEndian.Uint32(data[valueListOffset:])
	if repairedValue != format.InvalidOffset {
		t.Errorf("expected InvalidOffset (0x%X), got 0x%X", format.InvalidOffset, repairedValue)
	}

	// Verify
	err = module.Verify(data, diag)
	if err != nil {
		t.Errorf("Verify() failed: %v", err)
	}
}
