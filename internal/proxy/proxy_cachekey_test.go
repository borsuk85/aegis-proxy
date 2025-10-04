package proxy

import (
	"net/http/httptest"
	"testing"
)

func TestCacheKeyWithHeaders(t *testing.T) {
	// Proxy with Authorization header in cache key
	p, _ := New("http://example.com", 0, 0, []string{"Authorization"}, nil)

	req1 := httptest.NewRequest("GET", "/api/data", nil)
	req1.Header.Set("Authorization", "Bearer token1")

	req2 := httptest.NewRequest("GET", "/api/data", nil)
	req2.Header.Set("Authorization", "Bearer token2")

	req3 := httptest.NewRequest("GET", "/api/data", nil)
	req3.Header.Set("Authorization", "Bearer token1")

	key1 := p.cacheKey(req1)
	key2 := p.cacheKey(req2)
	key3 := p.cacheKey(req3)

	// Different tokens should produce different keys
	if key1 == key2 {
		t.Errorf("expected different cache keys for different Authorization headers, got %s and %s", key1, key2)
	}

	// Same token should produce same key
	if key1 != key3 {
		t.Errorf("expected same cache keys for same Authorization headers, got %s and %s", key1, key3)
	}

	// Verify key format
	expectedKey := "GET /api/data?|Authorization:Bearer token1"
	if key1 != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key1)
	}
}

func TestCacheKeyWithMultipleHeaders(t *testing.T) {
	// Proxy with multiple headers in cache key
	p, _ := New("http://example.com", 0, 0, []string{"Authorization", "Accept-Language"}, nil)

	req1 := httptest.NewRequest("GET", "/api/data", nil)
	req1.Header.Set("Authorization", "Bearer token1")
	req1.Header.Set("Accept-Language", "en-US")

	req2 := httptest.NewRequest("GET", "/api/data", nil)
	req2.Header.Set("Authorization", "Bearer token1")
	req2.Header.Set("Accept-Language", "pl-PL")

	key1 := p.cacheKey(req1)
	key2 := p.cacheKey(req2)

	// Different language should produce different keys
	if key1 == key2 {
		t.Errorf("expected different cache keys for different Accept-Language headers")
	}

	// Verify key format includes both headers
	expectedKey1 := "GET /api/data?|Authorization:Bearer token1|Accept-Language:en-US"
	if key1 != expectedKey1 {
		t.Errorf("expected key %s, got %s", expectedKey1, key1)
	}
}

func TestCacheKeyWithMissingHeaders(t *testing.T) {
	// Proxy configured to use Authorization in key
	p, _ := New("http://example.com", 0, 0, []string{"Authorization", "X-Custom"}, nil)

	req1 := httptest.NewRequest("GET", "/api/data", nil)
	req1.Header.Set("Authorization", "Bearer token1")
	// X-Custom header not set

	req2 := httptest.NewRequest("GET", "/api/data", nil)
	// Neither header set

	key1 := p.cacheKey(req1)
	key2 := p.cacheKey(req2)

	// Keys should be different (one has Authorization, other doesn't)
	if key1 == key2 {
		t.Error("expected different cache keys when headers are missing")
	}

	// Key should only include headers that are present
	expectedKey1 := "GET /api/data?|Authorization:Bearer token1"
	if key1 != expectedKey1 {
		t.Errorf("expected key %s, got %s", expectedKey1, key1)
	}

	expectedKey2 := "GET /api/data?"
	if key2 != expectedKey2 {
		t.Errorf("expected key %s, got %s", expectedKey2, key2)
	}
}

func TestCacheKeyWithoutHeaderConfig(t *testing.T) {
	// Proxy without header configuration (backward compatibility)
	p, _ := New("http://example.com", 0, 0, nil, nil)

	req := httptest.NewRequest("GET", "/api/data?page=1", nil)
	req.Header.Set("Authorization", "Bearer token1")
	req.Header.Set("Accept-Language", "en-US")

	key := p.cacheKey(req)

	// Key should not include any headers
	expectedKey := "GET /api/data?page=1"
	if key != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key)
	}
}

func TestCacheKeyHeadersCaseSensitive(t *testing.T) {
	// Test that header names in config match case-insensitively
	p, _ := New("http://example.com", 0, 0, []string{"authorization"}, nil)

	req := httptest.NewRequest("GET", "/api/data", nil)
	req.Header.Set("Authorization", "Bearer token1") // Capital A

	key := p.cacheKey(req)

	// Should still include the header (http.Header.Get is case-insensitive)
	expectedKey := "GET /api/data?|authorization:Bearer token1"
	if key != expectedKey {
		t.Errorf("expected key %s, got %s", expectedKey, key)
	}
}
