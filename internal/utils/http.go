package utils

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// CopyHeadersForUpstream copies headers from source to destination,
// filtering out hop-by-hop headers
func CopyHeadersForUpstream(dst, src http.Header) {
	for k, vv := range src {
		if IsHopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// CopyHeadersForClient copies headers from source to destination,
// filtering out hop-by-hop headers
func CopyHeadersForClient(dst, src http.Header) {
	for k, vv := range src {
		if IsHopByHopHeader(k) {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// CloneHeaderSanitized creates a sanitized copy of HTTP headers
func CloneHeaderSanitized(h http.Header) http.Header {
	out := make(http.Header, len(h))
	CopyHeadersForClient(out, h)
	return out
}

// IsHopByHopHeader checks if a header is a hop-by-hop header
func IsHopByHopHeader(key string) bool {
	k := http.CanonicalHeaderKey(key)
	switch k {
	case "Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade":
		return true
	default:
		return false
	}
}

// SingleSlashJoin joins two URL paths with a single slash
func SingleSlashJoin(a, b string) string {
	switch {
	case a == "" || a == "/":
		return EnsureLeadingSlash(b)
	case b == "" || b == "/":
		return EnsureLeadingSlash(a)
	default:
		return strings.TrimRight(a, "/") + "/" + strings.TrimLeft(b, "/")
	}
}

// EnsureLeadingSlash ensures a path starts with a slash
func EnsureLeadingSlash(s string) string {
	if s == "" {
		return "/"
	}
	if s[0] != '/' {
		return "/" + s
	}
	return s
}

// RequestContextWithTimeout creates a context with timeout,
// respecting parent's deadline if shorter
func RequestContextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) < d {
		// Parent already has shorter timeout - return no-op cancel
		return parent, func() {}
	}
	return context.WithTimeout(parent, d)
}

// ZeroOrExpiry returns zero time or expiry time based on TTL
func ZeroOrExpiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}
