package cache

import (
	"net/http"
	"sync"
	"time"
)

// Response represents a cached HTTP response
type Response struct {
	Status   int
	Header   http.Header
	Body     []byte
	SavedAt  time.Time
	ExpireAt time.Time // zero => no expiration
}

// Cache is a thread-safe in-memory cache for HTTP responses
type Cache struct {
	mu   sync.RWMutex
	data map[string]Response
}

// New creates a new cache instance
func New() *Cache {
	return &Cache{
		data: make(map[string]Response),
	}
}

// Get retrieves a cached response by key
// Returns the response and true if found and not expired, false otherwise
func (c *Cache) Get(key string) (Response, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.data[key]
	if !ok {
		return v, false
	}

	// TTL check
	if !v.ExpireAt.IsZero() && time.Now().After(v.ExpireAt) {
		return Response{}, false
	}

	return v, true
}

// Set stores a response in the cache
func (c *Cache) Set(key string, value Response) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Size returns the number of cached entries
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

// MemoryUsage returns approximate memory usage in bytes
func (c *Cache) MemoryUsage() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var total int64
	for k, v := range c.data {
		// key
		total += int64(len(k))
		// body
		total += int64(len(v.Body))
		// headers (approximate)
		for key, values := range v.Header {
			total += int64(len(key))
			for _, val := range values {
				total += int64(len(val))
			}
		}
	}
	return total
}
