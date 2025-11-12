package edit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestDynamicBufferSizingAllRealHives tests buffer sizing with all real hive files.
func TestDynamicBufferSizingAllRealHives(t *testing.T) {
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

			r, err := reader.OpenBytes(data, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("OpenBytes: %v", err)
			}
			defer r.Close()

			// Get original metadata
			rootID, _ := r.Root()
			origMeta, _ := r.StatKey(rootID)

			// Round-trip through editor to test buffer sizing
			ed := edit.NewEditor(r)
			tx := ed.Begin()

			w := &writer.MemWriter{}
			if commitErr := tx.Commit(w, hive.WriteOptions{}); commitErr != nil {
				t.Fatalf("Commit: %v", commitErr)
			}

			t.Logf("Buffer sizing: input=%d bytes, output=%d bytes", len(data), len(w.Buf))

			// Verify the rebuilt hive is valid
			r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
			if err != nil {
				t.Fatalf("Reopen: %v", err)
			}
			defer r2.Close()

			rootID2, _ := r2.Root()
			newMeta, _ := r2.StatKey(rootID2)

			// Verify structure preserved
			if newMeta.SubkeyN != origMeta.SubkeyN {
				t.Errorf("Subkey count mismatch: %d != %d", origMeta.SubkeyN, newMeta.SubkeyN)
			}
			if newMeta.ValueN != origMeta.ValueN {
				t.Errorf("Value count mismatch: %d != %d", origMeta.ValueN, newMeta.ValueN)
			}
		})
	}
}

// TestDynamicBufferSizingManyKeys tests buffer sizing with many keys.
func TestDynamicBufferSizingManyKeys(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Create 100 keys to stress test buffer sizing
	for i := range 100 {
		keyName := formatKeyName(i)
		if createErr := tx.CreateKey(keyName, hive.CreateKeyOptions{}); createErr != nil {
			t.Fatalf("CreateKey %d: %v", i, createErr)
		}
	}

	w := &writer.MemWriter{}
	if commitErr := tx.Commit(w, hive.WriteOptions{}); commitErr != nil {
		t.Fatalf("Commit: %v", commitErr)
	}

	// Verify all keys are present
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer r2.Close()

	rootID, _ := r2.Root()
	meta, _ := r2.StatKey(rootID)
	if meta.SubkeyN != 100 {
		t.Errorf("Expected 100 subkeys, got %d", meta.SubkeyN)
	}

	t.Logf("100 keys result: %d bytes", len(w.Buf))
}

// TestDynamicBufferSizingManyValues tests buffer sizing with the rlenvalue real hive
// which contains multiple values of varying sizes.
func TestDynamicBufferSizingManyValues(t *testing.T) {
	data, err := os.ReadFile("../../testdata/rlenvalue_test_hive")
	if err != nil {
		t.Skipf("rlenvalue_test_hive not available: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Just round-trip to test buffer sizing with real multi-value hive
	w := &writer.MemWriter{}
	if commitErr := tx.Commit(w, hive.WriteOptions{}); commitErr != nil {
		t.Fatalf("Commit: %v", commitErr)
	}

	t.Logf("Real multi-value hive: input=%d bytes, output=%d bytes", len(data), len(w.Buf))

	// Verify the hive is valid
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer r2.Close()

	rootID, _ := r2.Root()
	rootID2, _ := r.Root()
	meta1, _ := r.StatKey(rootID2)
	meta2, _ := r2.StatKey(rootID)

	if meta1.SubkeyN != meta2.SubkeyN {
		t.Errorf("Subkey count mismatch: %d != %d", meta1.SubkeyN, meta2.SubkeyN)
	}
}

// TestDynamicBufferSizingDeepNesting tests buffer sizing with deeply nested keys.
func TestDynamicBufferSizingDeepNesting(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Create deeply nested keys (20 levels)
	path := ""
	for i := range 20 {
		if i > 0 {
			path += "\\"
		}
		path += formatKeyName(i)
		if createErr := tx.CreateKey(path, hive.CreateKeyOptions{CreateParents: true}); createErr != nil {
			t.Fatalf("CreateKey depth %d: %v", i, createErr)
		}
	}

	w := &writer.MemWriter{}
	if commitErr := tx.Commit(w, hive.WriteOptions{}); commitErr != nil {
		t.Fatalf("Commit: %v", commitErr)
	}

	// Verify the nested structure
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer r2.Close()

	// Walk down the tree to verify depth
	currentID, _ := r2.Root()
	depth := 0
	for depth < 20 {
		children, subErr := r2.Subkeys(currentID)
		if subErr != nil || len(children) == 0 {
			break
		}
		currentID = children[0]
		depth++
	}

	if depth != 20 {
		t.Errorf("Expected depth 20, got %d", depth)
	}

	t.Logf("Deep nesting (20 levels) result: %d bytes", len(w.Buf))
}

// formatKeyName formats a key name with a numeric suffix.
func formatKeyName(i int) string {
	return "Key" + string(rune('0'+i%10)) + string(rune('A'+i/10))
}
