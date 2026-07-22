package main

import (
	"net/http"
	"sync"
	"time"
)

// CacheEntry stores a cached response.
type CacheEntry struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	expiry     time.Time
}

// Cache stores HTTP responses with TTL expiration.
type Cache struct {
	entries map[string]CacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

// NewCache creates a new Cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]CacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a cached entry by key.
// Returns the entry and true if found and not expired, nil and false otherwise.
func (c *Cache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiry) {
		return nil, false
	}

	return &entry, true
}

// Set stores a response in the cache.
func (c *Cache) Set(key string, statusCode int, headers http.Header, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clone headers to avoid mutation
	clonedHeaders := headers.Clone()

	c.entries[key] = CacheEntry{
		StatusCode: statusCode,
		Headers:    clonedHeaders,
		Body:       body,
		expiry:     time.Now().Add(c.ttl),
	}
}
