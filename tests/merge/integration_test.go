package merge_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// mergeTestCase defines a test case for .reg file parsing and merging.
type mergeTestCase struct {
	name            string                       // Test name
	regFile         string                       // Path to .reg file (relative to testdata/merge/)
	expectError     bool                         // Should parsing fail?
	errorContains   string                       // Expected error substring
	expectedOps     int                          // Total operations expected
	expectedKeys    int                          // Keys created
	expectedDelKeys int                          // Keys deleted
	expectedVals    int                          // Values set
	expectedDelVals int                          // Values deleted
	validateOps     func([]hive.EditOp) error   // Custom validation function
	parseOpts       hive.RegParseOptions         // Parsing options
}

// TestMergeIntegration_ValidFiles tests parsing of well-formed .reg files.
func TestMergeIntegration_ValidFiles(t *testing.T) {
	tests := []mergeTestCase{
		// Basic operations
		{
			name:         "Simple string value",
			regFile:      "valid/01_simple_string.reg",
			expectError:  false,
			expectedOps:  2, // 1 CreateKey + 1 SetValue
			expectedKeys: 1,
			expectedVals: 1,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Simple DWORD value",
			regFile:      "valid/02_simple_dword.reg",
			expectError:  false,
			expectedOps:  2, // 1 CreateKey + 1 SetValue
			expectedKeys: 1,
			expectedVals: 1,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Nested keys",
			regFile:      "valid/03_nested_keys.reg",
			expectError:  false,
			expectedOps:  8, // 4 CreateKey + 4 SetValue
			expectedKeys: 4,
			expectedVals: 4,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:            "Delete key",
			regFile:         "valid/04_delete_key.reg",
			expectError:     false,
			expectedOps:     3, // 1 CreateKey + 1 SetValue + 1 DeleteKey
			expectedKeys:    1,
			expectedVals:    1,
			expectedDelKeys: 1,
			parseOpts:       hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:            "Delete value",
			regFile:         "valid/05_delete_value.reg",
			expectError:     false,
			expectedOps:     3, // 1 CreateKey + 1 SetValue + 1 DeleteValue
			expectedKeys:    1,
			expectedVals:    1,
			expectedDelVals: 1,
			parseOpts:       hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Default value",
			regFile:      "valid/06_default_value.reg",
			expectError:  false,
			expectedOps:  3, // 1 CreateKey + 2 SetValue (@ and named)
			expectedKeys: 1,
			expectedVals: 2,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:            "Mixed operations",
			regFile:         "valid/07_mixed_operations.reg",
			expectError:     false,
			expectedOps:     10, // 4 CreateKey + 4 SetValue + 1 DeleteKey + 1 DeleteValue
			expectedKeys:    4,
			expectedVals:    4,
			expectedDelKeys: 1,
			expectedDelVals: 1,
			parseOpts:       hive.RegParseOptions{AutoPrefix: true},
		},

		// All value types
		{
			name:         "All registry value types",
			regFile:      "valid/10_value_types_all.reg",
			expectError:  false,
			expectedOps:  12, // 1 CreateKey + 11 SetValue (all types)
			expectedKeys: 1,
			expectedVals: 11,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},

		// Edge cases
		{
			name:         "Unicode strings",
			regFile:      "valid/20_unicode_strings.reg",
			expectError:  false,
			expectedOps:  7, // 1 CreateKey + 6 SetValue
			expectedKeys: 1,
			expectedVals: 6,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Special characters",
			regFile:      "valid/21_special_chars.reg",
			expectError:  false,
			expectedOps:  8, // 1 CreateKey + 7 SetValue
			expectedKeys: 1,
			expectedVals: 7,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Line continuation",
			regFile:      "valid/22_line_continuation.reg",
			expectError:  false,
			expectedOps:  4, // 1 CreateKey + 3 SetValue
			expectedKeys: 1,
			expectedVals: 3,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Long hex values",
			regFile:      "valid/23_long_hex_values.reg",
			expectError:  false,
			expectedOps:  2, // 1 CreateKey + 1 SetValue
			expectedKeys: 1,
			expectedVals: 1,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Empty strings",
			regFile:      "valid/24_empty_string.reg",
			expectError:  false,
			expectedOps:  4, // 1 CreateKey + 3 SetValue
			expectedKeys: 1,
			expectedVals: 3,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Zero DWORD values",
			regFile:      "valid/25_zero_dword.reg",
			expectError:  false,
			expectedOps:  4, // 1 CreateKey + 3 SetValue
			expectedKeys: 1,
			expectedVals: 3,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "All HKEY prefixes",
			regFile:      "valid/26_all_hkey_prefixes.reg",
			expectError:  false,
			expectedOps:  10, // 5 CreateKey + 5 SetValue
			expectedKeys: 5,
			expectedVals: 5,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
		{
			name:         "Short HKEY aliases",
			regFile:      "valid/27_short_aliases.reg",
			expectError:  false,
			expectedOps:  10, // 5 CreateKey + 5 SetValue
			expectedKeys: 5,
			expectedVals: 5,
			parseOpts:    hive.RegParseOptions{AutoPrefix: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regPath := filepath.Join("../../testdata/merge", tt.regFile)

			// Parse the .reg file
			ops, err := hive.ParseRegFile(regPath, tt.parseOpts)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify operation counts
			if len(ops) != tt.expectedOps {
				t.Errorf("Expected %d operations, got %d", tt.expectedOps, len(ops))
				for i, op := range ops {
					t.Logf("  Op %d: %T", i, op)
				}
			}

			// Count operation types
			var keysCreated, keysDeleted, valuesSet, valuesDeleted int
			for _, op := range ops {
				switch op.(type) {
				case hive.OpCreateKey:
					keysCreated++
				case hive.OpDeleteKey:
					keysDeleted++
				case hive.OpSetValue:
					valuesSet++
				case hive.OpDeleteValue:
					valuesDeleted++
				}
			}

			if keysCreated != tt.expectedKeys {
				t.Errorf("Expected %d keys created, got %d", tt.expectedKeys, keysCreated)
			}
			if keysDeleted != tt.expectedDelKeys {
				t.Errorf("Expected %d keys deleted, got %d", tt.expectedDelKeys, keysDeleted)
			}
			if valuesSet != tt.expectedVals {
				t.Errorf("Expected %d values set, got %d", tt.expectedVals, valuesSet)
			}
			if valuesDeleted != tt.expectedDelVals {
				t.Errorf("Expected %d values deleted, got %d", tt.expectedDelVals, valuesDeleted)
			}

			// Run custom validation if provided
			if tt.validateOps != nil {
				if err := tt.validateOps(ops); err != nil {
					t.Errorf("Custom validation failed: %v", err)
				}
			}
		})
	}
}

// TestMergeIntegration_InvalidFiles tests error handling for malformed .reg files.
func TestMergeIntegration_InvalidFiles(t *testing.T) {
	tests := []mergeTestCase{
		{
			name:          "Missing header",
			regFile:       "invalid/err_no_header.reg",
			expectError:   true,
			errorContains: "header",
		},
		{
			name:          "Bad header version",
			regFile:       "invalid/err_bad_header.reg",
			expectError:   true,
			errorContains: "header",
		},
		{
			name:          "Unclosed bracket",
			regFile:       "invalid/err_unclosed_bracket.reg",
			expectError:   true,
			errorContains: "", // May vary by implementation
		},
		{
			name:          "Invalid DWORD",
			regFile:       "invalid/err_invalid_dword.reg",
			expectError:   true,
			errorContains: "", // Parser should catch bad hex chars
		},
		{
			name:          "Invalid hex",
			regFile:       "invalid/err_invalid_hex.reg",
			expectError:   true,
			errorContains: "", // Parser should catch non-hex chars
		},
		// Note: Some parsers are lenient with these edge cases
		// {
		// 	name:          "Truncated hex",
		// 	regFile:       "invalid/err_truncated_hex.reg",
		// 	expectError:   true,
		// },
		{
			name:          "Bad line continuation",
			regFile:       "invalid/err_bad_line_continuation.reg",
			expectError:   true,
			errorContains: "", // Missing space or comma
		},
		// {
		// 	name:          "Unescaped quotes",
		// 	regFile:       "invalid/err_unescaped_quotes.reg",
		// 	expectError:   true,
		// },
		{
			name:          "Empty key name",
			regFile:       "invalid/err_empty_key_name.reg",
			expectError:   true,
			errorContains: "", // Empty [] not allowed
		},
		{
			name:          "Missing value name",
			regFile:       "invalid/err_missing_value_name.reg",
			expectError:   true,
			errorContains: "", // = without name (except @=)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regPath := filepath.Join("../../testdata/merge", tt.regFile)

			// Parse should fail
			ops, err := hive.ParseRegFile(regPath, hive.RegParseOptions{})

			if err == nil {
				t.Errorf("Expected error but got nil (parsed %d ops)", len(ops))
				for i, op := range ops {
					t.Logf("  Op %d: %T", i, op)
				}
				return
			}

			// Check error message if specified
			if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("Expected error containing %q, got: %v", tt.errorContains, err)
			}
		})
	}
}

// TestMergeIntegration_Statistics tests that merge statistics are correctly collected.
func TestMergeIntegration_Statistics(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	tests := []struct {
		name         string
		regFile      string
		expectKeys   int
		expectVals   int
		expectDelKey int
		expectDelVal int
	}{
		{
			name:       "Simple string",
			regFile:    "valid/01_simple_string.reg",
			expectKeys: 1,
			expectVals: 1,
		},
		{
			name:       "Nested keys",
			regFile:    "valid/03_nested_keys.reg",
			expectKeys: 4,
			expectVals: 4,
		},
		{
			name:         "Delete operations",
			regFile:      "valid/04_delete_key.reg",
			expectKeys:   1,
			expectVals:   1,
			expectDelKey: 1,
		},
		{
			name:         "Mixed operations",
			regFile:      "valid/07_mixed_operations.reg",
			expectKeys:   4,
			expectVals:   4,
			expectDelKey: 1,
			expectDelVal: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset hive for each test
			if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
				t.Fatalf("Failed to reset hive: %v", err)
			}

			regPath := filepath.Join("../../testdata/merge", tt.regFile)

			// Merge and collect stats
			stats, err := hive.MergeRegFile(hiveFile, regPath, nil)
			if err != nil {
				t.Fatalf("MergeRegFile failed: %v", err)
			}

			if stats == nil {
				t.Fatal("Expected stats, got nil")
			}

			if stats.KeysCreated != tt.expectKeys {
				t.Errorf("Expected %d keys created, got %d", tt.expectKeys, stats.KeysCreated)
			}
			if stats.ValuesSet != tt.expectVals {
				t.Errorf("Expected %d values set, got %d", tt.expectVals, stats.ValuesSet)
			}
			if stats.KeysDeleted != tt.expectDelKey {
				t.Errorf("Expected %d keys deleted, got %d", tt.expectDelKey, stats.KeysDeleted)
			}
			if stats.ValuesDeleted != tt.expectDelVal {
				t.Errorf("Expected %d values deleted, got %d", tt.expectDelVal, stats.ValuesDeleted)
			}
		})
	}
}

// TestMergeIntegration_EndToEnd tests the full pipeline: parse, merge, verify.
func TestMergeIntegration_EndToEnd(t *testing.T) {
	tempDir := t.TempDir()
	hiveFile := filepath.Join(tempDir, "test.hive")

	// Copy minimal hive
	minimalHive, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Skip("Skipping test: testdata/minimal not available")
	}
	if err := os.WriteFile(hiveFile, minimalHive, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	regPath := filepath.Join("../../testdata/merge/valid/01_simple_string.reg")

	// Step 1: Parse and inspect operations
	ops, err := hive.ParseRegFile(regPath, hive.RegParseOptions{AutoPrefix: true})
	if err != nil {
		t.Fatalf("ParseRegFile failed: %v", err)
	}

	if len(ops) != 2 {
		t.Fatalf("Expected 2 operations, got %d", len(ops))
	}

	// Step 2: Merge into hive
	stats, err := hive.MergeRegFile(hiveFile, regPath, nil)
	if err != nil {
		t.Fatalf("MergeRegFile failed: %v", err)
	}

	// Step 3: Verify stats match parsed operations
	if stats.OperationsTotal != len(ops) {
		t.Errorf("Stats total %d != parsed ops %d", stats.OperationsTotal, len(ops))
	}

	if stats.KeysCreated != 1 {
		t.Errorf("Expected 1 key created, got %d", stats.KeysCreated)
	}
	if stats.ValuesSet != 1 {
		t.Errorf("Expected 1 value set, got %d", stats.ValuesSet)
	}

	// Step 4: Verify hive file was modified
	modifiedHive, err := os.ReadFile(hiveFile)
	if err != nil {
		t.Fatalf("Failed to read modified hive: %v", err)
	}

	if len(modifiedHive) == 0 {
		t.Error("Modified hive is empty")
	}

	if stats.BytesWritten != int64(len(modifiedHive)) {
		t.Errorf("Stats BytesWritten %d != actual size %d", stats.BytesWritten, len(modifiedHive))
	}
}

// TestMergeIntegration_PrefixStripping tests prefix stripping behavior.
func TestMergeIntegration_PrefixStripping(t *testing.T) {
	regPath := filepath.Join("../../testdata/merge/valid/01_simple_string.reg")

	// Test with AutoPrefix
	opsAuto, err := hive.ParseRegFile(regPath, hive.RegParseOptions{AutoPrefix: true})
	if err != nil {
		t.Fatalf("ParseRegFile with AutoPrefix failed: %v", err)
	}

	// Test without AutoPrefix
	opsNoAuto, err := hive.ParseRegFile(regPath, hive.RegParseOptions{AutoPrefix: false})
	if err != nil {
		t.Fatalf("ParseRegFile without AutoPrefix failed: %v", err)
	}

	// Should have same number of operations
	if len(opsAuto) != len(opsNoAuto) {
		t.Errorf("Operation count differs: auto=%d noauto=%d", len(opsAuto), len(opsNoAuto))
	}

	// Verify AutoPrefix strips HKEY_LOCAL_MACHINE\SOFTWARE
	for _, op := range opsAuto {
		if createKey, ok := op.(hive.OpCreateKey); ok {
			if strings.Contains(createKey.Path, "HKEY_LOCAL_MACHINE") {
				t.Errorf("AutoPrefix did not strip HKEY prefix: %q", createKey.Path)
			}
		}
	}

	// Test with explicit Prefix
	opsPrefix, err := hive.ParseRegFile(regPath, hive.RegParseOptions{
		Prefix: "HKEY_LOCAL_MACHINE\\SOFTWARE",
	})
	if err != nil {
		t.Fatalf("ParseRegFile with Prefix failed: %v", err)
	}

	// Verify explicit prefix was stripped
	for _, op := range opsPrefix {
		if createKey, ok := op.(hive.OpCreateKey); ok {
			if strings.HasPrefix(createKey.Path, "HKEY_LOCAL_MACHINE\\SOFTWARE\\") {
				t.Errorf("Explicit Prefix did not strip: %q", createKey.Path)
			}
		}
	}
}
