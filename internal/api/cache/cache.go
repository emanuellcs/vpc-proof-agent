// Package cache provides a thread-safe, TTL-bounded cache for probe report
// data. Expensive probe executions are cached so that repeated HTTP requests
// do not re-run network probes on every call.
package cache

import (
	"sync"
	"time"

	"github.com/emanuellcs/vpc-proof-agent/internal/report"
)

// Cache stores the most recent probe report data for a configurable TTL.
//
// Concurrency: reads take a read lock and writes take a write lock, so many
// HTTP handlers may read the cached report concurrently while a refresh is
// serialized.
type Cache struct {
	mu   sync.RWMutex
	item *item
	ttl  time.Duration
	now  func() time.Time
}

// item is a single cached entry with its generation and expiry stamps.
type item struct {
	data        report.Data
	generatedAt time.Time
	expiresAt   time.Time
}

// New builds a cache with the given TTL. The clock function is injectable so
// tests can exercise expiry deterministically.
func New(ttl time.Duration) *Cache {
	return &Cache{ttl: ttl, now: time.Now}
}

// Get returns the cached report data and whether it is still valid. It
// returns false when nothing is cached or the entry has expired.
func (c *Cache) Get() (report.Data, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.item == nil {
		return report.Data{}, false
	}
	if !c.now().Before(c.item.expiresAt) {
		return report.Data{}, false
	}
	return c.item.data, true
}

// Put stores the report data, stamping its generation and expiry times.
func (c *Cache) Put(data *report.Data) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.item = &item{
		data:        *data,
		generatedAt: now,
		expiresAt:   now.Add(c.ttl),
	}
}

// GeneratedAt returns when the cached entry was generated, or the zero time
// when nothing is cached.
func (c *Cache) GeneratedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.item == nil {
		return time.Time{}
	}
	return c.item.generatedAt
}

// Invalidate clears the cache.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.item = nil
}
