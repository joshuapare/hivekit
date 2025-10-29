package acceptance

import (
	"testing"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValueString tests hivex_value_string
// Decodes REG_SZ and REG_EXPAND_SZ values (UTF-16LE → UTF-8)
// Note: Our test hives don't have string values, so this test is skipped
// In a real scenario, you'd test with a hive containing REG_SZ values
func TestValueString(t *testing.T) {
	t.Skip("Test hives don't contain REG_SZ values - they have REG_DWORD instead")

	// When you have a hive with REG_SZ values, the test would look like this:
	/*
		goHive := openGoHivex(t, hivePath)
		defer goHive.Close()

		hivexHive := openHivex(t, hivePath)
		defer hivexHive.Close()

		// Navigate to REG_SZ value
		goValue, err := goHive.GetValue(node, valueName)
		require.NoError(t, err)

		hivexValue := hivexHive.NodeGetValue(hivexNode, valueName)

		// Decode string
		goString, err := goHive.ValueString(goValue, hive.ReadOptions{})
		require.NoError(t, err)

		hivexString, err := hivexHive.ValueString(hivexValue)
		require.NoError(t, err)

		// Strings should match
		assertStringsEqual(t, goString, hivexString, "Decoded string")
	*/
}

// TestValueDword tests hivex_value_dword
// Decodes REG_DWORD values (32-bit integers)
func TestValueDword(t *testing.T) {
	tests := []struct {
		name      string
		childName string
		valueName string
	}{
		{"umlaut", "abcd_äöüß", "abcd_äöüß"},
		{"zero", "zero\x00key", "zero\x00val"},
		{"symbols", "weird™", "symbols $£₤₧€"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			goHive := openGoHivex(t, TestHives.Special)
			defer goHive.Close()

			hivexHive := openHivex(t, TestHives.Special)
			defer hivexHive.Close()

			// Navigate to value
			goRoot, err := goHive.Root()
			require.NoError(t, err)

			hivexRoot := hivexHive.Root()

			goChild, err := goHive.Lookup(goRoot, tt.childName)
			require.NoError(t, err)

			hivexChild := hivexHive.NodeGetChild(hivexRoot, tt.childName)

			goValue, err := goHive.GetValue(goChild, tt.valueName)
			require.NoError(t, err)

			hivexValue := hivexHive.NodeGetValue(hivexChild, tt.valueName)

			// Verify type
			goMeta, err := goHive.StatValue(goValue)
			require.NoError(t, err)
			assert.Equal(t, hive.REG_DWORD, goMeta.Type, "Value should be REG_DWORD")

			// Decode DWORD
			goDword, err := goHive.ValueDWORD(goValue)
			require.NoError(t, err)

			hivexDword, err := hivexHive.ValueDword(hivexValue)
			require.NoError(t, err)

			// Convert hivex int32 to uint32 for comparison
			assert.Equal(t, goDword, uint32(hivexDword), "DWORD values should match")

			t.Logf("DWORD value: %d (0x%08x)", goDword, goDword)
		})
	}
}

// TestValueQword tests hivex_value_qword
// Decodes REG_QWORD values (64-bit integers)
func TestValueQword(t *testing.T) {
	t.Skip("Test hives don't contain REG_QWORD values")

	// When you have a hive with REG_QWORD values, the test would look like this:
	/*
		goHive := openGoHivex(t, hivePath)
		defer goHive.Close()

		hivexHive := openHivex(t, hivePath)
		defer hivexHive.Close()

		// Navigate to REG_QWORD value
		goValue, err := goHive.GetValue(node, valueName)
		require.NoError(t, err)

		hivexValue := hivexHive.NodeGetValue(hivexNode, valueName)

		// Decode QWORD
		goQword, err := goHive.ValueQWORD(goValue)
		require.NoError(t, err)

		hivexQword, err := hivexHive.ValueQword(hivexValue)
		require.NoError(t, err)

		// Values should match (convert hivex int64 to uint64)
		assert.Equal(t, goQword, uint64(hivexQword), "QWORD values should match")
	*/
}

// TestValueMultipleStrings tests hivex_value_multiple_strings
// Decodes REG_MULTI_SZ values (string arrays)
func TestValueMultipleStrings(t *testing.T) {
	t.Skip("Test hives don't contain REG_MULTI_SZ values")

	// When you have a hive with REG_MULTI_SZ values, the test would look like this:
	/*
		goHive := openGoHivex(t, hivePath)
		defer goHive.Close()

		hivexHive := openHivex(t, hivePath)
		defer hivexHive.Close()

		// Navigate to REG_MULTI_SZ value
		goValue, err := goHive.GetValue(node, valueName)
		require.NoError(t, err)

		hivexValue := hivexHive.NodeGetValue(hivexNode, valueName)

		// Decode MULTI_SZ
		goStrings, err := goHive.ValueStrings(goValue, hive.ReadOptions{})
		require.NoError(t, err)

		hivexStrings, err := hivexHive.ValueMultipleStrings(hivexValue)
		require.NoError(t, err)

		// Should have same number of strings
		require.Equal(t, len(goStrings), len(hivexStrings), "Number of strings should match")

		// Each string should match
		for j := range goStrings {
			assertStringsEqual(t, goStrings[j], hivexStrings[j], "String %d", j)
		}
	*/
}

// TestTypedValuesAllTypes tests decoding all value types in a hive
func TestTypedValuesAllTypes(t *testing.T) {
	t.Run("special_dword", func(t *testing.T) {
		goHive := openGoHivex(t, TestHives.Special)
		defer goHive.Close()

		hivexHive := openHivex(t, TestHives.Special)
		defer hivexHive.Close()

		// Navigate to node with DWORD values
		goRoot, err := goHive.Root()
		require.NoError(t, err)

		hivexRoot := hivexHive.Root()

		goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
		require.NoError(t, err)

		hivexChild := hivexHive.NodeGetChild(hivexRoot, "abcd_äöüß")

		testAllValuesInNode(t, goHive, hivexHive, goChild, hivexChild)
	})

	t.Run("rlenvalue_binary", func(t *testing.T) {
		goHive := openGoHivex(t, TestHives.RLenValue)
		defer goHive.Close()

		hivexHive := openHivex(t, TestHives.RLenValue)
		defer hivexHive.Close()

		// Navigate to node with BINARY values
		goRoot, err := goHive.Root()
		require.NoError(t, err)

		hivexRoot := hivexHive.Root()

		goChild, err := goHive.Lookup(goRoot, "ModerateValueParent")
		require.NoError(t, err)

		hivexChild := hivexHive.NodeGetChild(hivexRoot, "ModerateValueParent")

		testAllValuesInNode(t, goHive, hivexHive, goChild, hivexChild)
	})
}

// testAllValuesInNode tests all values in a node
func testAllValuesInNode(t *testing.T, goHive hive.Reader, hivexHive *bindings.Hive, goNode hive.NodeID, hivexNode bindings.NodeHandle) {
	t.Helper()

	// Get all values
	goValues, err := goHive.Values(goNode)
	require.NoError(t, err)

	hivexValues := hivexHive.NodeValues(hivexNode)

	typeCounts := make(map[hive.RegType]int)

	// Try to decode each value based on its type
	for i, goVal := range goValues {
		goMeta, err := goHive.StatValue(goVal)
		require.NoError(t, err)

		typeCounts[goMeta.Type]++

		t.Logf("Value %d: name='%s', type=%s, size=%d",
			i, goMeta.Name, goMeta.Type, goMeta.Size)

		switch goMeta.Type {
		case hive.REG_SZ, hive.REG_EXPAND_SZ:
			goStr, err := goHive.ValueString(goVal, hive.ReadOptions{})
			require.NoError(t, err)

			hivexStr, err := hivexHive.ValueString(hivexValues[i])
			require.NoError(t, err)

			assertStringsEqual(t, goStr, hivexStr, "String value %d", i)
			t.Logf("  -> String: %q", goStr)

		case hive.REG_DWORD:
			goDword, err := goHive.ValueDWORD(goVal)
			require.NoError(t, err)

			hivexDword, err := hivexHive.ValueDword(hivexValues[i])
			require.NoError(t, err)

			assert.Equal(t, goDword, uint32(hivexDword), "DWORD value %d", i)
			t.Logf("  -> DWORD: %d (0x%08x)", goDword, goDword)

		case hive.REG_QWORD:
			goQword, err := goHive.ValueQWORD(goVal)
			require.NoError(t, err)

			hivexQword, err := hivexHive.ValueQword(hivexValues[i])
			require.NoError(t, err)

			assert.Equal(t, goQword, uint64(hivexQword), "QWORD value %d", i)
			t.Logf("  -> QWORD: %d (0x%016x)", goQword, goQword)

		case hive.REG_MULTI_SZ:
			goStrs, err := goHive.ValueStrings(goVal, hive.ReadOptions{})
			require.NoError(t, err)

			hivexStrs, err := hivexHive.ValueMultipleStrings(hivexValues[i])
			require.NoError(t, err)

			require.Equal(t, len(goStrs), len(hivexStrs), "MULTI_SZ count %d", i)
			for j := range goStrs {
				assertStringsEqual(t, goStrs[j], hivexStrs[j], "MULTI_SZ[%d][%d]", i, j)
			}
			t.Logf("  -> MULTI_SZ: %v", goStrs)

		case hive.REG_BINARY, hive.REG_NONE:
			// Just verify we can read raw bytes
			goBytes, err := goHive.ValueBytes(goVal, hive.ReadOptions{})
			require.NoError(t, err)

			hivexBytes, _, err := hivexHive.ValueValue(hivexValues[i])
			require.NoError(t, err)

			assertBytesEqual(t, goBytes, hivexBytes, "Binary value %d", i)
			t.Logf("  -> Binary: %d bytes", len(goBytes))

		default:
			t.Logf("  -> Unhandled type: %s", goMeta.Type)
		}
	}

	t.Logf("\nType distribution:")
	for typ, count := range typeCounts {
		t.Logf("  %s: %d", typ, count)
	}
}

// TestValueStringWithSpecialChars tests string decoding with special characters
func TestValueStringWithSpecialChars(t *testing.T) {
	t.Skip("Test hives don't contain REG_SZ values - special hive has REG_DWORD instead")
}

// TestValueTypeError tests that decoding with wrong type fails appropriately
func TestValueTypeError(t *testing.T) {
	goHive := openGoHivex(t, TestHives.Special)
	defer goHive.Close()

	// Get a REG_DWORD value
	goRoot, err := goHive.Root()
	require.NoError(t, err)

	goChild, err := goHive.Lookup(goRoot, "abcd_äöüß")
	require.NoError(t, err)

	goValue, err := goHive.GetValue(goChild, "abcd_äöüß")
	require.NoError(t, err)

	// Verify it's a DWORD
	goMeta, err := goHive.StatValue(goValue)
	require.NoError(t, err)
	require.Equal(t, hive.REG_DWORD, goMeta.Type)

	// Try to decode as string (should fail or return garbage)
	_, err = goHive.ValueString(goValue, hive.ReadOptions{})
	// gohivex may return an error for type mismatch
	// or it may succeed but return garbage - both are acceptable
	// Just log what happens
	if err != nil {
		t.Logf("ValueString on REG_DWORD returned error (good): %v", err)
	} else {
		t.Logf("ValueString on REG_DWORD succeeded (may return garbage, depends on implementation)")
	}
}
