package namecache

import (
	"sync"
	"testing"
)

func TestLookup_Miss(t *testing.T) {
	Reset()
	_, _, _, ok := Lookup([]byte("nonexistent"))
	if ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestStore_ThenLookup(t *testing.T) {
	Reset()
	data := []byte("Software")
	Store(data, "software", 0x1234, 0x5678)

	name, regHash, fnvHash, ok := Lookup(data)
	if !ok {
		t.Fatal("expected hit after store")
	}
	if name != "software" {
		t.Fatalf("name = %q, want %q", name, "software")
	}
	if regHash != 0x1234 {
		t.Fatalf("regHash = 0x%X, want 0x1234", regHash)
	}
	if fnvHash != 0x5678 {
		t.Fatalf("fnvHash = 0x%X, want 0x5678", fnvHash)
	}
}

func TestStore_UpdateExisting(t *testing.T) {
	Reset()
	data := []byte("Key")
	Store(data, "key", 1, 2)
	Store(data, "key_updated", 3, 4)

	name, regHash, fnvHash, ok := Lookup(data)
	if !ok {
		t.Fatal("expected hit")
	}
	if name != "key_updated" || regHash != 3 || fnvHash != 4 {
		t.Fatalf("got (%q, %d, %d), want (\"key_updated\", 3, 4)", name, regHash, fnvHash)
	}

	// Should still be one entry
	if Len() != 1 {
		t.Fatalf("Len() = %d, want 1", Len())
	}
}

func TestLRU_Eviction(t *testing.T) {
	SetCapacity(3)
	defer SetCapacity(defaultCapacity)
	Reset()

	Store([]byte("a"), "a", 1, 1)
	Store([]byte("b"), "b", 2, 2)
	Store([]byte("c"), "c", 3, 3)

	if Len() != 3 {
		t.Fatalf("Len() = %d, want 3", Len())
	}

	// Adding a 4th should evict "a" (LRU)
	Store([]byte("d"), "d", 4, 4)
	if Len() != 3 {
		t.Fatalf("Len() = %d, want 3 after eviction", Len())
	}

	_, _, _, ok := Lookup([]byte("a"))
	if ok {
		t.Fatal("expected 'a' to be evicted")
	}

	// b, c, d should still be present
	for _, key := range []string{"b", "c", "d"} {
		_, _, _, ok := Lookup([]byte(key))
		if !ok {
			t.Fatalf("expected %q to be present", key)
		}
	}
}

func TestLRU_AccessPromotes(t *testing.T) {
	SetCapacity(3)
	defer SetCapacity(defaultCapacity)
	Reset()

	Store([]byte("a"), "a", 1, 1)
	Store([]byte("b"), "b", 2, 2)
	Store([]byte("c"), "c", 3, 3)

	// Access "a" to promote it (move to front)
	Lookup([]byte("a"))

	// Now add "d" — should evict "b" (now LRU), not "a"
	Store([]byte("d"), "d", 4, 4)

	_, _, _, ok := Lookup([]byte("b"))
	if ok {
		t.Fatal("expected 'b' to be evicted (LRU after 'a' was promoted)")
	}

	_, _, _, ok = Lookup([]byte("a"))
	if !ok {
		t.Fatal("expected 'a' to survive (was promoted by access)")
	}
}

func TestSetCapacity_Shrink(t *testing.T) {
	SetCapacity(10)
	defer SetCapacity(defaultCapacity)
	Reset()

	for i := range 10 {
		Store([]byte{byte(i)}, "x", uint32(i), 0)
	}
	if Len() != 10 {
		t.Fatalf("Len() = %d, want 10", Len())
	}

	// Shrink to 3 — should evict 7 entries
	SetCapacity(3)
	if Len() != 3 {
		t.Fatalf("Len() = %d, want 3 after shrink", Len())
	}
}

func TestSetCapacity_Zero_Disables(t *testing.T) {
	defer SetCapacity(defaultCapacity)

	SetCapacity(0)
	Reset()

	Store([]byte("test"), "test", 1, 2)
	if Len() != 0 {
		t.Fatalf("Len() = %d, want 0 (caching disabled)", Len())
	}

	_, _, _, ok := Lookup([]byte("test"))
	if ok {
		t.Fatal("expected miss when caching is disabled")
	}
}

func TestReset_ClearsEntries(t *testing.T) {
	SetCapacity(100)
	defer SetCapacity(defaultCapacity)

	Store([]byte("x"), "x", 1, 1)
	Store([]byte("y"), "y", 2, 2)
	if Len() != 2 {
		t.Fatalf("Len() = %d, want 2", Len())
	}

	Reset()
	if Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after reset", Len())
	}

	_, _, _, ok := Lookup([]byte("x"))
	if ok {
		t.Fatal("expected miss after reset")
	}
}

func TestConcurrent_Access(t *testing.T) {
	SetCapacity(1000)
	defer SetCapacity(defaultCapacity)
	Reset()

	var wg sync.WaitGroup
	const goroutines = 8
	const opsPerGoroutine = 1000

	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range opsPerGoroutine {
				key := []byte{byte(id), byte(i)}
				Store(key, "val", uint32(id), uint32(i))
				Lookup(key)
			}
		}(g)
	}

	wg.Wait()

	// Just verify no panics occurred and cache is non-empty
	if Len() == 0 {
		t.Fatal("expected non-empty cache after concurrent access")
	}
}

func BenchmarkLookup_Hit(b *testing.B) {
	SetCapacity(defaultCapacity)
	Reset()
	data := []byte("Software")
	Store(data, "software", 0x1234, 0x5678)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _, _ = Lookup(data)
	}
}

func BenchmarkLookup_Miss(b *testing.B) {
	SetCapacity(defaultCapacity)
	Reset()
	data := []byte("NonExistentKey")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _, _ = Lookup(data)
	}
}

func BenchmarkStore(b *testing.B) {
	SetCapacity(defaultCapacity)
	Reset()
	data := []byte("Software")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		Store(data, "software", 0x1234, 0x5678)
	}
}
