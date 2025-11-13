package builder

import (
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/pkg/types"
	"github.com/stretchr/testify/require"
)

// builderReaderTestCase defines a test case that builds a hive and verifies it using Reader API
type builderReaderTestCase struct {
	name        string
	description string
	buildFn     func(t *testing.T, b *Builder) error
	verifyFn    func(t *testing.T, h *hive.Hive)
}

// builderReaderTestCases contains all test cases for builder + reader verification
var builderReaderTestCases = []builderReaderTestCase{
	// ===== VALUE TYPE TESTS =====
	{
		name:        "REG_SZ_string_value",
		description: "Test SetString with GetString",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Software", "Test"}, "StringValue", "Hello World")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node, "NodeID should be non-zero")

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2, "Find and FindParts should return same NodeID")

			// Verify value can be read
			val, err := h.GetString("Software\\Test", "StringValue")
			require.NoError(t, err)
			require.Equal(t, "Hello World", val)
		},
	},
	{
		name:        "REG_EXPAND_SZ_expandable_string",
		description: "Test SetExpandString with GetString",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetExpandString([]string{"Software", "Test"}, "PathValue", "%SystemRoot%\\System32")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2, "Find and FindParts should return same NodeID")

			// GetString works for both REG_SZ and REG_EXPAND_SZ
			val, err := h.GetString("Software\\Test", "PathValue")
			require.NoError(t, err)
			require.Equal(t, "%SystemRoot%\\System32", val)

			// Verify type is REG_EXPAND_SZ
			meta, _, err := h.GetValue("Software\\Test", "PathValue")
			require.NoError(t, err)
			require.Equal(t, types.REG_EXPAND_SZ, meta.Type)
		},
	},
	{
		name:        "REG_DWORD_32bit_integer",
		description: "Test SetDWORD with GetDWORD",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetDWORD([]string{"Software", "Test"}, "DwordValue", 0xDEADBEEF)
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			val, err := h.GetDWORD("Software\\Test", "DwordValue")
			require.NoError(t, err)
			require.Equal(t, uint32(0xDEADBEEF), val)
		},
	},
	{
		name:        "REG_DWORD_BE_big_endian",
		description: "Test SetDWORDBigEndian with GetValue",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetDWORDBigEndian([]string{"Software", "Test"}, "DwordBE", 0x12345678)
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			meta, data, err := h.GetValue("Software\\Test", "DwordBE")
			require.NoError(t, err)
			require.Equal(t, types.REG_DWORD_BE, meta.Type)
			require.Equal(t, 4, len(data))
			// Verify big-endian encoding
			require.Equal(t, []byte{0x12, 0x34, 0x56, 0x78}, data)
		},
	},
	{
		name:        "REG_QWORD_64bit_integer",
		description: "Test SetQWORD with GetQWORD",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetQWORD([]string{"Software", "Test"}, "QwordValue", 0x123456789ABCDEF0)
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			val, err := h.GetQWORD("Software\\Test", "QwordValue")
			require.NoError(t, err)
			require.Equal(t, uint64(0x123456789ABCDEF0), val)
		},
	},
	{
		name:        "REG_BINARY_byte_array",
		description: "Test SetBinary with GetValue",
		buildFn: func(t *testing.T, b *Builder) error {
			data := []byte{0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD}
			return b.SetBinary([]string{"Software", "Test"}, "BinaryValue", data)
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			meta, data, err := h.GetValue("Software\\Test", "BinaryValue")
			require.NoError(t, err)
			require.Equal(t, types.REG_BINARY, meta.Type)
			require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD}, data)
		},
	},
	{
		name:        "REG_MULTI_SZ_string_array",
		description: "Test SetMultiString with GetMultiString",
		buildFn: func(t *testing.T, b *Builder) error {
			values := []string{"First", "Second", "Third", "Fourth"}
			return b.SetMultiString([]string{"Software", "Test"}, "MultiValue", values)
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			val, err := h.GetMultiString("Software\\Test", "MultiValue")
			require.NoError(t, err)
			require.Equal(t, []string{"First", "Second", "Third", "Fourth"}, val)
		},
	},
	{
		name:        "REG_NONE_no_type",
		description: "Test SetNone with GetValue",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetNone([]string{"Software", "Test"}, "NoneValue")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			meta, data, err := h.GetValue("Software\\Test", "NoneValue")
			require.NoError(t, err)
			require.Equal(t, types.REG_NONE, meta.Type)
			require.Equal(t, 0, len(data))
		},
	},
	{
		name:        "REG_LINK_symbolic_link",
		description: "Test SetLink with GetValue",
		buildFn: func(t *testing.T, b *Builder) error {
			linkData := []byte("\\Registry\\Machine\\Software\\Link")
			return b.SetLink([]string{"Software", "Test"}, "LinkValue", linkData)
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			meta, data, err := h.GetValue("Software\\Test", "LinkValue")
			require.NoError(t, err)
			require.Equal(t, types.REG_LINK, meta.Type)
			require.Equal(t, []byte("\\Registry\\Machine\\Software\\Link"), data)
		},
	},

	// ===== BASIC EDGE CASES =====
	{
		name:        "empty_string_value",
		description: "Test SetString with empty string",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Software", "Test"}, "EmptyString", "")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			val, err := h.GetString("Software\\Test", "EmptyString")
			require.NoError(t, err)
			require.Equal(t, "", val)
		},
	},
	{
		name:        "empty_binary_value",
		description: "Test SetBinary with empty byte array",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetBinary([]string{"Software", "Test"}, "EmptyBinary", []byte{})
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			meta, data, err := h.GetValue("Software\\Test", "EmptyBinary")
			require.NoError(t, err)
			require.Equal(t, types.REG_BINARY, meta.Type)
			require.Equal(t, 0, len(data))
		},
	},
	{
		name:        "default_value_empty_name",
		description: "Test setting the default value (empty name)",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Software", "Test"}, "", "Default Value")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			val, err := h.GetString("Software\\Test", "")
			require.NoError(t, err)
			require.Equal(t, "Default Value", val)
		},
	},
	{
		name:        "multiple_values_one_key",
		description: "Test setting multiple different values in one key",
		buildFn: func(t *testing.T, b *Builder) error {
			if err := b.SetString([]string{"Software", "Test"}, "Val1", "String"); err != nil {
				return err
			}
			if err := b.SetDWORD([]string{"Software", "Test"}, "Val2", 42); err != nil {
				return err
			}
			if err := b.SetQWORD([]string{"Software", "Test"}, "Val3", 999); err != nil {
				return err
			}
			if err := b.SetBinary([]string{"Software", "Test"}, "Val4", []byte{0xFF}); err != nil {
				return err
			}
			return b.SetMultiString([]string{"Software", "Test"}, "Val5", []string{"A", "B"})
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works
			node, err := h.Find("Software\\Test")
			require.NoError(t, err, "Find should locate the key")
			require.NotZero(t, node)

			// Verify FindParts works
			node2, err := h.FindParts([]string{"Software", "Test"})
			require.NoError(t, err, "FindParts should locate the key")
			require.Equal(t, node, node2)

			// Verify all values exist using ListValues
			values, err := h.ListValues("Software\\Test")
			require.NoError(t, err)
			require.Equal(t, 5, len(values))

			// Verify each value
			s, err := h.GetString("Software\\Test", "Val1")
			require.NoError(t, err)
			require.Equal(t, "String", s)

			d, err := h.GetDWORD("Software\\Test", "Val2")
			require.NoError(t, err)
			require.Equal(t, uint32(42), d)

			q, err := h.GetQWORD("Software\\Test", "Val3")
			require.NoError(t, err)
			require.Equal(t, uint64(999), q)

			_, bin, err := h.GetValue("Software\\Test", "Val4")
			require.NoError(t, err)
			require.Equal(t, []byte{0xFF}, bin)

			ms, err := h.GetMultiString("Software\\Test", "Val5")
			require.NoError(t, err)
			require.Equal(t, []string{"A", "B"}, ms)
		},
	},
	{
		name:        "nested_key_structure",
		description: "Test creating nested key hierarchy A\\B\\C\\D",
		buildFn: func(t *testing.T, b *Builder) error {
			// SetString automatically creates the entire path
			return b.SetString([]string{"A", "B", "C", "D"}, "DeepValue", "Nested")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Verify Find works with backslash path
			node1, err := h.Find("A\\B\\C\\D")
			require.NoError(t, err, "Find should locate deep nested key")
			require.NotZero(t, node1)

			// Verify FindParts works with same key
			node2, err := h.FindParts([]string{"A", "B", "C", "D"})
			require.NoError(t, err, "FindParts should locate deep nested key")
			require.Equal(t, node1, node2, "Find and FindParts should return same node")

			// Verify the value
			val, err := h.GetString("A\\B\\C\\D", "DeepValue")
			require.NoError(t, err)
			require.Equal(t, "Nested", val)

			// Verify parent keys exist using both methods
			_, err = h.Find("A")
			require.NoError(t, err, "Parent key A should exist")
			_, err = h.FindParts([]string{"A", "B"})
			require.NoError(t, err, "Parent key A\\B should exist")
			_, err = h.Find("A\\B\\C")
			require.NoError(t, err, "Parent key A\\B\\C should exist")
		},
	},

	// ===== COMMON PREFIX TESTS =====
	{
		name:        "prefix_HKLM_backslash",
		description: "Test building and finding with HKLM\\ prefix",
		buildFn: func(t *testing.T, b *Builder) error {
			// Builder strips HKLM\ prefix by default
			return b.SetString([]string{"Software", "Test"}, "Value", "Data")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Find should work with HKLM prefix (gets stripped)
			node1, err := h.Find("HKLM\\Software\\Test")
			require.NoError(t, err)

			// Find should work without prefix
			node2, err := h.Find("Software\\Test")
			require.NoError(t, err)

			// Both should return the same node
			require.Equal(t, node1, node2)

			// Value should be accessible
			val, err := h.GetString("HKLM\\Software\\Test", "Value")
			require.NoError(t, err)
			require.Equal(t, "Data", val)
		},
	},
	{
		name:        "prefix_HKEY_LOCAL_MACHINE_full",
		description: "Test with full HKEY_LOCAL_MACHINE prefix",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Software", "Test"}, "Value", "Data")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Find should work with full prefix
			node1, err := h.Find("HKEY_LOCAL_MACHINE\\Software\\Test")
			require.NoError(t, err)

			// Find should work without prefix
			node2, err := h.Find("Software\\Test")
			require.NoError(t, err)

			// Both should return the same node
			require.Equal(t, node1, node2)
		},
	},
	{
		name:        "prefix_forward_slash_separator",
		description: "Test Find with forward slash separator",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Software", "Microsoft", "Test"}, "Value", "Data")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Find should work with forward slashes
			node1, err := h.Find("Software/Microsoft/Test")
			require.NoError(t, err)

			// Find should work with backslashes
			node2, err := h.Find("Software\\Microsoft\\Test")
			require.NoError(t, err)

			// Both should return the same node
			require.Equal(t, node1, node2)

			// Mixed separators should also work
			node3, err := h.Find("Software/Microsoft\\Test")
			require.NoError(t, err)
			require.Equal(t, node1, node3)
		},
	},
	{
		name:        "prefix_no_prefix_clean",
		description: "Test building and finding without any prefix",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"MyKey", "SubKey"}, "Value", "Data")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Find should work
			node, err := h.Find("MyKey\\SubKey")
			require.NoError(t, err)
			require.NotZero(t, node)

			// FindParts should work
			node2, err := h.FindParts([]string{"MyKey", "SubKey"})
			require.NoError(t, err)
			require.Equal(t, node, node2)
		},
	},

	// ===== READER METHOD COVERAGE TESTS =====
	{
		name:        "find_findparts_equivalence",
		description: "Verify Find and FindParts return same NodeID",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Software", "Microsoft", "Windows"}, "Version", "10")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Find with string path
			node1, err := h.Find("Software\\Microsoft\\Windows")
			require.NoError(t, err)

			// FindParts with array
			node2, err := h.FindParts([]string{"Software", "Microsoft", "Windows"})
			require.NoError(t, err)

			// Should be identical
			require.Equal(t, node1, node2)

			// Test at different levels
			node3, err := h.Find("Software\\Microsoft")
			require.NoError(t, err)

			node4, err := h.FindParts([]string{"Software", "Microsoft"})
			require.NoError(t, err)

			require.Equal(t, node3, node4)
		},
	},
	{
		name:        "getkey_metadata_verification",
		description: "Verify GetKey returns correct metadata",
		buildFn: func(t *testing.T, b *Builder) error {
			// Create a key with 3 subkeys and 2 values
			if err := b.SetString([]string{"Parent"}, "Val1", "A"); err != nil {
				return err
			}
			if err := b.SetDWORD([]string{"Parent"}, "Val2", 10); err != nil {
				return err
			}
			if err := b.SetString([]string{"Parent", "Child1"}, "X", "Y"); err != nil {
				return err
			}
			if err := b.SetString([]string{"Parent", "Child2"}, "X", "Y"); err != nil {
				return err
			}
			return b.SetString([]string{"Parent", "Child3"}, "X", "Y")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			meta, err := h.GetKey("Parent")
			require.NoError(t, err)
			require.Equal(t, "Parent", meta.Name)
			require.Equal(t, 3, meta.SubkeyN, "Should have 3 subkeys")
			require.Equal(t, 2, meta.ValueN, "Should have 2 values")
			require.False(t, meta.LastWrite.IsZero(), "Should have timestamp")
		},
	},
	{
		name:        "listsubkeys_verification",
		description: "Verify ListSubkeys returns all child keys",
		buildFn: func(t *testing.T, b *Builder) error {
			if err := b.SetString([]string{"Root", "Alpha"}, "V", "1"); err != nil {
				return err
			}
			if err := b.SetString([]string{"Root", "Beta"}, "V", "2"); err != nil {
				return err
			}
			return b.SetString([]string{"Root", "Gamma"}, "V", "3")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			subkeys, err := h.ListSubkeys("Root")
			require.NoError(t, err)
			require.Equal(t, 3, len(subkeys))

			// Extract names
			names := make([]string, len(subkeys))
			for i, key := range subkeys {
				names[i] = key.Name
			}

			// Verify all names present (order may vary)
			require.Contains(t, names, "Alpha")
			require.Contains(t, names, "Beta")
			require.Contains(t, names, "Gamma")
		},
	},
	{
		name:        "listvalues_verification",
		description: "Verify ListValues returns all values in key",
		buildFn: func(t *testing.T, b *Builder) error {
			if err := b.SetString([]string{"Test"}, "Name", "John"); err != nil {
				return err
			}
			if err := b.SetDWORD([]string{"Test"}, "Age", 30); err != nil {
				return err
			}
			if err := b.SetQWORD([]string{"Test"}, "ID", 123456); err != nil {
				return err
			}
			if err := b.SetBinary([]string{"Test"}, "Data", []byte{0xAA}); err != nil {
				return err
			}
			return b.SetMultiString([]string{"Test"}, "Tags", []string{"A", "B"})
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			values, err := h.ListValues("Test")
			require.NoError(t, err)
			require.Equal(t, 5, len(values))

			// Extract names and types
			nameTypeMap := make(map[string]types.RegType)
			for _, val := range values {
				nameTypeMap[val.Name] = val.Type
			}

			require.Equal(t, types.REG_SZ, nameTypeMap["Name"])
			require.Equal(t, types.REG_DWORD, nameTypeMap["Age"])
			require.Equal(t, types.REG_QWORD, nameTypeMap["ID"])
			require.Equal(t, types.REG_BINARY, nameTypeMap["Data"])
			require.Equal(t, types.REG_MULTI_SZ, nameTypeMap["Tags"])
		},
	},
	{
		name:        "walk_tree_traversal",
		description: "Verify Walk traverses entire tree structure",
		buildFn: func(t *testing.T, b *Builder) error {
			// Build a small tree:
			// Root
			//   ├─ A
			//   │   └─ A1
			//   └─ B
			//       ├─ B1
			//       └─ B2
			if err := b.SetString([]string{"A", "A1"}, "V", "1"); err != nil {
				return err
			}
			if err := b.SetString([]string{"B", "B1"}, "V", "2"); err != nil {
				return err
			}
			return b.SetString([]string{"B", "B2"}, "V", "3")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Walk from root and count all nodes
			count := 0
			names := []string{}
			err := h.Walk("", func(_ types.NodeID, meta types.KeyMeta) error {
				count++
				names = append(names, meta.Name)
				return nil
			})
			require.NoError(t, err)
			// Should visit: root, A, A1, B, B1, B2 = 6 nodes
			require.Equal(t, 6, count)
			require.Contains(t, names, "A")
			require.Contains(t, names, "A1")
			require.Contains(t, names, "B")
			require.Contains(t, names, "B1")
			require.Contains(t, names, "B2")

			// Walk from "B" should only visit B, B1, B2
			countB := 0
			err = h.Walk("B", func(_ types.NodeID, meta types.KeyMeta) error {
				countB++
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, 3, countB)
		},
	},
	{
		name:        "lookup_by_parts_navigation",
		description: "Test navigating key hierarchy using FindParts",
		buildFn: func(t *testing.T, b *Builder) error {
			return b.SetString([]string{"Level1", "Level2", "Level3", "Level4"}, "Deep", "Value")
		},
		verifyFn: func(t *testing.T, h *hive.Hive) {
			// Navigate step by step using FindParts
			level1, err := h.FindParts([]string{"Level1"})
			require.NoError(t, err)

			level2, err := h.FindParts([]string{"Level1", "Level2"})
			require.NoError(t, err)
			require.NotEqual(t, level1, level2)

			level3, err := h.FindParts([]string{"Level1", "Level2", "Level3"})
			require.NoError(t, err)
			require.NotEqual(t, level2, level3)

			level4, err := h.FindParts([]string{"Level1", "Level2", "Level3", "Level4"})
			require.NoError(t, err)
			require.NotEqual(t, level3, level4)

			// Verify we can read the value
			val, err := h.GetString("Level1\\Level2\\Level3\\Level4", "Deep")
			require.NoError(t, err)
			require.Equal(t, "Value", val)
		},
	},
}

// TestBuilder_ReaderVerification runs all builder + reader verification tests
func TestBuilder_ReaderVerification(t *testing.T) {
	for _, tc := range builderReaderTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary hive file
			tempDir := t.TempDir()
			hivePath := filepath.Join(tempDir, "test.hive")

			// Build the hive
			b, err := New(hivePath, DefaultOptions())
			require.NoError(t, err, "Failed to create builder")

			err = tc.buildFn(t, b)
			require.NoError(t, err, "Build function failed")

			err = b.Commit()
			require.NoError(t, err, "Failed to commit")

			// Close builder to flush all data
			err = b.Close()
			require.NoError(t, err, "Failed to close builder")

			// Reopen using Reader API
			h, err := hive.Open(hivePath)
			require.NoError(t, err, "Failed to open hive for reading")
			defer h.Close()

			// Run verification
			tc.verifyFn(t, h)
		})
	}
}
