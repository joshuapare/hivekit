package integration

import (
	"os"
	"sort"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/regtext"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestRegDebugMismatches provides detailed analysis of all value count mismatches
func TestRegDebugMismatches(t *testing.T) {
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

			gohivexStructure := buildGohivexStructureDebug(t, r, rootID, "")

			// Build maps
			regMap := make(map[string]*regtext.RegKey)
			for _, key := range regStats.Structure {
				regMap[key.Path] = key
			}

			gohivexMap := make(map[string]*GohivexKey)
			for _, key := range gohivexStructure {
				gohivexMap[key.Path] = key
			}

			// Find mismatches
			mismatches := 0
			for _, regKey := range regStats.Structure {
				gohivexKey, exists := gohivexMap[regKey.Path]
				if !exists {
					continue
				}

				if len(regKey.Values) != len(gohivexKey.Values) {
					mismatches++
					t.Logf("")
					t.Logf("=== Mismatch #%d at key: %s ===", mismatches, regKey.Path)
					t.Logf("  .reg values: %d, gohivex values: %d", len(regKey.Values), len(gohivexKey.Values))

					// Show .reg values
					t.Logf("  .reg value names:")
					regNames := make([]string, 0, len(regKey.Values))
					for _, val := range regKey.Values {
						name := val.Name
						if name == "" {
							name = "(default)"
						}
						regNames = append(regNames, name)
					}
					sort.Strings(regNames)
					for _, name := range regNames {
						t.Logf("    - %q", name)
					}

					// Show gohivex values
					t.Logf("  gohivex value names:")
					gohivexNames := make([]string, 0, len(gohivexKey.Values))
					for _, val := range gohivexKey.Values {
						name := val.Name
						if name == "" {
							name = "(default)"
						}
						gohivexNames = append(gohivexNames, name)
					}
					sort.Strings(gohivexNames)
					for _, name := range gohivexNames {
						t.Logf("    - %q", name)
					}

					// Find differences
					regSet := make(map[string]bool)
					for _, val := range regKey.Values {
						regSet[val.Name] = true
					}

					gohivexSet := make(map[string]bool)
					for _, val := range gohivexKey.Values {
						gohivexSet[val.Name] = true
					}

					// Values in .reg but not in gohivex
					missing := make([]string, 0)
					for name := range regSet {
						if !gohivexSet[name] {
							displayName := name
							if displayName == "" {
								displayName = "(default)"
							}
							missing = append(missing, displayName)
						}
					}
					if len(missing) > 0 {
						sort.Strings(missing)
						t.Logf("  Missing in gohivex:")
						for _, name := range missing {
							t.Logf("    - %q", name)
						}
					}

					// Values in gohivex but not in .reg
					extra := make([]string, 0)
					for name := range gohivexSet {
						if !regSet[name] {
							displayName := name
							if displayName == "" {
								displayName = "(default)"
							}
							extra = append(extra, displayName)
						}
					}
					if len(extra) > 0 {
						sort.Strings(extra)
						t.Logf("  Extra in gohivex:")
						for _, name := range extra {
							t.Logf("    - %q", name)
						}
					}
				}
			}

			if mismatches == 0 {
				t.Logf("✅ No value count mismatches found!")
			} else {
				t.Logf("")
				t.Logf("Total mismatches: %d", mismatches)
			}
		})
	}
}

// buildGohivexStructureDebug is like buildGohivexStructure but logs value read errors
func buildGohivexStructureDebug(t *testing.T, r hive.Reader, nodeID hive.NodeID, parentPath string) []*GohivexKey {
	result := make([]*GohivexKey, 0)

	// Get node metadata
	meta, err := r.StatKey(nodeID)
	if err != nil {
		t.Logf("Warning: failed to stat key: %v", err)
		return result
	}

	// Build path
	var path string
	if parentPath == "" {
		// Root node
		path = "\\"
	} else if parentPath == "\\" {
		// Direct child of root
		path = "\\" + meta.Name
	} else {
		// Deeper nesting
		path = parentPath + "\\" + meta.Name
	}

	// Get values for this key
	gohivexValues := make([]GohivexValue, 0)
	values, err := r.Values(nodeID)
	if err == nil {
		for _, valueID := range values {
			valueMeta, err := r.StatValue(valueID)
			if err != nil {
				// LOG THE ERROR with details
				t.Logf("ERROR reading value at key %q: %v (ValueID=%d)", path, err, valueID)
				continue
			}
			// Log if size is suspicious (might help identify big data records)
			if valueMeta.Size > 16384 {
				t.Logf("Large value at key %q: name=%q, size=%d bytes", path, valueMeta.Name, valueMeta.Size)
			}
			gohivexValues = append(gohivexValues, GohivexValue{
				Name: valueMeta.Name,
				Type: uint32(valueMeta.Type),
			})
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
			childStructure := buildGohivexStructureDebug(t, r, childID, path)
			result = append(result, childStructure...)
		}
	}

	return result
}
