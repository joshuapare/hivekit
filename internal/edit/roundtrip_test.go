package edit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/types"
)

// TestRoundTripAllValueTypes tests creating a hive from scratch with all registry value types,
// then reading it back to verify all data round-trips correctly.
func TestRoundTripAllValueTypes(t *testing.T) {
	tests := []struct {
		name     string
		typ      types.RegType
		makeData func() []byte
		validate func(t *testing.T, data []byte) // Optional custom validation
	}{
		{
			name: "REG_NONE",
			typ:  types.REG_NONE,
			makeData: func() []byte {
				return []byte{}
			},
		},
		{
			name: "REG_SZ",
			typ:  types.REG_SZ,
			makeData: func() []byte {
				s := "Test String Value"
				return encodeUTF16LE(s)
			},
		},
		{
			name: "REG_SZ_Empty",
			typ:  types.REG_SZ,
			makeData: func() []byte {
				return encodeUTF16LE("")
			},
		},
		{
			name: "REG_SZ_Unicode",
			typ:  types.REG_SZ,
			makeData: func() []byte {
				// Test Unicode: emoji, CJK, etc.
				s := "Hello 世界 🌍 Привет"
				return encodeUTF16LE(s)
			},
		},
		{
			name: "REG_EXPAND_SZ",
			typ:  types.REG_EXPAND_SZ,
			makeData: func() []byte {
				s := "%SystemRoot%\\System32"
				return encodeUTF16LE(s)
			},
		},
		{
			name: "REG_BINARY_Small",
			typ:  types.REG_BINARY,
			makeData: func() []byte {
				return []byte{0x01, 0x02, 0x03, 0x04, 0x05}
			},
		},
		{
			name: "REG_BINARY_Large",
			typ:  types.REG_BINARY,
			makeData: func() []byte {
				// Create 4KB of data
				data := make([]byte, 4096)
				for i := range data {
					data[i] = byte(i % 256)
				}
				return data
			},
		},
		{
			name: "REG_DWORD",
			typ:  types.REG_DWORD,
			makeData: func() []byte {
				data := make([]byte, 4)
				binary.LittleEndian.PutUint32(data, 0x12345678)
				return data
			},
		},
		{
			name: "REG_DWORD_Zero",
			typ:  types.REG_DWORD,
			makeData: func() []byte {
				data := make([]byte, 4)
				binary.LittleEndian.PutUint32(data, 0)
				return data
			},
		},
		{
			name: "REG_DWORD_Max",
			typ:  types.REG_DWORD,
			makeData: func() []byte {
				data := make([]byte, 4)
				binary.LittleEndian.PutUint32(data, 0xFFFFFFFF)
				return data
			},
		},
		{
			name: "REG_DWORD_BE",
			typ:  types.REG_DWORD_BE,
			makeData: func() []byte {
				data := make([]byte, 4)
				binary.BigEndian.PutUint32(data, 0x12345678)
				return data
			},
		},
		{
			name: "REG_LINK",
			typ:  types.REG_LINK,
			makeData: func() []byte {
				s := "\\Registry\\Machine\\Software\\Classes"
				return encodeUTF16LE(s)
			},
		},
		{
			name: "REG_MULTI_SZ",
			typ:  types.REG_MULTI_SZ,
			makeData: func() []byte {
				// Multiple null-terminated strings, ending with double null
				s := "String1\x00String2\x00String3\x00"
				return encodeUTF16LE(s)
			},
		},
		{
			name: "REG_MULTI_SZ_Empty",
			typ:  types.REG_MULTI_SZ,
			makeData: func() []byte {
				// Just double null
				return encodeUTF16LE("\x00")
			},
		},
		{
			name: "REG_MULTI_SZ_Unicode",
			typ:  types.REG_MULTI_SZ,
			makeData: func() []byte {
				s := "Hello\x00世界\x00🌍\x00"
				return encodeUTF16LE(s)
			},
		},
		{
			name: "REG_QWORD",
			typ:  types.REG_QWORD,
			makeData: func() []byte {
				data := make([]byte, 8)
				binary.LittleEndian.PutUint64(data, 0x123456789ABCDEF0)
				return data
			},
		},
		{
			name: "REG_QWORD_Zero",
			typ:  types.REG_QWORD,
			makeData: func() []byte {
				data := make([]byte, 8)
				binary.LittleEndian.PutUint64(data, 0)
				return data
			},
		},
		{
			name: "REG_QWORD_Max",
			typ:  types.REG_QWORD,
			makeData: func() []byte {
				data := make([]byte, 8)
				binary.LittleEndian.PutUint64(data, 0xFFFFFFFFFFFFFFFF)
				return data
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create hive from scratch
			ed := NewEditor(nil)
			tx := ed.Begin()

			// Create a test key
			if err := tx.CreateKey("TestKey", types.CreateKeyOptions{}); err != nil {
				t.Fatalf("CreateKey failed: %v", err)
			}

			// Set the test value
			data := tt.makeData()
			if err := tx.SetValue("TestKey", tt.name, tt.typ, data); err != nil {
				t.Fatalf("SetValue failed: %v", err)
			}

			// Commit to build the hive
			writer := &testWriter{}
			if err := tx.Commit(writer, types.WriteOptions{}); err != nil {
				t.Fatalf("Commit failed: %v", err)
			}

			if len(writer.data) == 0 {
				t.Fatal("Built hive is empty")
			}

			// Read the hive back
			r, err := reader.OpenBytes(writer.data, types.OpenOptions{})
			if err != nil {
				t.Fatalf("OpenBytes failed: %v", err)
			}
			defer r.Close()

			// Navigate to TestKey
			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Root failed: %v", err)
			}

			testKeyID, err := r.Lookup(rootID, "TestKey")
			if err != nil {
				t.Fatalf("Lookup TestKey failed: %v", err)
			}

			// Get the value
			values, err := r.Values(testKeyID)
			if err != nil {
				t.Fatalf("Values failed: %v", err)
			}

			if len(values) != 1 {
				t.Fatalf("Expected 1 value, got %d", len(values))
			}

			// Stat the value
			valMeta, err := r.StatValue(values[0])
			if err != nil {
				t.Fatalf("StatValue failed: %v", err)
			}

			// Verify value name
			if valMeta.Name != tt.name {
				t.Errorf("Value name: got %q, want %q", valMeta.Name, tt.name)
			}

			// Verify value type
			if valMeta.Type != tt.typ {
				t.Errorf("Value type: got %d (%s), want %d (%s)",
					valMeta.Type, valMeta.Type, tt.typ, tt.typ)
			}

			// Read value data
			readData, err := r.ValueBytes(values[0], types.ReadOptions{CopyData: true})
			if err != nil {
				t.Fatalf("ValueBytes failed: %v", err)
			}

			// Verify data matches
			if !bytes.Equal(data, readData) {
				t.Errorf("Data mismatch:\n  wrote: %d bytes %v\n  read:  %d bytes %v",
					len(data), data, len(readData), readData)
				// Show first difference
				minLen := len(data)
				if len(readData) < minLen {
					minLen = len(readData)
				}
				for i := 0; i < minLen; i++ {
					if data[i] != readData[i] {
						t.Errorf("  First difference at byte %d: wrote 0x%02x, read 0x%02x", i, data[i], readData[i])
						break
					}
				}
			}

			// Custom validation if provided
			if tt.validate != nil {
				tt.validate(t, readData)
			}

			t.Logf("✓ Round-trip successful: type=%s, size=%d bytes", tt.typ, len(data))
		})
	}
}

// TestRoundTripEdgeCases tests edge cases that might break the editor.
func TestRoundTripEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(tx types.Tx) error
		validate func(t *testing.T, r types.Reader)
	}{
		{
			name: "DeepNesting",
			setup: func(tx types.Tx) error {
				// Create 20 levels of nesting
				path := "Level0"
				for i := 1; i < 20; i++ {
					path = fmt.Sprintf("%s\\Level%d", path, i)
				}
				return tx.CreateKey(path, types.CreateKeyOptions{CreateParents: true})
			},
			validate: func(t *testing.T, r types.Reader) {
				// Verify we can navigate to the deepest level
				rootID, _ := r.Root()

				// First, look up Level0 under root
				currentID, err := r.Lookup(rootID, "Level0")
				if err != nil {
					t.Fatalf("Lookup Level0 failed: %v", err)
				}

				// Then navigate through Level1, Level2, ... Level19
				for i := 1; i < 20; i++ {
					segment := fmt.Sprintf("Level%d", i)
					nextID, err := r.Lookup(currentID, segment)
					if err != nil {
						t.Fatalf("Lookup %s failed at level %d: %v", segment, i, err)
					}
					currentID = nextID
				}
				t.Logf("✓ Successfully navigated 20 levels deep")
			},
		},
		{
			name: "ManyValues",
			setup: func(tx types.Tx) error {
				if err := tx.CreateKey("ManyValues", types.CreateKeyOptions{}); err != nil {
					return err
				}
				// Create 100 values in one key
				for i := 0; i < 100; i++ {
					valueName := fmt.Sprintf("Value%03d", i)
					data := make([]byte, 4)
					binary.LittleEndian.PutUint32(data, uint32(i))
					if err := tx.SetValue("ManyValues", valueName, types.REG_DWORD, data); err != nil {
						return err
					}
				}
				return nil
			},
			validate: func(t *testing.T, r types.Reader) {
				rootID, _ := r.Root()
				keyID, err := r.Lookup(rootID, "ManyValues")
				if err != nil {
					t.Fatalf("Lookup ManyValues failed: %v", err)
				}
				meta, err := r.StatKey(keyID)
				if err != nil {
					t.Fatalf("StatKey failed: %v", err)
				}
				if meta.ValueN != 100 {
					t.Errorf("Expected 100 values, got %d", meta.ValueN)
				}
				// Verify all values are present
				values, err := r.Values(keyID)
				if err != nil {
					t.Fatalf("Values failed: %v", err)
				}
				if len(values) != 100 {
					t.Fatalf("Expected 100 values, got %d", len(values))
				}

				// Collect all value names (order is not guaranteed)
				valueNames := make(map[string]bool)
				for _, valID := range values {
					meta, _ := r.StatValue(valID)
					valueNames[meta.Name] = true
				}

				// Verify all expected names exist
				missingCount := 0
				for i := 0; i < 100; i++ {
					expectedName := fmt.Sprintf("Value%03d", i)
					if !valueNames[expectedName] {
						t.Errorf("Missing value: %s", expectedName)
						missingCount++
						if missingCount >= 10 {
							t.Errorf("... and more (stopping after 10)")
							break
						}
					}
				}

				if missingCount == 0 {
					t.Logf("✓ All 100 values present and correct")
				}
			},
		},
		{
			name: "ManySubkeys",
			setup: func(tx types.Tx) error {
				if err := tx.CreateKey("ManySubkeys", types.CreateKeyOptions{}); err != nil {
					return err
				}
				// Create 100 subkeys
				for i := 0; i < 100; i++ {
					subkeyName := fmt.Sprintf("ManySubkeys\\Subkey%03d", i)
					if err := tx.CreateKey(subkeyName, types.CreateKeyOptions{CreateParents: true}); err != nil {
						return err
					}
				}
				return nil
			},
			validate: func(t *testing.T, r types.Reader) {
				rootID, _ := r.Root()
				keyID, err := r.Lookup(rootID, "ManySubkeys")
				if err != nil {
					t.Fatalf("Lookup ManySubkeys failed: %v", err)
				}
				meta, err := r.StatKey(keyID)
				if err != nil {
					t.Fatalf("StatKey failed: %v", err)
				}
				if meta.SubkeyN != 100 {
					t.Errorf("Expected 100 subkeys, got %d", meta.SubkeyN)
				}
				subkeys, err := r.Subkeys(keyID)
				if err != nil {
					t.Fatalf("Subkeys failed: %v", err)
				}
				if len(subkeys) != 100 {
					t.Fatalf("Expected 100 subkeys, got %d", len(subkeys))
				}
				t.Logf("✓ All 100 subkeys present")
			},
		},
		{
			name: "SpecialCharactersInNames",
			setup: func(tx types.Tx) error {
				// Test various special characters
				specialNames := []string{
					"Key with spaces",
					"Key-with-dashes",
					"Key_with_underscores",
					"Key.with.dots",
					"Key(with)parens",
					"Key[with]brackets",
					"Key{with}braces",
				}
				for _, name := range specialNames {
					if err := tx.CreateKey(name, types.CreateKeyOptions{}); err != nil {
						return fmt.Errorf("failed to create key %q: %w", name, err)
					}
					// Add a value with special chars too
					valueName := "Value with special: !@#$"
					data := []byte("test")
					if err := tx.SetValue(name, valueName, types.REG_BINARY, data); err != nil {
						return fmt.Errorf("failed to set value on key %q: %w", name, err)
					}
				}
				return nil
			},
			validate: func(t *testing.T, r types.Reader) {
				rootID, _ := r.Root()
				specialNames := []string{
					"Key with spaces",
					"Key-with-dashes",
					"Key_with_underscores",
					"Key.with.dots",
					"Key(with)parens",
					"Key[with]brackets",
					"Key{with}braces",
				}
				for _, name := range specialNames {
					keyID, err := r.Lookup(rootID, name)
					if err != nil {
						t.Errorf("Failed to lookup key %q: %v", name, err)
						continue
					}
					meta, err := r.StatKey(keyID)
					if err != nil {
						t.Errorf("Failed to stat key %q: %v", name, err)
						continue
					}
					if meta.Name != name {
						t.Errorf("Key name mismatch: got %q, want %q", meta.Name, name)
					}
				}
				t.Logf("✓ All special character keys readable")
			},
		},
		{
			name: "EmptyKeyName",
			setup: func(tx types.Tx) error {
				// Root always has empty name, this is valid
				// Just verify we can set values on root
				return tx.SetValue("", "RootValue", types.REG_DWORD, make([]byte, 4))
			},
			validate: func(t *testing.T, r types.Reader) {
				rootID, _ := r.Root()
				values, err := r.Values(rootID)
				if err != nil {
					t.Fatalf("Values on root failed: %v", err)
				}
				found := false
				for _, valID := range values {
					meta, _ := r.StatValue(valID)
					if meta.Name == "RootValue" {
						found = true
						break
					}
				}
				if !found {
					t.Error("RootValue not found on root key")
				}
				t.Logf("✓ Value on root (empty name key) works")
			},
		},
		{
			name: "LongKeyNames",
			setup: func(tx types.Tx) error {
				// Test very long key names (registry supports up to 255 chars)
				longName := strings.Repeat("A", 200)
				if err := tx.CreateKey(longName, types.CreateKeyOptions{}); err != nil {
					return err
				}
				// Add nested long name
				nestedLong := longName + "\\" + strings.Repeat("B", 200)
				return tx.CreateKey(nestedLong, types.CreateKeyOptions{CreateParents: true})
			},
			validate: func(t *testing.T, r types.Reader) {
				rootID, _ := r.Root()
				longName := strings.Repeat("A", 200)
				keyID, err := r.Lookup(rootID, longName)
				if err != nil {
					t.Fatalf("Lookup long name failed: %v", err)
				}
				nestedName := strings.Repeat("B", 200)
				_, err = r.Lookup(keyID, nestedName)
				if err != nil {
					t.Fatalf("Lookup nested long name failed: %v", err)
				}
				t.Logf("✓ Long key names (200 chars) work")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create hive
			ed := NewEditor(nil)
			tx := ed.Begin()

			// Setup the test case
			if err := tt.setup(tx); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			// Commit
			writer := &testWriter{}
			if err := tx.Commit(writer, types.WriteOptions{}); err != nil {
				t.Fatalf("Commit failed: %v", err)
			}

			// Read back
			r, err := reader.OpenBytes(writer.data, types.OpenOptions{})
			if err != nil {
				t.Fatalf("OpenBytes failed: %v", err)
			}
			defer r.Close()

			// Validate
			tt.validate(t, r)
		})
	}
}
