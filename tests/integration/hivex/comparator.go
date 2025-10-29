// +build hivex

package integration

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// ComparisonResult holds all differences found during tree comparison
type ComparisonResult struct {
	Mismatches []Mismatch
	NodesCompared int
	ValuesCompared int
}

// Mismatch represents a single difference between hivex and gohivex
type Mismatch struct {
	Path     string
	Category string // "key_name", "child_count", "value_count", "value_name", "value_type", "value_data"
	Message  string
	HivexValue    interface{}
	GohivexValue  interface{}
}

// CompareTreesRecursively compares entire tree structures starting from root nodes
func CompareTreesRecursively(
	hivexHandle *HivexHandle,
	hivexNode int64,
	gohivexReader hive.Reader,
	gohivexNode hive.NodeID,
	path string,
) (*ComparisonResult, error) {
	result := &ComparisonResult{
		Mismatches: []Mismatch{},
	}

	err := compareNodesRecursive(
		hivexHandle, hivexNode,
		gohivexReader, gohivexNode,
		path,
		result,
	)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// compareNodesRecursive recursively compares nodes and their children
func compareNodesRecursive(
	hivexHandle *HivexHandle,
	hivexNode int64,
	gohivexReader hive.Reader,
	gohivexNode hive.NodeID,
	path string,
	result *ComparisonResult,
) error {
	result.NodesCompared++

	// Compare node names
	hivexName, err := hivexHandle.NodeName(hivexNode)
	if err != nil {
		return fmt.Errorf("hivex NodeName failed at %s: %w", path, err)
	}

	gohivexMeta, err := gohivexReader.StatKey(gohivexNode)
	if err != nil {
		return fmt.Errorf("gohivex StatKey failed at %s: %w", path, err)
	}

	if hivexName != gohivexMeta.Name {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Path:     path,
			Category: "key_name",
			Message:  "Key names differ",
			HivexValue:    hivexName,
			GohivexValue:  gohivexMeta.Name,
		})
	}

	// Compare child counts
	hivexChildCount, err := hivexHandle.NodeNrChildren(hivexNode)
	if err != nil {
		return fmt.Errorf("hivex NodeNrChildren failed at %s: %w", path, err)
	}

	if int64(gohivexMeta.SubkeyN) != hivexChildCount {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Path:     path,
			Category: "child_count",
			Message:  fmt.Sprintf("Child count mismatch at %s", path),
			HivexValue:    hivexChildCount,
			GohivexValue:  gohivexMeta.SubkeyN,
		})
	}

	// Compare value counts
	hivexValueCount, err := hivexHandle.NodeNrValues(hivexNode)
	if err != nil {
		return fmt.Errorf("hivex NodeNrValues failed at %s: %w", path, err)
	}

	if int64(gohivexMeta.ValueN) != hivexValueCount {
		result.Mismatches = append(result.Mismatches, Mismatch{
			Path:     path,
			Category: "value_count",
			Message:  fmt.Sprintf("Value count mismatch at %s", path),
			HivexValue:    hivexValueCount,
			GohivexValue:  gohivexMeta.ValueN,
		})
	}

	// Compare values
	if err := compareValues(hivexHandle, hivexNode, gohivexReader, gohivexNode, path, result); err != nil {
		return err
	}

	// Get children from both implementations
	hivexChildren, err := hivexHandle.NodeChildren(hivexNode)
	if err != nil {
		return fmt.Errorf("hivex NodeChildren failed at %s: %w", path, err)
	}

	gohivexChildren, err := gohivexReader.Subkeys(gohivexNode)
	if err != nil {
		return fmt.Errorf("gohivex Subkeys failed at %s: %w", path, err)
	}

	// If child counts differ, we already logged it above, but we can't recurse safely
	if len(hivexChildren) != len(gohivexChildren) {
		// Log warning and continue with minimum count
		minCount := len(hivexChildren)
		if len(gohivexChildren) < minCount {
			minCount = len(gohivexChildren)
		}
		hivexChildren = hivexChildren[:minCount]
		gohivexChildren = gohivexChildren[:minCount]
	}

	// Build child name maps for matching
	hivexChildMap := make(map[string]int64)
	for _, childID := range hivexChildren {
		name, err := hivexHandle.NodeName(childID)
		if err != nil {
			return fmt.Errorf("hivex NodeName failed for child of %s: %w", path, err)
		}
		hivexChildMap[strings.ToLower(name)] = childID
	}

	gohivexChildMap := make(map[string]hive.NodeID)
	for _, childID := range gohivexChildren {
		meta, err := gohivexReader.StatKey(childID)
		if err != nil {
			return fmt.Errorf("gohivex StatKey failed for child of %s: %w", path, err)
		}
		gohivexChildMap[strings.ToLower(meta.Name)] = childID
	}

	// Find children in hivex but not in gohivex
	for name, hivexChildID := range hivexChildMap {
		gohivexChildID, found := gohivexChildMap[name]
		if !found {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     path,
				Category: "missing_child",
				Message:  fmt.Sprintf("Child %q exists in hivex but not in gohivex", name),
				HivexValue:    name,
				GohivexValue:  nil,
			})
			continue
		}

		// Recurse into matching children
		childPath := path
		if path == "\\" {
			childPath = "\\" + name
		} else {
			childPath = path + "\\" + name
		}

		if err := compareNodesRecursive(
			hivexHandle, hivexChildID,
			gohivexReader, gohivexChildID,
			childPath,
			result,
		); err != nil {
			return err
		}
	}

	// Find children in gohivex but not in hivex
	for name := range gohivexChildMap {
		if _, found := hivexChildMap[name]; !found {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     path,
				Category: "extra_child",
				Message:  fmt.Sprintf("Child %q exists in gohivex but not in hivex", name),
				HivexValue:    nil,
				GohivexValue:  name,
			})
		}
	}

	return nil
}

// compareValues compares all values at a given node
func compareValues(
	hivexHandle *HivexHandle,
	hivexNode int64,
	gohivexReader hive.Reader,
	gohivexNode hive.NodeID,
	path string,
	result *ComparisonResult,
) error {
	// Get values from both implementations
	hivexValues, err := hivexHandle.NodeValues(hivexNode)
	if err != nil {
		return fmt.Errorf("hivex NodeValues failed at %s: %w", path, err)
	}

	gohivexValues, err := gohivexReader.Values(gohivexNode)
	if err != nil {
		return fmt.Errorf("gohivex Values failed at %s: %w", path, err)
	}

	// Build value name maps
	hivexValueMap := make(map[string]int64)
	for _, valID := range hivexValues {
		name, err := hivexHandle.ValueKey(valID)
		if err != nil {
			return fmt.Errorf("hivex ValueKey failed at %s: %w", path, err)
		}
		hivexValueMap[strings.ToLower(name)] = valID
	}

	gohivexValueMap := make(map[string]hive.ValueID)
	for _, valID := range gohivexValues {
		meta, err := gohivexReader.StatValue(valID)
		if err != nil {
			return fmt.Errorf("gohivex StatValue failed at %s: %w", path, err)
		}
		gohivexValueMap[strings.ToLower(meta.Name)] = valID
	}

	// Compare values present in both
	for name, hivexValID := range hivexValueMap {
		result.ValuesCompared++

		gohivexValID, found := gohivexValueMap[name]
		if !found {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     path,
				Category: "missing_value",
				Message:  fmt.Sprintf("Value %q exists in hivex but not in gohivex at %s", name, path),
				HivexValue:    name,
				GohivexValue:  nil,
			})
			continue
		}

		// Compare value type
		hivexType, err := hivexHandle.ValueType(hivexValID)
		if err != nil {
			return fmt.Errorf("hivex ValueType failed for %s\\%s: %w", path, name, err)
		}

		gohivexMeta, err := gohivexReader.StatValue(gohivexValID)
		if err != nil {
			return fmt.Errorf("gohivex StatValue failed for %s\\%s: %w", path, name, err)
		}

		// Compare types as int32 to handle unsigned/signed representation differences.
		// This is safe and platform-independent because:
		//   - hivexType is int64 (from bindings.ValueType which is int32, cast to int64)
		//   - gohivexMeta.Type is hive.RegType which is uint32
		//   - Both int32 and uint32 are fixed-size types (always 32 bits on all platforms)
		//   - Casting to int32 reinterprets the bit pattern (e.g., 0xFFFF0019 = -65511)
		//   - This matches how hivex formats unknown types (as signed int32 values)
		if int32(hivexType) != int32(gohivexMeta.Type) {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     fmt.Sprintf("%s\\[%s]", path, name),
				Category: "value_type",
				Message:  fmt.Sprintf("Value type mismatch for %q", name),
				HivexValue:    HivexValueTypeName(hivexType),
				GohivexValue:  gohivexMeta.Type.String(),
			})
		}

		// Compare value data
		_, hivexData, err := hivexHandle.ValueValue(hivexValID)
		if err != nil {
			return fmt.Errorf("hivex ValueValue failed for %s\\%s: %w", path, name, err)
		}

		gohivexData, err := gohivexReader.ValueBytes(gohivexValID, hive.ReadOptions{CopyData: true})
		if err != nil {
			return fmt.Errorf("gohivex ValueBytes failed for %s\\%s: %w", path, name, err)
		}

		if !bytes.Equal(hivexData, gohivexData) {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     fmt.Sprintf("%s\\[%s]", path, name),
				Category: "value_data",
				Message:  fmt.Sprintf("Value data mismatch for %q (hivex: %d bytes, gohivex: %d bytes)", name, len(hivexData), len(gohivexData)),
				HivexValue:    fmt.Sprintf("%d bytes", len(hivexData)),
				GohivexValue:  fmt.Sprintf("%d bytes", len(gohivexData)),
			})
		}
	}

	// Find values in gohivex but not in hivex
	for name := range gohivexValueMap {
		if _, found := hivexValueMap[name]; !found {
			result.Mismatches = append(result.Mismatches, Mismatch{
				Path:     path,
				Category: "extra_value",
				Message:  fmt.Sprintf("Value %q exists in gohivex but not in hivex at %s", name, path),
				HivexValue:    nil,
				GohivexValue:  name,
			})
		}
	}

	return nil
}

// Summary returns a human-readable summary of the comparison
func (r *ComparisonResult) Summary() string {
	if len(r.Mismatches) == 0 {
		return fmt.Sprintf("✓ Perfect match: %d nodes, %d values compared, 0 differences",
			r.NodesCompared, r.ValuesCompared)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✗ Found %d differences (%d nodes, %d values compared):\n",
		len(r.Mismatches), r.NodesCompared, r.ValuesCompared))

	// Group by category
	categories := make(map[string]int)
	for _, m := range r.Mismatches {
		categories[m.Category]++
	}

	for category, count := range categories {
		sb.WriteString(fmt.Sprintf("  - %s: %d\n", category, count))
	}

	return sb.String()
}
