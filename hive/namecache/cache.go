// Package namecache provides a byte-level LRU decode cache for registry name decoding.
//
// The cache is keyed on raw name bytes (from mmap'd NK cells) and stores the
// decoded lowercase name along with precomputed regHash and fnvHash values.
// Cache hits are zero-allocation: Go optimizes map[string]V lookups with []byte
// keys to avoid the []byte→string heap allocation.
//
// Concurrency: 16-shard design with per-shard mutexes reduces contention
// when multiple goroutines decode names concurrently.
//
// The LRU is implemented as an intrusive doubly-linked list to eliminate
// container/list.Element allocations (each entry embeds its own prev/next pointers).
package namecache

import (
	"hash/fnv"
	"sync"
)

// defaultCapacity is the default maximum number of entries in the cache.
const defaultCapacity = 8192

// numShards is the number of independent cache shards.
// Must be a power of two for fast modulo via bitmask.
const numShards = 16

// cacheEntry stores the decoded result for a raw name byte sequence.
// Embeds intrusive list pointers to avoid container/list.Element allocations.
type cacheEntry struct {
	// Intrusive list pointers (front = MRU, back = LRU)
	prev, next *cacheEntry

	key     string // copy of the raw bytes as string (map key)
	name    string // decoded lowercase name
	regHash uint32
	fnvHash uint32
}

// lruCache is an LRU cache mapping raw name bytes to decoded results.
// Uses an intrusive doubly-linked list for O(1) LRU operations without
// per-element heap allocations.
type lruCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*cacheEntry

	// Sentinel nodes for intrusive doubly-linked list.
	// head.next is MRU, tail.prev is LRU.
	// Using sentinels eliminates nil checks in list operations.
	head, tail cacheEntry
}

// newCache creates an LRU cache with the given capacity.
// A capacity of 0 disables caching.
func newCache(capacity int) *lruCache {
	c := &lruCache{
		capacity: capacity,
		items:    make(map[string]*cacheEntry, capacity),
	}
	// Initialize empty list: head <-> tail
	c.head.next = &c.tail
	c.tail.prev = &c.head
	return c
}

// listLen returns the number of entries in the intrusive list.
func (c *lruCache) listLen() int {
	return len(c.items)
}

// insertAfter inserts entry e after node at.
func insertAfter(at, e *cacheEntry) {
	e.prev = at
	e.next = at.next
	at.next.prev = e
	at.next = e
}

// remove removes entry e from the list.
func remove(e *cacheEntry) {
	e.prev.next = e.next
	e.next.prev = e.prev
	e.prev = nil
	e.next = nil
}

// moveToFront moves entry e to the front (MRU position).
func (c *lruCache) moveToFront(e *cacheEntry) {
	remove(e)
	insertAfter(&c.head, e)
}

// pushFront adds entry e at the front (MRU position).
func (c *lruCache) pushFront(e *cacheEntry) {
	insertAfter(&c.head, e)
}

// back returns the LRU entry, or nil if list is empty.
func (c *lruCache) back() *cacheEntry {
	if c.tail.prev == &c.head {
		return nil
	}
	return c.tail.prev
}

// lookup checks the cache for the given raw name bytes.
// Returns the decoded name, regHash, fnvHash, and whether the entry was found.
// Zero-allocation on cache hit due to Go's map[string]V + []byte optimization.
func (c *lruCache) lookup(data []byte) (string, uint32, uint32, bool) {
	if c.capacity == 0 {
		return "", 0, 0, false
	}

	c.mu.Lock()
	entry, ok := c.items[string(data)]
	if !ok {
		c.mu.Unlock()
		return "", 0, 0, false
	}
	c.moveToFront(entry)
	name, regHash, fnvHash := entry.name, entry.regHash, entry.fnvHash
	c.mu.Unlock()
	return name, regHash, fnvHash, true
}

// store adds a decoded result to the cache, evicting the least-recently-used
// entry if the cache is at capacity.
// String allocation from data is deferred to the miss path only:
// the c.items[string(data)] lookup on the hit path is zero-alloc per Go compiler optimization.
func (c *lruCache) store(data []byte, name string, regHash, fnvHash uint32) {
	if c.capacity == 0 {
		return
	}

	c.mu.Lock()
	// Check if already present (race between lookup miss and store).
	// This map lookup with string(data) is zero-alloc per Go compiler optimization.
	if entry, ok := c.items[string(data)]; ok {
		c.moveToFront(entry)
		entry.name = name
		entry.regHash = regHash
		entry.fnvHash = fnvHash
		c.mu.Unlock()
		return
	}

	// Miss path: allocate the string key only for new entries
	key := string(data)

	// Evict LRU if at capacity
	if len(c.items) >= c.capacity {
		if lru := c.back(); lru != nil {
			remove(lru)
			delete(c.items, lru.key)
		}
	}

	entry := &cacheEntry{
		key:     key,
		name:    name,
		regHash: regHash,
		fnvHash: fnvHash,
	}
	c.pushFront(entry)
	c.items[key] = entry
	c.mu.Unlock()
}

// setCapacity changes the cache capacity. If the new capacity is smaller,
// excess entries are evicted. A capacity of 0 disables caching and clears
// all entries.
func (c *lruCache) setCapacity(n int) {
	c.mu.Lock()
	c.capacity = n
	for len(c.items) > n {
		if lru := c.back(); lru != nil {
			remove(lru)
			delete(c.items, lru.key)
		} else {
			break
		}
	}
	c.mu.Unlock()
}

// reset clears all entries without changing capacity.
func (c *lruCache) reset() {
	c.mu.Lock()
	c.items = make(map[string]*cacheEntry, c.capacity)
	// Re-initialize empty list
	c.head.next = &c.tail
	c.tail.prev = &c.head
	c.head.prev = nil
	c.tail.next = nil
	c.mu.Unlock()
}

// len returns the current number of cached entries.
func (c *lruCache) len() int {
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	return n
}

// shardedCache distributes entries across multiple lruCache shards
// to reduce mutex contention under concurrent access.
type shardedCache struct {
	shards [numShards]*lruCache
}

// newShardedCache creates a sharded cache. Each shard gets capacity/numShards entries.
func newShardedCache(capacity int) *shardedCache {
	sc := &shardedCache{}
	perShard := capacity / numShards
	if perShard < 1 && capacity > 0 {
		perShard = 1
	}
	for i := range sc.shards {
		sc.shards[i] = newCache(perShard)
	}
	return sc
}

// shardFor returns the shard index for the given raw bytes using FNV hash.
func shardFor(data []byte) int {
	h := fnv.New32a()
	h.Write(data) //nolint:errcheck // fnv hash.Write never errors
	return int(h.Sum32() & (numShards - 1))
}

func (sc *shardedCache) lookup(data []byte) (string, uint32, uint32, bool) {
	return sc.shards[shardFor(data)].lookup(data)
}

func (sc *shardedCache) store(data []byte, name string, regHash, fnvHash uint32) {
	sc.shards[shardFor(data)].store(data, name, regHash, fnvHash)
}

func (sc *shardedCache) setCapacity(n int) {
	perShard := n / numShards
	if perShard < 1 && n > 0 {
		perShard = 1
	}
	for _, s := range sc.shards {
		s.setCapacity(perShard)
	}
}

func (sc *shardedCache) reset() {
	for _, s := range sc.shards {
		s.reset()
	}
}

func (sc *shardedCache) len() int {
	total := 0
	for _, s := range sc.shards {
		total += s.len()
	}
	return total
}

// global is the package-level singleton sharded cache.
var global = newShardedCache(defaultCapacity)

// --- Package-level API (delegates to global singleton) ---

// Lookup checks the decode cache for the given raw name bytes.
// Returns the decoded lowercase name, regHash, fnvHash, and whether the
// entry was found. Zero-allocation on cache hit.
func Lookup(data []byte) (string, uint32, uint32, bool) {
	return global.lookup(data)
}

// Store adds a decoded result to the cache.
func Store(data []byte, name string, regHash, fnvHash uint32) {
	global.store(data, name, regHash, fnvHash)
}

// SetCapacity changes the cache capacity. Pass 0 to disable caching.
func SetCapacity(n int) {
	global.setCapacity(n)
}

// Reset clears all cached entries without changing capacity.
func Reset() {
	global.reset()
}

// Len returns the current number of cached entries.
func Len() int {
	return global.len()
}
