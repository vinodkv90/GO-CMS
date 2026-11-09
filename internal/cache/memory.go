package cache

import (
	"context"
	"sync"
	"time"
)

type memoryEntry struct {
	value     []byte
	expiresAt time.Time
	tags      []string
}

// MemoryCache is a high-performance, thread-safe in-memory cache with tag-based invalidation.
type MemoryCache struct {
	mu        sync.RWMutex
	items     map[string]memoryEntry
	tagIndex  map[string]map[string]struct{} // tag -> set of keys
	prefix    string
	stopChan  chan struct{}
	closeOnce sync.Once
}

// NewMemoryCache creates a new in-memory cache with periodic background eviction.
func NewMemoryCache(prefix string) *MemoryCache {
	c := &MemoryCache{
		items:    make(map[string]memoryEntry),
		tagIndex: make(map[string]map[string]struct{}),
		prefix:   prefix,
		stopChan: make(chan struct{}),
	}

	// Start background cleanup ticker for expired entries
	go c.startEvictionLoop(1 * time.Minute)

	return c
}

func (c *MemoryCache) formatKey(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + key
}

// Get retrieves an unexpired cached item.
func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	k := c.formatKey(key)

	c.mu.RLock()
	entry, exists := c.items[k]
	c.mu.RUnlock()

	if !exists {
		return nil, ErrCacheMiss
	}

	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		// Key has expired; trigger lazy removal
		_ = c.Delete(ctx, key)
		return nil, ErrCacheMiss
	}

	// Return a copy to prevent external mutation
	res := make([]byte, len(entry.value))
	copy(res, entry.value)
	return res, nil
}

// Set stores a key-value entry with TTL and associated tags.
func (c *MemoryCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration, tags ...string) error {
	k := c.formatKey(key)

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	valCopy := make([]byte, len(val))
	copy(valCopy, val)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update items table
	c.items[k] = memoryEntry{
		value:     valCopy,
		expiresAt: expiresAt,
		tags:      tags,
	}

	// Index tags
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		if _, ok := c.tagIndex[tag]; !ok {
			c.tagIndex[tag] = make(map[string]struct{})
		}
		c.tagIndex[tag][k] = struct{}{}
	}

	return nil
}

// Delete removes a key and unbinds it from indexed tags.
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	k := c.formatKey(key)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeKeyLocked(k)
	return nil
}

func (c *MemoryCache) removeKeyLocked(k string) {
	entry, exists := c.items[k]
	if !exists {
		return
	}

	// Remove from tag index
	for _, tag := range entry.tags {
		if keySet, ok := c.tagIndex[tag]; ok {
			delete(keySet, k)
			if len(keySet) == 0 {
				delete(c.tagIndex, tag)
			}
		}
	}

	delete(c.items, k)
}

// DeleteByTag removes all keys mapped to a specific tag.
func (c *MemoryCache) DeleteByTag(ctx context.Context, tag string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	keySet, exists := c.tagIndex[tag]
	if !exists {
		return nil
	}

	// Collect keys to delete
	keysToDelete := make([]string, 0, len(keySet))
	for k := range keySet {
		keysToDelete = append(keysToDelete, k)
	}

	for _, k := range keysToDelete {
		c.removeKeyLocked(k)
	}

	delete(c.tagIndex, tag)
	return nil
}

// Clear flushes all entries and tag indices.
func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]memoryEntry)
	c.tagIndex = make(map[string]map[string]struct{})
	return nil
}

// Close stops the background eviction routine and clears the memory store.
func (c *MemoryCache) Close() error {
	c.closeOnce.Do(func() {
		close(c.stopChan)
	})
	return c.Clear(context.Background())
}

func (c *MemoryCache) startEvictionLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case now := <-ticker.C:
			c.evictExpired(now)
		}
	}
}

func (c *MemoryCache) evictExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, entry := range c.items {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			c.removeKeyLocked(k)
		}
	}
}

