package values

import (
	"errors"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil"
)

// Test_Len tests the Len method.
func Test_Len(t *testing.T) {
	tests := []struct {
		name     string
		list     *List
		expected int
	}{
		{"nil list", nil, 0},
		{"empty list", &List{VKRefs: []uint32{}}, 0},
		{"one element", &List{VKRefs: []uint32{0x1000}}, 1},
		{"three elements", &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.list.Len()
			if result != tt.expected {
				t.Errorf("Len() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// Test_Append tests the Append method.
func Test_Append(t *testing.T) {
	// Append to nil list
	t.Run("append to nil", func(t *testing.T) {
		var nilList *List
		newList := nilList.Append(0x1000)
		if newList == nil {
			t.Fatal("Append to nil should create new list")
		}
		if newList.Len() != 1 {
			t.Errorf("Expected length 1, got %d", newList.Len())
		}
		if newList.VKRefs[0] != 0x1000 {
			t.Errorf("Expected VKRef 0x1000, got 0x%X", newList.VKRefs[0])
		}
	})

	// Append to empty list
	t.Run("append to empty", func(t *testing.T) {
		list := &List{VKRefs: []uint32{}}
		newList := list.Append(0x2000)
		if newList.Len() != 1 {
			t.Errorf("Expected length 1, got %d", newList.Len())
		}
		if newList.VKRefs[0] != 0x2000 {
			t.Errorf("Expected VKRef 0x2000, got 0x%X", newList.VKRefs[0])
		}
	})

	// Append to non-empty list
	t.Run("append to non-empty", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000}}
		newList := list.Append(0x3000)
		if newList.Len() != 3 {
			t.Errorf("Expected length 3, got %d", newList.Len())
		}
		expected := []uint32{0x1000, 0x2000, 0x3000}
		for i, exp := range expected {
			if newList.VKRefs[i] != exp {
				t.Errorf("VKRefs[%d] = 0x%X, want 0x%X", i, newList.VKRefs[i], exp)
			}
		}
	})

	// Verify immutability
	t.Run("immutability", func(t *testing.T) {
		original := &List{VKRefs: []uint32{0x1000}}
		newList := original.Append(0x2000)

		if original.Len() != 1 {
			t.Error("Original list was mutated")
		}
		if newList.Len() != 2 {
			t.Error("New list doesn't have appended element")
		}
	})
}

// Test_Remove tests the Remove method.
func Test_Remove(t *testing.T) {
	// Remove from nil list
	t.Run("remove from nil", func(t *testing.T) {
		var nilList *List
		result := nilList.Remove(0x1000)
		if result != nil {
			t.Error("Remove from nil should return nil")
		}
	})

	// Remove non-existent element
	t.Run("remove non-existent", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000}}
		newList := list.Remove(0x9999)
		if newList.Len() != 2 {
			t.Errorf("List length should be unchanged, got %d", newList.Len())
		}
	})

	// Remove first element
	t.Run("remove first", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}
		newList := list.Remove(0x1000)
		if newList.Len() != 2 {
			t.Errorf("Expected length 2, got %d", newList.Len())
		}
		expected := []uint32{0x2000, 0x3000}
		for i, exp := range expected {
			if newList.VKRefs[i] != exp {
				t.Errorf("VKRefs[%d] = 0x%X, want 0x%X", i, newList.VKRefs[i], exp)
			}
		}
	})

	// Remove middle element
	t.Run("remove middle", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}
		newList := list.Remove(0x2000)
		if newList.Len() != 2 {
			t.Errorf("Expected length 2, got %d", newList.Len())
		}
		expected := []uint32{0x1000, 0x3000}
		for i, exp := range expected {
			if newList.VKRefs[i] != exp {
				t.Errorf("VKRefs[%d] = 0x%X, want 0x%X", i, newList.VKRefs[i], exp)
			}
		}
	})

	// Remove last element
	t.Run("remove last", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}
		newList := list.Remove(0x3000)
		if newList.Len() != 2 {
			t.Errorf("Expected length 2, got %d", newList.Len())
		}
		expected := []uint32{0x1000, 0x2000}
		for i, exp := range expected {
			if newList.VKRefs[i] != exp {
				t.Errorf("VKRefs[%d] = 0x%X, want 0x%X", i, newList.VKRefs[i], exp)
			}
		}
	})

	// Remove only element
	t.Run("remove only", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000}}
		newList := list.Remove(0x1000)
		if newList.Len() != 0 {
			t.Errorf("Expected empty list, got length %d", newList.Len())
		}
	})

	// Verify immutability
	t.Run("immutability", func(t *testing.T) {
		original := &List{VKRefs: []uint32{0x1000, 0x2000}}
		newList := original.Remove(0x1000)

		if original.Len() != 2 {
			t.Error("Original list was mutated")
		}
		if newList.Len() != 1 {
			t.Error("New list doesn't have element removed")
		}
	})
}

// Test_Find tests the Find method.
func Test_Find(t *testing.T) {
	// Find in nil list
	t.Run("find in nil", func(t *testing.T) {
		var nilList *List
		index := nilList.Find(0x1000)
		if index != -1 {
			t.Errorf("Find in nil should return -1, got %d", index)
		}
	})

	// Find in empty list
	t.Run("find in empty", func(t *testing.T) {
		list := &List{VKRefs: []uint32{}}
		index := list.Find(0x1000)
		if index != -1 {
			t.Errorf("Find in empty should return -1, got %d", index)
		}
	})

	// Find existing elements
	t.Run("find existing", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}

		// Find first
		index := list.Find(0x1000)
		if index != 0 {
			t.Errorf("Find(0x1000) = %d, want 0", index)
		}

		// Find middle
		index = list.Find(0x2000)
		if index != 1 {
			t.Errorf("Find(0x2000) = %d, want 1", index)
		}

		// Find last
		index = list.Find(0x3000)
		if index != 2 {
			t.Errorf("Find(0x3000) = %d, want 2", index)
		}
	})

	// Find non-existent
	t.Run("find non-existent", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000}}
		index := list.Find(0x9999)
		if index != -1 {
			t.Errorf("Find(0x9999) = %d, want -1", index)
		}
	})

	// Find with duplicates (should return first)
	t.Run("find with duplicates", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000, 0x1000}}
		index := list.Find(0x1000)
		if index != 0 {
			t.Errorf("Find should return first occurrence, got %d", index)
		}
	})
}

// Test_parseValueList tests the parsing logic.
func Test_parseValueList(t *testing.T) {
	// Valid list
	t.Run("valid list", func(t *testing.T) {
		// Create list with 3 VK refs: 0x1000, 0x2000, 0x3000
		payload := make([]byte, 12)

		// VK ref 0: 0x1000
		payload[0] = 0x00
		payload[1] = 0x10
		payload[2] = 0x00
		payload[3] = 0x00

		// VK ref 1: 0x2000
		payload[4] = 0x00
		payload[5] = 0x20
		payload[6] = 0x00
		payload[7] = 0x00

		// VK ref 2: 0x3000
		payload[8] = 0x00
		payload[9] = 0x30
		payload[10] = 0x00
		payload[11] = 0x00

		refs, err := parseValueList(payload, 3)
		if err != nil {
			t.Fatalf("parseValueList failed: %v", err)
		}

		if len(refs) != 3 {
			t.Fatalf("Expected 3 refs, got %d", len(refs))
		}

		expected := []uint32{0x1000, 0x2000, 0x3000}
		for i, exp := range expected {
			if refs[i] != exp {
				t.Errorf("refs[%d] = 0x%X, want 0x%X", i, refs[i], exp)
			}
		}
	})

	// Truncated list
	t.Run("truncated list", func(t *testing.T) {
		payload := make([]byte, 4)           // Only 1 ref
		_, err := parseValueList(payload, 3) // Claim 3 refs
		if !errors.Is(err, ErrTruncated) {
			t.Errorf("Expected ErrTruncated, got %v", err)
		}
	})

	// Empty list
	t.Run("empty list", func(t *testing.T) {
		payload := make([]byte, 0)
		refs, err := parseValueList(payload, 0)
		if err != nil {
			t.Errorf("Empty list should not error: %v", err)
		}
		if len(refs) != 0 {
			t.Errorf("Expected 0 refs, got %d", len(refs))
		}
	})
}

// Test_Write_EmptyList tests writing empty lists.
func Test_Write_EmptyList(t *testing.T) {
	// Nil list
	t.Run("nil list", func(t *testing.T) {
		// Can't fully test without hive/allocator, but verify logic
		var nilList *List
		if nilList.Len() != 0 {
			t.Error("Nil list should have length 0")
		}
	})

	// Empty list
	t.Run("empty list", func(t *testing.T) {
		emptyList := &List{VKRefs: []uint32{}}
		if emptyList.Len() != 0 {
			t.Error("Empty list should have length 0")
		}
	})
}

// Test_EdgeCases tests various edge cases.
func Test_EdgeCases(t *testing.T) {
	// Multiple appends
	t.Run("multiple appends", func(t *testing.T) {
		var list *List
		list = list.Append(0x1000)
		list = list.Append(0x2000)
		list = list.Append(0x3000)

		if list.Len() != 3 {
			t.Errorf("Expected 3 elements, got %d", list.Len())
		}
	})

	// Multiple removes
	t.Run("multiple removes", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}
		list = list.Remove(0x1000)
		list = list.Remove(0x3000)

		if list.Len() != 1 {
			t.Errorf("Expected 1 element, got %d", list.Len())
		}
		if list.VKRefs[0] != 0x2000 {
			t.Errorf("Expected 0x2000, got 0x%X", list.VKRefs[0])
		}
	})

	// Append after remove
	t.Run("append after remove", func(t *testing.T) {
		list := &List{VKRefs: []uint32{0x1000}}
		list = list.Remove(0x1000)
		list = list.Append(0x2000)

		if list.Len() != 1 {
			t.Errorf("Expected 1 element, got %d", list.Len())
		}
		if list.VKRefs[0] != 0x2000 {
			t.Errorf("Expected 0x2000, got 0x%X", list.VKRefs[0])
		}
	})
}

// setupTestHive creates a minimal test hive with an allocator for testing writer functions.
func setupTestHive(t *testing.T) (*hive.Hive, *alloc.FastAllocator, func()) {
	return testutil.SetupTestHiveWithAllocator(t)
}

// Test_Write tests the Write function with real hive.
func Test_Write(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	tests := []struct {
		name     string
		list     *List
		wantNull bool // Expect 0xFFFFFFFF for empty lists
	}{
		{"nil list", nil, true},
		{"empty list", &List{VKRefs: []uint32{}}, true},
		{"single value", &List{VKRefs: []uint32{0x1000}}, false},
		{"multiple values", &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000}}, false},
		{"many values", &List{VKRefs: make([]uint32, 100)}, false}, // 100 values
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := Write(h, allocator, tt.list)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			if tt.wantNull {
				if ref != 0xFFFFFFFF {
					t.Errorf("Write() = 0x%X, want 0xFFFFFFFF for empty list", ref)
				}
			} else {
				if ref == 0xFFFFFFFF {
					t.Error("Write() returned null ref for non-empty list")
				}
				if ref == 0 {
					t.Error("Write() returned zero ref")
				}
			}
		})
	}
}

// Test_Write_Read_Roundtrip tests writing and reading back.
func Test_Write_Read_Roundtrip(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	// Create a test list
	originalList := &List{VKRefs: []uint32{0x1000, 0x2000, 0x3000, 0x4000}}

	// Write it
	listRef, err := Write(h, allocator, originalList)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Create a minimal NK cell to use with Read()
	// NK needs: signature, value count, value list offset
	nkPayloadSize := 0x50 // Minimal NK size
	nkTotalSize := nkPayloadSize + 4
	nkRef, nkBuf, err := allocator.Alloc(int32(nkTotalSize), alloc.ClassNK)
	if err != nil {
		t.Fatalf("Failed to allocate NK: %v", err)
	}

	// Write NK signature
	nkBuf[0] = 'n'
	nkBuf[1] = 'k'

	// Write value count (offset 0x24)
	count := uint32(originalList.Len())
	nkBuf[0x24] = byte(count)
	nkBuf[0x25] = byte(count >> 8)
	nkBuf[0x26] = byte(count >> 16)
	nkBuf[0x27] = byte(count >> 24)

	// Write value list offset (offset 0x28)
	nkBuf[0x28] = byte(listRef)
	nkBuf[0x29] = byte(listRef >> 8)
	nkBuf[0x2A] = byte(listRef >> 16)
	nkBuf[0x2B] = byte(listRef >> 24)

	// Now read back using Read()
	nkPayload, err := h.ResolveCellPayload(nkRef)
	if err != nil {
		t.Fatalf("Failed to resolve NK: %v", err)
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		t.Fatalf("Failed to parse NK: %v", err)
	}

	// Read the value list back
	readList, err := Read(h, nk)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Verify the list matches
	if readList.Len() != originalList.Len() {
		t.Errorf("Read list length = %d, want %d", readList.Len(), originalList.Len())
	}

	for i, expectedRef := range originalList.VKRefs {
		if readList.VKRefs[i] != expectedRef {
			t.Errorf("VKRefs[%d] = 0x%X, want 0x%X", i, readList.VKRefs[i], expectedRef)
		}
	}
}

// Test_Read_NoValueList tests Read with NK that has no values.
func Test_Read_NoValueList(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	// Create NK with no values
	nkPayloadSize := 0x50
	nkTotalSize := nkPayloadSize + 4
	nkRef, nkBuf, err := allocator.Alloc(int32(nkTotalSize), alloc.ClassNK)
	if err != nil {
		t.Fatalf("Failed to allocate NK: %v", err)
	}

	// Write NK signature
	nkBuf[0] = 'n'
	nkBuf[1] = 'k'

	// Value count = 0 (offset 0x24)
	nkBuf[0x24] = 0
	nkBuf[0x25] = 0
	nkBuf[0x26] = 0
	nkBuf[0x27] = 0

	// Value list offset = 0xFFFFFFFF (offset 0x28)
	nkBuf[0x28] = 0xFF
	nkBuf[0x29] = 0xFF
	nkBuf[0x2A] = 0xFF
	nkBuf[0x2B] = 0xFF

	// Parse NK
	nkPayload, err := h.ResolveCellPayload(nkRef)
	if err != nil {
		t.Fatalf("Failed to resolve NK: %v", err)
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		t.Fatalf("Failed to parse NK: %v", err)
	}

	// Read should return ErrNoValueList
	_, err = Read(h, nk)
	if !errors.Is(err, ErrNoValueList) {
		t.Errorf("Read() error = %v, want ErrNoValueList", err)
	}
}

// Test_UpdateNK tests UpdateNK function.
func Test_UpdateNK(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	// Create NK cell
	nkPayloadSize := 0x50
	nkTotalSize := nkPayloadSize + 4
	nkRef, nkBuf, err := allocator.Alloc(int32(nkTotalSize), alloc.ClassNK)
	if err != nil {
		t.Fatalf("Failed to allocate NK: %v", err)
	}

	// Write NK signature
	nkBuf[0] = 'n'
	nkBuf[1] = 'k'

	// Update with test values
	testListRef := uint32(0x12345678)
	testCount := uint32(42)

	err = UpdateNK(h, nkRef, testListRef, testCount)
	if err != nil {
		t.Fatalf("UpdateNK() error = %v", err)
	}

	// Verify the values were written correctly
	nkPayload, err := h.ResolveCellPayload(nkRef)
	if err != nil {
		t.Fatalf("Failed to resolve NK: %v", err)
	}

	nk, err := hive.ParseNK(nkPayload)
	if err != nil {
		t.Fatalf("Failed to parse NK: %v", err)
	}

	if nk.ValueCount() != testCount {
		t.Errorf("NK value count = %d, want %d", nk.ValueCount(), testCount)
	}

	if nk.ValueListOffsetRel() != testListRef {
		t.Errorf("NK value list offset = 0x%X, want 0x%X", nk.ValueListOffsetRel(), testListRef)
	}
}

// Test_Read_ErrorPaths tests error handling in Read.
func Test_Read_ErrorPaths(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	// Create NK with invalid value list offset
	nkSize := format.NKFixedHeaderSize + 4
	totalSize := nkSize + format.CellHeaderSize
	nkRef, nkBuf, err := allocator.Alloc(int32(totalSize), alloc.ClassNK)
	if err != nil {
		t.Fatalf("Failed to allocate NK: %v", err)
	}

	// Write NK signature
	nkBuf[0] = 'n'
	nkBuf[1] = 'k'

	// Set value count but invalid list offset
	format.PutU32(nkBuf, format.NKValueCountOffset, 1)
	format.PutU32(nkBuf, format.NKValueListOffset, 0xFFFFFF00) // Out of bounds

	nkPayload, _ := resolveCell(h, nkRef)
	nk, _ := hive.ParseNK(nkPayload)

	// Try to read value list - should handle error gracefully
	_, err = Read(h, nk)
	if err == nil {
		t.Error("Read() should return error for invalid value list offset")
	}

	t.Logf("Correctly handled invalid value list offset: %v", err)
}
