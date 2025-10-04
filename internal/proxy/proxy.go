package proxy

import (
	"Aegis/internal/cache"
	"Aegis/internal/utils"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Proxy is a caching reverse proxy
type Proxy struct {
	upstream   *url.URL
	client     *http.Client
	cache      *cache.Cache
	ttl        time.Duration
	keyHeaders []string
}

// New creates a new proxy instance
func New(upstreamStr string, timeout time.Duration, ttl time.Duration, keyHeaders []string) (*Proxy, error) {
	u, err := url.Parse(upstreamStr)
	if err != nil {
		return nil, fmt.Errorf("parse upstream: %w", err)
	}

	// Transport with reasonable timeouts
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Proxy{
		upstream: u,
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		cache:      cache.New(),
		ttl:        ttl,
		keyHeaders: keyHeaders,
	}, nil
}

// ServeHTTP handles HTTP requests
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Cache only for GET and HEAD
	cacheable := r.Method == http.MethodGet || r.Method == http.MethodHead
	var cacheKey string
	if cacheable {
		cacheKey = p.cacheKey(r)
	}

	// Build upstream URL: base + path + query
	upURL := *p.upstream
	upURL.Path = utils.SingleSlashJoin(p.upstream.Path, r.URL.Path)
	upURL.RawQuery = r.URL.RawQuery

	// Copy request
	var body io.ReadCloser
	if r.Body != nil {
		body = r.Body
	}
	ctx, cancel := utils.RequestContextWithTimeout(r.Context(), p.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		r.Method,
		upURL.String(),
		body,
	)
	if err != nil {
		if cacheable {
			p.tryServeFromCache(w, r, cacheKey, fmt.Errorf("build request: %w", err))
		} else {
			http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		}
		return
	}
	utils.CopyHeadersForUpstream(req.Header, r.Header)

	// Send to upstream
	resp, err := p.client.Do(req)
	if err != nil {
		if cacheable {
			p.tryServeFromCache(w, r, cacheKey, err)
		} else {
			http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		}
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		if cacheable {
			p.tryServeFromCache(w, r, cacheKey, fmt.Errorf("read upstream body: %w", err))
		} else {
			http.Error(w, "Bad Gateway: "+err.Error(), http.StatusBadGateway)
		}
		return
	}

	// If 5xx -> fallback to cache (only for cacheable)
	if resp.StatusCode >= 500 && cacheable {
		p.tryServeFromCache(w, r, cacheKey, fmt.Errorf("upstream status %d", resp.StatusCode))
		return
	}

	// Forward response to client
	utils.CopyHeadersForClient(w.Header(), resp.Header)
	w.Header().Set("X-Served-By", "Aegis")

	// Success (2xx): save to cache (only for cacheable)
	saved := false
	if cacheable && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		entry := cache.Response{
			Status:   resp.StatusCode,
			Header:   utils.CloneHeaderSanitized(resp.Header),
			Body:     respBody,
			SavedAt:  time.Now(),
			ExpireAt: utils.ZeroOrExpiry(p.ttl),
		}
		p.cache.Set(cacheKey, entry)
		saved = true
	}

	// Set X-Cache header
	if saved {
		w.Header().Set("X-Cache", "MISS")
	} else if cacheable {
		w.Header().Set("X-Cache", "PASS")
	} else {
		w.Header().Set("X-Cache", "BYPASS")
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (p *Proxy) tryServeFromCache(w http.ResponseWriter, r *http.Request, key string, cause error) {
	if cached, ok := p.cache.Get(key); ok {
		// We have a cached copy - send as backup
		utils.CopyHeadersForClient(w.Header(), cached.Header)
		w.Header().Set("X-Served-By", "Aegis")
		w.Header().Set("X-Cache", "HIT-BACKUP")
		w.Header().Set("X-Backup-Saved-At", cached.SavedAt.Format(time.RFC3339))
		w.WriteHeader(cached.Status)
		_, _ = w.Write(cached.Body)
		return
	}
	// No cache - return 502 error
	http.Error(w, "Bad Gateway (no cached backup): "+cause.Error(), http.StatusBadGateway)
}

func (p *Proxy) cacheKey(r *http.Request) string {
	key := r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery

	// Include configured headers in cache key
	if len(p.keyHeaders) > 0 {
		for _, headerName := range p.keyHeaders {
			headerValue := r.Header.Get(headerName)
			if headerValue != "" {
				key += "|" + headerName + ":" + headerValue
			}
		}
	}

	return key
}

// StatsHandler returns cache statistics as JSON
func (p *Proxy) StatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	size := p.cache.Size()
	memBytes := p.cache.MemoryUsage()
	memKB := float64(memBytes) / 1024
	memMB := memKB / 1024
	fmt.Fprintf(w, `{"cache_size": %d, "memory_bytes": %d, "memory_kb": %.2f, "memory_mb": %.2f}`,
		size, memBytes, memKB, memMB)
}
