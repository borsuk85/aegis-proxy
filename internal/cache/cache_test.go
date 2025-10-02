package cache

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestCacheBasicOperations(t *testing.T) {
	c := New()

	// Test empty cache
	if _, ok := c.Get("nonexistent"); ok {
		t.Error("expected cache miss for nonexistent key")
	}

	// Test set and get
	resp := Response{
		Status: 200,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   []byte("test body"),
	}
	c.Set("key1", resp)

	got, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.Status != 200 {
		t.Errorf("expected status 200, got %d", got.Status)
	}
	if string(got.Body) != "test body" {
		t.Errorf("expected body 'test body', got %s", string(got.Body))
	}
}

func TestCacheTTL(t *testing.T) {
	c := New()

	// Set entry with expiry in the past
	expired := Response{
		Status:   200,
		Body:     []byte("expired"),
		ExpireAt: time.Now().Add(-1 * time.Second),
	}
	c.Set("expired", expired)

	// Should return false for expired entry
	if _, ok := c.Get("expired"); ok {
		t.Error("expected cache miss for expired entry")
	}

	// Set entry with future expiry
	valid := Response{
		Status:   200,
		Body:     []byte("valid"),
		ExpireAt: time.Now().Add(10 * time.Second),
	}
	c.Set("valid", valid)

	// Should return true for valid entry
	if _, ok := c.Get("valid"); !ok {
		t.Error("expected cache hit for valid entry")
	}

	// Entry with zero expiry (永久)
	permanent := Response{
		Status:   200,
		Body:     []byte("permanent"),
		ExpireAt: time.Time{},
	}
	c.Set("permanent", permanent)

	if _, ok := c.Get("permanent"); !ok {
		t.Error("expected cache hit for permanent entry")
	}
}

func TestCacheSize(t *testing.T) {
	c := New()

	if c.Size() != 0 {
		t.Errorf("expected size 0, got %d", c.Size())
	}

	c.Set("key1", Response{Body: []byte("test")})
	c.Set("key2", Response{Body: []byte("test2")})

	if c.Size() != 2 {
		t.Errorf("expected size 2, got %d", c.Size())
	}
}

func TestCacheMemoryUsage(t *testing.T) {
	c := New()

	resp := Response{
		Status: 200,
		Header: http.Header{"Content-Type": []string{"text/plain"}},
		Body:   []byte("test body"),
	}
	c.Set("testkey", resp)

	mem := c.MemoryUsage()
	// Should include key + body + headers
	expectedMin := int64(len("testkey") + len("test body") + len("Content-Type") + len("text/plain"))
	if mem < expectedMin {
		t.Errorf("expected memory >= %d, got %d", expectedMin, mem)
	}
}

func TestCacheConcurrency(t *testing.T) {
	c := New()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n%26))
			c.Set(key, Response{Body: []byte("data")})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := string(rune('a' + n%26))
			c.Get(key)
		}(i)
	}

	wg.Wait()
}
