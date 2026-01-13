package subkeys

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joshuapare/hivekit/hive"
	"github.com/joshuapare/hivekit/hive/alloc"
	"github.com/joshuapare/hivekit/internal/format"
	"github.com/joshuapare/hivekit/internal/testutil"
)

// Test_Hash verifies the Windows hash function.
func Test_Hash(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint32
	}{
		{"empty", "", 0},
		{"single char", "a", 'A'},
		{"two chars", "ab", 'A'*37 + 'B'},
		{"lowercase", "test", 'T'*37*37*37 + 'E'*37*37 + 'S'*37 + 'T'},
		{"uppercase", "TEST", 'T'*37*37*37 + 'E'*37*37 + 'S'*37 + 'T'},
		{"mixed case", "TeSt", 'T'*37*37*37 + 'E'*37*37 + 'S'*37 + 'T'},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Hash(tt.input)
			if result != tt.expected {
				t.Errorf("Hash(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// Test_ListInsert verifies Insert operation.
func Test_ListInsert(t *testing.T) {
	list := &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x1000},
			{NameLower: "ccc", NKRef: 0x3000},
			{NameLower: "eee", NKRef: 0x5000},
		},
	}

	// Insert in middle
	newList := list.Insert(Entry{NameLower: "bbb", NKRef: 0x2000})
	if newList.Len() != 4 {
		t.Errorf("Expected 4 entries, got %d", newList.Len())
	}

	// Verify order
	expected := []string{"aaa", "bbb", "ccc", "eee"}
	for i, e := range newList.Entries {
		if e.NameLower != expected[i] {
			t.Errorf("Entry %d: expected %q, got %q", i, expected[i], e.NameLower)
		}
	}

	// Insert duplicate (should replace)
	replaceList := newList.Insert(Entry{NameLower: "bbb", NKRef: 0x9999})
	if replaceList.Len() != 4 {
		t.Errorf("Expected 4 entries after replace, got %d", replaceList.Len())
	}

	entry, found := replaceList.Find("bbb")
	if !found {
		t.Error("Entry 'bbb' not found")
	}
	if entry.NKRef != 0x9999 {
		t.Errorf("Expected NKRef 0x9999, got 0x%X", entry.NKRef)
	}
}

// Test_ListRemove verifies Remove operation.
func Test_ListRemove(t *testing.T) {
	list := &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x1000},
			{NameLower: "bbb", NKRef: 0x2000},
			{NameLower: "ccc", NKRef: 0x3000},
		},
	}

	// Remove middle entry
	newList := list.Remove("bbb")
	if newList.Len() != 2 {
		t.Errorf("Expected 2 entries, got %d", newList.Len())
	}

	// Verify remaining entries
	expected := []string{"aaa", "ccc"}
	for i, e := range newList.Entries {
		if e.NameLower != expected[i] {
			t.Errorf("Entry %d: expected %q, got %q", i, expected[i], e.NameLower)
		}
	}

	// Remove non-existent entry
	noChangeList := newList.Remove("zzz")
	if noChangeList.Len() != 2 {
		t.Errorf("Expected 2 entries after removing non-existent, got %d", noChangeList.Len())
	}
}

// Test_ListFind verifies Find operation.
func Test_ListFind(t *testing.T) {
	list := &List{
		Entries: []Entry{
			{NameLower: "aaa", NKRef: 0x1000},
			{NameLower: "bbb", NKRef: 0x2000},
			{NameLower: "ccc", NKRef: 0x3000},
		},
	}

	// Find existing entry
	entry, found := list.Find("bbb")
	if !found {
		t.Error("Expected to find 'bbb'")
	}
	if entry.NKRef != 0x2000 {
		t.Errorf("Expected NKRef 0x2000, got 0x%X", entry.NKRef)
	}

	// Find non-existent entry
	_, found = list.Find("zzz")
	if found {
		t.Error("Should not have found 'zzz'")
	}
}

// setupTestHive creates a minimal test hive with an allocator for testing writer functions.
func setupTestHive(t *testing.T) (*hive.Hive, *alloc.FastAllocator, func()) {
	return testutil.SetupTestHiveWithAllocator(t)
}

// Test_Writer_VariousListSizes tests the Write function with various list sizes.
func Test_Writer_VariousListSizes(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	tests := []struct {
		name         string
		entries      []Entry
		expectedType string // "LF", "LH", or "RI"
	}{
		{"1 entry - LF", makeEntries(1), "LF"},
		{"12 entries - LF", makeEntries(12), "LF"},
		{"13 entries - LH", makeEntries(13), "LH"},
		{"100 entries - LH", makeEntries(100), "LH"},
		{"1024 entries - LH", makeEntries(1024), "LH"},
		{"1025 entries - RI", makeEntries(1025), "RI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := Write(h, allocator, tt.entries)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			if ref == 0 || ref == 0xFFFFFFFF {
				t.Errorf("Write() returned invalid ref: 0x%X", ref)
			}

			// Verify we can read it back and check the type
			payload, err := h.ResolveCellPayload(ref)
			if err != nil {
				t.Fatalf("Failed to resolve subkey list: %v", err)
			}

			if len(payload) < 2 {
				t.Fatal("Subkey list payload too small")
			}

			signature := string(payload[0:2])
			if signature != "lf" && signature != "lh" && signature != "ri" {
				t.Errorf("Invalid signature: %s", signature)
			}

			// Map signatures to expected types (case-insensitive)
			var actualType string
			switch signature {
			case "lf":
				actualType = "LF"
			case "lh":
				actualType = "LH"
			case "ri":
				actualType = "RI"
			}

			if actualType != tt.expectedType {
				t.Errorf("Write created %s list, want %s", actualType, tt.expectedType)
			}
		})
	}
}

// makeEntries creates n test entries for testing.
func makeEntries(n int) []Entry {
	entries := make([]Entry, n)
	for i := range n {
		// Create unique names: "key000", "key001", etc.
		name := string(rune('a' + (i % 26)))
		if n > 26 {
			name = name + string(rune('0'+(i/26%10))) + string(rune('0'+(i%10)))
		}
		entries[i] = Entry{
			NameLower: name,
			NKRef:     uint32(0x1000 + i*0x100),
		}
	}
	return entries
}

// Test_Writer_EmptyEntries tests that empty entry slices are handled correctly.
func Test_Writer_EmptyEntries(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	entries := []Entry{}

	ref, err := Write(h, allocator, entries)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Empty entries should return 0xFFFFFFFF
	if ref != 0xFFFFFFFF {
		t.Errorf("Write() = 0x%X, want 0xFFFFFFFF for empty entries", ref)
	}
}

// Test_Read_Write_Roundtrip tests writing and reading back subkey lists.
func Test_Read_Write_Roundtrip(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	tests := []struct {
		name         string
		numEntries   int
		expectedType string // "LF", "LH", or "RI"
	}{
		{"1 entry - LF", 1, "LF"},
		{"12 entries - LF", 12, "LF"},
		{"13 entries - LH", 13, "LH"},
		{"100 entries - LH", 100, "LH"},
		{"1025 entries - RI", 1025, "RI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create entries with NK cells
			entries := make([]Entry, tt.numEntries)
			nkRefs := make([]uint32, tt.numEntries)

			for i := range tt.numEntries {
				// Create unique name
				name := fmt.Sprintf("testkey%04d", i)
				nameLower := strings.ToLower(name)

				// Allocate NK cell for this entry
				nkRef, nkBuf := allocateTestNK(t, h, allocator, name)
				nkRefs[i] = nkRef

				entries[i] = Entry{
					NameLower: nameLower,
					NKRef:     nkRef,
				}

				// Store nkBuf to prevent it from being optimized away
				_ = nkBuf
			}

			// Write the subkey list
			listRef, err := Write(h, allocator, entries)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}

			// Read it back
			readList, err := Read(h, listRef)
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}

			// Verify the list matches
			if readList.Len() != len(entries) {
				t.Errorf("Read list length = %d, want %d", readList.Len(), len(entries))
			}

			// Verify entries (order may differ due to sorting)
			for i, expectedEntry := range entries {
				found := false
				for _, readEntry := range readList.Entries {
					if readEntry.NKRef == expectedEntry.NKRef {
						found = true
						if readEntry.NameLower != expectedEntry.NameLower {
							t.Errorf(
								"Entry %d name = %s, want %s",
								i,
								readEntry.NameLower,
								expectedEntry.NameLower,
							)
						}
						break
					}
				}
				if !found {
					t.Errorf("Entry %d (NKRef=0x%X) not found in read list", i, expectedEntry.NKRef)
				}
			}
		})
	}
}

// allocateTestNK allocates a minimal NK cell with the given name for testing.
func allocateTestNK(
	t *testing.T,
	_ *hive.Hive,
	allocator *alloc.FastAllocator,
	name string,
) (uint32, []byte) {
	nameBytes := []byte(name)
	nameLen := len(nameBytes)

	// Calculate NK size: fixed header + name
	nkPayloadSize := format.NKFixedHeaderSize + nameLen
	nkTotalSize := nkPayloadSize + 4 // +4 for cell header

	nkRef, nkBuf, err := allocator.Alloc(int32(nkTotalSize), alloc.ClassNK)
	if err != nil {
		t.Fatalf("Failed to allocate NK: %v", err)
	}

	// Write NK signature
	nkBuf[0] = 'n'
	nkBuf[1] = 'k'

	// Write flags: compressed name (0x0020)
	nkBuf[0x02] = 0x20
	nkBuf[0x03] = 0x00

	// Write name length at offset 0x48
	nkBuf[0x48] = byte(nameLen)
	nkBuf[0x49] = byte(nameLen >> 8)

	// Write name at offset 0x4C
	copy(nkBuf[0x4C:], nameBytes)

	return nkRef, nkBuf
}

// Test_Read_EmptyList tests reading empty/null lists.
func Test_Read_EmptyList(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	t.Run("null_ref", func(t *testing.T) {
		list, err := Read(h, 0xFFFFFFFF)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if list.Len() != 0 {
			t.Errorf("Expected empty list, got %d entries", list.Len())
		}
	})

	t.Run("zero_ref", func(t *testing.T) {
		list, err := Read(h, 0)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if list.Len() != 0 {
			t.Errorf("Expected empty list, got %d entries", list.Len())
		}
	})

	// Suppress unused variable warning
	_ = allocator
}

// Test_Read_CorruptedList tests error handling with corrupted lists.
func Test_Read_CorruptedList(t *testing.T) {
	h, allocator, cleanup := setupTestHive(t)
	defer cleanup()

	// Create a corrupted list with invalid signature
	corruptedData := make([]byte, 100)
	corruptedData[0] = 'x'
	corruptedData[1] = 'x' // Invalid signature

	// Allocate cell for corrupted data
	totalSize := len(corruptedData) + 4
	ref, buf, err := allocator.Alloc(int32(totalSize), alloc.ClassLF)
	if err != nil {
		t.Fatalf("Failed to allocate corrupted cell: %v", err)
	}

	copy(buf, corruptedData)

	// Try to read it - should get an error
	_, err = Read(h, ref)
	if err == nil {
		t.Error("Read() should return error for corrupted list")
	}
}
