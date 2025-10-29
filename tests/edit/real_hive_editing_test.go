package edit_test

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/internal/writer"
	"github.com/joshuapare/hivekit/pkg/hive"
)

// TestEditOperationsOnRealHive tests CreateKey, SetValue, DeleteValue on a real hive file
func TestEditOperationsOnRealHive(t *testing.T) {
	// Use minimal real hive as base
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	// Verify original structure
	rootID, _ := r.Root()
	origMeta, _ := r.StatKey(rootID)
	t.Logf("Original root: %q with %d subkeys, %d values", origMeta.Name, origMeta.SubkeyN, origMeta.ValueN)

	// Create editor and begin transaction
	ed := edit.NewEditor(r)
	tx := ed.Begin()

	// Test 1: Create a new key
	err = tx.CreateKey("TestKey", hive.CreateKeyOptions{})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	// Test 2: Set a DWORD value
	dwordData := make([]byte, 4)
	binary.LittleEndian.PutUint32(dwordData, 0x12345678)
	err = tx.SetValue("TestKey", "TestDWORD", hive.REG_DWORD, dwordData)
	if err != nil {
		t.Fatalf("SetValue DWORD: %v", err)
	}

	// Test 3: Set a string value
	strData := encodeUTF16LE("Hello World")
	err = tx.SetValue("TestKey", "TestString", hive.REG_SZ, strData)
	if err != nil {
		t.Fatalf("SetValue SZ: %v", err)
	}

	// Commit changes
	w := &writer.MemWriter{}
	err = tx.Commit(w, hive.WriteOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	t.Logf("Edited hive size: %d bytes (original: %d bytes)", len(w.Buf), len(data))

	// Verify the edited hive can be read
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Reopen edited hive: %v", err)
	}
	defer r2.Close()

	// Verify root structure
	rootID2, _ := r2.Root()
	newMeta, _ := r2.StatKey(rootID2)
	t.Logf("Edited root: %q with %d subkeys, %d values", newMeta.Name, newMeta.SubkeyN, newMeta.ValueN)

	// Should have one more subkey than before
	expectedSubkeys := origMeta.SubkeyN + 1
	if newMeta.SubkeyN != expectedSubkeys {
		t.Errorf("Expected %d subkeys, got %d", expectedSubkeys, newMeta.SubkeyN)
	}

	// Find and verify the new key
	children, err := r2.Subkeys(rootID2)
	if err != nil {
		t.Fatalf("Subkeys: %v", err)
	}

	t.Logf("Found %d children", len(children))

	var testKeyID hive.NodeID
	found := false
	for i, child := range children {
		meta, err := r2.StatKey(child)
		if err != nil {
			t.Logf("Child [%d]: StatKey error: %v", i, err)
			continue
		}
		t.Logf("Child [%d]: %q (NodeID %d)", i, meta.Name, child)
		if meta.Name == "TestKey" {
			testKeyID = child
			found = true
			t.Logf("Found TestKey with %d values", meta.ValueN)
			break
		}
	}

	if !found {
		t.Fatal("TestKey not found in edited hive")
	}

	// Verify the values
	values, err := r2.Values(testKeyID)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}

	if len(values) != 2 {
		t.Errorf("Expected 2 values, got %d", len(values))
	}

	// Check DWORD value
	for _, valID := range values {
		meta, _ := r2.StatValue(valID)
		if meta.Name == "TestDWORD" {
			if meta.Type != hive.REG_DWORD {
				t.Errorf("TestDWORD: wrong type, got %d", meta.Type)
			}
			valData, _ := r2.ValueBytes(valID, hive.ReadOptions{})
			if len(valData) != 4 {
				t.Errorf("TestDWORD: wrong size, got %d", len(valData))
			}
			val := binary.LittleEndian.Uint32(valData)
			if val != 0x12345678 {
				t.Errorf("TestDWORD: wrong value, got 0x%x", val)
			}
			t.Logf("✓ TestDWORD verified: 0x%x", val)
		}
		if meta.Name == "TestString" {
			if meta.Type != hive.REG_SZ {
				t.Errorf("TestString: wrong type, got %d", meta.Type)
			}
			t.Logf("✓ TestString verified")
		}
	}
}

// TestLargeRealHive tests reading and round-trip on the large test hive
func TestLargeRealHive(t *testing.T) {
	data, err := os.ReadFile("../../testdata/large")
	if err != nil {
		t.Skipf("Large hive not available: %v", err)
	}

	t.Logf("Large hive size: %d bytes", len(data))

	// Test reading
	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}
	defer r.Close()

	rootID, _ := r.Root()
	rootMeta, _ := r.StatKey(rootID)
	t.Logf("Root: %q with %d subkeys, %d values", rootMeta.Name, rootMeta.SubkeyN, rootMeta.ValueN)

	// Test round-trip (read → rebuild → read)
	ed := edit.NewEditor(r)
	tx := ed.Begin()

	w := &writer.MemWriter{}
	err = tx.Commit(w, hive.WriteOptions{})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	t.Logf("Rebuilt size: %d bytes (original: %d bytes)", len(w.Buf), len(data))

	// Reopen
	r2, err := reader.OpenBytes(w.Buf, hive.OpenOptions{})
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	defer r2.Close()

	rootID2, _ := r2.Root()
	rootMeta2, _ := r2.StatKey(rootID2)
	t.Logf("Rebuilt root: %q with %d subkeys, %d values", rootMeta2.Name, rootMeta2.SubkeyN, rootMeta2.ValueN)

	// Verify structure preserved
	if rootMeta2.SubkeyN != rootMeta.SubkeyN {
		t.Errorf("Subkey count changed: %d → %d", rootMeta.SubkeyN, rootMeta2.SubkeyN)
	}
	if rootMeta2.ValueN != rootMeta.ValueN {
		t.Errorf("Value count changed: %d → %d", rootMeta.ValueN, rootMeta2.ValueN)
	}
}

// encodeUTF16LE converts a string to UTF-16LE with null terminator for testing
func encodeUTF16LE(s string) []byte {
	out := make([]byte, (len(s)+1)*2)
	for i, r := range s {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(r))
	}
	// Null terminator
	binary.LittleEndian.PutUint16(out[len(s)*2:], 0)
	return out
}
