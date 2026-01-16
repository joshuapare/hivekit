package index

import (
	"testing"
)

// TestAddVKHash_NoCollision tests the basic fast path where there's no collision.
func TestAddVKHash_NoCollision(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Add a value using hash-based method
	nameBytes := []byte("TestValue")
	hash := Fnv32LowerBytes(nameBytes)
	idx.AddVKHash(1000, hash, nameBytes, 5000)

	// Verify it can be retrieved
	offset, found := idx.GetVK(1000, "testvalue")
	if !found {
		t.Fatal("expected to find value")
	}
	if offset != 5000 {
		t.Errorf("expected offset 5000, got %d", offset)
	}
}

// TestAddVKHash_Update tests that updating an existing entry works.
func TestAddVKHash_Update(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	nameBytes := []byte("TestValue")
	hash := Fnv32LowerBytes(nameBytes)

	// Add initial value
	idx.AddVKHash(1000, hash, nameBytes, 5000)

	// Update with new offset
	idx.AddVKHash(1000, hash, nameBytes, 6000)

	// Verify updated value
	offset, found := idx.GetVK(1000, "testvalue")
	if !found {
		t.Fatal("expected to find value")
	}
	if offset != 6000 {
		t.Errorf("expected offset 6000, got %d", offset)
	}
}

// TestAddVKHash_CaseInsensitive tests case-insensitive matching.
func TestAddVKHash_CaseInsensitive(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Add using uppercase bytes
	nameBytes := []byte("TESTVALUE")
	hash := Fnv32LowerBytes(nameBytes)
	idx.AddVKHash(1000, hash, nameBytes, 5000)

	// Retrieve with various case combinations
	testCases := []string{
		"testvalue",
		"TESTVALUE",
		"TestValue",
		"tEsTvAlUe",
	}

	for _, name := range testCases {
		offset, found := idx.GetVK(1000, name)
		if !found {
			t.Errorf("expected to find value with name %q", name)
			continue
		}
		if offset != 5000 {
			t.Errorf("expected offset 5000 for %q, got %d", name, offset)
		}
	}
}

// TestFnv32LowerBytes_Consistency verifies hash consistency with existing fnv32Lower.
func TestFnv32LowerBytes_Consistency(t *testing.T) {
	testCases := []struct {
		input string
	}{
		{"hello"},
		{"HELLO"},
		{"Hello"},
		{"HeLLo"},
		{"test123"},
		{"TEST123"},
		{"MixedCase"},
		{""},
		{"a"},
		{"A"},
		{"abcdefghijklmnopqrstuvwxyz"},
		{"ABCDEFGHIJKLMNOPQRSTUVWXYZ"},
		{"0123456789"},
		{"special_chars-here.now"},
	}

	for _, tc := range testCases {
		// fnv32Lower takes a string and lowercases during hashing
		expected := fnv32Lower(tc.input)

		// Fnv32LowerBytes should produce the same hash for the byte slice
		actual := Fnv32LowerBytes([]byte(tc.input))

		if expected != actual {
			t.Errorf("hash mismatch for %q: fnv32Lower=%d, Fnv32LowerBytes=%d",
				tc.input, expected, actual)
		}
	}
}

// TestAddVKHash_ParentSeparation tests that different parents have separate namespaces.
func TestAddVKHash_ParentSeparation(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	nameBytes := []byte("SameName")
	hash := Fnv32LowerBytes(nameBytes)

	// Add same name under different parents
	idx.AddVKHash(1000, hash, nameBytes, 5000)
	idx.AddVKHash(2000, hash, nameBytes, 6000)

	// Verify they are stored separately
	offset1, found1 := idx.GetVK(1000, "samename")
	offset2, found2 := idx.GetVK(2000, "samename")

	if !found1 {
		t.Fatal("expected to find value under parent 1000")
	}
	if !found2 {
		t.Fatal("expected to find value under parent 2000")
	}
	if offset1 != 5000 {
		t.Errorf("expected offset 5000 for parent 1000, got %d", offset1)
	}
	if offset2 != 6000 {
		t.Errorf("expected offset 6000 for parent 2000, got %d", offset2)
	}
}

// TestAddVKHash_WithCollisionMap tests behavior when collision map is initialized.
func TestAddVKHash_WithCollisionMap(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Initialize collision map (simulating a previous collision)
	idx.valueCollisions = make(map[uint64][]collisionEntry)

	nameBytes := []byte("TestValue")
	hash := Fnv32LowerBytes(nameBytes)

	// Add using hash-based method
	idx.AddVKHash(1000, hash, nameBytes, 5000)

	// Should still work correctly
	offset, found := idx.GetVK(1000, "testvalue")
	if !found {
		t.Fatal("expected to find value")
	}
	if offset != 5000 {
		t.Errorf("expected offset 5000, got %d", offset)
	}
}

// TestAddVKHash_TrueCollision tests handling of actual hash collisions.
func TestAddVKHash_TrueCollision(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Initialize collision map
	idx.valueCollisions = make(map[uint64][]collisionEntry)

	// Add first entry
	name1 := []byte("first")
	hash1 := Fnv32LowerBytes(name1)
	idx.AddVKHash(1000, hash1, name1, 5000)

	// Manually create a collision by using the same key with different name
	key := makeNumericKey(1000, hash1)
	idx.valueCollisions[key] = []collisionEntry{
		{name: "first", offset: 5000},
	}

	// Now add another entry with same hash (simulated collision)
	name2 := []byte("second")
	// Force same hash by using same hash value (simulating collision)
	idx.AddVKHash(1000, hash1, name2, 6000)

	// The collision list should now have both entries
	entries, ok := idx.valueCollisions[key]
	if !ok {
		t.Fatal("expected collision entries to exist")
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 collision entries, got %d", len(entries))
	}
}

// TestAddNKHash_NoCollision tests the basic NK fast path.
func TestAddNKHash_NoCollision(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	nameBytes := []byte("SubKey")
	hash := Fnv32LowerBytes(nameBytes)
	idx.AddNKHash(1000, hash, nameBytes, 5000)

	offset, found := idx.GetNK(1000, "subkey")
	if !found {
		t.Fatal("expected to find NK")
	}
	if offset != 5000 {
		t.Errorf("expected offset 5000, got %d", offset)
	}
}

// TestAddNKHash_CaseInsensitive tests case-insensitive NK matching.
func TestAddNKHash_CaseInsensitive(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	nameBytes := []byte("SubKeyName")
	hash := Fnv32LowerBytes(nameBytes)
	idx.AddNKHash(1000, hash, nameBytes, 5000)

	testCases := []string{
		"subkeyname",
		"SUBKEYNAME",
		"SubKeyName",
		"sUbKeYnAmE",
	}

	for _, name := range testCases {
		offset, found := idx.GetNK(1000, name)
		if !found {
			t.Errorf("expected to find NK with name %q", name)
			continue
		}
		if offset != 5000 {
			t.Errorf("expected offset 5000 for %q, got %d", name, offset)
		}
	}
}

// BenchmarkFnv32LowerBytes benchmarks the byte-based hash function.
func BenchmarkFnv32LowerBytes(b *testing.B) {
	data := []byte("SomeTypicalValueName")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Fnv32LowerBytes(data)
	}
}

// BenchmarkFnv32_String benchmarks the string-based hash function for comparison.
func BenchmarkFnv32_String(b *testing.B) {
	s := "sometypicalvaluename" // pre-lowercased
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fnv32(s)
	}
}

// BenchmarkAddVKHash benchmarks the hash-based VK addition.
func BenchmarkAddVKHash(b *testing.B) {
	idx := NewNumericIndex(b.N, b.N)
	nameBytes := []byte("SomeValueName")
	hash := Fnv32LowerBytes(nameBytes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.AddVKHash(uint32(i), hash, nameBytes, uint32(i*100))
	}
}

// BenchmarkAddVKLower benchmarks the string-based VK addition for comparison.
func BenchmarkAddVKLower(b *testing.B) {
	idx := NewNumericIndex(b.N, b.N)
	nameLower := "somevaluename"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.AddVKLower(uint32(i), nameLower, uint32(i*100))
	}
}
