package repair

import (
	"bytes"
	"strings"
	"testing"
)

func TestTransactionLog_AddEntry(t *testing.T) {
	txLog := NewTransactionLog()

	oldData := []byte{0x01, 0x02, 0x03, 0x04}
	newData := []byte{0xFF, 0xFF, 0xFF, 0xFF}

	d := Diagnostic{
		Offset:  0x1000,
		Issue:   "test issue",
		Structure: "NK",
	}

	txLog.AddEntry(0x1000, oldData, newData, d, "TestModule")

	if txLog.TotalCount() != 1 {
		t.Errorf("expected 1 entry, got %d", txLog.TotalCount())
	}

	if txLog.AppliedCount() != 0 {
		t.Errorf("expected 0 applied, got %d", txLog.AppliedCount())
	}

	entries := txLog.GetEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Offset != 0x1000 {
		t.Errorf("expected offset 0x1000, got 0x%X", entry.Offset)
	}

	if entry.Size != 4 {
		t.Errorf("expected size 4, got %d", entry.Size)
	}

	if !bytes.Equal(entry.OldData, oldData) {
		t.Errorf("old data mismatch")
	}

	if !bytes.Equal(entry.NewData, newData) {
		t.Errorf("new data mismatch")
	}

	if entry.Applied {
		t.Errorf("expected Applied=false")
	}

	if entry.Module != "TestModule" {
		t.Errorf("expected module TestModule, got %s", entry.Module)
	}
}

func TestTransactionLog_MarkApplied(t *testing.T) {
	txLog := NewTransactionLog()

	d := Diagnostic{Offset: 0x1000}
	txLog.AddEntry(0x1000, []byte{0x01}, []byte{0x02}, d, "TestModule")

	// Mark as applied
	if err := txLog.MarkApplied(); err != nil {
		t.Fatalf("MarkApplied failed: %v", err)
	}

	if txLog.AppliedCount() != 1 {
		t.Errorf("expected 1 applied, got %d", txLog.AppliedCount())
	}

	entries := txLog.GetEntries()
	if !entries[0].Applied {
		t.Errorf("expected entry to be marked as applied")
	}
}

func TestTransactionLog_MarkApplied_Empty(t *testing.T) {
	txLog := NewTransactionLog()

	// Should error on empty log
	err := txLog.MarkApplied()
	if err == nil {
		t.Fatal("expected error when marking applied on empty log")
	}

	if _, ok := err.(*TransactionError); !ok {
		t.Errorf("expected TransactionError, got %T", err)
	}
}

func TestTransactionLog_Rollback(t *testing.T) {
	txLog := NewTransactionLog()

	// Create test data
	data := []byte{
		0x00, 0x00, 0x00, 0x00, // offset 0
		0x01, 0x01, 0x01, 0x01, // offset 4
		0x02, 0x02, 0x02, 0x02, // offset 8
	}

	// Add two repairs
	d1 := Diagnostic{Offset: 0}
	txLog.AddEntry(0, []byte{0x00, 0x00, 0x00, 0x00}, []byte{0xFF, 0xFF, 0xFF, 0xFF}, d1, "Module1")
	txLog.MarkApplied()

	d2 := Diagnostic{Offset: 8}
	txLog.AddEntry(8, []byte{0x02, 0x02, 0x02, 0x02}, []byte{0xAA, 0xAA, 0xAA, 0xAA}, d2, "Module2")
	txLog.MarkApplied()

	// Apply repairs
	copy(data[0:4], []byte{0xFF, 0xFF, 0xFF, 0xFF})
	copy(data[8:12], []byte{0xAA, 0xAA, 0xAA, 0xAA})

	// Verify repairs were applied
	if !bytes.Equal(data[0:4], []byte{0xFF, 0xFF, 0xFF, 0xFF}) {
		t.Fatal("repair 1 not applied")
	}
	if !bytes.Equal(data[8:12], []byte{0xAA, 0xAA, 0xAA, 0xAA}) {
		t.Fatal("repair 2 not applied")
	}

	// Rollback
	count, err := txLog.Rollback(data)
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 repairs rolled back, got %d", count)
	}

	// Verify original data restored
	expected := []byte{
		0x00, 0x00, 0x00, 0x00,
		0x01, 0x01, 0x01, 0x01,
		0x02, 0x02, 0x02, 0x02,
	}

	if !bytes.Equal(data, expected) {
		t.Errorf("data not properly rolled back\nexpected: %v\ngot:      %v", expected, data)
	}
}

func TestTransactionLog_Rollback_SkipsUnapplied(t *testing.T) {
	txLog := NewTransactionLog()

	data := []byte{0x00, 0x00, 0x00, 0x00}

	// Add one applied and one unapplied
	d1 := Diagnostic{Offset: 0}
	txLog.AddEntry(0, []byte{0x00, 0x00}, []byte{0xFF, 0xFF}, d1, "Module1")
	txLog.MarkApplied()

	d2 := Diagnostic{Offset: 2}
	txLog.AddEntry(2, []byte{0x00, 0x00}, []byte{0xAA, 0xAA}, d2, "Module2")
	// Don't mark as applied

	// Apply only first repair
	copy(data[0:2], []byte{0xFF, 0xFF})

	// Rollback
	count, err := txLog.Rollback(data)
	if err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 repair rolled back, got %d", count)
	}

	// Only first repair should be rolled back
	expected := []byte{0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(data, expected) {
		t.Errorf("data not properly rolled back\nexpected: %v\ngot:      %v", expected, data)
	}
}

func TestTransactionLog_Rollback_Empty(t *testing.T) {
	txLog := NewTransactionLog()
	data := []byte{0x00, 0x00, 0x00, 0x00}

	count, err := txLog.Rollback(data)
	if err != nil {
		t.Fatalf("rollback on empty log should not error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 rolled back, got %d", count)
	}
}

func TestTransactionLog_Rollback_InvalidOffset(t *testing.T) {
	txLog := NewTransactionLog()

	// Create entry with offset beyond data size
	d := Diagnostic{Offset: 1000}
	txLog.AddEntry(1000, []byte{0x00}, []byte{0xFF}, d, "Module")
	txLog.MarkApplied()

	data := []byte{0x00, 0x00, 0x00, 0x00}

	_, err := txLog.Rollback(data)
	if err == nil {
		t.Fatal("expected error for invalid offset")
	}

	if _, ok := err.(*TransactionError); !ok {
		t.Errorf("expected TransactionError, got %T", err)
	}
}

func TestTransactionLog_Export(t *testing.T) {
	txLog := NewTransactionLog()

	d := Diagnostic{
		Offset:  0x1000,
		Issue:   "test issue",
		Structure: "NK",
		Repair: &RepairAction{
			Description: "fix the issue",
		},
	}

	txLog.AddEntry(0x1000, []byte{0x01, 0x02}, []byte{0xFF, 0xFE}, d, "TestModule")
	txLog.MarkApplied()

	export := txLog.Export()

	// Check for key information in export
	if !strings.Contains(export, "APPLIED") {
		t.Error("export should contain APPLIED status")
	}

	if !strings.Contains(export, "TestModule") {
		t.Error("export should contain module name")
	}

	if !strings.Contains(export, "0x00001000") {
		t.Error("export should contain offset")
	}

	if !strings.Contains(export, "test issue") {
		t.Error("export should contain issue description")
	}

	if !strings.Contains(export, "fix the issue") {
		t.Error("export should contain repair description")
	}

	if !strings.Contains(export, "01 02") {
		t.Error("export should contain old data")
	}

	if !strings.Contains(export, "FF FE") {
		t.Error("export should contain new data")
	}
}

func TestTransactionLog_Export_Empty(t *testing.T) {
	txLog := NewTransactionLog()

	export := txLog.Export()

	if !strings.Contains(export, "empty") {
		t.Error("export of empty log should indicate empty")
	}
}

func TestTransactionLog_Clear(t *testing.T) {
	txLog := NewTransactionLog()

	d := Diagnostic{Offset: 0x1000}
	txLog.AddEntry(0x1000, []byte{0x01}, []byte{0x02}, d, "TestModule")

	if txLog.TotalCount() != 1 {
		t.Fatal("expected 1 entry before clear")
	}

	txLog.Clear()

	if txLog.TotalCount() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", txLog.TotalCount())
	}
}

func TestTransactionLog_MultipleEntries(t *testing.T) {
	txLog := NewTransactionLog()

	// Add multiple entries
	for i := 0; i < 10; i++ {
		d := Diagnostic{Offset: uint64(i * 4)}
		txLog.AddEntry(uint64(i*4), []byte{byte(i)}, []byte{0xFF}, d, "Module")
		if i%2 == 0 {
			txLog.MarkApplied()
		}
	}

	if txLog.TotalCount() != 10 {
		t.Errorf("expected 10 entries, got %d", txLog.TotalCount())
	}

	if txLog.AppliedCount() != 5 {
		t.Errorf("expected 5 applied, got %d", txLog.AppliedCount())
	}
}

func TestTransactionLog_DeepCopy(t *testing.T) {
	txLog := NewTransactionLog()

	oldData := []byte{0x01, 0x02, 0x03}
	newData := []byte{0xFF, 0xFE, 0xFD}

	d := Diagnostic{Offset: 0}
	txLog.AddEntry(0, oldData, newData, d, "Module")

	// Modify original slices
	oldData[0] = 0xAA
	newData[0] = 0xBB

	// Verify transaction log has unmodified copies
	entries := txLog.GetEntries()
	if entries[0].OldData[0] != 0x01 {
		t.Error("old data should not be affected by modifications to original slice")
	}
	if entries[0].NewData[0] != 0xFF {
		t.Error("new data should not be affected by modifications to original slice")
	}
}
