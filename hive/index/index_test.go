package index

import (
	"testing"
)

// Test_StringIndex_AddNK tests adding NK entries.
func Test_StringIndex_AddNK(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add some NKs
	idx.AddNK(0, "Root", 0x1000)
	idx.AddNK(0x1000, "System", 0x2000)
	idx.AddNK(0x1000, "Software", 0x3000)

	// Verify they can be retrieved
	if offset, ok := idx.GetNK(0, "Root"); !ok || offset != 0x1000 {
		t.Errorf("GetNK(0, Root) = %x, %v; want 0x1000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0x1000, "System"); !ok || offset != 0x2000 {
		t.Errorf("GetNK(0x1000, System) = %x, %v; want 0x2000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0x1000, "Software"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Software) = %x, %v; want 0x3000, true", offset, ok)
	}

	// Verify stats
	stats := idx.Stats()
	if stats.NKCount != 3 {
		t.Errorf("Expected 3 NK entries, got %d", stats.NKCount)
	}
	if stats.Impl != "StringIndex" {
		t.Errorf("Expected impl=StringIndex, got %s", stats.Impl)
	}
}

// Test_StringIndex_AddVK tests adding VK entries.
func Test_StringIndex_AddVK(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add some VKs
	idx.AddVK(0x1000, "Version", 0x4000)
	idx.AddVK(0x1000, "InstallDate", 0x5000)
	idx.AddVK(0x1000, "", 0x6000) // Default value

	// Verify they can be retrieved
	if offset, ok := idx.GetVK(0x1000, "Version"); !ok || offset != 0x4000 {
		t.Errorf("GetVK(0x1000, Version) = %x, %v; want 0x4000, true", offset, ok)
	}

	if offset, ok := idx.GetVK(0x1000, "InstallDate"); !ok || offset != 0x5000 {
		t.Errorf("GetVK(0x1000, InstallDate) = %x, %v; want 0x5000, true", offset, ok)
	}

	if offset, ok := idx.GetVK(0x1000, ""); !ok || offset != 0x6000 {
		t.Errorf("GetVK(0x1000, ) = %x, %v; want 0x6000, true (default value)", offset, ok)
	}

	// Verify stats
	stats := idx.Stats()
	if stats.VKCount != 3 {
		t.Errorf("Expected 3 VK entries, got %d", stats.VKCount)
	}
}

// Test_StringIndex_RemoveNK tests removing NK entries.
func Test_StringIndex_RemoveNK(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add some entries
	idx.AddNK(0, "Root", 0x1000)
	idx.AddNK(0x1000, "System", 0x2000)
	idx.AddNK(0x1000, "Software", 0x3000)

	// Remove one
	idx.RemoveNK(0x1000, "System")

	// Verify it's gone
	if offset, ok := idx.GetNK(0x1000, "System"); ok {
		t.Errorf("GetNK(0x1000, System) should return false after removal, got %x", offset)
	}

	// Verify others still exist
	if offset, ok := idx.GetNK(0, "Root"); !ok || offset != 0x1000 {
		t.Errorf("GetNK(0, Root) = %x, %v; want 0x1000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0x1000, "Software"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Software) = %x, %v; want 0x3000, true", offset, ok)
	}

	// Verify stats updated
	stats := idx.Stats()
	if stats.NKCount != 2 {
		t.Errorf("Expected 2 NK entries after removal, got %d", stats.NKCount)
	}
}

// Test_StringIndex_RemoveVK tests removing VK entries.
func Test_StringIndex_RemoveVK(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add some entries
	idx.AddVK(0x1000, "Version", 0x4000)
	idx.AddVK(0x1000, "InstallDate", 0x5000)
	idx.AddVK(0x1000, "", 0x6000)

	// Remove one
	idx.RemoveVK(0x1000, "InstallDate")

	// Verify it's gone
	if offset, ok := idx.GetVK(0x1000, "InstallDate"); ok {
		t.Errorf("GetVK(0x1000, InstallDate) should return false after removal, got %x", offset)
	}

	// Verify others still exist
	if offset, ok := idx.GetVK(0x1000, "Version"); !ok || offset != 0x4000 {
		t.Errorf("GetVK(0x1000, Version) = %x, %v; want 0x4000, true", offset, ok)
	}

	if offset, ok := idx.GetVK(0x1000, ""); !ok || offset != 0x6000 {
		t.Errorf("GetVK(0x1000, ) = %x, %v; want 0x6000, true", offset, ok)
	}

	// Verify stats updated
	stats := idx.Stats()
	if stats.VKCount != 2 {
		t.Errorf("Expected 2 VK entries after removal, got %d", stats.VKCount)
	}
}

// Test_StringIndex_RemoveNonExistent tests removing entries that don't exist.
func Test_StringIndex_RemoveNonExistent(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add one entry
	idx.AddNK(0, "Root", 0x1000)

	// Remove non-existent - should not panic
	idx.RemoveNK(0, "NonExistent")
	idx.RemoveNK(0x1000, "Foo")
	idx.RemoveVK(0x1000, "Bar")

	// Verify original entry still exists
	if offset, ok := idx.GetNK(0, "Root"); !ok || offset != 0x1000 {
		t.Errorf("GetNK(0, Root) = %x, %v; want 0x1000, true", offset, ok)
	}

	// Stats should be unchanged
	stats := idx.Stats()
	if stats.NKCount != 1 {
		t.Errorf("Expected 1 NK entry, got %d", stats.NKCount)
	}
	if stats.VKCount != 0 {
		t.Errorf("Expected 0 VK entries, got %d", stats.VKCount)
	}
}

// Test_StringIndex_RemoveThenAdd tests removing and re-adding at same key.
func Test_StringIndex_RemoveThenAdd(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add entry
	idx.AddNK(0x1000, "Foo", 0x2000)

	// Verify
	if offset, ok := idx.GetNK(0x1000, "Foo"); !ok || offset != 0x2000 {
		t.Errorf("GetNK(0x1000, Foo) = %x, %v; want 0x2000, true", offset, ok)
	}

	// Remove
	idx.RemoveNK(0x1000, "Foo")

	// Verify removed
	if offset, ok := idx.GetNK(0x1000, "Foo"); ok {
		t.Errorf("GetNK(0x1000, Foo) should return false after removal, got %x", offset)
	}

	// Re-add with DIFFERENT offset (simulates offset reuse)
	idx.AddNK(0x1000, "Foo", 0x3000)

	// Verify new offset
	if offset, ok := idx.GetNK(0x1000, "Foo"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Foo) = %x, %v; want 0x3000, true (new offset)", offset, ok)
	}
}

// Test_StringIndex_OverwriteEntry tests that Add overwrites existing entry.
func Test_StringIndex_OverwriteEntry(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// Add entry
	idx.AddNK(0x1000, "Foo", 0x2000)

	// Add again with different offset (overwrite)
	idx.AddNK(0x1000, "Foo", 0x3000)

	// Should have latest value
	if offset, ok := idx.GetNK(0x1000, "Foo"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Foo) = %x, %v; want 0x3000, true (overwritten)", offset, ok)
	}

	// Stats should show 1 entry (not 2)
	stats := idx.Stats()
	if stats.NKCount != 1 {
		t.Errorf("Expected 1 NK entry (overwrite, not duplicate), got %d", stats.NKCount)
	}
}

// Test_StringIndex_CaseSensitivity tests case handling.
func Test_StringIndex_CaseSensitivity(t *testing.T) {
	idx := NewStringIndex(10, 10)

	// StringIndex now handles case-insensitivity internally (Windows Registry semantics)
	// Adding different case variants should overwrite the same entry (last one wins)
	idx.AddNK(0, "Foo", 0x1000)
	idx.AddNK(0, "foo", 0x2000)
	idx.AddNK(0, "FOO", 0x3000)

	// All case variants should return the same (latest) entry
	if offset, ok := idx.GetNK(0, "Foo"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0, Foo) = %x, %v; want 0x3000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0, "foo"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0, foo) = %x, %v; want 0x3000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0, "FOO"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0, FOO) = %x, %v; want 0x3000, true", offset, ok)
	}

	// Remove using any case variant
	idx.RemoveNK(0, "FoO")

	// Entry should be gone regardless of case used for lookup
	if _, ok := idx.GetNK(0, "foo"); ok {
		t.Error("GetNK(0, foo) should return false after removal")
	}

	if _, ok := idx.GetNK(0, "Foo"); ok {
		t.Error("GetNK(0, Foo) should return false after removal")
	}

	if _, ok := idx.GetNK(0, "FOO"); ok {
		t.Error("GetNK(0, FOO) should return false after removal")
	}
}

// ====================
// UniqueIndex Tests
// ====================

// Test_UniqueIndex_AddNK tests adding NK entries.
func Test_UniqueIndex_AddNK(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add some NKs
	idx.AddNK(0, "Root", 0x1000)
	idx.AddNK(0x1000, "System", 0x2000)
	idx.AddNK(0x1000, "Software", 0x3000)

	// Verify they can be retrieved
	if offset, ok := idx.GetNK(0, "Root"); !ok || offset != 0x1000 {
		t.Errorf("GetNK(0, Root) = %x, %v; want 0x1000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0x1000, "System"); !ok || offset != 0x2000 {
		t.Errorf("GetNK(0x1000, System) = %x, %v; want 0x2000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0x1000, "Software"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Software) = %x, %v; want 0x3000, true", offset, ok)
	}

	// Verify stats
	stats := idx.Stats()
	if stats.NKCount != 3 {
		t.Errorf("Expected 3 NK entries, got %d", stats.NKCount)
	}
	if stats.Impl != "UniqueIndex(single-intern)" {
		t.Errorf("Expected impl=UniqueIndex(single-intern), got %s", stats.Impl)
	}
}

// Test_UniqueIndex_AddVK tests adding VK entries.
func Test_UniqueIndex_AddVK(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add some VKs
	idx.AddVK(0x1000, "Version", 0x4000)
	idx.AddVK(0x1000, "InstallDate", 0x5000)
	idx.AddVK(0x1000, "", 0x6000) // Default value

	// Verify they can be retrieved
	if offset, ok := idx.GetVK(0x1000, "Version"); !ok || offset != 0x4000 {
		t.Errorf("GetVK(0x1000, Version) = %x, %v; want 0x4000, true", offset, ok)
	}

	if offset, ok := idx.GetVK(0x1000, "InstallDate"); !ok || offset != 0x5000 {
		t.Errorf("GetVK(0x1000, InstallDate) = %x, %v; want 0x5000, true", offset, ok)
	}

	if offset, ok := idx.GetVK(0x1000, ""); !ok || offset != 0x6000 {
		t.Errorf("GetVK(0x1000, ) = %x, %v; want 0x6000, true (default value)", offset, ok)
	}

	// Verify stats
	stats := idx.Stats()
	if stats.VKCount != 3 {
		t.Errorf("Expected 3 VK entries, got %d", stats.VKCount)
	}
}

// Test_UniqueIndex_RemoveNK tests removing NK entries.
func Test_UniqueIndex_RemoveNK(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add some entries
	idx.AddNK(0, "Root", 0x1000)
	idx.AddNK(0x1000, "System", 0x2000)
	idx.AddNK(0x1000, "Software", 0x3000)

	// Remove one
	idx.RemoveNK(0x1000, "System")

	// Verify it's gone
	if offset, ok := idx.GetNK(0x1000, "System"); ok {
		t.Errorf("GetNK(0x1000, System) should return false after removal, got %x", offset)
	}

	// Verify others still exist
	if offset, ok := idx.GetNK(0, "Root"); !ok || offset != 0x1000 {
		t.Errorf("GetNK(0, Root) = %x, %v; want 0x1000, true", offset, ok)
	}

	if offset, ok := idx.GetNK(0x1000, "Software"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Software) = %x, %v; want 0x3000, true", offset, ok)
	}

	// Verify stats updated
	stats := idx.Stats()
	if stats.NKCount != 2 {
		t.Errorf("Expected 2 NK entries after removal, got %d", stats.NKCount)
	}
}

// Test_UniqueIndex_RemoveVK tests removing VK entries.
func Test_UniqueIndex_RemoveVK(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add some entries
	idx.AddVK(0x1000, "Version", 0x4000)
	idx.AddVK(0x1000, "InstallDate", 0x5000)
	idx.AddVK(0x1000, "", 0x6000)

	// Remove one
	idx.RemoveVK(0x1000, "InstallDate")

	// Verify it's gone
	if offset, ok := idx.GetVK(0x1000, "InstallDate"); ok {
		t.Errorf("GetVK(0x1000, InstallDate) should return false after removal, got %x", offset)
	}

	// Verify others still exist
	if offset, ok := idx.GetVK(0x1000, "Version"); !ok || offset != 0x4000 {
		t.Errorf("GetVK(0x1000, Version) = %x, %v; want 0x4000, true", offset, ok)
	}

	if offset, ok := idx.GetVK(0x1000, ""); !ok || offset != 0x6000 {
		t.Errorf("GetVK(0x1000, ) = %x, %v; want 0x6000, true", offset, ok)
	}

	// Verify stats updated
	stats := idx.Stats()
	if stats.VKCount != 2 {
		t.Errorf("Expected 2 VK entries after removal, got %d", stats.VKCount)
	}
}

// Test_UniqueIndex_RemoveNonExistent tests removing entries that don't exist.
func Test_UniqueIndex_RemoveNonExistent(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add one entry
	idx.AddNK(0, "Root", 0x1000)

	// Remove non-existent - should not panic
	idx.RemoveNK(0, "NonExistent")
	idx.RemoveNK(0x1000, "Foo")
	idx.RemoveVK(0x1000, "Bar")

	// Verify original entry still exists
	if offset, ok := idx.GetNK(0, "Root"); !ok || offset != 0x1000 {
		t.Errorf("GetNK(0, Root) = %x, %v; want 0x1000, true", offset, ok)
	}

	// Stats should be unchanged
	stats := idx.Stats()
	if stats.NKCount != 1 {
		t.Errorf("Expected 1 NK entry, got %d", stats.NKCount)
	}
	if stats.VKCount != 0 {
		t.Errorf("Expected 0 VK entries, got %d", stats.VKCount)
	}
}

// Test_UniqueIndex_RemoveThenAdd tests removing and re-adding at same key.
func Test_UniqueIndex_RemoveThenAdd(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add entry
	idx.AddNK(0x1000, "Foo", 0x2000)

	// Verify
	if offset, ok := idx.GetNK(0x1000, "Foo"); !ok || offset != 0x2000 {
		t.Errorf("GetNK(0x1000, Foo) = %x, %v; want 0x2000, true", offset, ok)
	}

	// Remove
	idx.RemoveNK(0x1000, "Foo")

	// Verify removed
	if offset, ok := idx.GetNK(0x1000, "Foo"); ok {
		t.Errorf("GetNK(0x1000, Foo) should return false after removal, got %x", offset)
	}

	// Re-add with DIFFERENT offset (simulates offset reuse)
	idx.AddNK(0x1000, "Foo", 0x3000)

	// Verify new offset
	if offset, ok := idx.GetNK(0x1000, "Foo"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Foo) = %x, %v; want 0x3000, true (new offset)", offset, ok)
	}
}

// Test_UniqueIndex_OverwriteEntry tests that Add overwrites existing entry.
func Test_UniqueIndex_OverwriteEntry(t *testing.T) {
	idx := NewUniqueIndex(10, 10)

	// Add entry
	idx.AddNK(0x1000, "Foo", 0x2000)

	// Add again with different offset (overwrite)
	idx.AddNK(0x1000, "Foo", 0x3000)

	// Should have latest value
	if offset, ok := idx.GetNK(0x1000, "Foo"); !ok || offset != 0x3000 {
		t.Errorf("GetNK(0x1000, Foo) = %x, %v; want 0x3000, true (overwritten)", offset, ok)
	}

	// Stats should show 1 entry (not 2)
	stats := idx.Stats()
	if stats.NKCount != 1 {
		t.Errorf("Expected 1 NK entry (overwrite, not duplicate), got %d", stats.NKCount)
	}
}

// ====================
// Interface Compliance Tests
// ====================

// Test_Index_Interface verifies both implementations satisfy Index interface.
func Test_Index_Interface(t *testing.T) {
	// This test just verifies compile-time interface compliance
	var _ Index = (*StringIndex)(nil)
	var _ Index = (*UniqueIndex)(nil)

	var _ ReadOnlyIndex = (*StringIndex)(nil)
	var _ ReadOnlyIndex = (*UniqueIndex)(nil)
}

// Test_BothImplementations_Consistent tests that both implementations behave identically.
func Test_BothImplementations_Consistent(t *testing.T) {
	stringIdx := NewStringIndex(100, 100)
	uniqueIdx := NewUniqueIndex(100, 100)

	// Perform same operations on both
	operations := []struct {
		name string
		fn   func(Index)
	}{
		{"AddNK Root", func(idx Index) { idx.AddNK(0, "Root", 0x1000) }},
		{"AddNK System", func(idx Index) { idx.AddNK(0x1000, "System", 0x2000) }},
		{"AddNK Software", func(idx Index) { idx.AddNK(0x1000, "Software", 0x3000) }},
		{"AddVK Version", func(idx Index) { idx.AddVK(0x2000, "Version", 0x4000) }},
		{"AddVK Install", func(idx Index) { idx.AddVK(0x2000, "InstallDate", 0x5000) }},
		{"RemoveNK Software", func(idx Index) { idx.RemoveNK(0x1000, "Software") }},
		{"RemoveVK Install", func(idx Index) { idx.RemoveVK(0x2000, "InstallDate") }},
	}

	for _, op := range operations {
		op.fn(stringIdx)
		op.fn(uniqueIdx)
	}

	// Verify both have same results
	stringStats := stringIdx.Stats()
	uniqueStats := uniqueIdx.Stats()

	if stringStats.NKCount != uniqueStats.NKCount {
		t.Errorf("NKCount mismatch: StringIndex=%d, UniqueIndex=%d", stringStats.NKCount, uniqueStats.NKCount)
	}

	if stringStats.VKCount != uniqueStats.VKCount {
		t.Errorf("VKCount mismatch: StringIndex=%d, UniqueIndex=%d", stringStats.VKCount, uniqueStats.VKCount)
	}

	// Verify lookups return same results
	testLookups := []struct {
		desc        string
		getNK       bool
		parentOff   uint32
		name        string
		shouldExist bool
		wantOffset  uint32
	}{
		{"Root exists", true, 0, "Root", true, 0x1000},
		{"System exists", true, 0x1000, "System", true, 0x2000},
		{"Software removed", true, 0x1000, "Software", false, 0},
		{"Version exists", false, 0x2000, "Version", true, 0x4000},
		{"InstallDate removed", false, 0x2000, "InstallDate", false, 0},
	}

	for _, tt := range testLookups {
		var stringOff, uniqueOff uint32
		var stringOK, uniqueOK bool

		if tt.getNK {
			stringOff, stringOK = stringIdx.GetNK(tt.parentOff, tt.name)
			uniqueOff, uniqueOK = uniqueIdx.GetNK(tt.parentOff, tt.name)
		} else {
			stringOff, stringOK = stringIdx.GetVK(tt.parentOff, tt.name)
			uniqueOff, uniqueOK = uniqueIdx.GetVK(tt.parentOff, tt.name)
		}

		if stringOK != uniqueOK {
			t.Errorf("%s: existence mismatch - StringIndex=%v, UniqueIndex=%v", tt.desc, stringOK, uniqueOK)
		}

		if stringOK != tt.shouldExist {
			t.Errorf("%s: expected exist=%v, got %v", tt.desc, tt.shouldExist, stringOK)
		}

		if stringOK && stringOff != uniqueOff {
			t.Errorf("%s: offset mismatch - StringIndex=%x, UniqueIndex=%x", tt.desc, stringOff, uniqueOff)
		}

		if stringOK && stringOff != tt.wantOffset {
			t.Errorf("%s: expected offset=%x, got %x", tt.desc, tt.wantOffset, stringOff)
		}
	}
}
