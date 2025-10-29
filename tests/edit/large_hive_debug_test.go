package edit_test

import (
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestLargeHiveOriginalSubkeys tests if we can read subkeys from the original large hive
func TestLargeHiveOriginalSubkeys(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Skipf("Large hive not available: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	rootID, err := r.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	meta, err := r.StatKey(rootID)
	if err != nil {
		t.Fatalf("StatKey: %v", err)
	}

	t.Logf("Root: %q with %d subkeys", meta.Name, meta.SubkeyN)

	// Try to read the subkeys
	children, err := r.Subkeys(rootID)
	if err != nil {
		t.Fatalf("Failed to read subkeys from ORIGINAL large hive: %v", err)
	}

	t.Logf("Successfully read %d subkeys from original large hive", len(children))

	// Try to read each child
	for i, childID := range children {
		childMeta, err := r.StatKey(childID)
		if err != nil {
			t.Errorf("Failed to read child %d: %v", i, err)
			continue
		}
		t.Logf("  Child %d: %q", i, childMeta.Name)
	}
}

// TestLargeHiveRebuiltSubkeys tests if we can read subkeys from the rebuilt large hive
func TestLargeHiveRebuiltSubkeys(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Skipf("Large hive not available: %v", err)
	}

	r1, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes original: %v", err)
	}
	defer r1.Close()

	// Rebuild
	ed := edit.NewEditor(r1)
	tx := ed.Begin()

	w := &writer.MemWriter{}
	if err := tx.Commit(w, hive.WriteOptions{}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	t.Logf("Rebuilt hive: %d bytes", len(w.Buf))

	// Try to read rebuilt hive
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes rebuilt: %v", err)
	}
	defer r2.Close()

	rootID, err := r2.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}

	meta, err := r2.StatKey(rootID)
	if err != nil {
		t.Fatalf("StatKey: %v", err)
	}

	t.Logf("Root: %q with %d subkeys", meta.Name, meta.SubkeyN)

	// Try to read the subkeys
	children, err := r2.Subkeys(rootID)
	if err != nil {
		t.Fatalf("Failed to read subkeys from REBUILT large hive: %v", err)
	}

	t.Logf("Successfully read %d subkeys from rebuilt large hive", len(children))

	// Try to read each child
	for i, childID := range children {
		childMeta, err := r2.StatKey(childID)
		if err != nil {
			t.Errorf("Failed to read child %d: %v", i, err)
			continue
		}
		t.Logf("  Child %d: %q", i, childMeta.Name)
	}
}
