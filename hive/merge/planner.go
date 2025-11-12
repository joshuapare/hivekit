package merge

import (
	"encoding/json"
	"fmt"

	"github.com/joshuapare/hivekit/internal/format"
)

// PatchOperation represents a single operation in a JSON patch.
type PatchOperation struct {
	Op        string   `json:"op"`       // "ensure_key", "delete_key", "set_value", "delete_value"
	KeyPath   []string `json:"key_path"` // Path segments
	ValueName string   `json:"value_name,omitempty"`
	ValueType string   `json:"value_type,omitempty"` // "REG_SZ", "REG_DWORD", etc.
	Data      []byte   `json:"data,omitempty"`
}

// Patch represents a collection of operations in JSON format.
type Patch struct {
	Operations []PatchOperation `json:"operations"`
}

// ParseJSONPatch parses a JSON patch into a Plan.
func ParseJSONPatch(data []byte) (*Plan, error) {
	var patch Patch
	if err := json.Unmarshal(data, &patch); err != nil {
		return nil, fmt.Errorf("invalid JSON patch: %w", err)
	}

	plan := NewPlan()

	for i, patchOp := range patch.Operations {
		op, err := convertPatchOp(&patchOp)
		if err != nil {
			return nil, fmt.Errorf("operation %d: %w", i, err)
		}
		plan.Ops = append(plan.Ops, *op)
	}

	return plan, nil
}

// convertPatchOp converts a PatchOperation to an Op.
func convertPatchOp(patchOp *PatchOperation) (*Op, error) {
	op := &Op{
		KeyPath: patchOp.KeyPath,
	}

	// Parse operation type
	switch patchOp.Op {
	case "ensure_key":
		op.Type = OpEnsureKey
	case "delete_key":
		op.Type = OpDeleteKey
	case "set_value":
		op.Type = OpSetValue
		op.ValueName = patchOp.ValueName
		op.Data = patchOp.Data

		// Convert value type string to uint32
		valueType, err := parseValueType(patchOp.ValueType)
		if err != nil {
			return nil, fmt.Errorf("invalid value type: %w", err)
		}
		op.ValueType = valueType

	case "delete_value":
		op.Type = OpDeleteValue
		op.ValueName = patchOp.ValueName

	default:
		return nil, fmt.Errorf("unknown operation: %s", patchOp.Op)
	}

	// Validate key path
	if len(op.KeyPath) == 0 {
		return nil, fmt.Errorf("empty key path for operation %s", patchOp.Op)
	}

	return op, nil
}

// parseValueType converts a string value type to its uint32 code.
func parseValueType(valueType string) (uint32, error) {
	// Windows registry value types
	// See: https://docs.microsoft.com/en-us/windows/win32/sysinfo/registry-value-types
	types := map[string]uint32{
		"REG_NONE":                       format.REGNone,
		"REG_SZ":                         format.REGSZ,
		"REG_EXPAND_SZ":                  format.REGExpandSZ,
		"REG_BINARY":                     format.REGBinary,
		"REG_DWORD":                      format.REGDWORD,
		"REG_DWORD_LITTLE_ENDIAN":        format.REGDWORD,
		"REG_DWORD_BIG_ENDIAN":           format.REGDWORDBigEndian,
		"REG_LINK":                       format.REGLink,
		"REG_MULTI_SZ":                   format.REGMultiSZ,
		"REG_RESOURCE_LIST":              format.REGResourceList,
		"REG_FULL_RESOURCE_DESCRIPTOR":   format.REGFullResourceDescriptor,
		"REG_RESOURCE_REQUIREMENTS_LIST": format.REGResourceRequirementsList,
		"REG_QWORD":                      format.REGQWORD,
		"REG_QWORD_LITTLE_ENDIAN":        format.REGQWORD,
	}

	code, ok := types[valueType]
	if !ok {
		return 0, fmt.Errorf("unknown value type: %s", valueType)
	}

	return code, nil
}

// MarshalPlan converts a Plan to JSON format.
func MarshalPlan(plan *Plan) ([]byte, error) {
	patch := Patch{
		Operations: make([]PatchOperation, 0, len(plan.Ops)),
	}

	for _, op := range plan.Ops {
		patchOp, err := convertOpToPatch(&op)
		if err != nil {
			return nil, fmt.Errorf("convert operation: %w", err)
		}
		patch.Operations = append(patch.Operations, *patchOp)
	}

	return json.MarshalIndent(patch, "", "  ")
}

// convertOpToPatch converts an Op to a PatchOperation.
func convertOpToPatch(op *Op) (*PatchOperation, error) {
	patchOp := &PatchOperation{
		KeyPath: op.KeyPath,
	}

	switch op.Type {
	case OpEnsureKey:
		patchOp.Op = "ensure_key"
	case OpDeleteKey:
		patchOp.Op = "delete_key"
	case OpSetValue:
		patchOp.Op = "set_value"
		patchOp.ValueName = op.ValueName
		patchOp.ValueType = formatValueType(op.ValueType)
		patchOp.Data = op.Data
	case OpDeleteValue:
		patchOp.Op = "delete_value"
		patchOp.ValueName = op.ValueName
	default:
		return nil, fmt.Errorf("unknown operation type: %d", op.Type)
	}

	return patchOp, nil
}

// formatValueType converts a uint32 value type code to its string representation.
func formatValueType(code uint32) string {
	types := map[uint32]string{
		0:  "REG_NONE",
		1:  "REG_SZ",
		2:  "REG_EXPAND_SZ",
		3:  "REG_BINARY",
		4:  "REG_DWORD",
		5:  "REG_DWORD_BIG_ENDIAN",
		6:  "REG_LINK",
		7:  "REG_MULTI_SZ",
		8:  "REG_RESOURCE_LIST",
		9:  "REG_FULL_RESOURCE_DESCRIPTOR",
		10: "REG_RESOURCE_REQUIREMENTS_LIST",
		11: "REG_QWORD",
	}

	if name, ok := types[code]; ok {
		return name
	}

	return fmt.Sprintf("UNKNOWN_%d", code)
}
