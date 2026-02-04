// Package namecache provides a byte-level LRU decode cache for registry name decoding.
//
// The cache is keyed on raw name bytes (from mmap'd NK cells) and stores the
// decoded lowercase name along with precomputed regHash and fnvHash values.
// Cache hits are zero-allocation: Go optimizes map[string]V lookups with []byte
// keys to avoid the []byte→string heap allocation.
//
// Concurrency: protected by a single sync.Mutex. Each hive walker is single-threaded;
// contention is only between concurrent sessions.
package namecache

import (
	"container/list"
	"sync"
)

// defaultCapacity is the default maximum number of entries in the cache.
const defaultCapacity = 8192

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

// global is the package-level singleton cache.
var global = newCache(defaultCapacity)

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
func (c *lruCache) store(data []byte, name string, regHash, fnvHash uint32) {
	if c.capacity == 0 {
		return
	}

	key := string(data)

	c.mu.Lock()
	// Check if already present (race between lookup miss and store)
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.name = name
		entry.regHash = regHash
		entry.fnvHash = fnvHash
		c.mu.Unlock()
		return
	}

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
