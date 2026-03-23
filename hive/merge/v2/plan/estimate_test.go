package plan_test

import (
	"testing"

	"github.com/joshuapare/hivekit/hive/merge/v2/plan"
	"github.com/joshuapare/hivekit/hive/merge/v2/trie"
	"github.com/joshuapare/hivekit/internal/format"
)

// buildRoot constructs a virtual root node with the given direct children,
// leaving the root itself in its zero state (Exists=false, no operations).
func buildRoot(children ...*trie.Node) *trie.Node {
	return &trie.Node{
		Name:     "",
		Children: children,
	}
}

// newNode creates a minimal trie node with the given name. Exists and CellIdx
// are left at their zero values (Exists=false, CellIdx=0).
func newNode(name string) *trie.Node {
	return &trie.Node{
		Name:      name,
		NameLower: name,
		CellIdx:   format.InvalidOffset,
	}
}

// newExistingNode creates a trie node that has been annotated as already
// present in the hive.
func newExistingNode(name string, subKeyCount, valueCount uint32) *trie.Node {
	return &trie.Node{
		Name:         name,
		NameLower:    name,
		Exists:       true,
		CellIdx:      0x20, // arbitrary non-invalid offset
		SubKeyCount:  subKeyCount,
		ValueCount:   valueCount,
		SubKeyListRef: 0x100,
		ValueListRef:  0x200,
	}
}

// TestEstimate_EmptyTrie verifies that an empty trie (root with no children)
// produces a zero-valued SpacePlan.
func TestEstimate_EmptyTrie(t *testing.T) {
	root := buildRoot()
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.TotalNewBytes != 0 {
		t.Errorf("expected TotalNewBytes=0, got %d", sp.TotalNewBytes)
	}
	if sp.NewNKCount != 0 {
		t.Errorf("expected NewNKCount=0, got %d", sp.NewNKCount)
	}
	if len(sp.Manifest) != 0 {
		t.Errorf("expected empty Manifest, got len=%d", len(sp.Manifest))
	}
}

// TestEstimate_NewKeysOnly verifies that a single node marked !Exists with an
// EnsureKey operation produces a non-zero TotalNewBytes and NewNKCount==1.
func TestEstimate_NewKeysOnly(t *testing.T) {
	child := newNode("Software")
	child.EnsureKey = true

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.TotalNewBytes <= 0 {
		t.Errorf("expected TotalNewBytes > 0, got %d", sp.TotalNewBytes)
	}
	if sp.NewNKCount != 1 {
		t.Errorf("expected NewNKCount=1, got %d", sp.NewNKCount)
	}

	// Manifest should contain exactly one NK entry.
	nkEntries := 0
	for _, e := range sp.Manifest {
		if e.Kind == plan.CellNK {
			nkEntries++
		}
	}
	if nkEntries != 1 {
		t.Errorf("expected 1 NK entry in Manifest, got %d", nkEntries)
	}
}

// TestEstimate_ExistingKeyNoAlloc verifies that a node marked Exists=true
// with no value operations does not cause an NK allocation.
func TestEstimate_ExistingKeyNoAlloc(t *testing.T) {
	child := newExistingNode("Software", 0, 0)
	child.EnsureKey = true

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.NewNKCount != 0 {
		t.Errorf("existing key should not allocate NK; got NewNKCount=%d", sp.NewNKCount)
	}
}

// TestEstimate_ValueOps verifies that non-delete ValueOps on a new node result
// in VK and data cell allocations being tracked.
func TestEstimate_ValueOps(t *testing.T) {
	child := newNode("Control")
	child.EnsureKey = true
	child.Values = []trie.ValueOp{
		{
			Name:   "Version",
			Type:   format.RegSz,
			Data:   []byte("hello world"), // 11 bytes — not inline
			Delete: false,
		},
	}

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.NewNKCount != 1 {
		t.Errorf("expected NewNKCount=1, got %d", sp.NewNKCount)
	}
	if sp.NewVKCount != 1 {
		t.Errorf("expected NewVKCount=1, got %d", sp.NewVKCount)
	}
	if sp.NewDataCount != 1 {
		t.Errorf("expected NewDataCount=1 (non-inline data), got %d", sp.NewDataCount)
	}
	if sp.TotalNewBytes <= 0 {
		t.Errorf("expected TotalNewBytes > 0, got %d", sp.TotalNewBytes)
	}
}

// TestEstimate_InlineDWORD verifies that a value with 4-byte data (inline
// DWORD) does not produce a separate data cell allocation.
func TestEstimate_InlineDWORD(t *testing.T) {
	child := newNode("Config")
	child.EnsureKey = true
	child.Values = []trie.ValueOp{
		{
			Name:   "Flags",
			Type:   format.RegDword,
			Data:   []byte{0x01, 0x00, 0x00, 0x00}, // 4 bytes — inline
			Delete: false,
		},
	}

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.NewVKCount != 1 {
		t.Errorf("expected NewVKCount=1, got %d", sp.NewVKCount)
	}
	// 4-byte data is stored inline in the VK DataOffset field — no separate cell.
	if sp.NewDataCount != 0 {
		t.Errorf("expected NewDataCount=0 for inline DWORD, got %d", sp.NewDataCount)
	}
}

// TestEstimate_SubkeyListRebuild verifies that an existing parent with at
// least one new (non-existing) child causes ListRebuilds to be incremented.
func TestEstimate_SubkeyListRebuild(t *testing.T) {
	// Parent exists in hive with 2 existing subkeys.
	parent := newExistingNode("Software", 2, 0)

	// One new child that does not exist in the hive.
	newChild := newNode("NewApp")
	newChild.EnsureKey = true
	parent.Children = []*trie.Node{newChild}

	root := buildRoot(parent)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.ListRebuilds < 1 {
		t.Errorf("expected ListRebuilds >= 1, got %d", sp.ListRebuilds)
	}

	// The manifest should include a subkey list entry.
	skEntries := 0
	for _, e := range sp.Manifest {
		if e.Kind == plan.CellSubkeyList {
			skEntries++
		}
	}
	if skEntries < 1 {
		t.Errorf("expected >= 1 CellSubkeyList entry in Manifest, got %d", skEntries)
	}
}

// TestEstimate_NewKeyWithValues verifies that a new key (!Exists) with non-delete
// value operations includes a CellValueList entry in the manifest and accounts
// for the value list size in TotalNewBytes.
func TestEstimate_NewKeyWithValues(t *testing.T) {
	child := newNode("AppKey")
	child.EnsureKey = true
	child.Values = []trie.ValueOp{
		{Name: "Val1", Type: format.RegSz, Data: []byte("hello world"), Delete: false},
		{Name: "Val2", Type: format.RegSz, Data: []byte("second value"), Delete: false},
		{Name: "Val3", Type: format.RegDword, Data: []byte{0x01, 0x00, 0x00, 0x00}, Delete: false},
	}

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}

	// Should have 3 VK cells.
	if sp.NewVKCount != 3 {
		t.Errorf("expected NewVKCount=3, got %d", sp.NewVKCount)
	}

	// Manifest must contain a CellValueList entry for the new key's value list.
	vlistEntries := 0
	for _, e := range sp.Manifest {
		if e.Kind == plan.CellValueList {
			vlistEntries++
		}
	}
	if vlistEntries != 1 {
		t.Errorf("expected 1 CellValueList entry in Manifest for new key with values, got %d", vlistEntries)
	}

	// ListRebuilds should be >= 1 (the value list rebuild).
	if sp.ListRebuilds < 1 {
		t.Errorf("expected ListRebuilds >= 1, got %d", sp.ListRebuilds)
	}

	// TotalNewBytes must include the value list cell. Compute the expected
	// value list size: align8(CellHeaderSize + valueListEntrySize * 3).
	expectedVListSize := format.Align8(format.CellHeaderSize + format.OffsetFieldSize*3)
	if int(sp.TotalNewBytes) < expectedVListSize {
		t.Errorf("TotalNewBytes %d is less than the expected value list size alone (%d)",
			sp.TotalNewBytes, expectedVListSize)
	}
}

// TestEstimate_NewKeyDeleteOnlyValues verifies that a new key with only delete
// value operations does NOT produce a CellValueList entry.
func TestEstimate_NewKeyDeleteOnlyValues(t *testing.T) {
	child := newNode("AppKey")
	child.EnsureKey = true
	child.Values = []trie.ValueOp{
		{Name: "OldVal", Delete: true},
	}

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}

	for _, e := range sp.Manifest {
		if e.Kind == plan.CellValueList {
			t.Error("delete-only value ops on new key should not produce CellValueList entry")
		}
	}
}

// TestEstimate_DeleteValueNoAlloc verifies that delete ValueOps do not produce
// VK or data cell allocations.
func TestEstimate_DeleteValueNoAlloc(t *testing.T) {
	child := newExistingNode("Control", 0, 1)
	child.Values = []trie.ValueOp{
		{
			Name:   "OldValue",
			Delete: true,
		},
	}

	root := buildRoot(child)
	sp, err := plan.Estimate(root)
	if err != nil {
		t.Fatalf("Estimate failed: %v", err)
	}
	if sp.NewVKCount != 0 {
		t.Errorf("delete op should not allocate VK; got NewVKCount=%d", sp.NewVKCount)
	}
	if sp.NewDataCount != 0 {
		t.Errorf("delete op should not allocate data; got NewDataCount=%d", sp.NewDataCount)
	}
}
