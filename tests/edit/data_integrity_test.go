package edit_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestDataIntegrityAllRealHives verifies that rebuilding preserves ALL data
// This is critical - smaller output is only acceptable if no data is lost!
func TestDataIntegrityAllRealHives(t *testing.T) {
	testFiles := []string{
		"../../testdata/minimal",
		"../../testdata/special",
		"../../testdata/rlenvalue_test_hive",
		"../../testdata/large",
	}

	for _, testFile := range testFiles {
		t.Run(filepath.Base(testFile), func(t *testing.T) {
			data, err := os.ReadFile(testFile)
			if err != nil {
				t.Skipf("File not available: %v", err)
			}

			// Open original hive
			r1, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("OpenBytes original: %v", err)
			}
			defer r1.Close()

			// Rebuild through editor
			ed := edit.NewEditor(r1)
			tx := ed.Begin()

			w := &writer.MemWriter{}
			if commitErr := tx.Commit(w, hive.WriteOptions{}); commitErr != nil {
				t.Fatalf("Commit: %v", commitErr)
			}

			t.Logf("Size: input=%d bytes, output=%d bytes (%.1f%% of original)",
				len(data), len(w.Buf), float64(len(w.Buf))/float64(len(data))*100)

			// Open rebuilt hive
			r2, openErr := reader.OpenBytes(w.Buf, hive.OpenOptions{})
			if openErr != nil {
				t.Fatalf("OpenBytes rebuilt: %v", openErr)
			}
			defer r2.Close()

			// Verify complete tree structure matches
			rootID1, rootErr := r1.Root()
			if rootErr != nil {
				t.Fatalf("Failed to get original root: %v", rootErr)
			}

			rootID2, root2Err := r2.Root()
			if root2Err != nil {
				t.Fatalf("Failed to get rebuilt root: %v", root2Err)
			}

			if verifyErr := verifyTreeIntegrity(t, r1, r2, rootID1, rootID2, ""); verifyErr != nil {
				t.Fatalf("Data integrity check failed: %v", verifyErr)
			}

			t.Logf("✓ All data verified identical")
		})
	}
}

// verifyTreeIntegrity recursively verifies that two trees are identical.
func verifyTreeIntegrity(t *testing.T, r1, r2 hive.Reader, id1, id2 hive.NodeID, path string) error {
	// Get metadata for both keys
	meta1, err := r1.StatKey(id1)
	if err != nil {
		t.Logf("Error reading original key at path %q (ID %d): %v", path, id1, err)
		return err
	}

	meta2, err := r2.StatKey(id2)
	if err != nil {
		t.Logf("Error reading rebuilt key at path %q (ID %d): %v", path, id2, err)
		return err
	}

	// Verify key names match
	if meta1.Name != meta2.Name {
		t.Errorf("Path %s: name mismatch: %q != %q", path, meta1.Name, meta2.Name)
		return nil // Continue checking
	}

	// Verify subkey counts match
	if meta1.SubkeyN != meta2.SubkeyN {
		t.Errorf("Path %s: subkey count mismatch: %d != %d", path, meta1.SubkeyN, meta2.SubkeyN)
	}

	// Verify value counts match
	if meta1.ValueN != meta2.ValueN {
		t.Errorf("Path %s: value count mismatch: %d != %d", path, meta1.ValueN, meta2.ValueN)
	}

	// Verify all values match
	if valErr := verifyValues(t, r1, r2, id1, id2, path); valErr != nil {
		return valErr
	}

	// Recursively verify all subkeys
	children1, err := r1.Subkeys(id1)
	if err != nil {
		t.Logf("Error getting subkeys from original at path %q: %v", path, err)
		return err
	}

	children2, err := r2.Subkeys(id2)
	if err != nil {
		t.Logf("Error getting subkeys from rebuilt at path %q: %v", path, err)
		return err
	}

	if len(children1) != len(children2) {
		t.Errorf("Path %s: children count mismatch: %d != %d", path, len(children1), len(children2))
		return nil
	}

	// Build maps of children by name for comparison
	children1Map := make(map[string]hive.NodeID)
	for _, childID := range children1 {
		childMeta, _ := r1.StatKey(childID)
		children1Map[childMeta.Name] = childID
	}

	children2Map := make(map[string]hive.NodeID)
	for _, childID := range children2 {
		childMeta, _ := r2.StatKey(childID)
		children2Map[childMeta.Name] = childID
	}

	// Verify all children exist in both
	for name, childID1 := range children1Map {
		childID2, exists := children2Map[name]
		if !exists {
			t.Errorf("Path %s: child %q missing in rebuilt hive", path, name)
			continue
		}

		childPath := path
		if childPath != "" {
			childPath += "\\"
		}
		childPath += name

		// Recursively verify child
		if childErr := verifyTreeIntegrity(t, r1, r2, childID1, childID2, childPath); childErr != nil {
			return childErr
		}
	}

	// Check for extra children in rebuilt hive
	for name := range children2Map {
		if _, exists := children1Map[name]; !exists {
			t.Errorf("Path %s: extra child %q in rebuilt hive", path, name)
		}
	}

	return nil
}

// verifyValues verifies that all values in two keys match.
func verifyValues(t *testing.T, r1, r2 hive.Reader, id1, id2 hive.NodeID, path string) error {
	values1, err := r1.Values(id1)
	if err != nil && err.Error() != "no values" {
		return err
	}

	values2, err := r2.Values(id2)
	if err != nil && err.Error() != "no values" {
		return err
	}

	// Build maps of values by name
	values1Map := make(map[string]hive.ValueID)
	for _, valID := range values1 {
		valMeta, _ := r1.StatValue(valID)
		values1Map[valMeta.Name] = valID
	}

	values2Map := make(map[string]hive.ValueID)
	for _, valID := range values2 {
		valMeta, _ := r2.StatValue(valID)
		values2Map[valMeta.Name] = valID
	}

	// Verify all values exist in both
	for name, valID1 := range values1Map {
		valID2, exists := values2Map[name]
		if !exists {
			t.Errorf("Path %s: value %q missing in rebuilt hive", path, name)
			continue
		}

		// Verify value metadata
		valMeta1, _ := r1.StatValue(valID1)
		valMeta2, _ := r2.StatValue(valID2)

		if valMeta1.Type != valMeta2.Type {
			t.Errorf("Path %s, value %q: type mismatch: %d != %d",
				path, name, valMeta1.Type, valMeta2.Type)
		}

		if valMeta1.Size != valMeta2.Size {
			t.Errorf("Path %s, value %q: size mismatch: %d != %d",
				path, name, valMeta1.Size, valMeta2.Size)
		}

		// Verify value data
		data1, readErr := r1.ValueBytes(valID1, hive.ReadOptions{CopyData: true})
		if readErr != nil {
			t.Errorf("Path %s, value %q: failed to read original data: %v", path, name, readErr)
			continue
		}

		data2, read2Err := r2.ValueBytes(valID2, hive.ReadOptions{CopyData: true})
		if read2Err != nil {
			t.Errorf("Path %s, value %q: failed to read rebuilt data: %v", path, name, read2Err)
			continue
		}

		if !bytes.Equal(data1, data2) {
			t.Errorf("Path %s, value %q: data mismatch: %d bytes vs %d bytes",
				path, name, len(data1), len(data2))
			// Show first difference
			minLen := len(data1)
			if len(data2) < minLen {
				minLen = len(data2)
			}
			for i := range minLen {
				if data1[i] != data2[i] {
					t.Errorf("  First difference at byte %d: 0x%02x != 0x%02x", i, data1[i], data2[i])
					break
				}
			}
		}
	}

	// Check for extra values in rebuilt hive
	for name := range values2Map {
		if _, exists := values1Map[name]; !exists {
			t.Errorf("Path %s: extra value %q in rebuilt hive", path, name)
		}
	}

	return nil
}
