package hivexval

import (
	"fmt"
	"reflect"
	"strings"
)

// Compare compares this validator with another implementation.
//
// This performs a deep, recursive comparison of the entire tree structure,
// comparing keys, values, types, and data between the two validators.
//
// Returns a ComparisonResult with detailed mismatches.
//
// Example:
//
//	v1 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseBindings: true}))
//	v2 := hivexval.Must(hivexval.New(path, &hivexval.Options{UseReader: true}))
//
//	result, err := v1.Compare(v2)
//	if !result.Match {
//	    for _, m := range result.Mismatches {
//	        t.Errorf("[%s] %s: %s", m.Category, m.Path, m.Message)
//	    }
//	}
func (v *Validator) Compare(other *Validator) (*ComparisonResult, error) {
	result := &ComparisonResult{
		Match:      true,
		Mismatches: make([]Mismatch, 0),
	}

	// Get roots from both validators
	root1, err := v.Root()
	if err != nil {
		return nil, fmt.Errorf("get root from first validator: %w", err)
	}

	root2, err := other.Root()
	if err != nil {
		return nil, fmt.Errorf("get root from second validator: %w", err)
	}

	// Compare trees recursively
	v.compareNode(root1, root2, "", v, other, result)

	// Set overall match status
	result.Match = len(result.Mismatches) == 0

	return result, nil
}

// compareNode recursively compares two nodes and their children.
func (v *Validator) compareNode(
	node1 interface{},
	node2 interface{},
	path string,
	val1 *Validator,
	val2 *Validator,
	result *ComparisonResult,
) {
	result.NodesCompared++

	// Get node names
	name1, err1 := val1.GetKeyName(node1)
	_, err2 := val2.GetKeyName(node2)

	if err1 != nil || err2 != nil {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Category: "key_name",
			Path:     path,
			Message:  "Error getting key names",
			Expected: err1,
			Actual:   err2,
		})
		return
	}

	// Build current path
	currentPath := path
	if path == "" || path == "\\" {
		if name1 != "" {
			currentPath = "\\" + name1
		} else {
			currentPath = "\\"
		}
	} else {
		if name1 != "" {
			currentPath = path + "\\" + name1
		}
	}

	// Compare subkey counts
	count1, err1 := val1.GetSubkeyCount(node1)
	count2, err2 := val2.GetSubkeyCount(node2)

	if err1 == nil && err2 == nil && count1 != count2 {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Category: "subkey_count",
			Path:     currentPath,
			Message:  "Different subkey counts",
			Expected: count1,
			Actual:   count2,
		})
	}

	// Compare value counts
	vcount1, err1 := val1.GetValueCount(node1)
	vcount2, err2 := val2.GetValueCount(node2)

	if err1 == nil && err2 == nil && vcount1 != vcount2 {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Category: "value_count",
			Path:     currentPath,
			Message:  "Different value counts",
			Expected: vcount1,
			Actual:   vcount2,
		})
	}

	// Compare values
	v.compareValues(node1, node2, currentPath, val1, val2, result)

	// Get children from both nodes
	children1, err1 := val1.GetSubkeys(node1)
	children2, err2 := val2.GetSubkeys(node2)

	if err1 != nil && err2 != nil {
		// Both failed to get children, skip
		return
	}

	if (err1 != nil && err2 == nil) || (err1 == nil && err2 != nil) {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Category: "subkeys",
			Path:     currentPath,
			Message:  "One validator can list children, the other cannot",
			Expected: err1,
			Actual:   err2,
		})
		return
	}

	// Build maps of children by name for easier comparison
	childMap1 := make(map[string]interface{})
	for _, child := range children1 {
		name, err := val1.GetKeyName(child)
		if err == nil {
			childMap1[strings.ToLower(name)] = child
		}
	}

	childMap2 := make(map[string]interface{})
	for _, child := range children2 {
		name, err := val2.GetKeyName(child)
		if err == nil {
			childMap2[strings.ToLower(name)] = child
		}
	}

	// Find children only in first validator
	for name, child1 := range childMap1 {
		if _, exists := childMap2[name]; !exists {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Category: "missing_key",
				Path:     currentPath + "\\" + name,
				Message:  "Key exists in first validator but not in second",
			})
		} else {
			// Compare matching children recursively
			child2 := childMap2[name]
			v.compareNode(child1, child2, currentPath, val1, val2, result)
		}
	}

	// Find children only in second validator
	for name := range childMap2 {
		if _, exists := childMap1[name]; !exists {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Category: "extra_key",
				Path:     currentPath + "\\" + name,
				Message:  "Key exists in second validator but not in first",
			})
		}
	}
}

// compareValues compares values between two nodes.
func (v *Validator) compareValues(
	node1 interface{},
	node2 interface{},
	path string,
	val1 *Validator,
	val2 *Validator,
	result *ComparisonResult,
) {
	// Get values from both nodes
	values1, err1 := val1.GetValues(node1)
	values2, err2 := val2.GetValues(node2)

	if err1 != nil && err2 != nil {
		// Both failed, skip
		return
	}

	// Build maps of values by name
	valueMap1 := make(map[string]interface{})
	for _, val := range values1 {
		name, err := val1.GetValueName(val)
		if err == nil {
			valueMap1[strings.ToLower(name)] = val
		}
	}

	valueMap2 := make(map[string]interface{})
	for _, val := range values2 {
		name, err := val2.GetValueName(val)
		if err == nil {
			valueMap2[strings.ToLower(name)] = val
		}
	}

	// Compare values
	for name, value1 := range valueMap1 {
		result.ValuesCompared++

		value2, exists := valueMap2[name]
		if !exists {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Category: "missing_value",
				Path:     path,
				Message:  fmt.Sprintf("Value '%s' exists in first validator but not in second", name),
			})
			continue
		}

		// Compare value types
		type1, err1 := val1.GetValueType(value1)
		type2, err2 := val2.GetValueType(value2)

		if err1 == nil && err2 == nil && type1 != type2 {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Category: "value_type",
				Path:     path,
				Message:  fmt.Sprintf("Value '%s' has different types", name),
				Expected: type1,
				Actual:   type2,
			})
			continue
		}

		// Compare value data
		data1, err1 := val1.GetValueData(value1)
		data2, err2 := val2.GetValueData(value2)

		if err1 == nil && err2 == nil && !reflect.DeepEqual(data1, data2) {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Category: "value_data",
				Path:     path,
				Message:  fmt.Sprintf("Value '%s' has different data", name),
				Expected: data1,
				Actual:   data2,
			})
		}
	}

	// Find values only in second validator
	for name := range valueMap2 {
		if _, exists := valueMap1[name]; !exists {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Category: "extra_value",
				Path:     path,
				Message:  fmt.Sprintf("Value '%s' exists in second validator but not in first", name),
			})
		}
	}
}
