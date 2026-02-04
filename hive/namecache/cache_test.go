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

	// Should still be one entry (in the shard that owns this key)
	if Len() != 1 {
		t.Fatalf("Len() = %d, want 1", Len())
	}
}

// TestLRU_Eviction tests eviction on a single shard to verify LRU ordering.
func TestLRU_Eviction(t *testing.T) {
	c := newCache(3)

	c.store([]byte("a"), "a", 1, 1)
	c.store([]byte("b"), "b", 2, 2)
	c.store([]byte("c"), "c", 3, 3)

	if c.len() != 3 {
		t.Fatalf("len() = %d, want 3", c.len())
	}

	// Adding a 4th should evict "a" (LRU)
	c.store([]byte("d"), "d", 4, 4)
	if c.len() != 3 {
		t.Fatalf("len() = %d, want 3 after eviction", c.len())
	}

	_, _, _, ok := c.lookup([]byte("a"))
	if ok {
		t.Fatal("expected 'a' to be evicted")
	}

	// b, c, d should still be present
	for _, key := range []string{"b", "c", "d"} {
		_, _, _, ok := c.lookup([]byte(key))
		if !ok {
			t.Fatalf("expected %q to be present", key)
		}
	}
}

// TestLRU_AccessPromotes tests that accessing an entry promotes it in LRU order.
func TestLRU_AccessPromotes(t *testing.T) {
	c := newCache(3)

	c.store([]byte("a"), "a", 1, 1)
	c.store([]byte("b"), "b", 2, 2)
	c.store([]byte("c"), "c", 3, 3)

	// Access "a" to promote it (move to front)
	c.lookup([]byte("a"))

	// Now add "d" — should evict "b" (now LRU), not "a"
	c.store([]byte("d"), "d", 4, 4)

	_, _, _, ok := c.lookup([]byte("b"))
	if ok {
		t.Fatal("expected 'b' to be evicted (LRU after 'a' was promoted)")
	}

	_, _, _, ok = c.lookup([]byte("a"))
	if !ok {
		t.Fatal("expected 'a' to survive (was promoted by access)")
	}
}

// TestSetCapacity_Shrink tests that shrinking capacity evicts excess entries.
func TestSetCapacity_Shrink(t *testing.T) {
	c := newCache(10)

	for i := range 10 {
		c.store([]byte{byte(i)}, "x", uint32(i), 0)
	}
	if c.len() != 10 {
		t.Fatalf("len() = %d, want 10", c.len())
	}

	// Shrink to 3 — should evict 7 entries
	c.setCapacity(3)
	if c.len() != 3 {
		t.Fatalf("len() = %d, want 3 after shrink", c.len())
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

// TestShardedCache_Distribution verifies entries distribute across shards.
func TestShardedCache_Distribution(t *testing.T) {
	SetCapacity(defaultCapacity)
	defer SetCapacity(defaultCapacity)
	Reset()

	// Store many entries — they should spread across shards
	for i := range 100 {
		key := []byte{byte(i), byte(i >> 8)}
		Store(key, "val", uint32(i), 0)
	}

	if Len() != 100 {
		t.Fatalf("Len() = %d, want 100", Len())
	}

	// Verify all entries are retrievable
	for i := range 100 {
		key := []byte{byte(i), byte(i >> 8)}
		_, _, _, ok := Lookup(key)
		if !ok {
			t.Fatalf("expected entry %d to be present", i)
		}
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
