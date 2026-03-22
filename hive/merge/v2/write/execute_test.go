package write_test

import (
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/merge"
	"github.com/joshuapare/hivekit/hive/merge/v2/plan"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/hive/merge/v2/walk"
	"github.com/joshuapare/hivekit/hive/merge/v2/write"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil"
)

const largeHivePath = "testdata/large"

// TestExecute_CreateNewKey verifies that Execute creates a new NK cell for a
// trie node that does not exist in the hive.
func TestExecute_CreateNewKey(t *testing.T) {
	h, fa, cleanup := testutil.SetupTestHiveWithAllocator(t)
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"NewTestKey"}},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}

	if sp.TotalNewBytes > 0 {
		if bumpErr := fa.EnableBumpMode(sp.TotalNewBytes); bumpErr != nil {
			t.Fatalf("EnableBumpMode failed: %v", bumpErr)
		}
		defer fa.FinalizeBumpMode()
	}

	updates, stats, err := write.Execute(h, root, sp, fa)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if stats.KeysCreated != 1 {
		t.Errorf("KeysCreated: got %d, want 1", stats.KeysCreated)
	}

	// The new key node should have a valid CellIdx now.
	child := root.Children[0]
	if child.CellIdx == format.InvalidOffset || child.CellIdx == 0 {
		t.Errorf("child.CellIdx not set: 0x%X", child.CellIdx)
	}

	// Verify the NK cell is readable.
	payload, resolveErr := h.ResolveCellPayload(child.CellIdx)
	if resolveErr != nil {
		t.Fatalf("ResolveCellPayload failed: %v", resolveErr)
	}

	nk, parseErr := hive.ParseNK(payload)
	if parseErr != nil {
		t.Fatalf("ParseNK failed: %v", parseErr)
	}

	if string(nk.Name()) != "NewTestKey" {
		t.Errorf("NK name: got %q, want %q", string(nk.Name()), "NewTestKey")
	}

	// Should have in-place updates for the root's subkey list.
	if len(updates) == 0 {
		t.Error("expected in-place updates for subkey list rebuild")
	}
}

// TestExecute_CreateKeyWithValues verifies that Execute creates NK + VK cells
// for a new key with values.
func TestExecute_CreateKeyWithValues(t *testing.T) {
	h, fa, cleanup := testutil.SetupTestHiveWithAllocator(t)
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"ValKey"}},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ValKey"},
			ValueName: "Version",
			ValueType: format.REGDWORD,
			Data:      []byte{0x01, 0x00, 0x00, 0x00},
		},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}

	if sp.TotalNewBytes > 0 {
		if bumpErr := fa.EnableBumpMode(sp.TotalNewBytes); bumpErr != nil {
			t.Fatalf("EnableBumpMode failed: %v", bumpErr)
		}
		defer fa.FinalizeBumpMode()
	}

	_, stats, err := write.Execute(h, root, sp, fa)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if stats.KeysCreated != 1 {
		t.Errorf("KeysCreated: got %d, want 1", stats.KeysCreated)
	}
	if stats.ValuesSet != 1 {
		t.Errorf("ValuesSet: got %d, want 1", stats.ValuesSet)
	}

	// Verify the NK cell has value count = 1.
	child := root.Children[0]
	payload, resolveErr := h.ResolveCellPayload(child.CellIdx)
	if resolveErr != nil {
		t.Fatalf("ResolveCellPayload failed: %v", resolveErr)
	}

	_, parseErr := hive.ParseNK(payload)
	if parseErr != nil {
		t.Fatalf("ParseNK failed: %v", parseErr)
	}

	// Value count and list ref are set via the trie node's cached state
	// (they were updated during Execute).
	if child.ValueCount != 1 {
		t.Errorf("ValueCount: got %d, want 1", child.ValueCount)
	}

	// The VK cell should be readable too.
	if child.ValueListRef != format.InvalidOffset {
		vlPayload, vlErr := h.ResolveCellPayload(child.ValueListRef)
		if vlErr != nil {
			t.Fatalf("ResolveCellPayload for value list failed: %v", vlErr)
		}

		vkRef := format.ReadU32(vlPayload, 0)
		vkPayload, vkErr := h.ResolveCellPayload(vkRef)
		if vkErr != nil {
			t.Fatalf("ResolveCellPayload for VK failed: %v", vkErr)
		}

		vk, vkParseErr := hive.ParseVK(vkPayload)
		if vkParseErr != nil {
			t.Fatalf("ParseVK failed: %v", vkParseErr)
		}

		if string(vk.Name()) != "Version" {
			t.Errorf("VK name: got %q, want %q", string(vk.Name()), "Version")
		}

		if vk.Type() != format.REGDWORD {
			t.Errorf("VK type: got %d, want %d", vk.Type(), format.REGDWORD)
		}

		// Data should be inline (4 bytes for DWORD).
		if !vk.IsSmallData() {
			t.Fatalf("expected inline data for DWORD")
		}
		// DataLen returns the actual data length (3 bytes since [0x01, 0x00, 0x00, 0x00] is 4 bytes).
		if vk.DataLen() != 4 {
			t.Errorf("VK data len: got %d, want 4", vk.DataLen())
		}
	}
}

// TestExecute_CreateNestedKeys verifies Execute handles multi-level key creation.
func TestExecute_CreateNestedKeys(t *testing.T) {
	h, fa, cleanup := testutil.SetupTestHiveWithAllocator(t)
	defer cleanup()

	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"Parent", "Child"}},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}

	if sp.TotalNewBytes > 0 {
		if bumpErr := fa.EnableBumpMode(sp.TotalNewBytes); bumpErr != nil {
			t.Fatalf("EnableBumpMode failed: %v", bumpErr)
		}
		defer fa.FinalizeBumpMode()
	}

	_, stats, err := write.Execute(h, root, sp, fa)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if stats.KeysCreated != 2 {
		t.Errorf("KeysCreated: got %d, want 2", stats.KeysCreated)
	}

	// Both Parent and Child should have valid CellIdx.
	parentNode := root.Children[0]
	if parentNode.CellIdx == format.InvalidOffset {
		t.Error("Parent CellIdx not set")
	}

	if len(parentNode.Children) == 0 {
		t.Fatal("expected Parent to have children")
	}

	childNode := parentNode.Children[0]
	if childNode.CellIdx == format.InvalidOffset {
		t.Error("Child CellIdx not set")
	}

	// Verify both NK cells are readable.
	parentPayload, err := h.ResolveCellPayload(parentNode.CellIdx)
	if err != nil {
		t.Fatalf("resolve Parent NK: %v", err)
	}
	parentNK, err := hive.ParseNK(parentPayload)
	if err != nil {
		t.Fatalf("parse Parent NK: %v", err)
	}
	if string(parentNK.Name()) != "Parent" {
		t.Errorf("Parent name: got %q, want %q", string(parentNK.Name()), "Parent")
	}

	childPayload, err := h.ResolveCellPayload(childNode.CellIdx)
	if err != nil {
		t.Fatalf("resolve Child NK: %v", err)
	}
	childNK, err := hive.ParseNK(childPayload)
	if err != nil {
		t.Fatalf("parse Child NK: %v", err)
	}
	if string(childNK.Name()) != "Child" {
		t.Errorf("Child name: got %q, want %q", string(childNK.Name()), "Child")
	}
}

// TestExecute_EmptyTrie verifies Execute handles an empty trie gracefully.
func TestExecute_EmptyTrie(t *testing.T) {
	h, fa, cleanup := testutil.SetupTestHiveWithAllocator(t)
	defer cleanup()

	root := &trie.Node{}

	updates, stats, err := write.Execute(h, root, &plan.SpacePlan{}, fa)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(updates) != 0 {
		t.Errorf("expected no updates, got %d", len(updates))
	}
	if stats.KeysCreated != 0 {
		t.Errorf("expected no keys created, got %d", stats.KeysCreated)
	}
}

// TestExecute_ValueWithExternalData tests creating a value with data larger
// than 4 bytes (external storage).
func TestExecute_ValueWithExternalData(t *testing.T) {
	h, fa, cleanup := testutil.SetupTestHiveWithAllocator(t)
	defer cleanup()

	longData := []byte("This is a string value that is definitely longer than 4 bytes")
	ops := []merge.Op{
		{Type: merge.OpEnsureKey, KeyPath: []string{"ExtKey"}},
		{
			Type:      merge.OpSetValue,
			KeyPath:   []string{"ExtKey"},
			ValueName: "LongValue",
			ValueType: format.REGSZ,
			Data:      longData,
		},
	}
	root := trie.Build(ops)

	if err := walk.Annotate(h, root); err != nil {
		t.Fatalf("Annotate failed: %v", err)
	}

	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}

	if sp.TotalNewBytes > 0 {
		if bumpErr := fa.EnableBumpMode(sp.TotalNewBytes); bumpErr != nil {
			t.Fatalf("EnableBumpMode failed: %v", bumpErr)
		}
		defer fa.FinalizeBumpMode()
	}

	_, stats, err := write.Execute(h, root, sp, fa)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if stats.KeysCreated != 1 {
		t.Errorf("KeysCreated: got %d, want 1", stats.KeysCreated)
	}
	if stats.ValuesSet != 1 {
		t.Errorf("ValuesSet: got %d, want 1", stats.ValuesSet)
	}

	// Verify the VK cell references external data.
	child := root.Children[0]
	if child.ValueListRef == format.InvalidOffset {
		t.Fatal("expected value list ref to be set")
	}

	vlPayload, vlErr := h.ResolveCellPayload(child.ValueListRef)
	if vlErr != nil {
		t.Fatalf("resolve value list: %v", vlErr)
	}

	vkRef := format.ReadU32(vlPayload, 0)
	vkPayload, vkErr := h.ResolveCellPayload(vkRef)
	if vkErr != nil {
		t.Fatalf("resolve VK: %v", vkErr)
	}

	vk, parseErr := hive.ParseVK(vkPayload)
	if parseErr != nil {
		t.Fatalf("parse VK: %v", parseErr)
	}

	if vk.IsSmallData() {
		t.Error("expected external data, got inline")
	}

	if vk.DataLen() != len(longData) {
		t.Errorf("data len: got %d, want %d", vk.DataLen(), len(longData))
	}
}
