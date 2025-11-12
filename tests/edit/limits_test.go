package edit_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/internal/edit"
	"github.com/joshuapare/hivekit/internal/reader"
	"github.com/joshuapare/hivekit/pkg/ast"
	"github.com/joshuapare/hivekit/pkg/hive"
)

func TestLimitsEnforcement_MaxSubkeys(t *testing.T) {
	// Load minimal hive
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	// Create editor with strict limits
	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxSubkeys = 2 // Very restrictive
	tx := ed.BeginWithLimits(limits)

	// Add 3 root-level keys (exceeds limit of 2)
	if createErr := tx.CreateKey("Key1", hive.CreateKeyOptions{}); createErr != nil {
		t.Fatalf("Failed to create key1: %v", createErr)
	}
	if create2Err := tx.CreateKey("Key2", hive.CreateKeyOptions{}); create2Err != nil {
		t.Fatalf("Failed to create key2: %v", create2Err)
	}
	if create3Err := tx.CreateKey("Key3", hive.CreateKeyOptions{}); create3Err != nil {
		t.Fatalf("Failed to create key3: %v", create3Err)
	}

	// Commit should fail due to limit violation
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxSubkeys limit")
	}

	// Verify it's a limit violation error
	if !strings.Contains(err.Error(), "MaxSubkeys") {
		t.Errorf("Expected MaxSubkeys error, got: %v", err)
	}
}

func TestLimitsEnforcement_MaxValues(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxValues = 2
	tx := ed.BeginWithLimits(limits)

	// Create a key
	if createErr := tx.CreateKey("TestKey", hive.CreateKeyOptions{}); createErr != nil {
		t.Fatalf("Failed to create key: %v", createErr)
	}

	// Add 3 values (exceeds limit of 2)
	tx.SetValue("TestKey", "Value1", hive.REG_SZ, []byte("data1"))
	tx.SetValue("TestKey", "Value2", hive.REG_SZ, []byte("data2"))
	tx.SetValue("TestKey", "Value3", hive.REG_SZ, []byte("data3"))

	// Commit should fail
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxValues limit")
	}

	if !strings.Contains(err.Error(), "MaxValues") {
		t.Errorf("Expected MaxValues error, got: %v", err)
	}
}

func TestLimitsEnforcement_MaxValueSize(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxValueSize = 100 // Only 100 bytes allowed
	tx := ed.BeginWithLimits(limits)

	if createErr := tx.CreateKey("TestKey", hive.CreateKeyOptions{}); createErr != nil {
		t.Fatalf("Failed to create key: %v", createErr)
	}

	// Add a value larger than limit
	largeData := make([]byte, 101)
	tx.SetValue("TestKey", "LargeValue", hive.REG_BINARY, largeData)

	// Commit should fail
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxValueSize limit")
	}

	if !strings.Contains(err.Error(), "MaxValueSize") {
		t.Errorf("Expected MaxValueSize error, got: %v", err)
	}
}

func TestLimitsEnforcement_MaxTreeDepth(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxTreeDepth = 3 // Very shallow
	tx := ed.BeginWithLimits(limits)

	// Create a deep tree: Software -> Level1 -> Level2 -> Level3 -> Level4
	// This will exceed depth of 3
	if createErr := tx.CreateKey("Level1\\Level2\\Level3\\Level4", hive.CreateKeyOptions{CreateParents: true}); createErr != nil {
		t.Fatalf("Failed to create keys: %v", createErr)
	}

	// Commit should fail
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxTreeDepth limit")
	}

	if !strings.Contains(err.Error(), "MaxTreeDepth") {
		t.Errorf("Expected MaxTreeDepth error, got: %v", err)
	}
}

func TestLimitsEnforcement_DefaultLimits(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.Begin() // Uses default limits

	// Normal operations should succeed with default limits
	if createErr := tx.CreateKey("TestKey", hive.CreateKeyOptions{}); createErr != nil {
		t.Fatalf("Failed to create key: %v", createErr)
	}

	tx.SetValue("TestKey", "TestValue", hive.REG_SZ, []byte("test data"))

	// Commit should succeed
	buf := &bytes.Buffer{}
	if commitErr := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); commitErr != nil {
		t.Fatalf("Commit failed with default limits: %v", commitErr)
	}

	// Verify result is valid
	if buf.Len() == 0 {
		t.Error("Expected non-empty buffer after commit")
	}
}

func TestLimitsEnforcement_RelaxedLimits(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	tx := ed.BeginWithLimits(ast.RelaxedLimits())

	// Add many subkeys (would fail with strict limits)
	for i := range 100 {
		path := "Key" + string(rune('0'+i%10)) + string(rune('0'+i/10))
		if createErr := tx.CreateKey(path, hive.CreateKeyOptions{}); createErr != nil {
			t.Fatalf("Failed to create key %d: %v", i, createErr)
		}
	}

	// Should succeed with relaxed limits
	buf := &bytes.Buffer{}
	if commitErr := tx.Commit(&bufWriter{buf}, hive.WriteOptions{}); commitErr != nil {
		t.Fatalf("Commit failed with relaxed limits: %v", commitErr)
	}
}

func TestLimitsEnforcement_KeyNameLength(t *testing.T) {
	data, err := os.ReadFile("../../testdata/minimal")
	if err != nil {
		t.Fatal(err)
	}

	r, err := reader.OpenBytes(data, hive.OpenOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ed := edit.NewEditor(r)
	limits := ast.StrictLimits()
	limits.MaxKeyNameLen = 10
	tx := ed.BeginWithLimits(limits)

	// Try to create a key with name longer than 10 characters
	longName := strings.Repeat("a", 11)
	if createErr := tx.CreateKey(longName, hive.CreateKeyOptions{}); createErr != nil {
		t.Fatalf("Failed to create key: %v", createErr)
	}

	// Commit should fail
	buf := &bytes.Buffer{}
	err = tx.Commit(&bufWriter{buf}, hive.WriteOptions{})
	if err == nil {
		t.Fatal("Expected commit to fail due to MaxKeyNameLen limit")
	}

	if !strings.Contains(err.Error(), "MaxKeyNameLen") {
		t.Errorf("Expected MaxKeyNameLen error, got: %v", err)
	}
}

// bufWriter implements hive.Writer.
type bufWriter struct {
	buf *bytes.Buffer
}

func (w *bufWriter) WriteHive(data []byte) error {
	_, err := w.buf.Write(data)
	return err
}
