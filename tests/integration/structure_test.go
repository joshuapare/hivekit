package integration

import (
	"fmt"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// GohivexKey represents a key from gohivex with its values.
type GohivexKey struct {
	Path   string
	Values []GohivexValue
}

// GohivexValue represents a value from gohivex.
type GohivexValue struct {
	Name string
	Type uint32
}

// TestRegStructuralIntegrity validates FULL structural integrity against .reg files
// This goes beyond counts and validates:
// - Every key path matches
// - Every value name matches
// - Value types match.
func TestRegStructuralIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	for _, tc := range suiteHives {
		t.Run(tc.name, func(t *testing.T) {
			// Check if files exist
			if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
				t.Skipf("Hive file not found: %s", tc.hivePath)
			}
			if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
				t.Skipf(".reg file not found: %s", tc.regPath)
			}

			// Parse .reg file
			regFile, err := os.Open(tc.regPath)
			if err != nil {
				t.Fatalf("Failed to open .reg file: %v", err)
			}
			defer regFile.Close()

			regStats, err := regtext.ParseRegFile(regFile)
			if err != nil {
				t.Fatalf("Failed to parse .reg file: %v", err)
			}

			t.Logf("Loaded .reg structure: %d keys, %d values", len(regStats.Structure), regStats.ValueCount)

			// Open hive with gohivex
			data, err := os.ReadFile(tc.hivePath)
			if err != nil {
				t.Fatalf("Failed to read hive: %v", err)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Failed to open hive: %v", err)
			}
			defer r.Close()

			// Build gohivex structure
			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Failed to get root: %v", err)
			}

			gohivexStructure := buildGohivexStructure(t, r, rootID, "")
			t.Logf("Built gohivex structure: %d keys", len(gohivexStructure))

			// Compare structures
			errors := compareStructures(regStats.Structure, gohivexStructure)
			if len(errors) > 0 {
				// Show first 10 errors
				maxErrors := 10
				if len(errors) < maxErrors {
					maxErrors = len(errors)
				}
				for i := range maxErrors {
					t.Errorf("Structure mismatch: %s", errors[i])
				}
				if len(errors) > maxErrors {
					t.Errorf("... and %d more errors", len(errors)-maxErrors)
				}
			} else {
				t.Logf("Full structural integrity validated!")
			}
		})
	}
}

// buildGohivexStructure walks the gohivex tree and builds a comparable structure.
func buildGohivexStructure(t *testing.T, r hive.Reader, nodeID hive.NodeID, parentPath string) []*GohivexKey {
	result := make([]*GohivexKey, 0)

	// Get node metadata
	meta, err := r.StatKey(nodeID)
	if err != nil {
		t.Logf("Warning: failed to stat key: %v", err)
		return result
	}

	// Build path
	var path string
	switch parentPath {
	case "":
		// Root node
		path = "\\"
	case "\\":
		// Direct child of root
		path = "\\" + meta.Name
	default:
		// Deeper nesting
		path = parentPath + "\\" + meta.Name
	}

	// Get values for this key
	gohivexValues := make([]GohivexValue, 0)
	values, err := r.Values(nodeID)
	if err == nil {
		for _, valueID := range values {
			valueMeta, valErr := r.StatValue(valueID)
			if valErr == nil {
				gohivexValues = append(gohivexValues, GohivexValue{
					Name: valueMeta.Name,
					Type: uint32(valueMeta.Type),
				})
			}
		}
	}

	// Add this key to result
	result = append(result, &GohivexKey{
		Path:   path,
		Values: gohivexValues,
	})

	// Recursively process children
	children, err := r.Subkeys(nodeID)
	if err == nil {
		for _, childID := range children {
			childStructure := buildGohivexStructure(t, r, childID, path)
			result = append(result, childStructure...)
		}
	}

	return result
}

// compareStructures compares .reg structure with gohivex structure.
func compareStructures(regStructure []*regtext.RegKey, gohivexStructure []*GohivexKey) []string {
	errors := make([]string, 0)

	// Build maps for efficient lookup
	regMap := make(map[string]*regtext.RegKey)
	for _, key := range regStructure {
		regMap[key.Path] = key
	}

	gohivexMap := make(map[string]*GohivexKey)
	for _, key := range gohivexStructure {
		gohivexMap[key.Path] = key
	}

	// Check all .reg keys exist in gohivex
	for _, regKey := range regStructure {
		gohivexKey, exists := gohivexMap[regKey.Path]
		if !exists {
			errors = append(errors, fmt.Sprintf("Key missing in gohivex: %q", regKey.Path))
			continue
		}

		// Compare values at this key
		valueErrors := compareKeyValues(regKey, gohivexKey)
		errors = append(errors, valueErrors...)
	}

	// Check for extra keys in gohivex (shouldn't happen, but verify)
	for _, gohivexKey := range gohivexStructure {
		if _, exists := regMap[gohivexKey.Path]; !exists {
			errors = append(errors, fmt.Sprintf("Extra key in gohivex: %q", gohivexKey.Path))
		}
	}

	return errors
}

// compareKeyValues compares values at a single key.
func compareKeyValues(regKey *regtext.RegKey, gohivexKey *GohivexKey) []string {
	errors := make([]string, 0)

	// Count check
	if len(regKey.Values) != len(gohivexKey.Values) {
		errors = append(errors, fmt.Sprintf("Value count mismatch at %q: .reg=%d, gohivex=%d",
			regKey.Path, len(regKey.Values), len(gohivexKey.Values)))
		return errors // Don't compare individual values if counts don't match
	}

	// Build value name maps
	regValueMap := make(map[string]*regtext.RegValue)
	for _, val := range regKey.Values {
		regValueMap[val.Name] = val
	}

	gohivexValueMap := make(map[string]GohivexValue)
	for _, val := range gohivexKey.Values {
		gohivexValueMap[val.Name] = val
	}

	// Check all .reg values exist in gohivex
	for _, regValue := range regKey.Values {
		gohivexValue, exists := gohivexValueMap[regValue.Name]
		if !exists {
			valueName := regValue.Name
			if valueName == "" {
				valueName = "(default)"
			}
			errors = append(errors, fmt.Sprintf("Value %q missing in gohivex at key %q",
				valueName, regKey.Path))
			continue
		}

		// Compare value types
		expectedType := regTypeToHiveType(regValue.Type)
		if expectedType != 0 && gohivexValue.Type != expectedType {
			errors = append(
				errors,
				fmt.Sprintf("Value type mismatch at %q, value %q: expected %s (type %d), got type %d",
					regKey.Path, regValue.Name, regValue.Type, expectedType, gohivexValue.Type),
			)
		}
	}

	// Check for extra values in gohivex
	for _, gohivexValue := range gohivexKey.Values {
		if _, exists := regValueMap[gohivexValue.Name]; !exists {
			errors = append(errors, fmt.Sprintf("Extra value %q in gohivex at key %q",
				gohivexValue.Name, regKey.Path))
		}
	}

	return errors
}

// regTypeToHiveType converts .reg type string to Windows registry type constant.
func regTypeToHiveType(regType string) uint32 {
	switch regType {
	case "string":
		return 1 // REG_SZ
	case "dword":
		return 4 // REG_DWORD
	case "binary", "hex":
		return 3 // REG_BINARY
	case "hex(1)":
		return 1 // REG_SZ
	case "hex(2)":
		return 2 // REG_EXPAND_SZ
	case "hex(3)":
		return 3 // REG_BINARY
	case "hex(4)":
		return 4 // REG_DWORD
	case "hex(7)":
		return 7 // REG_MULTI_SZ
	case "hex(a)", "hex(10)":
		return 0xa // REG_RESOURCE_LIST
	case "hex(b)", "hex(11)":
		return 0xb // REG_RESOURCE_REQUIREMENTS_LIST
	default:
		return 0 // Unknown type - don't validate
	}
}

// TestRegStructuralIntegritySummary provides a summary of structural validation.
func TestRegStructuralIntegritySummary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
	perfectMatches := 0
	totalHives := 0
	totalErrors := 0

	for _, tc := range suiteHives {
		// Check if files exist
		if _, err := os.Stat(tc.hivePath); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
			continue
		}

		totalHives++

		// Parse .reg file
		regFile, err := os.Open(tc.regPath)
		if err != nil {
			continue
		}
		regStats, err := regtext.ParseRegFile(regFile)
		regFile.Close()
		if err != nil {
			continue
		}

		// Open hive
		data, err := os.ReadFile(tc.hivePath)
		if err != nil {
			continue
		}
		r, err := reader.OpenBytes(data, hive.OpenOptions{})
		if err != nil {
			continue
		}
		rootID, _ := r.Root()
		gohivexStructure := buildGohivexStructure(t, r, rootID, "")
		r.Close()

		// Compare
		errors := compareStructures(regStats.Structure, gohivexStructure)
		if len(errors) == 0 {
			perfectMatches++
		} else {
			totalErrors += len(errors)
		}
	}

	t.Logf("")
	t.Logf("=== .reg Structural Integrity Summary ===")
	t.Logf(
		"Perfect structural matches: %d/%d (%.1f%%)",
		perfectMatches,
		totalHives,
		float64(perfectMatches)/float64(totalHives)*100,
	)
	if totalErrors > 0 {
		t.Logf("Total structural errors found: %d", totalErrors)
	}
	t.Logf("")

	if perfectMatches == totalHives {
		t.Logf("ALL hives have perfect structural integrity!")
	}
}
