package merge_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/pkg/hive"
)

// assertion defines what to check after a merge operation
type assertion struct {
	keyPath     string      // Key path to check (e.g., "\\TestKey")
	valueName   string      // Value name to check (empty string for key existence only)
	expectType  string      // Expected value type (e.g., "REG_SZ", "REG_DWORD")
	expectData  interface{} // Expected value: string, uint32, uint64, []byte, or []string
	shouldExist bool        // Whether the key/value should exist
}

// mergeAssertionTest defines a complete merge test case
type mergeAssertionTest struct {
	name         string      // Test name
	sourceHive   string      // Path to source hive (relative to testdata/)
	setupRegs    []string    // .reg files to apply before test (for setup)
	testReg      string      // The .reg file being tested
	assertions   []assertion // Assertions to verify after merge
}

// ============================================================================
// Helper Functions
// ============================================================================

// assertKeyExists verifies that a key exists in the hive
func assertKeyExists(t *testing.T, hivePath, keyPath string) {
	t.Helper()
	_, err := hive.GetKeyDetail(hivePath, keyPath)
	if err != nil {
		t.Errorf("Expected key %q to exist, but got error: %v", keyPath, err)
	}
}

// assertKeyNotExists verifies that a key does NOT exist in the hive
func assertKeyNotExists(t *testing.T, hivePath, keyPath string) {
	t.Helper()
	_, err := hive.GetKeyDetail(hivePath, keyPath)
	if err == nil {
		t.Errorf("Expected key %q to NOT exist, but it does", keyPath)
	}
}

// assertValueEquals verifies that a value matches the expected type and data
func assertValueEquals(t *testing.T, hivePath, keyPath, valueName string, expectedType string, expectedData interface{}) {
	t.Helper()

	valueInfo, err := hive.GetValue(hivePath, keyPath, valueName)
	if err != nil {
		t.Fatalf("Failed to get value %q at key %q: %v", valueName, keyPath, err)
	}

	// Check type
	if valueInfo.Type != expectedType {
		t.Errorf("Value %q type: expected %q, got %q", valueName, expectedType, valueInfo.Type)
	}

	// Check data based on type
	switch expectedType {
	case "REG_SZ", "REG_EXPAND_SZ":
		expected, ok := expectedData.(string)
		if !ok {
			t.Fatalf("Expected data for %s should be string, got %T", expectedType, expectedData)
		}
		if valueInfo.StringVal != expected {
			t.Errorf("Value %q data: expected %q, got %q", valueName, expected, valueInfo.StringVal)
		}

	case "REG_DWORD":
		expected, ok := expectedData.(uint32)
		if !ok {
			t.Fatalf("Expected data for REG_DWORD should be uint32, got %T", expectedData)
		}
		if valueInfo.DWordVal != expected {
			t.Errorf("Value %q data: expected %d (0x%08x), got %d (0x%08x)",
				valueName, expected, expected, valueInfo.DWordVal, valueInfo.DWordVal)
		}

	case "REG_QWORD":
		expected, ok := expectedData.(uint64)
		if !ok {
			t.Fatalf("Expected data for REG_QWORD should be uint64, got %T", expectedData)
		}
		if valueInfo.QWordVal != expected {
			t.Errorf("Value %q data: expected %d (0x%016x), got %d (0x%016x)",
				valueName, expected, expected, valueInfo.QWordVal, valueInfo.QWordVal)
		}

	case "REG_MULTI_SZ":
		expected, ok := expectedData.([]string)
		if !ok {
			t.Fatalf("Expected data for REG_MULTI_SZ should be []string, got %T", expectedData)
		}
		if len(valueInfo.StringVals) != len(expected) {
			t.Errorf("Value %q multi-string count: expected %d, got %d",
				valueName, len(expected), len(valueInfo.StringVals))
			return
		}
		for i, exp := range expected {
			if i >= len(valueInfo.StringVals) || valueInfo.StringVals[i] != exp {
				t.Errorf("Value %q multi-string[%d]: expected %q, got %q",
					valueName, i, exp, valueInfo.StringVals[i])
			}
		}

	case "REG_BINARY", "REG_DWORD_BE", "REG_LINK",
		 "REG_RESOURCE_LIST", "REG_FULL_RESOURCE_DESCRIPTOR", "REG_RESOURCE_REQUIREMENTS_LIST":
		expected, ok := expectedData.([]byte)
		if !ok {
			t.Fatalf("Expected data for %s should be []byte, got %T", expectedType, expectedData)
		}
		if !bytes.Equal(valueInfo.Data, expected) {
			t.Errorf("Value %q binary data mismatch:\nexpected: %x\ngot:      %x",
				valueName, expected, valueInfo.Data)
		}

	default:
		t.Errorf("Unknown value type %q for assertion", expectedType)
	}
}

// assertValueNotExists verifies that a value does NOT exist
func assertValueNotExists(t *testing.T, hivePath, keyPath, valueName string) {
	t.Helper()

	_, err := hive.GetValue(hivePath, keyPath, valueName)
	if err == nil {
		t.Errorf("Expected value %q at key %q to NOT exist, but it does", valueName, keyPath)
	}
}

// runMergeAssertionTest executes a merge assertion test
func runMergeAssertionTest(t *testing.T, test mergeAssertionTest) {
	t.Helper()

	// Create temp directory
	tempDir := t.TempDir()

	// Source hive path
	sourceHivePath := filepath.Join("../../testdata", test.sourceHive)

	// Copy hive to temp directory
	hiveData, err := os.ReadFile(sourceHivePath)
	if err != nil {
		t.Fatalf("Failed to read source hive %s: %v", sourceHivePath, err)
	}

	testHivePath := filepath.Join(tempDir, "test.hive")
	if err := os.WriteFile(testHivePath, hiveData, 0644); err != nil {
		t.Fatalf("Failed to create test hive: %v", err)
	}

	// Apply setup .reg files if any
	for _, setupReg := range test.setupRegs {
		setupRegPath := filepath.Join("../../testdata/merge_assertions", setupReg)
		if _, err := hive.MergeRegFile(testHivePath, setupRegPath, nil); err != nil {
			t.Fatalf("Failed to apply setup .reg %s: %v", setupReg, err)
		}
	}

	// Apply the test .reg file
	testRegPath := filepath.Join("../../testdata/merge_assertions", test.testReg)
	stats, err := hive.MergeRegFile(testHivePath, testRegPath, nil)
	if err != nil {
		t.Fatalf("Failed to merge test .reg %s: %v", test.testReg, err)
	}

	t.Logf("Merge stats: %d operations (%d keys created, %d values set, %d keys deleted, %d values deleted)",
		stats.OperationsTotal, stats.KeysCreated, stats.ValuesSet, stats.KeysDeleted, stats.ValuesDeleted)

	// Run all assertions
	for i, assertion := range test.assertions {
		t.Run(fmt.Sprintf("Assertion_%d", i), func(t *testing.T) {
			if assertion.valueName == "" {
				// Key existence check
				if assertion.shouldExist {
					assertKeyExists(t, testHivePath, assertion.keyPath)
				} else {
					assertKeyNotExists(t, testHivePath, assertion.keyPath)
				}
			} else {
				// Value check
				if assertion.shouldExist {
					assertValueEquals(t, testHivePath, assertion.keyPath, assertion.valueName,
						assertion.expectType, assertion.expectData)
				} else {
					assertValueNotExists(t, testHivePath, assertion.keyPath, assertion.valueName)
				}
			}
		})
	}
}

// ============================================================================
// Test Cases
// ============================================================================

// TestMergeAssertions_Add tests adding new keys and values
func TestMergeAssertions_Add(t *testing.T) {
	tests := []mergeAssertionTest{
		{
			name:       "Add simple key with string value",
			sourceHive: "minimal",
			testReg:    "add/add_simple_key.reg",
			assertions: []assertion{
				{keyPath: "\\TestAddedKey", shouldExist: true},
				{keyPath: "\\TestAddedKey", valueName: "TestString", expectType: "REG_SZ",
					expectData: "Hello World", shouldExist: true},
			},
		},
		{
			name:       "Add nested keys (4 levels)",
			sourceHive: "minimal",
			testReg:    "add/add_nested_keys.reg",
			assertions: []assertion{
				{keyPath: "\\Level1", shouldExist: true},
				{keyPath: "\\Level1\\Level2", shouldExist: true},
				{keyPath: "\\Level1\\Level2\\Level3", shouldExist: true},
				{keyPath: "\\Level1\\Level2\\Level3\\Level4", shouldExist: true},
				{keyPath: "\\Level1\\Level2\\Level3\\Level4", valueName: "DeepValue",
					expectType: "REG_SZ", expectData: "Found me at level 4", shouldExist: true},
			},
		},
		{
			name:       "Add DWORD values",
			sourceHive: "minimal",
			testReg:    "add/add_dword.reg",
			assertions: []assertion{
				{keyPath: "\\TestDWORD", valueName: "ZeroValue",
					expectType: "REG_DWORD", expectData: uint32(0), shouldExist: true},
				{keyPath: "\\TestDWORD", valueName: "SmallValue",
					expectType: "REG_DWORD", expectData: uint32(42), shouldExist: true},
				{keyPath: "\\TestDWORD", valueName: "LargeValue",
					expectType: "REG_DWORD", expectData: uint32(0xffffffff), shouldExist: true},
			},
		},
		{
			name:       "Add QWORD values",
			sourceHive: "minimal",
			testReg:    "add/add_qword.reg",
			assertions: []assertion{
				{keyPath: "\\TestQWORD", valueName: "QWordValue",
					expectType: "REG_QWORD", expectData: uint64(0xffffffffffffffff), shouldExist: true},
				{keyPath: "\\TestQWORD", valueName: "SmallQWord",
					expectType: "REG_QWORD", expectData: uint64(42), shouldExist: true},
			},
		},
		{
			name:       "Add binary data",
			sourceHive: "minimal",
			testReg:    "add/add_binary.reg",
			assertions: []assertion{
				{keyPath: "\\TestBinary", valueName: "BinaryData",
					expectType: "REG_BINARY",
					expectData: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					shouldExist: true},
				{keyPath: "\\TestBinary", valueName: "EmptyBinary",
					expectType: "REG_BINARY", expectData: []byte{}, shouldExist: true},
			},
		},
		{
			name:       "Add multi-string value",
			sourceHive: "minimal",
			testReg:    "add/add_multi_sz.reg",
			assertions: []assertion{
				{keyPath: "\\TestMultiSZ", valueName: "MultiString",
					expectType: "REG_MULTI_SZ",
					expectData: []string{"First", "Second", "Third"},
					shouldExist: true},
			},
		},
		{
			name:       "Add expandable string",
			sourceHive: "minimal",
			testReg:    "add/add_expand_sz.reg",
			assertions: []assertion{
				{keyPath: "\\TestExpandSZ", valueName: "ExpandableString",
					expectType: "REG_EXPAND_SZ", expectData: "%SystemRoot%", shouldExist: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runMergeAssertionTest(t, tt)
		})
	}
}

// TestMergeAssertions_Modify tests modifying existing values
func TestMergeAssertions_Modify(t *testing.T) {
	tests := []mergeAssertionTest{
		{
			name:       "Modify string value",
			sourceHive: "minimal",
			setupRegs: []string{
				// First create a key with a string value
				"add/add_simple_key.reg",
			},
			testReg: "modify/modify_string.reg",
			assertions: []assertion{
				{keyPath: "\\ModifyTest", valueName: "StringValue",
					expectType: "REG_SZ", expectData: "Modified String Value", shouldExist: true},
			},
		},
		{
			name:       "Modify DWORD value",
			sourceHive: "minimal",
			setupRegs: []string{
				"add/add_dword.reg",
			},
			testReg: "modify/modify_dword.reg",
			assertions: []assertion{
				{keyPath: "\\ModifyTest", valueName: "DWordValue",
					expectType: "REG_DWORD", expectData: uint32(0x12345678), shouldExist: true},
			},
		},
		{
			name:       "Change value type (string to DWORD)",
			sourceHive: "minimal",
			setupRegs: []string{
				"add/add_simple_key.reg",
			},
			testReg: "modify/modify_type_change.reg",
			assertions: []assertion{
				{keyPath: "\\ModifyTest", valueName: "TypeChange",
					expectType: "REG_DWORD", expectData: uint32(0x42), shouldExist: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runMergeAssertionTest(t, tt)
		})
	}
}

// TestMergeAssertions_Delete tests deleting keys and values
func TestMergeAssertions_Delete(t *testing.T) {
	tests := []mergeAssertionTest{
		{
			name:       "Delete key",
			sourceHive: "minimal",
			setupRegs: []string{
				// Create a key to delete
				"add/add_simple_key.reg",
			},
			testReg: "delete/delete_key.reg",
			assertions: []assertion{
				{keyPath: "\\DeleteMe", shouldExist: false},
			},
		},
		{
			name:       "Delete value",
			sourceHive: "minimal",
			setupRegs: []string{
				// Create keys with values
				"add/add_simple_key.reg",
			},
			testReg: "delete/delete_value.reg",
			assertions: []assertion{
				{keyPath: "\\DeleteTest", valueName: "KeepThis",
					expectType: "REG_SZ", expectData: "Keep", shouldExist: true},
				{keyPath: "\\DeleteTest", valueName: "DeleteThis", shouldExist: false},
			},
		},
		{
			name:       "Delete key with subkeys",
			sourceHive: "minimal",
			setupRegs: []string{
				// Create nested keys
				"add/add_nested_keys.reg",
			},
			testReg: "delete/delete_recursive.reg",
			assertions: []assertion{
				{keyPath: "\\DeleteParent", shouldExist: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runMergeAssertionTest(t, tt)
		})
	}
}

// TestMergeAssertions_Mixed tests mixed operations
func TestMergeAssertions_Mixed(t *testing.T) {
	tests := []mergeAssertionTest{
		{
			name:       "Complex nested structure",
			sourceHive: "minimal",
			testReg:    "mixed/complex_nested.reg",
			assertions: []assertion{
				{keyPath: "\\ComplexTest", valueName: "Root",
					expectType: "REG_SZ", expectData: "At root", shouldExist: true},
				{keyPath: "\\ComplexTest\\Branch1", shouldExist: true},
				{keyPath: "\\ComplexTest\\Branch1\\Leaf1", valueName: "Value2",
					expectType: "REG_DWORD", expectData: uint32(1), shouldExist: true},
				{keyPath: "\\ComplexTest\\Branch2\\Leaf2\\DeepLeaf", valueName: "Value5",
					expectType: "REG_SZ", expectData: "Very deep", shouldExist: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runMergeAssertionTest(t, tt)
		})
	}
}

// TestMergeAssertions_AllTypes tests all registry value types
func TestMergeAssertions_AllTypes(t *testing.T) {
	test := mergeAssertionTest{
		name:       "All registry value types",
		sourceHive: "minimal",
		testReg:    "types/all_value_types.reg",
		assertions: []assertion{
			{keyPath: "\\AllTypes", valueName: "StringValue",
				expectType: "REG_SZ", expectData: "Test String", shouldExist: true},
			{keyPath: "\\AllTypes", valueName: "ExpandString",
				expectType: "REG_EXPAND_SZ", expectData: "%Path%", shouldExist: true},
			{keyPath: "\\AllTypes", valueName: "BinaryValue",
				expectType: "REG_BINARY", expectData: []byte{0xde, 0xad, 0xbe, 0xef}, shouldExist: true},
			{keyPath: "\\AllTypes", valueName: "DWordValue",
				expectType: "REG_DWORD", expectData: uint32(0x42), shouldExist: true},
			{keyPath: "\\AllTypes", valueName: "DWordBE",
				expectType: "REG_DWORD_BE", expectData: []byte{0x00, 0x00, 0x00, 0x2a}, shouldExist: true},
			{keyPath: "\\AllTypes", valueName: "MultiString",
				expectType: "REG_MULTI_SZ", expectData: []string{"One", "Two"}, shouldExist: true},
			{keyPath: "\\AllTypes", valueName: "QWordValue",
				expectType: "REG_QWORD", expectData: uint64(0x8899aabbccddeeff), shouldExist: true},
		},
	}

	runMergeAssertionTest(t, test)
}

// Helper to convert little-endian bytes to uint32
func bytesToUint32LE(b []byte) uint32 {
	if len(b) != 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

// Helper to convert little-endian bytes to uint64
func bytesToUint64LE(b []byte) uint64 {
	if len(b) != 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}
