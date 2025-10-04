package proxy

import (
	"Aegis/internal/cache"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyForwarding(t *testing.T) {
	// Mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("upstream response"))
	}))
	defer upstream.Close()

	// Create proxy
	p, err := New(upstream.URL, 5*time.Second, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "upstream response" {
		t.Errorf("expected body 'upstream response', got %s", rec.Body.String())
	}
	if rec.Header().Get("X-Served-By") != "Aegis" {
		t.Error("expected X-Served-By header")
	}
}

func TestProxyCacheFailover(t *testing.T) {
	requestCount := 0
	shouldFail := false

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if shouldFail {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer upstream.Close()

	p, err := New(upstream.URL, 5*time.Second, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// First request - succeed and cache
	req1 := httptest.NewRequest("GET", "/test", nil)
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec1.Code)
	}
	if rec1.Body.String() != "success" {
		t.Errorf("expected body 'success', got %s", rec1.Body.String())
	}

	// Second request - upstream fails, should serve from cache
	shouldFail = true
	req2 := httptest.NewRequest("GET", "/test", nil)
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected status 200 from cache, got %d", rec2.Code)
	}
	if rec2.Body.String() != "success" {
		t.Errorf("expected cached body 'success', got %s", rec2.Body.String())
	}
	if rec2.Header().Get("X-Cache") != "HIT-BACKUP" {
		t.Errorf("expected X-Cache: HIT-BACKUP, got %s", rec2.Header().Get("X-Cache"))
	}
}

func TestProxyNoCacheForPOST(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("post response"))
	}))
	defer upstream.Close()

	p, err := New(upstream.URL, 5*time.Second, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest("POST", "/test", strings.NewReader("data"))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Header().Get("X-Cache") != "BYPASS" {
		t.Errorf("expected X-Cache: BYPASS for POST, got %s", rec.Header().Get("X-Cache"))
	}

	// Cache should be empty
	if p.cache.Size() != 0 {
		t.Errorf("expected cache size 0, got %d", p.cache.Size())
	}
}

func TestProxyStatsHandler(t *testing.T) {
	p, err := New("http://example.com", 5*time.Second, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Add some items to cache
	p.cache.Set("key1", cache.Response{Body: []byte("test")})
	p.cache.Set("key2", cache.Response{Body: []byte("test2")})

	req := httptest.NewRequest("GET", "/stats", nil)
	rec := httptest.NewRecorder()
	p.StatsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("failed to parse stats JSON: %v", err)
	}

	if stats["cache_size"].(float64) != 2 {
		t.Errorf("expected cache_size 2, got %v", stats["cache_size"])
	}
}

func TestProxyTimeout(t *testing.T) {
	// Upstream that delays
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Proxy with very short timeout
	p, err := New(upstream.URL, 50*time.Millisecond, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest("GET", "/slow", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	// Should timeout and return 502
	if rec.Code != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", rec.Code)
	}
}

func TestProxyCacheWithTTL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("response"))
	}))
	defer upstream.Close()

	// Proxy with 100ms TTL
	p, err := New(upstream.URL, 5*time.Second, 100*time.Millisecond, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// First request
	req1 := httptest.NewRequest("GET", "/ttl-test", nil)
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec1.Code)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Cache entry should be expired
	cacheKey := p.cacheKey(req1)
	if _, ok := p.cache.Get(cacheKey); ok {
		t.Error("expected cache entry to be expired")
	}
}

func TestProxyHeaderPropagation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check that custom headers are forwarded
		if r.Header.Get("X-Custom-Header") != "test-value" {
			t.Errorf("expected X-Custom-Header to be forwarded to upstream")
		}
		// Check that hop-by-hop headers are NOT forwarded
		if r.Header.Get("Connection") != "" {
			t.Errorf("expected Connection header to NOT be forwarded to upstream")
		}
		w.Header().Set("X-Upstream-Header", "upstream-value")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	p, err := New(upstream.URL, 5*time.Second, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom-Header", "test-value")
	req.Header.Set("Connection", "keep-alive")

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	// Check that upstream headers are forwarded to client
	if rec.Header().Get("X-Upstream-Header") != "upstream-value" {
		t.Error("expected X-Upstream-Header to be forwarded to client")
	}
}

func TestProxyCacheKey(t *testing.T) {
	p, _ := New("http://example.com", 5*time.Second, 0, nil, nil)

	req1 := httptest.NewRequest("GET", "/api/users?page=1", nil)
	req2 := httptest.NewRequest("GET", "/api/users?page=2", nil)
	req3 := httptest.NewRequest("GET", "/api/users?page=1", nil)

	key1 := p.cacheKey(req1)
	key2 := p.cacheKey(req2)
	key3 := p.cacheKey(req3)

	if key1 == key2 {
		t.Error("expected different cache keys for different query params")
	}
	if key1 != key3 {
		t.Error("expected same cache keys for identical requests")
	}
}

func TestFullProxyFlow(t *testing.T) {
	requestCount := 0
	shouldFail := false

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if shouldFail {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"ok"}`)
	}))
	defer upstream.Close()

	p, err := New(upstream.URL, 5*time.Second, 0, nil, nil)
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stats", p.StatsHandler)
	mux.Handle("/", p)

	server := httptest.NewServer(mux)
	defer server.Close()

	// 1. First request - should succeed
	resp1, err := http.Get(server.URL + "/api/data")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp1.StatusCode)
	}
	if resp1.Header.Get("X-Cache") != "MISS" {
		t.Errorf("expected X-Cache: MISS, got %s", resp1.Header.Get("X-Cache"))
	}
	if resp1.Header.Get("X-Served-By") != "Aegis" {
		t.Error("expected X-Served-By: Aegis")
	}

	// 2. Make upstream fail
	shouldFail = true

	// 3. Second request - should serve from cache
	resp2, err := http.Get(server.URL + "/api/data")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 from cache, got %d", resp2.StatusCode)
	}
	if resp2.Header.Get("X-Cache") != "HIT-BACKUP" {
		t.Errorf("expected X-Cache: HIT-BACKUP, got %s", resp2.Header.Get("X-Cache"))
	}

	body, _ := io.ReadAll(resp2.Body)
	if string(body) != `{"status":"ok"}` {
		t.Errorf("expected cached body, got %s", string(body))
	}

	// 4. Check stats
	resp3, err := http.Get(server.URL + "/stats")
	if err != nil {
		t.Fatalf("stats request failed: %v", err)
	}
	defer resp3.Body.Close()

	var stats map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&stats)

	if stats["cache_size"].(float64) < 1 {
		t.Error("expected at least 1 item in cache")
	}
}
