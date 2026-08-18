// Package cache implements a simple LRU (Least Recently Used) cache for
// recently read values. The cache sits between the store layer and the on-disk
// segments, reducing repeated disk reads for hot keys.
package cache

import (
	"container/list"
	"sync"
)

// entry is a key-value pair stored in the doubly-linked list.
type entry struct {
	key   string
	value []byte
}

// LRU is a fixed-capacity LRU cache that is safe for concurrent use.
type LRU struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[string]*list.Element
	hits     int64
	misses   int64
	evicts   int64
}

// New creates an LRU cache with the given capacity. If capacity <= 0, all
// operations are no-ops (the cache is effectively disabled).
func New(capacity int) *LRU {
	return &LRU{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

// Get retrieves the cached value for key. The second return value is false on a
// miss. On a hit the entry is promoted to the front of the eviction order.
func (c *LRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.capacity <= 0 {
		c.misses++
		return nil, false
	}
	elem, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}
	c.ll.MoveToFront(elem)
	c.hits++
	e := elem.Value.(*entry)
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, true
}

// Put inserts or updates a key-value pair. If the cache is at capacity, the
// least recently used item is evicted.
func (c *LRU) Put(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.capacity <= 0 {
		return
	}
	if elem, ok := c.items[key]; ok {
		c.ll.MoveToFront(elem)
		e := elem.Value.(*entry)
		e.value = copyBytes(value)
		return
	}
	if c.ll.Len() >= c.capacity {
		c.evictOldest()
	}
	e := &entry{key: key, value: copyBytes(value)}
	elem := c.ll.PushFront(e)
	c.items[key] = elem
}

// Delete removes key from the cache if present.
func (c *LRU) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return
	}
	c.removeElement(elem)
}

// Len returns the number of entries currently cached.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// Capacity returns the maximum number of entries.
func (c *LRU) Capacity() int { return c.capacity }

// Clear evicts all entries.
func (c *LRU) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element, c.capacity)
}

// Keys returns the cached keys in LRU order (most recent first).
func (c *LRU) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, c.ll.Len())
	for e := c.ll.Front(); e != nil; e = e.Next() {
		out = append(out, e.Value.(*entry).key)
	}
	return out
}

// Contains reports whether key is in the cache without updating recency.
func (c *LRU) Contains(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[key]
	return ok
}

// Peek returns the value without updating recency. Useful for diagnostics.
func (c *LRU) Peek(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	e := elem.Value.(*entry)
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, true
}

// Stats returns cache hit/miss/evict counters.
func (c *LRU) Stats() (hits, misses, evicts int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.evicts
}

// ResetStats zeroes the hit/miss/evict counters.
func (c *LRU) ResetStats() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits = 0
	c.misses = 0
	c.evicts = 0
}

// Resize changes the cache capacity. If the new capacity is smaller than the
// current number of entries, the oldest entries are evicted.
func (c *LRU) Resize(newCap int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capacity = newCap
	for c.ll.Len() > c.capacity && c.capacity > 0 {
		c.evictOldest()
	}
}

func (c *LRU) evictOldest() {
	tail := c.ll.Back()
	if tail == nil {
		return
	}
	c.removeElement(tail)
	c.evicts++
}

func (c *LRU) removeElement(elem *list.Element) {
	c.ll.Remove(elem)
	e := elem.Value.(*entry)
	delete(c.items, e.key)
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
