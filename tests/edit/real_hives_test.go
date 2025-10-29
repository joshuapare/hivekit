package edit_test

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// Test that we can successfully open and read real hive files
func TestReadRealHives(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		rootName string // Expected root key name (if known)
	}{
		{"minimal", "../../testdata/minimal", ""},
		{"special", "../../testdata/special", ""},
		{"rlenvalue", "../../testdata/rlenvalue_test_hive", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("OpenBytes: %v", err)
			}
			defer r.Close()

			// Get root
			rootID, err := r.Root()
			if err != nil {
				t.Fatalf("Root: %v", err)
			}
			t.Logf("Root ID: %v", rootID)

			// Get root metadata
			rootMeta, err := r.StatKey(rootID)
			if err != nil {
				t.Fatalf("StatKey root: %v", err)
			}
			t.Logf("Root name: %q", rootMeta.Name)
			t.Logf("Root has %d subkeys, %d values", rootMeta.SubkeyN, rootMeta.ValueN)

			// List some subkeys if present
			if rootMeta.SubkeyN > 0 {
				children, err := r.Subkeys(rootID)
				if err != nil {
					t.Fatalf("Subkeys: %v", err)
				}
				t.Logf("Root has %d subkeys:", len(children))
				for i, child := range children {
					childMeta, err := r.StatKey(child)
					if err != nil {
						t.Logf("  [%d]: error: %v", i, err)
					} else {
						t.Logf("  [%d]: %q (%d subkeys, %d values)", i, childMeta.Name, childMeta.SubkeyN, childMeta.ValueN)
					}
					if i >= 4 {
						t.Logf("  ... (%d more)", len(children)-i-1)
						break
					}
				}
			}
		})
	}
}

// Test round-trip: read real hive → commit with no changes → verify readable
func TestRoundTripRealHives(t *testing.T) {
	testCases := []string{
		"../../testdata/minimal",
		"../../testdata/special",
	}

	for _, file := range testCases {
		t.Run(file, func(t *testing.T) {
			// Read original
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("OpenBytes: %v", err)
			}
			defer r.Close()

			// Get original root info
			rootID, _ := r.Root()
			origMeta, _ := r.StatKey(rootID)
			t.Logf("Original: %q with %d subkeys, %d values", origMeta.Name, origMeta.SubkeyN, origMeta.ValueN)

			// Rebuild without changes
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			w := &writer.MemWriter{}
			err = tx.Commit(w, hive.WriteOptions{})
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}

			t.Logf("Rebuilt hive: %d bytes (original: %d bytes)", len(w.Buf), len(data))

			// Try to reopen rebuilt hive
			r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Reopen rebuilt: %v", err)
			}
			defer r2.Close()

			// Verify root
			rootID2, err := r2.Root()
			if err != nil {
				t.Fatalf("Root (rebuilt): %v", err)
			}

			rebuiltMeta, err := r2.StatKey(rootID2)
			if err != nil {
				t.Fatalf("StatKey root (rebuilt): %v", err)
			}

			t.Logf("Rebuilt: %q with %d subkeys, %d values", rebuiltMeta.Name, rebuiltMeta.SubkeyN, rebuiltMeta.ValueN)

			// Basic validation: same number of subkeys and values
			if rebuiltMeta.SubkeyN != origMeta.SubkeyN {
				t.Errorf("Subkey count mismatch: orig=%d, rebuilt=%d", origMeta.SubkeyN, rebuiltMeta.SubkeyN)
			}
			if rebuiltMeta.ValueN != origMeta.ValueN {
				t.Errorf("Value count mismatch: orig=%d, rebuilt=%d", origMeta.ValueN, rebuiltMeta.ValueN)
			}
		})
	}
}

// Test specific known content from the test files
func TestRLenValue(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rlenvalue_test_hive")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	// Find ModerateValueParent
	rootID, _ := r.Root()
	children, err := r.Subkeys(rootID)
	if err != nil {
		t.Fatalf("Subkeys: %v", err)
	}

	var parentID hive.NodeID
	foundParent := false
	for _, child := range children {
		meta, _ := r.StatKey(child)
		if meta.Name == "ModerateValueParent" {
			parentID = child
			foundParent = true
			break
		}
	}

	if !foundParent {
		t.Fatalf("ModerateValueParent not found")
	}

	// Get the "33Bytes" value
	values, err := r.Values(parentID)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	var foundValue bool
	var valueData []byte
	for _, valID := range values {
		meta, err := r.StatValue(valID)
		if err != nil {
			continue
		}
		if meta.Name == "33Bytes" {
			valueData, err = r.ValueBytes(valID, hive.ReadOptions{})
			if err != nil {
				t.Fatalf("ValueBytes: %v", err)
			}
			t.Logf("33Bytes value: type=%d, len=%d", meta.Type, len(valueData))
			foundValue = true
			break
		}
	}

	if !foundValue {
		t.Fatalf("33Bytes value not found")
	}

	// The value is named "33Bytes" and should contain 33 bytes
	expectedLen := 33
	if len(valueData) != expectedLen {
		t.Errorf("Expected length %d, got %d", expectedLen, len(valueData))
	}
}

func TestSpecialCharacters(t *testing.T) {
	data, err := os.ReadFile("../../testdata/special")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	rootID, _ := r.Root()
	rootMeta, _ := r.StatKey(rootID)

	// Should have 3 children
	if rootMeta.SubkeyN != 3 {
		t.Errorf("Expected 3 children, got %d", rootMeta.SubkeyN)
	}

	children, err := r.Subkeys(rootID)
	if err != nil {
		t.Fatalf("Subkeys: %v", err)
	}

	// Key names are stored as compressed ASCII (Latin-1/Windows-1252 encoding)
	// when the compressed flag is set, but DecodeKeyName converts them to UTF-8 strings
	expectedKeys := map[string]bool{
		"abcd_äöüß":   false, // Decoded from Windows-1252 to UTF-8
		"zero\x00key": false,
		"weird™":      false, // ™ = 0x99 in Windows-1252
	}

	for _, child := range children {
		meta, _ := r.StatKey(child)
		t.Logf("Found child: %q (%d values)", meta.Name, meta.ValueN)

		if _, ok := expectedKeys[meta.Name]; ok {
			expectedKeys[meta.Name] = true
		}

		// Each should have exactly 1 value
		if meta.ValueN != 1 {
			t.Errorf("Child %q should have 1 value, got %d", meta.Name, meta.ValueN)
		}
	}

	// Verify all expected keys were found
	for key, found := range expectedKeys {
		if !found {
			t.Errorf("Expected key %q not found", key)
		}
	}
}
