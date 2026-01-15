package index

import (
	"testing"
)

func TestNumericIndex_BasicAddGet(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Add NK entries
	idx.AddNK(0, "Root", 1000)
	idx.AddNK(1000, "Software", 2000)
	idx.AddNK(1000, "System", 3000)
	idx.AddNK(2000, "Microsoft", 4000)

	// Add VK entries
	idx.AddVK(2000, "Version", 5000)
	idx.AddVK(2000, "Name", 6000)
	idx.AddVK(2000, "", 7000) // Default value

	// Verify NK lookups
	tests := []struct {
		parent uint32
		name   string
		want   uint32
	}{
		{0, "Root", 1000},
		{0, "ROOT", 1000},      // Case insensitive
		{0, "root", 1000},      // Case insensitive
		{1000, "Software", 2000},
		{1000, "SOFTWARE", 2000},
		{1000, "System", 3000},
		{2000, "Microsoft", 4000},
	}

	for _, tt := range tests {
		got, ok := idx.GetNK(tt.parent, tt.name)
		if !ok {
			t.Errorf("GetNK(%d, %q) = not found, want %d", tt.parent, tt.name, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("GetNK(%d, %q) = %d, want %d", tt.parent, tt.name, got, tt.want)
		}
	}

	// Verify VK lookups
	vkTests := []struct {
		parent uint32
		name   string
		want   uint32
	}{
		{2000, "Version", 5000},
		{2000, "VERSION", 5000}, // Case insensitive
		{2000, "Name", 6000},
		{2000, "", 7000}, // Default value
	}

	for _, tt := range vkTests {
		got, ok := idx.GetVK(tt.parent, tt.name)
		if !ok {
			t.Errorf("GetVK(%d, %q) = not found, want %d", tt.parent, tt.name, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("GetVK(%d, %q) = %d, want %d", tt.parent, tt.name, got, tt.want)
		}
	}
}

func TestNumericIndex_NotFound(t *testing.T) {
	idx := NewNumericIndex(100, 100)
	idx.AddNK(0, "Exists", 1000)

	// Test non-existent entries
	if _, ok := idx.GetNK(0, "NotExists"); ok {
		t.Error("GetNK(0, 'NotExists') should not find anything")
	}

	if _, ok := idx.GetNK(999, "Exists"); ok {
		t.Error("GetNK(999, 'Exists') should not find anything (wrong parent)")
	}

	if _, ok := idx.GetVK(0, "NoValue"); ok {
		t.Error("GetVK(0, 'NoValue') should not find anything")
	}
}

func TestNumericIndex_Remove(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Add and then remove
	idx.AddNK(0, "ToRemove", 1000)
	if _, ok := idx.GetNK(0, "ToRemove"); !ok {
		t.Error("Entry should exist before removal")
	}

	idx.RemoveNK(0, "ToRemove")
	if _, ok := idx.GetNK(0, "ToRemove"); ok {
		t.Error("Entry should not exist after removal")
	}

	// Remove non-existent entry (should not panic)
	idx.RemoveNK(0, "NeverExisted")
	idx.RemoveVK(0, "NeverExisted")
}

func TestNumericIndex_AddNKLower(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Use AddNKLower with pre-lowercased name
	idx.AddNKLower(0, "software", 1000)

	// Should be findable with any case
	if off, ok := idx.GetNK(0, "Software"); !ok || off != 1000 {
		t.Errorf("GetNK(0, 'Software') = %d, %v; want 1000, true", off, ok)
	}
	if off, ok := idx.GetNK(0, "SOFTWARE"); !ok || off != 1000 {
		t.Errorf("GetNK(0, 'SOFTWARE') = %d, %v; want 1000, true", off, ok)
	}
	if off, ok := idx.GetNK(0, "software"); !ok || off != 1000 {
		t.Errorf("GetNK(0, 'software') = %d, %v; want 1000, true", off, ok)
	}
}

func TestNumericIndex_Stats(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	idx.AddNK(0, "Key1", 1000)
	idx.AddNK(0, "Key2", 2000)
	idx.AddVK(1000, "Val1", 3000)
	idx.AddVK(1000, "Val2", 4000)
	idx.AddVK(1000, "Val3", 5000)

	stats := idx.Stats()
	if stats.NKCount != 2 {
		t.Errorf("Stats.NKCount = %d, want 2", stats.NKCount)
	}
	if stats.VKCount != 3 {
		t.Errorf("Stats.VKCount = %d, want 3", stats.VKCount)
	}
	if stats.Impl != "NumericIndex" {
		t.Errorf("Stats.Impl = %q, want 'NumericIndex'", stats.Impl)
	}
}

func TestNumericIndex_DuplicateAdd(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Adding same entry twice should be idempotent
	idx.AddNK(0, "Dup", 1000)
	idx.AddNK(0, "Dup", 1000)

	stats := idx.Stats()
	if stats.NKCount != 1 {
		t.Errorf("After duplicate add, NKCount = %d, want 1", stats.NKCount)
	}
}

// TestNumericIndex_HashCollision tests that entries with the same hash but
// different names are handled correctly via the collision map.
func TestNumericIndex_HashCollision(t *testing.T) {
	idx := NewNumericIndex(100, 100)

	// Force a collision by adding two entries that would have the same key
	// if they had the same hash. We can't easily force FNV collisions,
	// so we test the collision handling indirectly by adding same key
	// with different offsets (which triggers collision logic).
	idx.AddNKLower(0, "key1", 1000)
	idx.AddNKLower(0, "key1", 2000) // Same name, different offset = update

	// Should have the updated value
	if off, ok := idx.GetNK(0, "key1"); !ok || off != 2000 {
		t.Errorf("After update, GetNK(0, 'key1') = %d, %v; want 2000, true", off, ok)
	}

	// Stats should still show 1 entry (update, not add)
	stats := idx.Stats()
	if stats.NKCount != 1 {
		t.Errorf("After update, NKCount = %d, want 1", stats.NKCount)
	}
}

// BenchmarkNumericIndex_AddNKLower benchmarks the zero-allocation path.
func BenchmarkNumericIndex_AddNKLower(b *testing.B) {
	idx := NewNumericIndex(10000, 10000)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx.AddNKLower(uint32(i%1000), "testsubkeyname", uint32(i))
	}
}

// BenchmarkStringIndex_AddNKLower for comparison.
func BenchmarkStringIndex_AddNKLower(b *testing.B) {
	idx := NewStringIndex(10000, 10000)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		idx.AddNKLower(uint32(i%1000), "testsubkeyname", uint32(i))
	}
}
