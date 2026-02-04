// Package namecache provides a byte-level LRU decode cache for registry name decoding.
//
// The cache is keyed on raw name bytes (from mmap'd NK cells) and stores the
// decoded lowercase name along with precomputed regHash and fnvHash values.
// Cache hits are zero-allocation: Go optimizes map[string]V lookups with []byte
// keys to avoid the []byte→string heap allocation.
//
// Concurrency: 16-shard design with per-shard mutexes reduces contention
// when multiple goroutines decode names concurrently.
package namecache

import (
	"container/list"
	"hash/fnv"
	"sync"
)

// defaultCapacity is the default maximum number of entries in the cache.
const defaultCapacity = 8192

// numShards is the number of independent cache shards.
// Must be a power of two for fast modulo via bitmask.
const numShards = 16

// cacheEntry stores the decoded result for a raw name byte sequence.
type cacheEntry struct {
	key     string // copy of the raw bytes as string (map key)
	name    string // decoded lowercase name
	regHash uint32
	fnvHash uint32
}

// lruCache is an LRU cache mapping raw name bytes to decoded results.
type lruCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List // front = most recently used
}

// newCache creates an LRU cache with the given capacity.
// A capacity of 0 disables caching.
func newCache(capacity int) *lruCache {
	return &lruCache{
		capacity: capacity,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

// lookup checks the cache for the given raw name bytes.
// Returns the decoded name, regHash, fnvHash, and whether the entry was found.
// Zero-allocation on cache hit due to Go's map[string]V + []byte optimization.
func (c *lruCache) lookup(data []byte) (string, uint32, uint32, bool) {
	if c.capacity == 0 {
		return "", 0, 0, false
	}

	c.mu.Lock()
	elem, ok := c.items[string(data)]
	if !ok {
		c.mu.Unlock()
		return "", 0, 0, false
	}
	c.order.MoveToFront(elem)
	entry := elem.Value.(*cacheEntry)
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
	if elem, ok := c.items[string(data)]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.name = name
		entry.regHash = regHash
		entry.fnvHash = fnvHash
		c.mu.Unlock()
		return
	}

	// Miss path: allocate the string key only for new entries
	key := string(data)

	// Evict LRU if at capacity
	if c.order.Len() >= c.capacity {
		back := c.order.Back()
		if back != nil {
			evicted := c.order.Remove(back).(*cacheEntry)
			delete(c.items, evicted.key)
		}
	}

	entry := &cacheEntry{
		key:     key,
		name:    name,
		regHash: regHash,
		fnvHash: fnvHash,
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
	c.mu.Unlock()
}

// setCapacity changes the cache capacity. If the new capacity is smaller,
// excess entries are evicted. A capacity of 0 disables caching and clears
// all entries.
func (c *lruCache) setCapacity(n int) {
	c.mu.Lock()
	c.capacity = n
	for c.order.Len() > n {
		back := c.order.Back()
		if back == nil {
			break
		}
		evicted := c.order.Remove(back).(*cacheEntry)
		delete(c.items, evicted.key)
	}
	c.mu.Unlock()
}

// reset clears all entries without changing capacity.
func (c *lruCache) reset() {
	c.mu.Lock()
	c.items = make(map[string]*list.Element, c.capacity)
	c.order.Init()
	c.mu.Unlock()
}

// len returns the current number of cached entries.
func (c *lruCache) len() int {
	c.mu.Lock()
	n := c.order.Len()
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
