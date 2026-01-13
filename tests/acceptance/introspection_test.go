package acceptance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Note: TestLastModified already exists in open_test.go and tests hivex_last_modified
// It uses Info().LastWrite which is the gohivex equivalent

// TestNodeNameLen tests hivex_node_name_len
// Gets the UTF-8 decoded string length of a node name.
func TestNodeNameLen(t *testing.T) {
	tests := []struct {
		name     string
		hivePath string
		nodeName string
	}{
		{"special_umlaut", TestHives.Special, "abcd_äöüß"},
		{"special_zero", TestHives.Special, "zero\x00key"},
		{"special_trademark", TestHives.Special, "weird™"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root and child
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			goChild, err := goHive.Lookup(goRoot, tt.nodeName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.nodeName)

			// Get name lengths using hivex-compatible method
			goNameLen, err := goHive.KeyNameLenDecoded(goChild)
			require.NoError(t, err)

			hivexNameLen := hivexHive.NodeNameLen(hivexChild)

			// Lengths should match
			assert.Equal(t, int(hivexNameLen), goNameLen, "Name lengths should match")

			// Verify we got a reasonable non-zero length
			assert.Positive(t, goNameLen, "Name length should be positive")

			t.Logf("Node '%s' decoded name length: %d bytes", tt.nodeName, goNameLen)
		})
	}
}

// TestValueKeyLen tests hivex_value_key_len
// Gets the UTF-8 decoded string length of a value name.
func TestValueKeyLen(t *testing.T) {
	tests := []struct {
		name      string
		hivePath  string
		nodeName  string
		valueName string
	}{
		{"special_umlaut", TestHives.Special, "abcd_äöüß", "abcd_äöüß"},
		{"special_zero", TestHives.Special, "zero\x00key", "zero\x00val"},
		{"special_symbols", TestHives.Special, "weird™", "symbols $£₤₧€"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, tt.hivePath)
			defer goHive.Close()

			hivexHive := openHivex(t, tt.hivePath)
			defer hivexHive.Close()

			// Get root and child
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			goChild, err := goHive.Lookup(goRoot, tt.nodeName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.nodeName)

			// Get value
			goValue, err := goHive.GetValue(goChild, tt.valueName)
			require.NoError(t, err)

			hivexValue := hivexHive.NodeGetValue(hivexChild, tt.valueName)

			// Get value name lengths using hivex-compatible method
			goValueNameLen, err := goHive.ValueNameLenDecoded(goValue)
			require.NoError(t, err)

			hivexValueNameLen := hivexHive.ValueKeyLen(hivexValue)

			// Lengths should match
			assert.Equal(t, int(hivexValueNameLen), goValueNameLen, "Value name lengths should match")

			// Verify we got a reasonable non-zero length
			assert.Positive(t, goValueNameLen, "Value name length should be positive")

			t.Logf("Value '%s' decoded name length: %d bytes", tt.valueName, goValueNameLen)
		})
	}
}

// TestNodeStructLength tests hivex_node_struct_length
// Gets the calculated minimum size of the NK record structure.
func TestNodeStructLength(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	// Get NK struct length using hivex-compatible method
	goStructLen, err := goHive.NodeStructSizeCalculated(goRoot)
	require.NoError(t, err)

	hivexStructLen := hivexHive.NodeStructLength(hivexRoot)

	// Lengths should match
	assert.Equal(t, int(hivexStructLen), goStructLen, "NK struct lengths should match")
	t.Logf("Root NK calculated struct length: %d bytes", goStructLen)

	// Get a child and test that too
	goChildren, err := goHive.Subkeys(goRoot)
	require.NoError(t, err)

	hivexChildren := hivexHive.NodeChildren(hivexRoot)

	if len(goChildren) > 0 {
		goChildLen, childErr := goHive.NodeStructSizeCalculated(goChildren[0])
		require.NoError(t, childErr)

		hivexChildLen := hivexHive.NodeStructLength(hivexChildren[0])

		assert.Equal(t, int(hivexChildLen), goChildLen, "Child NK struct lengths should match")
		t.Logf("Child NK calculated struct length: %d bytes", goChildLen)
	}
}

// TestValueStructLength tests hivex_value_struct_length
// Gets the calculated minimum size of the VK record structure.
func TestValueStructLength(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root and child
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
	require.NoError(t, err)

	hivexChild := hivexHive.NodeGetChild(hivexRoot, "abcd_äöüß")

	// Get values
	goValues, err := goHive.Values(goChild)
	require.NoError(t, err)

	hivexValues := hivexHive.NodeValues(hivexChild)

	if len(goValues) > 0 {
		// Get VK struct length using hivex-compatible method
		goStructLen, structErr := goHive.ValueStructSizeCalculated(goValues[0])
		require.NoError(t, structErr)

		hivexStructLen := hivexHive.ValueStructLength(hivexValues[0])

		// Lengths should match
		assert.Equal(t, int(hivexStructLen), goStructLen, "VK struct lengths should match")
		t.Logf("VK calculated struct length: %d bytes", goStructLen)
	}
}

// TestValueDataCellOffset tests hivex_value_data_cell_offset
// Gets the file offset and length of the data cell for a value.
func TestValueDataCellOffset(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root and child
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
	require.NoError(t, err)

	hivexChild := hivexHive.NodeGetChild(hivexRoot, "abcd_äöüß")

	// Get values
	goValues, err := goHive.Values(goChild)
	require.NoError(t, err)

	hivexValues := hivexHive.NodeValues(hivexChild)

	if len(goValues) > 0 {
		// Get data cell offset using hivex-compatible method
		goOffset, goLen, offsetErr := goHive.ValueDataCellOffsetHivex(goValues[0])
		require.NoError(t, offsetErr)

		hivexOffset, hivexLen := hivexHive.ValueDataCellOffset(hivexValues[0])

		// For inline values, both should return 0 offset and 0 length (hivex behavior)
		if goOffset == 0 && goLen == 0 {
			assert.Equal(t, uint64(0), hivexOffset, "Inline value should have 0 offset")
			assert.Equal(t, uint64(0), hivexLen, "Inline value should have 0 length (hivex flag)")
		} else {
			// Account for offset representation differences (HBIN-relative vs absolute)
			// See OFFSET_DIFFERENCES.md for details
			const hbinStart = 0x1000
			goAbsoluteOffset := uint64(goOffset) + hbinStart

			// Offsets and lengths should match
			assert.Equal(t, hivexOffset, goAbsoluteOffset, "Data cell offsets should match")
			assert.Equal(t, int(hivexLen), goLen, "Data cell lengths should match")
		}

		t.Logf("Value data cell: offset=0x%x, length=%d bytes", goOffset, goLen)
	}
}

// TestIntrospectionRecursive tests all introspection functions recursively
// This provides comprehensive coverage of all nodes/values in a hive.
func TestIntrospectionRecursive(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	hivexHive := openHivex(t, TestHives.Special)
	defer hivexHive.Close()

	// Get root
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	hivexRoot := hivexHive.Root()

	// Walk tree and test all introspection functions
	nodeCount := 0
	valueCount := 0

	var walkNode func(goNode hive.NodeID, hivexNode bindings.NodeHandle, depth int)
	walkNode = func(goNode hive.NodeID, hivexNode bindings.NodeHandle, depth int) {
		if depth > 100 {
			return
		}

		nodeCount++

		// Test node introspection using hivex-compatible methods
		goNameLen, nameLenErr := goHive.KeyNameLenDecoded(goNode)
		require.NoError(t, nameLenErr)

		hivexNameLen := hivexHive.NodeNameLen(hivexNode)
		assert.Equal(t, int(hivexNameLen), goNameLen, "Name length mismatch at depth %d", depth)

		goStructLen, structErr := goHive.NodeStructSizeCalculated(goNode)
		require.NoError(t, structErr)

		hivexStructLen := hivexHive.NodeStructLength(hivexNode)
		assert.Equal(t, int(hivexStructLen), goStructLen, "Struct length mismatch at depth %d", depth)

		// Test value introspection using hivex-compatible methods
		goValues, _ := goHive.Values(goNode)
		hivexValues := hivexHive.NodeValues(hivexNode)

		for i, goVal := range goValues {
			valueCount++

			goValNameLen, valNameLenErr := goHive.ValueNameLenDecoded(goVal)
			require.NoError(t, valNameLenErr)

			hivexValNameLen := hivexHive.ValueKeyLen(hivexValues[i])
			assert.Equal(t, int(hivexValNameLen), goValNameLen, "Value name length mismatch")

			goValStructLen, valStructErr := goHive.ValueStructSizeCalculated(goVal)
			require.NoError(t, valStructErr)

			hivexValStructLen := hivexHive.ValueStructLength(hivexValues[i])
			assert.Equal(t, int(hivexValStructLen), goValStructLen, "Value struct length mismatch")

			goOffset, goLen, offsetErr := goHive.ValueDataCellOffsetHivex(goVal)
			require.NoError(t, offsetErr)

			hivexOffset, hivexLen := hivexHive.ValueDataCellOffset(hivexValues[i])

			// Handle inline values (offset=0, length=0) separately
			if goOffset == 0 && goLen == 0 {
				assert.Equal(t, uint64(0), hivexOffset, "Inline value should have 0 offset")
				assert.Equal(t, uint64(0), hivexLen, "Inline value should have 0 length (hivex flag)")
			} else {
				const hbinStart = 0x1000
				assert.Equal(t, hivexOffset, uint64(goOffset)+hbinStart, "Data cell offset mismatch")
				assert.Equal(t, int(hivexLen), goLen, "Data cell length mismatch")
			}
		}

		// Recurse to children
		goChildren, _ := goHive.Subkeys(goNode)
		hivexChildren := hivexHive.NodeChildren(hivexNode)

		for i, goChild := range goChildren {
			walkNode(goChild, hivexChildren[i], depth+1)
		}
	}

	walkNode(goRoot, hivexRoot, 0)

	t.Logf("Tested introspection on %d nodes and %d values", nodeCount, valueCount)
}
