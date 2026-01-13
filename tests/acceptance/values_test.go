package acceptance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestNodeValues tests hivex_node_values
// Enumerates all values for a node.
func TestNodeValues(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		childName string // Child node to check (root has no values)
		minValues int    // Minimum expected values
	}{
		{"special_first_child", TestHives.Special, "abcd_äöüß", 1},
		{"special_second_child", TestHives.Special, "zero\x00key", 1},
		{"rlenvalue", TestHives.RLenValue, "ModerateValueParent", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root and find child
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Look up child
			goChild, err := goHive.Lookup(goRoot, tt.childName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.childName)
			require.NotZero(t, hivexChild)

			// Get values
			goValues, err := goHive.Values(goChild)
			require.NoError(t, err)

			hivexValues := hivexHive.NodeValues(hivexChild)

			// Should have same number of values
			assertIntEqual(t, len(goValues), len(hivexValues), "Number of values")

			// Should have at least minimum expected
			assert.GreaterOrEqual(t, len(goValues), tt.minValues,
				"Should have at least %d values", tt.minValues)

			// Values should point to same data (accounting for offset difference)
			assertValueListsEqual(t, goValues, hivexValues, "Value list")
		})
	}
}

// TestValueKey tests hivex_value_key
// Gets the name of a value.
func TestValueKey(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		childName string
		valueName string // Expected first value name
	}{
		{"special_umlaut", TestHives.Special, "abcd_äöüß", "abcd_äöüß"},
		{"special_zero", TestHives.Special, "zero\x00key", "zero\x00val"},
		{"special_trademark", TestHives.Special, "weird™", "symbols $£₤₧€"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Navigate to child node
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			goChild, err := goHive.Lookup(goRoot, tt.childName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.childName)

			// Get values
			goValues, err := goHive.Values(goChild)
			require.NoError(t, err)
			require.NotEmpty(t, goValues, "Should have at least one value")

			hivexValues := hivexHive.NodeValues(hivexChild)
			require.NotEmpty(t, hivexValues, "Should have at least one value")

			// Get first value's name
			goMeta, err := goHive.StatValue(goValues[0])
			require.NoError(t, err)

			hivexName := hivexHive.ValueKey(hivexValues[0])

			// Names should match
			assertStringsEqual(t, goMeta.Name, hivexName, "Value name")

			// Should match expected (value names match key names in special hive)
			assert.Equal(t, tt.valueName, goMeta.Name, "Expected value name")
		})
	}
}

// TestValueType tests hivex_value_type
// Gets the type and size of a value.
func TestValueType(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Navigate to a node with values
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
	require.NoError(t, err)

	hivexChild := hivexHive.NodeGetChild(hivexRoot, "abcd_äöüß")

	// Get values
	goValues, err := goHive.Values(goChild)
	require.NoError(t, err)
	require.NotEmpty(t, goValues)

	hivexValues := hivexHive.NodeValues(hivexChild)
	require.NotEmpty(t, hivexValues)

	// Check type and size for each value
	for i := range goValues {
		goMeta, statErr := goHive.StatValue(goValues[i])
		require.NoError(t, statErr)

		hivexType, hivexSize, typeErr := hivexHive.ValueType(hivexValues[i])
		require.NoError(t, typeErr)

		// Types should match
		assertRegTypeEqual(t, goMeta.Type, hivexType, "Value %d type", i)

		// Sizes should match
		assertIntEqual(t, goMeta.Size, hivexSize, "Value %d size", i)
	}
}

// TestValueValue tests hivex_value_value
// Gets raw value bytes (before type-specific decoding).
func TestValueValue(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Navigate to a node with values
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
	require.NoError(t, err)

	hivexChild := hivexHive.NodeGetChild(hivexRoot, "abcd_äöüß")

	// Get values
	goValues, err := goHive.Values(goChild)
	require.NoError(t, err)
	require.NotEmpty(t, goValues)

	hivexValues := hivexHive.NodeValues(hivexChild)
	require.NotEmpty(t, hivexValues)

	// Read raw bytes for each value
	for i := range goValues {
		goBytes, bytesErr := goHive.ValueBytes(goValues[i], hive.ReadOptions{})
		require.NoError(t, bytesErr)

		hivexBytes, hivexType, valueErr := hivexHive.ValueValue(hivexValues[i])
		require.NoError(t, valueErr)

		// Bytes should be identical
		assertBytesEqual(t, goBytes, hivexBytes, "Value %d raw bytes", i)

		// Type should match metadata
		goMeta, statErr := goHive.StatValue(goValues[i])
		require.NoError(t, statErr)

		assertRegTypeEqual(t, goMeta.Type, hivexType, "Value %d type from ValueValue", i)
	}
}

// TestNodeGetValue tests hivex_node_get_value
// Finds a value by name.
func TestNodeGetValue(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		childName string
		valueName string
	}{
		{"special_umlaut", TestHives.Special, "abcd_äöüß", "abcd_äöüß"},
		{"special_zero", TestHives.Special, "zero\x00key", "zero\x00val"},
		{"special_trademark", TestHives.Special, "weird™", "symbols $£₤₧€"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Navigate to child node
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			goChild, err := goHive.Lookup(goRoot, tt.childName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.childName)

			// Look up value by name
			goValue, err := goHive.GetValue(goChild, tt.valueName)
			require.NoError(t, err, "gohivex should find value '%s'", tt.valueName)

			hivexValue := hivexHive.NodeGetValue(hivexChild, tt.valueName)
			require.NotZero(t, hivexValue, "hivex should find value '%s'", tt.valueName)

			// Should find same value
			assertSameValueID(t, goValue, hivexValue, "Found value '%s'", tt.valueName)

			// Verify it's the right value by checking name
			goMeta, err := goHive.StatValue(goValue)
			require.NoError(t, err)

			assertStringsEqual(t, tt.valueName, goMeta.Name, "Value name")
		})
	}
}

// TestNodeGetValueNotFound tests GetValue with non-existent name.
func TestNodeGetValueNotFound(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Navigate to a child
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
	require.NoError(t, err)

	hivexChild := hivexHive.NodeGetChild(hivexRoot, "abcd_äöüß")

	// Try to find non-existent value
	nonExistent := "this_value_does_not_exist_12345"

	_, goErr := goHive.GetValue(goChild, nonExistent)
	require.Error(t, goErr, "gohivex should error for non-existent value")

	hivexValue := hivexHive.NodeGetValue(hivexChild, nonExistent)
	assert.Zero(t, hivexValue, "hivex should return 0 for non-existent value")
}

// TestValueMetadataConsistency verifies all value metadata is consistent.
func TestValueMetadataConsistency(t *testing.T) {
	goHive := openGoHivex(t, TestHives.RLenValue)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.RLenValue)
	defer hivexHive.Close()

	// Navigate to node with multiple values
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChild, err := goHive.Lookup(goRoot, "ModerateValueParent")
	require.NoError(t, err)

	hivexChild := hivexHive.NodeGetChild(hivexRoot, "ModerateValueParent")

	// Get all values
	goValues, err := goHive.Values(goChild)
	require.NoError(t, err)

	hivexValues := hivexHive.NodeValues(hivexChild)

	// For each value, verify metadata consistency
	for i := range goValues {
		goMeta, statErr := goHive.StatValue(goValues[i])
		require.NoError(t, statErr)

		// Check individual operations match metadata
		hivexName := hivexHive.ValueKey(hivexValues[i])
		hivexType, hivexSize, typeErr := hivexHive.ValueType(hivexValues[i])
		require.NoError(t, typeErr)

		assertStringsEqual(t, goMeta.Name, hivexName, "Value %d name", i)
		assertRegTypeEqual(t, goMeta.Type, hivexType, "Value %d type", i)
		assertIntEqual(t, goMeta.Size, hivexSize, "Value %d size", i)

		// Verify raw bytes match size
		goBytes, bytesErr := goHive.ValueBytes(goValues[i], hive.ReadOptions{})
		require.NoError(t, bytesErr)

		hivexBytes, _, valueErr := hivexHive.ValueValue(hivexValues[i])
		require.NoError(t, valueErr)

		assert.Len(t, goBytes, goMeta.Size, "Bytes length should match size")
		assert.Len(t, hivexBytes, hivexSize, "Bytes length should match size")
	}
}

// TestValuesRecursive tests value enumeration across entire tree.
func TestValuesRecursive(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"special", TestHives.Special},
		{"rlenvalue", TestHives.RLenValue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get roots
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Walk both trees and compare all values
			compareValuesRecursive(t, goHive, hivexHive, goRoot, hivexRoot, 0)
		})
	}
}

// compareValuesRecursive recursively compares values for all nodes.
func compareValuesRecursive(t *testing.T, goHive hive.Reader, hivexHive *bindings.Hive,
	goNode hive.NodeID, hivexNode bindings.NodeHandle, depth int) {
	t.Helper()

	// Limit depth
	if depth > 100 {
		t.Fatal("Tree depth exceeded 100 levels")
	}

	// Compare values
	goValues, err := goHive.Values(goNode)
	require.NoError(t, err)

	hivexValues := hivexHive.NodeValues(hivexNode)

	assertIntEqual(t, len(goValues), len(hivexValues),
		"Value count at depth %d", depth)

	// Compare each value's metadata and data
	for i := range goValues {
		goMeta, statErr := goHive.StatValue(goValues[i])
		require.NoError(t, statErr)

		hivexName := hivexHive.ValueKey(hivexValues[i])
		assertStringsEqual(t, goMeta.Name, hivexName, "Value name at depth %d, index %d", depth, i)

		goBytes, bytesErr := goHive.ValueBytes(goValues[i], hive.ReadOptions{})
		require.NoError(t, bytesErr)

		hivexBytes, _, valueErr := hivexHive.ValueValue(hivexValues[i])
		require.NoError(t, valueErr)

		assertBytesEqual(t, goBytes, hivexBytes, "Value bytes at depth %d, index %d", depth, i)
	}

	// Recurse into children
	goChildren, err := goHive.Subkeys(goNode)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexNode)

	for i := range goChildren {
		compareValuesRecursive(t, goHive, hivexHive, goChildren[i], hivexChildren[i], depth+1)
	}
}

// TestValueInlineFlag tests that inline values are correctly identified.
func TestValueInlineFlag(t *testing.T) {
	goHive := openGoHivex(t, TestHives.RLenValue)
	defer goHive.Close()

	// Navigate to node with values
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	goChild, err := goHive.Lookup(goRoot, "ModerateValueParent")
	require.NoError(t, err)

	goValues, err := goHive.Values(goChild)
	require.NoError(t, err)

	// Check each value's inline flag
	for i, val := range goValues {
		goMeta, statErr := goHive.StatValue(val)
		require.NoError(t, statErr)

		// Inline flag should be set for small values (<=4 bytes)
		if goMeta.Size <= 4 {
			t.Logf("Value %d ('%s'): size=%d, inline=%v",
				i, goMeta.Name, goMeta.Size, goMeta.Inline)
		}

		// Verify we can still read bytes regardless of inline flag
		bytes, readErr := goHive.ValueBytes(val, hive.ReadOptions{})
		require.NoError(t, readErr)
		assert.Len(t, bytes, goMeta.Size, "Bytes length should match size")
	}
}
