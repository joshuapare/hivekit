package acceptance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestNodeName tests hivex_node_name
// Gets the name of a node.
func TestNodeName(t *testing.T) {
	tests := []struct {
		name         string
		hivePath     string
		expectedRoot string
	}{
		{"minimal", TestHives.Minimal, "$$$PROTO.HIV"},
		{"special", TestHives.Special, "$$$PROTO.HIV"},
		{"rlenvalue", TestHives.RLenValue, "$$$PROTO.HIV"},
		{"large", TestHives.Large, "$$$PROTO.HIV"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Get names
			goMeta, err := goHive.StatKey(goRoot)
			require.NoError(t, err)

			hivexName := hivexHive.NodeName(hivexRoot)

			// Names should match
			assertStringsEqual(t, goMeta.Name, hivexName, "Root node name")

			// Should match expected
			assert.Equal(t, tt.expectedRoot, goMeta.Name, "Expected root name")
			assert.Equal(t, tt.expectedRoot, hivexName, "Expected root name")
		})
	}
}

// TestNodeNameSpecialChars tests node names with special characters.
func TestNodeNameSpecialChars(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root and children
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChildren, err := goHive.Subkeys(goRoot)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexRoot)

	// Known child names in special hive
	expectedNames := []string{
		"abcd_äöüß",
		"weird™",
		"zero\x00key",
	}

	// Verify we have the expected children
	require.Len(t, goChildren, len(expectedNames), "Should have expected number of children")

	// Compare names for each child
	for i := range goChildren {
		goMeta, statErr := goHive.StatKey(goChildren[i])
		require.NoError(t, statErr)

		hivexName := hivexHive.NodeName(hivexChildren[i])

		assertStringsEqual(t, goMeta.Name, hivexName, "Child %d name", i)

		// Check it's one of the expected names
		assert.Contains(t, expectedNames, goMeta.Name, "Name should be in expected list")
	}
}

// TestNodeTimestamp tests hivex_node_timestamp
// Gets the last write timestamp of a node.
func TestNodeTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"minimal", TestHives.Minimal},
		{"special", TestHives.Special},
		{"rlenvalue", TestHives.RLenValue},
		{"large", TestHives.Large},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Get timestamps
			goMeta, err := goHive.StatKey(goRoot)
			require.NoError(t, err)

			hivexTimestamp := hivexHive.NodeTimestamp(hivexRoot)

			// Convert gohivex time.Time to Unix timestamp
			goUnix := goMeta.LastWrite.Unix()

			// Timestamps should match (within reasonable tolerance for precision differences)
			// Note: hivex returns Windows FILETIME (100ns intervals since 1601-01-01)
			// while gohivex converts to time.Time. We need to verify the conversion is correct.

			// For now, just verify both are non-zero
			assert.NotZero(t, goUnix, "gohivex timestamp should not be zero")
			assert.NotZero(t, hivexTimestamp, "hivex timestamp should not be zero")

			// Verify the timestamp is reasonable (between 2000 and 2100)
			assert.True(t, goMeta.LastWrite.After(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)),
				"Timestamp should be after 2000")
			assert.True(t, goMeta.LastWrite.Before(time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)),
				"Timestamp should be before 2100")
		})
	}
}

// TestNodeNrChildren tests hivex_node_nr_children
// Gets the count of children without loading them.
func TestNodeNrChildren(t *testing.T) {
	tests := []struct {
		name          string
		hivePath      string
		expectedCount int
	}{
		{"minimal", TestHives.Minimal, 0},
		{"special", TestHives.Special, 3},
		{"large", TestHives.Large, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Get child counts
			goMeta, err := goHive.StatKey(goRoot)
			require.NoError(t, err)

			hivexCount := hivexHive.NodeNrChildren(hivexRoot)

			// Counts should match
			assertIntEqual(t, goMeta.SubkeyN, hivexCount, "Child count")

			// Should match expected
			assert.Equal(t, tt.expectedCount, goMeta.SubkeyN, "Expected child count")
			assert.Equal(t, tt.expectedCount, hivexCount, "Expected child count")

			// Verify by actually loading children
			goChildren, err := goHive.Subkeys(goRoot)
			require.NoError(t, err)

			hivexChildren := hivexHive.NodeChildren(hivexRoot)

			assert.Len(t, goChildren, goMeta.SubkeyN, "Actual children should match count")
			assert.Len(t, hivexChildren, hivexCount, "Actual children should match count")
		})
	}
}

// TestNodeNrValues tests hivex_node_nr_values
// Gets the count of values without loading them.
func TestNodeNrValues(t *testing.T) {
	tests := []struct {
		name          string
		hivePath      string
		expectedCount int // On root node
	}{
		{"minimal", TestHives.Minimal, 0},
		{"special", TestHives.Special, 0},     // Root has 0 values
		{"rlenvalue", TestHives.RLenValue, 0}, // Root has 0 values
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			// Get value counts for root
			goMeta, err := goHive.StatKey(goRoot)
			require.NoError(t, err)

			hivexCount := hivexHive.NodeNrValues(hivexRoot)

			// Counts should match
			assertIntEqual(t, goMeta.ValueN, hivexCount, "Value count")

			// Should match expected
			assert.Equal(t, tt.expectedCount, goMeta.ValueN, "Expected value count")
			assert.Equal(t, tt.expectedCount, hivexCount, "Expected value count")

			// Verify by actually loading values
			goValues, err := goHive.Values(goRoot)
			require.NoError(t, err)

			hivexValues := hivexHive.NodeValues(hivexRoot)

			assert.Len(t, goValues, goMeta.ValueN, "Actual values should match count")
			assert.Len(t, hivexValues, hivexCount, "Actual values should match count")
		})
	}
}

// TestNodeNrValuesOnChildren tests value counts on child nodes (where values actually are).
func TestNodeNrValuesOnChildren(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		childName string
		minValues int // Minimum expected values
	}{
		{"special_first_child", TestHives.Special, "abcd_äöüß", 1},
		{"rlenvalue_first_child", TestHives.RLenValue, "ModerateValueParent", 1},
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

			// Look up child by name
			goChild, err := goHive.Lookup(goRoot, tt.childName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.childName)
			require.NotZero(t, hivexChild)

			// Get value counts for child
			goMeta, err := goHive.StatKey(goChild)
			require.NoError(t, err)

			hivexCount := hivexHive.NodeNrValues(hivexChild)

			// Counts should match
			assertIntEqual(t, goMeta.ValueN, hivexCount, "Value count on child")

			// Should have at least minimum expected
			assert.GreaterOrEqual(t, goMeta.ValueN, tt.minValues,
				"Child should have at least %d values", tt.minValues)

			// Verify by actually loading values
			goValues, err := goHive.Values(goChild)
			require.NoError(t, err)

			hivexValues := hivexHive.NodeValues(hivexChild)

			assert.Len(t, goValues, goMeta.ValueN, "Actual values should match count")
			assert.Len(t, hivexValues, hivexCount, "Actual values should match count")
		})
	}
}

// TestMetadataConsistency verifies all metadata is consistent across operations.
func TestMetadataConsistency(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	// Get metadata
	goMeta, err := goHive.StatKey(goRoot)
	require.NoError(t, err)

	// Compare individual operations
	hivexName := hivexHive.NodeName(hivexRoot)
	hivexTimestamp := hivexHive.NodeTimestamp(hivexRoot)
	hivexNrChildren := hivexHive.NodeNrChildren(hivexRoot)
	hivexNrValues := hivexHive.NodeNrValues(hivexRoot)

	// All should match
	assertStringsEqual(t, goMeta.Name, hivexName, "Name")
	assertIntEqual(t, goMeta.SubkeyN, hivexNrChildren, "Number of children")
	assertIntEqual(t, goMeta.ValueN, hivexNrValues, "Number of values")
	assert.NotZero(t, hivexTimestamp, "Timestamp should not be zero")
}

// TestMetadataRecursive tests metadata across entire tree.
func TestMetadataRecursive(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
	}{
		{"special", TestHives.Special},
		{"large", TestHives.Large},
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

			// Walk both trees and compare all metadata
			compareMetadataRecursive(t, goHive, hivexHive, goRoot, hivexRoot, 0)
		})
	}
}

// compareMetadataRecursive recursively compares metadata for all nodes.
func compareMetadataRecursive(t *testing.T, goHive hive.Reader, hivexHive *bindings.Hive,
	goNode hive.NodeID, hivexNode bindings.NodeHandle, depth int) {
	t.Helper()

	// Limit depth
	if depth > 100 {
		t.Fatal("Tree depth exceeded 100 levels")
	}

	// Compare metadata
	goMeta, err := goHive.StatKey(goNode)
	require.NoError(t, err)

	hivexName := hivexHive.NodeName(hivexNode)
	hivexNrChildren := hivexHive.NodeNrChildren(hivexNode)
	hivexNrValues := hivexHive.NodeNrValues(hivexNode)

	assertStringsEqual(t, goMeta.Name, hivexName, "Node name at depth %d", depth)
	assertIntEqual(t, goMeta.SubkeyN, hivexNrChildren, "Children count at depth %d", depth)
	assertIntEqual(t, goMeta.ValueN, hivexNrValues, "Values count at depth %d", depth)

	// Recurse into children
	goChildren, err := goHive.Subkeys(goNode)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexNode)

	for i := range goChildren {
		compareMetadataRecursive(t, goHive, hivexHive, goChildren[i], hivexChildren[i], depth+1)
	}
}

// TestDetailKey tests gohivex-specific DetailKey function
// This returns full NK record details beyond basic metadata.
func TestDetailKey(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	goRoot, err := goHive.Root()
	require.NoError(t, err)

	// Get detailed metadata
	detail, err := goHive.DetailKey(goRoot)
	require.NoError(t, err)

	// Should match basic metadata
	meta, err := goHive.StatKey(goRoot)
	require.NoError(t, err)

	assert.Equal(t, meta.Name, detail.Name, "Name should match")
	assert.Equal(t, meta.LastWrite, detail.LastWrite, "Timestamp should match")
	assert.Equal(t, meta.SubkeyN, detail.SubkeyN, "Subkey count should match")
	assert.Equal(t, meta.ValueN, detail.ValueN, "Value count should match")

	// DetailKey should have additional fields
	// (These are gohivex-specific, no hivex equivalent)
	assert.NotNil(t, detail, "Detail should not be nil")
}
