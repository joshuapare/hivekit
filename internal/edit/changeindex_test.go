package edit

import (
	"testing"

	"github.com/joshuapare/hivekit/pkg/types"
)

// TestChangeIndex_HasExact tests exact path change detection
func TestChangeIndex_HasExact(t *testing.T) {
	// Create a mock transaction with some changes
	// Paths must be normalized (lowercase) as they would be in real transaction
	tx := &transaction{
		createdKeys: map[string]*keyNode{
			normalizePath(`Software\Test`): {exists: false, name: "Test"},
		},
		deletedKeys: map[string]bool{
			normalizePath(`Software\OldKey`): true,
		},
		setValues: map[valueKey]valueData{
			{path: normalizePath(`Software\Config`), name: "Setting"}: {typ: types.REG_SZ, data: []byte("value")},
		},
		deletedVals: map[valueKey]bool{
			{path: normalizePath(`Software\Legacy`), name: "OldSetting"}: true,
		},
	}

	idx := buildChangeIndex(tx)

	tests := []struct {
		path     string
		expected bool
	}{
		// Created keys
		{`Software\Test`, true},
		{`SOFTWARE\TEST`, true}, // case-insensitive

		// Deleted keys
		{`Software\OldKey`, true},

		// Value changes
		{`Software\Config`, true},
		{`Software\Legacy`, true},

		// No changes
		{`Software`, false},
		{`Software\NoChanges`, false},
		{`Other`, false},
		{``, false}, // root
	}

	for _, tc := range tests {
		got := idx.HasExact(normalizePath(tc.path))
		if got != tc.expected {
			t.Errorf("HasExact(%q) = %v, want %v", tc.path, got, tc.expected)
		}
	}
}

// TestChangeIndex_HasSubtree tests subtree change detection
func TestChangeIndex_HasSubtree(t *testing.T) {
	tx := &transaction{
		createdKeys: map[string]*keyNode{
			normalizePath(`A\B\C`):       {exists: false, name: "C"},
			normalizePath(`A\B\D`):       {exists: false, name: "D"},
			normalizePath(`X\Y\Z\Deep`): {exists: false, name: "Deep"},
		},
		deletedKeys: map[string]bool{
			normalizePath(`M\N\O`): true,
		},
		setValues: map[valueKey]valueData{
			{path: normalizePath(`P\Q\R`), name: "Val"}: {typ: types.REG_DWORD, data: []byte{1, 2, 3, 4}},
		},
		deletedVals: make(map[valueKey]bool),
	}

	idx := buildChangeIndex(tx)

	tests := []struct {
		path     string
		expected bool
		reason   string
	}{
		// Exact matches
		{`A\B\C`, true, "exact created key"},
		{`M\N\O`, true, "exact deleted key"},
		{`P\Q\R`, true, "exact value change"},

		// Ancestor paths with descendants changed
		{`A`, true, "ancestor of A\\B\\C"},
		{`A\B`, true, "ancestor of A\\B\\C and A\\B\\D"},
		{`X`, true, "ancestor of X\\Y\\Z\\Deep"},
		{`X\Y`, true, "ancestor of X\\Y\\Z\\Deep"},
		{`X\Y\Z`, true, "ancestor of X\\Y\\Z\\Deep"},
		{`M`, true, "ancestor of M\\N\\O"},
		{`M\N`, true, "ancestor of M\\N\\O"},
		{`P`, true, "ancestor of P\\Q\\R"},
		{`P\Q`, true, "ancestor of P\\Q\\R"},

		// Root has changes (descendants exist)
		{``, true, "root with descendants changed"},

		// No changes
		{`A\B\C\NoChild`, false, "no descendants"},
		{`Z`, false, "no changes at all"},
		{`A\NoSuchPath`, false, "sibling path with no changes"},
		{`M\N\O\Deeper`, false, "descendant of deleted key (but not in index)"},
	}

	for _, tc := range tests {
		got := idx.HasSubtree(normalizePath(tc.path))
		if got != tc.expected {
			t.Errorf("HasSubtree(%q) = %v, want %v (%s)", tc.path, got, tc.expected, tc.reason)
		}
	}
}

// TestChangeIndex_TypeSpecific tests type-specific change queries
func TestChangeIndex_TypeSpecific(t *testing.T) {
	tx := &transaction{
		createdKeys: map[string]*keyNode{
			normalizePath(`New\Key`): {exists: false, name: "Key"},
		},
		deletedKeys: map[string]bool{
			normalizePath(`Deleted\Key`): true,
		},
		setValues: map[valueKey]valueData{
			{path: normalizePath(`Value\Path`), name: "Val"}: {typ: types.REG_SZ, data: []byte("test")},
		},
		deletedVals: map[valueKey]bool{
			{path: normalizePath(`Value\Path`), name: "OldVal"}: true,
		},
	}

	idx := buildChangeIndex(tx)

	// Test HasCreated
	if !idx.HasCreated(normalizePath(`New\Key`)) {
		t.Error("HasCreated should return true for created key")
	}
	if idx.HasCreated(normalizePath(`Deleted\Key`)) {
		t.Error("HasCreated should return false for deleted key")
	}

	// Test HasDeleted
	if !idx.HasDeleted(normalizePath(`Deleted\Key`)) {
		t.Error("HasDeleted should return true for deleted key")
	}
	if idx.HasDeleted(normalizePath(`New\Key`)) {
		t.Error("HasDeleted should return false for created key")
	}

	// Test HasValueChanges
	if !idx.HasValueChanges(normalizePath(`Value\Path`)) {
		t.Error("HasValueChanges should return true for path with value changes")
	}
	if idx.HasValueChanges(normalizePath(`New\Key`)) {
		t.Error("HasValueChanges should return false for key without value changes")
	}
}

// TestChangeIndex_ChangeCount tests the total change count
func TestChangeIndex_ChangeCount(t *testing.T) {
	tx := &transaction{
		createdKeys: map[string]*keyNode{
			normalizePath(`A`): {exists: false, name: "A"},
			normalizePath(`B`): {exists: false, name: "B"},
		},
		deletedKeys: map[string]bool{
			normalizePath(`C`): true,
		},
		setValues: map[valueKey]valueData{
			{path: normalizePath(`D`), name: "Val1"}: {typ: types.REG_SZ, data: []byte("v1")},
			{path: normalizePath(`D`), name: "Val2"}: {typ: types.REG_SZ, data: []byte("v2")},
		},
		deletedVals: map[valueKey]bool{
			{path: normalizePath(`E`), name: "OldVal"}: true,
		},
	}

	idx := buildChangeIndex(tx)

	// Should have 5 unique paths: A, B, C, D, E
	expected := 5
	got := idx.ChangeCount()
	if got != expected {
		t.Errorf("ChangeCount() = %d, want %d", got, expected)
	}
}

// TestChangeIndex_EmptyTransaction tests behavior with no changes
func TestChangeIndex_EmptyTransaction(t *testing.T) {
	tx := &transaction{
		createdKeys: make(map[string]*keyNode),
		deletedKeys: make(map[string]bool),
		setValues:   make(map[valueKey]valueData),
		deletedVals: make(map[valueKey]bool),
	}

	idx := buildChangeIndex(tx)

	if idx.HasExact(normalizePath("any path")) {
		t.Error("HasExact should return false for empty transaction")
	}

	if idx.HasSubtree(normalizePath("")) {
		t.Error("HasSubtree should return false for root in empty transaction")
	}

	if idx.ChangeCount() != 0 {
		t.Errorf("ChangeCount should be 0 for empty transaction, got %d", idx.ChangeCount())
	}
}

// TestChangeIndex_Normalization tests path normalization
func TestChangeIndex_Normalization(t *testing.T) {
	tx := &transaction{
		createdKeys: map[string]*keyNode{
			normalizePath(` Software\Test `): {exists: false, name: "Test"}, // with spaces
		},
		deletedKeys: map[string]bool{
			normalizePath(`Software\Old\`): true, // trailing backslash
		},
		setValues: map[valueKey]valueData{
			{path: normalizePath(`\Software\Config`), name: "Val"}: {typ: types.REG_SZ, data: []byte("v")}, // leading backslash
		},
		deletedVals: make(map[valueKey]bool),
	}

	idx := buildChangeIndex(tx)

	// All these should match due to normalization
	tests := []struct {
		path     string
		expected bool
	}{
		{`Software\Test`, true},
		{` Software\Test `, true},
		{`SOFTWARE\TEST`, true},

		{`Software\Old`, true},
		{`Software\Old\`, true},

		{`Software\Config`, true},
		{`\Software\Config`, true},
	}

	for _, tc := range tests {
		got := idx.HasExact(normalizePath(tc.path))
		if got != tc.expected {
			t.Errorf("HasExact(%q) = %v, want %v (normalization test)", tc.path, got, tc.expected)
		}
	}
}

// TestChangeIndex_DeepSubtree tests subtree detection with deep nesting
func TestChangeIndex_DeepSubtree(t *testing.T) {
	tx := &transaction{
		createdKeys: map[string]*keyNode{
			normalizePath(`Level1\Level2\Level3\Level4\Level5`): {exists: false, name: "Level5"},
		},
		deletedKeys: make(map[string]bool),
		setValues:   make(map[valueKey]valueData),
		deletedVals: make(map[valueKey]bool),
	}

	idx := buildChangeIndex(tx)

	// All ancestor paths should return true
	ancestors := []string{
		`Level1`,
		`Level1\Level2`,
		`Level1\Level2\Level3`,
		`Level1\Level2\Level3\Level4`,
		`Level1\Level2\Level3\Level4\Level5`,
	}

	for _, path := range ancestors {
		if !idx.HasSubtree(normalizePath(path)) {
			t.Errorf("HasSubtree(%q) should return true", path)
		}
	}

	// Sibling and unrelated paths should return false
	unrelated := []string{
		`Level1\Level2\Level3\Level4\Different`,
		`Level1\Level2\Other`,
		`Other`,
	}

	for _, path := range unrelated {
		if idx.HasSubtree(normalizePath(path)) {
			t.Errorf("HasSubtree(%q) should return false", path)
		}
	}
}
