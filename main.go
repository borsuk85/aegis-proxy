package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ---------------- Cache ----------------

type cachedResponse struct {
	Status   int
	Header   http.Header
	Body     []byte
	SavedAt  time.Time
	ExpireAt time.Time // zero => bezterminowo
}

type cache struct {
	mu   sync.RWMutex
	data map[string]cachedResponse
}

func newCache() *cache { return &cache{data: make(map[string]cachedResponse)} }

func (c *cache) get(k string) (cachedResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[k]
	if !ok {
		return v, false
	}
	// TTL check
	if !v.ExpireAt.IsZero() && time.Now().After(v.ExpireAt) {
		return cachedResponse{}, false
	}
	return v, true
}

func (c *cache) set(k string, v cachedResponse) {
	c.mu.Lock()
	c.data[k] = v
	c.mu.Unlock()
}

// -------------- Proxy ------------------

type proxy struct {
	upstream *url.URL
	client   *http.Client
	cache    *cache
	ttl      time.Duration
}

func newProxy(upstreamStr string, timeout time.Duration, ttl time.Duration) (*proxy, error) {
	u, err := url.Parse(upstreamStr)
	if err != nil {
		return nil, fmt.Errorf("parse upstream: %w", err)
	}

	// Transport z rozsądnymi timeoutami
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

	return &proxy{
		upstream: u,
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout, // całkowity timeout żądania do upstream
		},
		cache: newCache(),
		ttl:   ttl,
	}, nil
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cacheKey := p.cacheKey(r)

	// Zbuduj URL do upstream: base + path + query
	upURL := *p.upstream
	upURL.Path = singleSlashJoin(p.upstream.Path, r.URL.Path)
	upURL.RawQuery = r.URL.RawQuery

	// Skopiuj żądanie
	var body io.ReadCloser
	if r.Body != nil {
		// Uwaga: to zużyje body, ale http.Client sam sobie poradzi
		body = r.Body
	}
	req, err := http.NewRequestWithContext(requestContextWithTimeout(r.Context(), p.client.Timeout), r.Method, upURL.String(), body)
	if err != nil {
		p.tryServeFromCache(w, r, cacheKey, fmt.Errorf("build request: %w", err))
		return
	}
	copyHeadersForUpstream(req.Header, r.Header)

	// Wyślij do upstream
	resp, err := p.client.Do(req)
	if err != nil {
		p.tryServeFromCache(w, r, cacheKey, err)
		return
	}
	defer resp.Body.Close()

	// Wczytaj ciało odpowiedzi
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		p.tryServeFromCache(w, r, cacheKey, fmt.Errorf("read upstream body: %w", err))
		return
	}

	// Jeśli 5xx -> fallback z cache
	if resp.StatusCode >= 500 {
		p.tryServeFromCache(w, r, cacheKey, fmt.Errorf("upstream status %d", resp.StatusCode))
		return
	}

	// Sukces (np. 2xx): zapisz w cache (domyślnie cache tylko 2xx)
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		entry := cachedResponse{
			Status:   resp.StatusCode,
			Header:   cloneHeaderSanitized(resp.Header),
			Body:     respBody,
			SavedAt:  time.Now(),
			ExpireAt: zeroOrExpiry(p.ttl),
		}
		p.cache.set(cacheKey, entry)
	}

	// Przekaż świeżą odpowiedź do klienta
	copyHeadersForClient(w.Header(), resp.Header)
	w.Header().Set("X-Cache", "SAVE")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (p *proxy) tryServeFromCache(w http.ResponseWriter, r *http.Request, key string, cause error) {
	if cached, ok := p.cache.get(key); ok {
		// Mamy kopię w pamięci – wyślij jako „backup”
		copyHeadersForClient(w.Header(), cached.Header)
		w.Header().Set("X-Cache", "HIT-BACKUP")
		w.Header().Set("X-Backup-Saved-At", cached.SavedAt.Format(time.RFC3339))
		w.WriteHeader(cached.Status)
		_, _ = w.Write(cached.Body)
		return
	}
	// Brak cache – zwróć błąd 502
	http.Error(w, "Bad Gateway (no cached backup): "+cause.Error(), http.StatusBadGateway)
}

func (p *proxy) cacheKey(r *http.Request) string {
	// Metoda + ścieżka + query; jeśli chcesz cache’ować tylko GET, odkomentuj warunek poniżej
	// if r.Method != http.MethodGet { return "" } // i użyj innej logiki
	return r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery
}

// -------------- Helpers ----------------

func singleSlashJoin(a, b string) string {
	switch {
	case a == "" || a == "/":
		return ensureLeadingSlash(b)
	case b == "" || b == "/":
		return ensureLeadingSlash(a)
	default:
		return strings.TrimRight(a, "/") + "/" + strings.TrimLeft(b, "/")
	}
}

func ensureLeadingSlash(s string) string {
	if s == "" {
		return "/"
	}
	if s[0] != '/' {
		return "/" + s
	}
	return s
}

func copyHeadersForUpstream(dst, src http.Header) {
	for k, vv := range src {
		// Nie przenoś hop-by-hop
		if hopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func copyHeadersForClient(dst, src http.Header) {
	for k, vv := range src {
		if hopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func cloneHeaderSanitized(h http.Header) http.Header {
	out := make(http.Header, len(h))
	copyHeadersForClient(out, h)
	return out
}

func hopByHopHeader(k string) bool {
	k = http.CanonicalHeaderKey(k)
	switch k {
	case "Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

func requestContextWithTimeout(parent context.Context, d time.Duration) context.Context {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) < d {
		// Rodzic już ma krótszy timeout
		return parent
	}
	ctx, _ := context.WithTimeout(parent, d)
	return ctx
}

func zeroOrExpiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}

// -------------- main -------------------

func main() {
	listen := flag.String("listen", ":8009", "adres nasłuchu (np. :8080)")
	up := flag.String("upstream", "http://localhost:3030", "URL serwisu upstream (np. http://127.0.0.1:3030)")
	timeout := flag.Duration("timeout", 2*time.Second, "timeout całego żądania do upstream")
	ttl := flag.Duration("ttl", 0, "TTL cache (0 = bezterminowo)")
	flag.Parse()

	p, err := newProxy(*up, *timeout, *ttl)
	if err != nil {
		log.Fatalf("init proxy: %v", err)
	}

	log.Printf("listening on %s, upstream %s, ttl=%s, timeout=%s", *listen, *up, ttl.String(), timeout.String())
	if err := http.ListenAndServe(*listen, p); err != nil {
		log.Fatal(err)
	}
}
