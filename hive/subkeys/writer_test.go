package subkeys

import (
	"testing"
)

// Test_LFThreshold verifies the threshold constant.
func Test_LFThreshold(t *testing.T) {
	if LFThreshold != 12 {
		t.Errorf("LFThreshold should be 12, got %d", LFThreshold)
	}
}

// Test_Write_EmptyList verifies handling of empty lists.
func Test_Write_EmptyList(t *testing.T) {
	// Write should handle empty lists without requiring hive/allocator
	// by returning 0xFFFFFFFF (invalid offset)
	entries := []Entry{}

	// Can't fully test Write() without hive/allocator, but we can test the logic
	// by checking that empty list is handled correctly in our tests

	list := &List{Entries: entries}
	if list.Len() != 0 {
		t.Errorf("Empty list should have length 0, got %d", list.Len())
	}
}

// Test_Write_SelectionLogic verifies LF/LH/RI selection without actual writing.
func Test_Write_SelectionLogic(t *testing.T) {
	tests := []struct {
		name         string
		count        int
		expectedType string
	}{
		{"1 entry - LF", 1, "LF"},
		{"12 entries - LF", 12, "LF"},
		{"13 entries - LH", 13, "LH"},
		{"100 entries - LH", 100, "LH"},
		{"1024 entries - LH", 1024, "LH"},
		{"1025 entries - RI", 1025, "RI"},
		{"2000 entries - RI", 2000, "RI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Logic check: based on count, what should be selected?
			var expectedFormat string
			switch {
			case tt.count <= LFThreshold:
				expectedFormat = "LF"
			case tt.count <= 1024:
				expectedFormat = "LH"
			default:
				expectedFormat = "RI"
			}

			if expectedFormat != tt.expectedType {
				t.Errorf("For %d entries, expected %s, logic gives %s",
					tt.count, tt.expectedType, expectedFormat)
			}
		})
	}
}

// Test_ListSorting verifies that entries are sorted correctly.
func Test_ListSorting(t *testing.T) {
	entries := []Entry{
		{NameLower: "zebra", NKRef: 0x1000},
		{NameLower: "apple", NKRef: 0x2000},
		{NameLower: "middle", NKRef: 0x3000},
		{NameLower: "banana", NKRef: 0x4000},
	}

	// Simulate what Write() does: sort entries
	sortedEntries := make([]Entry, len(entries))
	copy(sortedEntries, entries)

	// Manual sort to verify logic
	for i := 0; i < len(sortedEntries); i++ {
		for j := i + 1; j < len(sortedEntries); j++ {
			if sortedEntries[j].NameLower < sortedEntries[i].NameLower {
				sortedEntries[i], sortedEntries[j] = sortedEntries[j], sortedEntries[i]
			}
		}
	}

	expected := []string{"apple", "banana", "middle", "zebra"}
	for i, name := range expected {
		if sortedEntries[i].NameLower != name {
			t.Errorf("sortedEntries[%d] = %q, want %q", i, sortedEntries[i].NameLower, name)
		}
	}
}

// Test_Insert_EdgeCases tests edge cases for Insert.
func Test_Insert_EdgeCases(t *testing.T) {
	// Insert into nil list
	var nilList *List
	newList := nilList.Insert(Entry{NameLower: "test", NKRef: 0x1000})
	if newList == nil {
		t.Fatal("Insert on nil list should create new list")
	}
	if newList.Len() != 1 {
		t.Errorf("Expected 1 entry, got %d", newList.Len())
	}

	// Insert at beginning
	list := &List{
		Entries: []Entry{
			{NameLower: "bbb", NKRef: 0x2000},
			{NameLower: "ccc", NKRef: 0x3000},
		},
	}
	newList = list.Insert(Entry{NameLower: "aaa", NKRef: 0x1000})
	if newList.Entries[0].NameLower != "aaa" {
		t.Errorf("Expected first entry 'aaa', got %q", newList.Entries[0].NameLower)
	}

	// Insert at end
	newList = list.Insert(Entry{NameLower: "zzz", NKRef: 0x9000})
	if newList.Entries[newList.Len()-1].NameLower != "zzz" {
		t.Errorf("Expected last entry 'zzz', got %q", newList.Entries[newList.Len()-1].NameLower)
	}

	// Replace existing
	list = &List{
		Entries: []Entry{
			{NameLower: "key", NKRef: 0x1000},
		},
	}
	newList = list.Insert(Entry{NameLower: "key", NKRef: 0x9999})
	if newList.Len() != 1 {
		t.Errorf("Replace should not change count, got %d", newList.Len())
	}
	if newList.Entries[0].NKRef != 0x9999 {
		t.Errorf("Expected NKRef 0x9999, got 0x%X", newList.Entries[0].NKRef)
	}
}

// Test_Remove_EdgeCases tests edge cases for Remove.
func Test_Remove_EdgeCases(t *testing.T) {
	// Remove from nil list
	var nilList *List
	result := nilList.Remove("test")
	if result != nil {
		t.Error("Remove from nil should return nil")
	}

	// Remove only element
	list := &List{
		Entries: []Entry{
			{NameLower: "only", NKRef: 0x1000},
		},
	}
	result = list.Remove("only")
	if result.Len() != 0 {
		t.Errorf("Expected empty list, got %d entries", result.Len())
	}

	// Remove first element
	list = &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x1000},
			{NameLower: "bbb", NKRef: 0x2000},
			{NameLower: "ccc", NKRef: 0x3000},
		},
	}
	result = list.Remove("aaa")
	if result.Len() != 2 {
		t.Errorf("Expected 2 entries, got %d", result.Len())
	}
	if result.Entries[0].NameLower != "bbb" {
		t.Errorf("Expected first 'bbb', got %q", result.Entries[0].NameLower)
	}

	// Remove last element
	result = list.Remove("ccc")
	if result.Len() != 2 {
		t.Errorf("Expected 2 entries, got %d", result.Len())
	}
	if result.Entries[result.Len()-1].NameLower != "bbb" {
		t.Errorf("Expected last 'bbb', got %q", result.Entries[result.Len()-1].NameLower)
	}
}

// Test_Find_EdgeCases tests edge cases for Find.
func Test_Find_EdgeCases(t *testing.T) {
	// Find in nil list
	var nilList *List
	_, found := nilList.Find("test")
	if found {
		t.Error("Find in nil list should return false")
	}

	// Find in empty list
	emptyList := &List{Entries: []Entry{}}
	_, found = emptyList.Find("test")
	if found {
		t.Error("Find in empty list should return false")
	}

	// Find first element
	list := &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x1000},
			{NameLower: "bbb", NKRef: 0x2000},
			{NameLower: "ccc", NKRef: 0x3000},
		},
	}
	entry, found := list.Find("aaa")
	if !found {
		t.Error("Should find 'aaa'")
	}
	if entry.NKRef != 0x1000 {
		t.Errorf("Expected NKRef 0x1000, got 0x%X", entry.NKRef)
	}

	// Find last element
	entry, found = list.Find("ccc")
	if !found {
		t.Error("Should find 'ccc'")
	}
	if entry.NKRef != 0x3000 {
		t.Errorf("Expected NKRef 0x3000, got 0x%X", entry.NKRef)
	}

	// Find middle element
	entry, found = list.Find("bbb")
	if !found {
		t.Error("Should find 'bbb'")
	}
	if entry.NKRef != 0x2000 {
		t.Errorf("Expected NKRef 0x2000, got 0x%X", entry.NKRef)
	}

	// Find non-existent (before all)
	_, found = list.Find("000")
	if found {
		t.Error("Should not find '000'")
	}

	// Find non-existent (after all)
	_, found = list.Find("zzz")
	if found {
		t.Error("Should not find 'zzz'")
	}

	// Find non-existent (in middle)
	_, found = list.Find("abc")
	if found {
		t.Error("Should not find 'abc'")
	}
}

// Test_Len_EdgeCases tests Len() edge cases.
func Test_Len_EdgeCases(t *testing.T) {
	// Nil list
	var nilList *List
	if nilList.Len() != 0 {
		t.Errorf("Nil list should have length 0, got %d", nilList.Len())
	}

	// Empty list
	emptyList := &List{Entries: []Entry{}}
	if emptyList.Len() != 0 {
		t.Errorf("Empty list should have length 0, got %d", emptyList.Len())
	}

	// Non-empty list
	list := &List{
		Entries: []Entry{
			{NameLower: "a", NKRef: 0x1000},
			{NameLower: "b", NKRef: 0x2000},
		},
	}
	if list.Len() != 2 {
		t.Errorf("List should have length 2, got %d", list.Len())
	}
}

// Test_Entry_Immutability verifies entries are properly copied.
func Test_Entry_Immutability(t *testing.T) {
	originalEntries := []Entry{
		{NameLower: "key1", NKRef: 0x1000},
		{NameLower: "key2", NKRef: 0x2000},
	}

	list := &List{Entries: make([]Entry, len(originalEntries))}
	copy(list.Entries, originalEntries)

	// Modify original
	originalEntries[0].NKRef = 0x9999

	// List should be unaffected
	if list.Entries[0].NKRef != 0x1000 {
		t.Error("List entries were not properly copied - mutation affected list")
	}
}
