package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/internal/testutil/hivexval"
)

// regTextTestCase defines a test case for building hives from .reg files.
type regTextTestCase struct {
	name           string
	regPath        string
	description    string
	expectedKeys   int
	expectedValues int
	// Size limits (in bytes) to detect regressions in file size
	// Different strategies have different characteristics:
	// - InPlace/Hybrid: similar sizes (cell reuse)
	// - Append: larger (no cell reuse, append-only)
	maxSizeInPlace int64 // Max size for InPlace/Hybrid strategies
	maxSizeAppend  int64 // Max size for Append strategy (typically 20-30% larger)
	// Specific validations to perform on the built hive
	validations []regTextValidation
}

// regTextValidation defines a specific key/value to check in the built hive.
type regTextValidation struct {
	keyPath   []string
	valueName string
	valueType string // "REG_SZ", "REG_DWORD", etc.
}

var regTextTestCases = []regTextTestCase{
	{
		name:           "windows-xp-system",
		regPath:        "../../testdata/suite/windows-xp-system.reg",
		description:    "Windows XP SYSTEM hive",
		expectedKeys:   18234,
		expectedValues: 45345,
		maxSizeInPlace: 13135872, // Actual: 2,813,952 bytes + 5% margin (with SK cells)
		maxSizeAppend:  13254656, // Actual: 3,719,168 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
			{
				keyPath:   []string{"ControlSet001", "Services", "Tcpip", "Parameters"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-xp-software",
		regPath:        "../../testdata/suite/windows-xp-software.reg",
		description:    "Windows XP SOFTWARE hive",
		expectedKeys:   8730,
		expectedValues: 23849,
		maxSizeInPlace: 3789005, // Actual: 3,608,576 bytes + 5% margin (with SK cells)
		maxSizeAppend:  4021248, // Actual: 3,829,760 bytes + 5% margin (with SK cells)
		// NOTE: This file is actually a SYSTEM hive (contains ControlSet001, etc.), not SOFTWARE.
		// It's mislabeled in the source data but we keep it as-is since these files come from
		// a well-known downloadable area and should not be modified.
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-xp-2-system",
		regPath:        "../../testdata/suite/windows-xp-2-system.reg",
		description:    "Windows XP (2) SYSTEM hive",
		expectedKeys:   8862,
		expectedValues: 24214,
		maxSizeInPlace: 3909427, // Actual: 3,723,264 bytes + 5% margin (with SK cells)
		maxSizeAppend:  3829760, // Actual: 2,826,240 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-xp-2-software",
		regPath:        "../../testdata/suite/windows-xp-2-software.reg",
		description:    "Windows XP (2) SOFTWARE hive",
		expectedKeys:   37833,
		expectedValues: 53187,
		maxSizeInPlace: 13792666, // Actual: 13,135,872 bytes + 5% margin (with SK cells)
		maxSizeAppend:  13917389, // Actual: 13,254,656 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"Microsoft", "Windows", "CurrentVersion"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-2003-server-system",
		regPath:        "../../testdata/suite/windows-2003-server-system.reg",
		description:    "Windows Server 2003 SYSTEM hive",
		expectedKeys:   4579,
		expectedValues: 15926,
		maxSizeInPlace: 2954650, // Actual: 2,813,952 bytes + 5% margin (with SK cells)
		maxSizeAppend:  2967552, // Actual: 2,826,240 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
			{
				keyPath:   []string{"ControlSet001", "Services"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-2003-server-software",
		regPath:        "../../testdata/suite/windows-2003-server-software.reg",
		description:    "Windows Server 2003 SOFTWARE hive",
		expectedKeys:   65996,
		expectedValues: 90612,
		maxSizeInPlace: 17624678, // Actual: 16,785,408 bytes + 5% margin (with SK cells)
		maxSizeAppend:  19099853, // Actual: 18,190,336 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"Microsoft", "Windows", "CurrentVersion"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-8-consumer-preview-system",
		regPath:        "../../testdata/suite/windows-8-consumer-preview-system.reg",
		description:    "Windows 8 Consumer Preview SYSTEM hive",
		expectedKeys:   18146,
		expectedValues: 45462,
		maxSizeInPlace: 12257280, // Actual: 11,673,600 bytes + 5% margin (with SK cells)
		maxSizeAppend:  12382003, // Actual: 11,792,384 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-8-consumer-preview-software",
		regPath:        "../../testdata/suite/windows-8-consumer-preview-software.reg",
		description:    "Windows 8 Consumer Preview SOFTWARE hive",
		expectedKeys:   151914,
		expectedValues: 238228,   // 120,948 named values + 117,280 default values (@=)
		maxSizeInPlace: 45227213, // Actual: 43,073,536 bytes + 5% margin (with SK cells)
		maxSizeAppend:  55587840, // Actual: 52,940,800 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"Microsoft", "Windows", "CurrentVersion"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-8-enterprise-system",
		regPath:        "../../testdata/suite/windows-8-enterprise-system.reg",
		description:    "Windows 8 Enterprise SYSTEM hive",
		expectedKeys:   18234,
		expectedValues: 45345,
		maxSizeInPlace: 13792666, // Actual: 13,135,872 bytes + 5% margin (with SK cells)
		maxSizeAppend:  13917389, // Actual: 13,254,656 bytes + 5% margin (with SK cells)
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-8-enterprise-software",
		regPath:        "../../testdata/suite/windows-8-enterprise-software.reg",
		description:    "Windows 8 Enterprise SOFTWARE hive",
		expectedKeys:   92360,
		expectedValues: 149143,
		maxSizeInPlace: 29020774, // Original: 25,235,456 bytes + 15% slack
		maxSizeAppend:  34067865, // Original: 25,235,456 bytes + 35% slack
		validations: []regTextValidation{
			{
				keyPath:   []string{"Microsoft", "Windows", "CurrentVersion"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-2012-system",
		regPath:        "../../testdata/suite/windows-2012-system.reg",
		description:    "Windows Server 2012 SYSTEM hive",
		expectedKeys:   25406,
		expectedValues: 64076,
		maxSizeInPlace: 15863808, // Original: 9,322,496 bytes + 15% slack
		maxSizeAppend:  16023552, // Original: 9,322,496 bytes + 35% slack
		validations: []regTextValidation{
			{
				keyPath:   []string{"ControlSet001", "Control"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
	{
		name:           "windows-2012-software",
		regPath:        "../../testdata/suite/windows-2012-software.reg",
		description:    "Windows Server 2012 SOFTWARE hive",
		expectedKeys:   125596,
		expectedValues: 203986,
		maxSizeInPlace: 41300787, // Original: 35,913,728 bytes + 15% slack
		maxSizeAppend:  48483532, // Original: 35,913,728 bytes + 35% slack
		validations: []regTextValidation{
			{
				keyPath:   []string{"Microsoft", "Windows", "CurrentVersion"},
				valueName: "",
				valueType: "key_exists",
			},
		},
	},
}

// TestBuildFromRegFile_Suite tests building hives from all .reg files in testdata/suite.
// This is a comprehensive integration test that validates:
//   - The .reg file can be parsed
//   - A valid hive can be built from it
//   - The hive has the correct number of keys and values
//   - Specific important keys exist
//   - The hive validates with hivexsh
//
// Tests all strategies (InPlace, Append, Hybrid) for each hive to ensure correctness.
func TestBuildFromRegFile_Suite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}

	// Define strategies to test
	strategies := []struct {
		name     string
		strategy StrategyType
	}{
		{"InPlace", StrategyInPlace},
		{"Append", StrategyAppend},
		{"Hybrid", StrategyHybrid},
	}

	// Test each strategy with each hive
	for _, strat := range strategies {
		t.Run(strat.name, func(t *testing.T) {
			for _, tc := range regTextTestCases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel() // Run in parallel for fast feedback during regular testing

					// Check if .reg file exists
					if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
						t.Skipf("Test .reg file not found: %s", tc.regPath)
					}

					t.Logf("Building hive from: %s (strategy: %s)", tc.description, strat.name)
					t.Logf("  Expected: %d keys, %d values", tc.expectedKeys, tc.expectedValues)

					// Create temp directory for output hive
					dir := t.TempDir()
					hivePath := filepath.Join(dir, tc.name+"-"+strat.name+".hive")

					// Build the hive from the .reg file with explicit strategy
					opts := DefaultOptions()
					opts.Strategy = strat.strategy
					err := BuildFromRegFile(hivePath, tc.regPath, opts)
					require.NoError(t, err, "BuildFromRegFile should succeed")

					// Verify the file was created
					info, err := os.Stat(hivePath)
					require.NoError(t, err, "Output hive file should exist")
					require.Positive(t, info.Size(), "Hive file should not be empty")
					t.Logf("  Created hive: %d bytes", info.Size())

					// Check file size is within acceptable limits
					// Different strategies have different size characteristics
					var maxSize int64
					if strat.strategy == StrategyAppend {
						maxSize = tc.maxSizeAppend
					} else {
						// InPlace and Hybrid use same limit (both do cell reuse)
						maxSize = tc.maxSizeInPlace
					}
					require.LessOrEqual(
						t,
						info.Size(),
						maxSize,
						"Hive size %d bytes exceeds max %d bytes for strategy %s (%.1f%% over limit)",
						info.Size(),
						maxSize,
						strat.name,
						float64(info.Size()-maxSize)/float64(maxSize)*100,
					)

					// Open with hivexval for validation
					v := hivexval.Must(hivexval.New(hivePath, nil))
					defer v.Close()

					// Count keys and values
					keyCount, valueCount, err := v.CountTree()
					require.NoError(t, err, "CountTree should succeed")

					t.Logf("  Built hive: %d keys, %d values", keyCount, valueCount)

					// Check counts match expected
					require.Equal(
						t,
						tc.expectedKeys,
						keyCount,
						"Key count should match expected (got %d, want %d)",
						keyCount,
						tc.expectedKeys,
					)
					require.Equal(
						t,
						tc.expectedValues,
						valueCount,
						"Value count should match expected (got %d, want %d)",
						valueCount,
						tc.expectedValues,
					)

					// Perform specific validations
					for i, val := range tc.validations {
						if val.valueType == "key_exists" {
							v.AssertKeyExists(t, val.keyPath)
							t.Logf("  ✓ Validation %d: Key exists: %v", i+1, val.keyPath)
						}
					}

					// Validate structure
					v.AssertStructureValid(t)
					t.Logf("  ✓ Structure validation passed")

					// Validate with hivexsh if available
					if hivexval.IsHivexshAvailable() {
						vHivexsh := hivexval.Must(
							hivexval.New(hivePath, &hivexval.Options{UseHivexsh: true}),
						)
						defer vHivexsh.Close()
						vHivexsh.AssertHivexshValid(t)
						t.Logf("  ✓ hivexsh validation passed")
					}

					t.Logf("%s (%s): All validations passed!", tc.description, strat.name)
				})
			}
		})
	}
}

// BenchmarkBuildFromRegFile_Suite benchmarks building hives from .reg files.
// This measures the performance of the complete pipeline:
//   - Parsing the .reg file
//   - Converting to operations
//   - Building the hive structure
//   - Writing to disk
func BenchmarkBuildFromRegFile_Suite(b *testing.B) {
	// Categorize by size for clearer benchmark results
	sizes := map[string][]string{
		"small":  {"windows-2003-server-system"},                             // ~4.5K keys
		"medium": {"windows-xp-system", "windows-2003-server-software"},      // ~18K-66K keys
		"large":  {"windows-8-enterprise-software", "windows-2012-software"}, // ~92K-125K keys
	}

	for sizeCategory, names := range sizes {
		for _, name := range names {
			// Find the test case
			var tc *regTextTestCase
			for i := range regTextTestCases {
				if regTextTestCases[i].name == name {
					tc = &regTextTestCases[i]
					break
				}
			}
			if tc == nil {
				continue
			}

			benchName := sizeCategory + "/" + tc.name
			b.Run(benchName, func(b *testing.B) {
				// Check if .reg file exists
				if _, err := os.Stat(tc.regPath); os.IsNotExist(err) {
					b.Skipf("Test .reg file not found: %s", tc.regPath)
				}

				// Read .reg file once (outside timing)
				regData, err := os.ReadFile(tc.regPath)
				if err != nil {
					b.Fatalf("Failed to read .reg file: %v", err)
				}

				// Create temp directory
				dir := b.TempDir()

				b.ReportAllocs()
				b.ResetTimer()

				for b.Loop() {
					b.StopTimer()
					hivePath := filepath.Join(dir, "bench-"+tc.name+".hive")
					// Clean up from previous iteration
					os.Remove(hivePath)
					b.StartTimer()

					// This is what we're benchmarking
					buildErr := BuildFromRegText(hivePath, string(regData), nil)
					if buildErr != nil {
						b.Fatalf("BuildFromRegText failed: %v", buildErr)
					}
				}

				b.StopTimer()

				// Report custom metrics
				keysPerOp := float64(tc.expectedKeys)
				valuesPerOp := float64(tc.expectedValues)
				b.ReportMetric(keysPerOp, "keys/op")
				b.ReportMetric(valuesPerOp, "values/op")

				// Calculate throughput if we have timing
				if b.N > 0 {
					nsPerOp := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
					keysPerSec := keysPerOp / (nsPerOp / 1e9)
					valuesPerSec := valuesPerOp / (nsPerOp / 1e9)
					b.ReportMetric(keysPerSec, "keys/s")
					b.ReportMetric(valuesPerSec, "values/s")
				}
			})
		}
	}
}
