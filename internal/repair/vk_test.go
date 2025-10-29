package repair

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/joshuapare/hivekit/internal/format"
)

// TestVKModule_CanRepair tests the CanRepair method.
func TestVKModule_CanRepair(t *testing.T) {
	module := NewVKModule()

	tests := []struct {
		name     string
		diag     Diagnostic
		expected bool
	}{
		{
			name: "VK repair with action",
			diag: Diagnostic{
				Structure: "VK",
				Repair:    &RepairAction{Type: RepairDefault},
			},
			expected: true,
		},
		{
			name: "VK without repair action",
			diag: Diagnostic{
				Structure: "VK",
				Repair:    nil,
			},
			expected: false,
		},
		{
			name: "NK structure",
			diag: Diagnostic{
				Structure: "NK",
				Repair:    &RepairAction{Type: RepairDefault},
			},
			expected: false,
		},
		{
			name: "REGF structure",
			diag: Diagnostic{
				Structure: "REGF",
				Repair:    &RepairAction{Type: RepairDefault},
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

// TestVKModule_ValidateRepairDefault tests validation of default repairs.
func TestVKModule_ValidateRepairDefault(t *testing.T) {
	module := NewVKModule()

	// Create test data with VK signature
	data := make([]byte, 0x2000)
	vkOffset := uint64(0x1020) // HBIN starts at 0x1000, VK at offset 0x20 within HBIN data
	copy(data[vkOffset:], format.VKSignature)

	// Field offset (e.g., data offset field within VK)
	fieldOffset := vkOffset + 0x10

	diag := Diagnostic{
		Structure: "VK",
		Offset:    fieldOffset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	err := module.Validate(data, diag)
	if err != nil {
		t.Errorf("Validate() unexpected error: %v", err)
	}
}

// TestVKModule_ValidateInvalidOffset tests validation with invalid offset.
func TestVKModule_ValidateInvalidOffset(t *testing.T) {
	module := NewVKModule()

	// Data too small
	data := make([]byte, 10)

	diag := Diagnostic{
		Structure: "VK",
		Offset:    100,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	err := module.Validate(data, diag)
	if err == nil {
		t.Error("Validate() should fail with invalid offset")
	}
}

// TestVKModule_ApplyDefaultRepair tests applying a default repair.
func TestVKModule_ApplyDefaultRepair(t *testing.T) {
	module := NewVKModule()

	// Create test data
	data := make([]byte, 100)
	offset := uint64(10)

	// Set some non-default value
	binary.LittleEndian.PutUint32(data[offset:], 0x12345678)

	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	err := module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}

	// Verify data was set to InvalidOffset
	result := binary.LittleEndian.Uint32(data[offset:])
	if result != format.InvalidOffset {
		t.Errorf("Apply() result = 0x%X, want 0x%X", result, format.InvalidOffset)
	}
}

// TestVKModule_ApplyDefaultRepairWithExpected tests applying a default repair with Expected value.
func TestVKModule_ApplyDefaultRepairWithExpected(t *testing.T) {
	module := NewVKModule()

	// Create test data
	data := make([]byte, 100)
	offset := uint64(10)

	// Set some non-default value
	binary.LittleEndian.PutUint32(data[offset:], 0x12345678)

	expectedValue := uint32(0)
	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairDefault,
		},
	}

	err := module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}

	// Verify data was set to expected value (0)
	result := binary.LittleEndian.Uint32(data[offset:])
	if result != expectedValue {
		t.Errorf("Apply() result = 0x%X, want 0x%X", result, expectedValue)
	}
}

// TestVKModule_ApplyReplaceRepair tests applying a replace repair.
func TestVKModule_ApplyReplaceRepair(t *testing.T) {
	module := NewVKModule()

	// Create test data
	data := make([]byte, 100)
	offset := uint64(10)

	// Set some value
	binary.LittleEndian.PutUint32(data[offset:], 0x12345678)

	expectedValue := uint32(0x2000)
	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairReplace,
		},
	}

	err := module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}

	// Verify data was set to expected value
	result := binary.LittleEndian.Uint32(data[offset:])
	if result != expectedValue {
		t.Errorf("Apply() result = 0x%X, want 0x%X", result, expectedValue)
	}
}

// TestVKModule_ApplyTruncateRepair tests applying a truncate repair.
func TestVKModule_ApplyTruncateRepair(t *testing.T) {
	module := NewVKModule()

	// Create test data
	data := make([]byte, 100)
	offset := uint64(10)

	// Set large data length
	binary.LittleEndian.PutUint32(data[offset:], 0x10000000)

	expectedValue := uint32(0x100) // Truncated to smaller size
	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairTruncate,
		},
	}

	err := module.Apply(data, diag)
	if err != nil {
		t.Fatalf("Apply() unexpected error: %v", err)
	}

	// Verify data was truncated to expected value
	result := binary.LittleEndian.Uint32(data[offset:])
	if result != expectedValue {
		t.Errorf("Apply() result = 0x%X, want 0x%X", result, expectedValue)
	}
}

// TestVKModule_Verify tests the Verify method.
func TestVKModule_Verify(t *testing.T) {
	module := NewVKModule()

	// Create test data
	data := make([]byte, 100)
	offset := uint64(10)

	expectedValue := uint32(0x2000)
	binary.LittleEndian.PutUint32(data[offset:], expectedValue)

	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairReplace,
		},
	}

	err := module.Verify(data, diag)
	if err != nil {
		t.Errorf("Verify() unexpected error: %v", err)
	}
}

// TestVKModule_VerifyFailsOnWrongValue tests verification failure.
func TestVKModule_VerifyFailsOnWrongValue(t *testing.T) {
	module := NewVKModule()

	// Create test data
	data := make([]byte, 100)
	offset := uint64(10)

	expectedValue := uint32(0x2000)
	wrongValue := uint32(0x3000)
	binary.LittleEndian.PutUint32(data[offset:], wrongValue)

	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairReplace,
		},
	}

	err := module.Verify(data, diag)
	if err == nil {
		t.Error("Verify() should fail when value doesn't match expected")
	}
}

// TestVKModule_VerifyWithVKSignature tests verification with VK signature present.
func TestVKModule_VerifyWithVKSignature(t *testing.T) {
	module := NewVKModule()

	// Create test data with VK signature
	data := make([]byte, 0x2000)
	vkOffset := uint64(0x1020) // VK start
	copy(data[vkOffset:], format.VKSignature)

	// Field offset within VK
	fieldOffset := vkOffset + 0x10
	expectedValue := uint32(0x2000)
	binary.LittleEndian.PutUint32(data[fieldOffset:], expectedValue)

	diag := Diagnostic{
		Structure: "VK",
		Offset:    fieldOffset,
		Expected:  expectedValue,
		Repair: &RepairAction{
			Type: RepairReplace,
		},
	}

	err := module.Verify(data, diag)
	if err != nil {
		t.Errorf("Verify() unexpected error: %v", err)
	}

	// Verify VK signature is still intact
	if !bytes.Equal(data[vkOffset:vkOffset+uint64(len(format.VKSignature))], format.VKSignature) {
		t.Error("VK signature was corrupted after repair")
	}
}

// TestVKModule_UnsupportedRepairType tests handling of unsupported repair types.
func TestVKModule_UnsupportedRepairType(t *testing.T) {
	module := NewVKModule()

	data := make([]byte, 100)
	offset := uint64(10)

	diag := Diagnostic{
		Structure: "VK",
		Offset:    offset,
		Repair: &RepairAction{
			Type: RepairRebuild, // Unsupported for VK
		},
	}

	// Should fail validation
	err := module.Validate(data, diag)
	if err == nil {
		t.Error("Validate() should fail with unsupported repair type")
	}

	// Should fail apply
	err = module.Apply(data, diag)
	if err == nil {
		t.Error("Apply() should fail with unsupported repair type")
	}
}

// TestVKModule_ValidateReplaceSuspiciousValue tests validation of suspicious values.
func TestVKModule_ValidateReplaceSuspiciousValue(t *testing.T) {
	module := NewVKModule()

	data := make([]byte, 0x2000)

	tests := []struct {
		name          string
		expectedValue uint32
		shouldFail    bool
	}{
		{
			name:          "InvalidOffset is valid",
			expectedValue: format.InvalidOffset,
			shouldFail:    false,
		},
		{
			name:          "Valid offset after HBIN start",
			expectedValue: 0x2000,
			shouldFail:    false,
		},
		{
			name:          "Suspicious offset before HBIN start",
			expectedValue: 0x100,
			shouldFail:    true,
		},
		{
			name:          "Zero is valid",
			expectedValue: 0,
			shouldFail:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diag := Diagnostic{
				Structure: "VK",
				Offset:    0x1020,
				Expected:  tt.expectedValue,
				Repair: &RepairAction{
					Type: RepairReplace,
				},
			}

			err := module.Validate(data, diag)
			if tt.shouldFail && err == nil {
				t.Error("Validate() should fail with suspicious value")
			}
			if !tt.shouldFail && err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})
	}
}

// TestVKModule_Name tests the Name method.
func TestVKModule_Name(t *testing.T) {
	module := NewVKModule()
	if module.Name() != "VK" {
		t.Errorf("Name() = %q, want %q", module.Name(), "VK")
	}
}

// TestVKModule_DanglingDataOffset tests repairing a dangling data offset.
func TestVKModule_DanglingDataOffset(t *testing.T) {
	module := NewVKModule()

	// Simulate a VK with dangling data offset
	data := make([]byte, 0x2000)
	vkOffset := uint64(0x1020)
	copy(data[vkOffset:], format.VKSignature)

	// Data offset field at VK + 0x10 (example field)
	dataOffsetField := vkOffset + 0x10
	binary.LittleEndian.PutUint32(data[dataOffsetField:], 0xDEADBEEF) // Invalid offset

	diag := Diagnostic{
		Structure: "VK",
		Offset:    dataOffsetField,
		Issue:     "Value data offset is invalid",
		Expected:  format.InvalidOffset,
		Repair: &RepairAction{
			Type:      RepairDefault,
			AutoApply: true,
			Risk:      RiskLow,
		},
	}

	// Validate
	if err := module.Validate(data, diag); err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	// Apply
	if err := module.Apply(data, diag); err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Verify
	if err := module.Verify(data, diag); err != nil {
		t.Fatalf("Verify() failed: %v", err)
	}

	// Check result
	result := binary.LittleEndian.Uint32(data[dataOffsetField:])
	if result != format.InvalidOffset {
		t.Errorf("After repair, data offset = 0x%X, want 0x%X", result, format.InvalidOffset)
	}

	// VK signature should still be intact
	if !bytes.Equal(data[vkOffset:vkOffset+uint64(len(format.VKSignature))], format.VKSignature) {
		t.Error("VK signature was corrupted after repair")
	}
}
