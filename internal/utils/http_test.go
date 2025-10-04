package utils

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestSingleSlashJoin(t *testing.T) {
	tests := []struct {
		a, b, expected string
	}{
		{"", "", "/"},
		{"", "/", "/"},
		{"/", "", "/"},
		{"/", "/", "/"},
		{"/api", "/v1", "/api/v1"},
		{"/api/", "/v1", "/api/v1"},
		{"/api", "v1", "/api/v1"},
		{"/api/", "v1", "/api/v1"},
		{"api", "v1", "api/v1"},
		{"api/", "/v1", "api/v1"},
	}

	for _, tt := range tests {
		result := SingleSlashJoin(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("SingleSlashJoin(%q, %q) = %q, expected %q", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestEnsureLeadingSlash(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"", "/"},
		{"/", "/"},
		{"api", "/api"},
		{"/api", "/api"},
		{"api/v1", "/api/v1"},
		{"/api/v1", "/api/v1"},
	}

	for _, tt := range tests {
		result := EnsureLeadingSlash(tt.input)
		if result != tt.expected {
			t.Errorf("EnsureLeadingSlash(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestIsHopByHopHeader(t *testing.T) {
	hopHeaders := []string{
		"Connection", "connection", "CONNECTION",
		"Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer",
		"Transfer-Encoding", "Upgrade",
	}

	for _, h := range hopHeaders {
		if !IsHopByHopHeader(h) {
			t.Errorf("expected %q to be hop-by-hop header", h)
		}
	}

	normalHeaders := []string{
		"Content-Type", "Content-Length", "Authorization",
		"Accept", "User-Agent", "X-Custom-Header",
	}

	for _, h := range normalHeaders {
		if IsHopByHopHeader(h) {
			t.Errorf("expected %q to NOT be hop-by-hop header", h)
		}
	}
}

func TestCopyHeadersForUpstream(t *testing.T) {
	src := http.Header{
		"Content-Type":      []string{"application/json"},
		"Authorization":     []string{"Bearer token"},
		"Connection":        []string{"keep-alive"}, // hop-by-hop
		"Transfer-Encoding": []string{"chunked"},    // hop-by-hop
		"X-Custom":          []string{"value"},
	}

	dst := make(http.Header)
	CopyHeadersForUpstream(dst, src)

	if dst.Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type to be copied")
	}
	if dst.Get("Authorization") != "Bearer token" {
		t.Error("expected Authorization to be copied")
	}
	if dst.Get("X-Custom") != "value" {
		t.Error("expected X-Custom to be copied")
	}
	if dst.Get("Connection") != "" {
		t.Error("expected Connection to NOT be copied (hop-by-hop)")
	}
	if dst.Get("Transfer-Encoding") != "" {
		t.Error("expected Transfer-Encoding to NOT be copied (hop-by-hop)")
	}
}

func TestCloneHeaderSanitized(t *testing.T) {
	src := http.Header{
		"Content-Type": []string{"text/html"},
		"Connection":   []string{"close"}, // hop-by-hop
		"X-Custom":     []string{"value"},
	}

	dst := CloneHeaderSanitized(src)

	if dst.Get("Content-Type") != "text/html" {
		t.Error("expected Content-Type to be cloned")
	}
	if dst.Get("X-Custom") != "value" {
		t.Error("expected X-Custom to be cloned")
	}
	if dst.Get("Connection") != "" {
		t.Error("expected Connection to NOT be cloned (hop-by-hop)")
	}
}

func TestRequestContextWithTimeout(t *testing.T) {
	// Parent context with no deadline
	parent := context.Background()
	ctx, cancel := RequestContextWithTimeout(parent, 1*time.Second)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Error("expected context to have deadline")
	}
	if time.Until(deadline) > 1*time.Second {
		t.Error("expected deadline within 1 second")
	}

	// Parent context with shorter deadline
	parentWithDeadline, cancelParent := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelParent()
	ctx2, cancel2 := RequestContextWithTimeout(parentWithDeadline, 1*time.Second)
	defer cancel2()

	// Should keep parent's shorter deadline
	deadline2, ok := ctx2.Deadline()
	if !ok {
		t.Error("expected context to have deadline")
	}
	if time.Until(deadline2) > 150*time.Millisecond {
		t.Error("expected to keep parent's shorter deadline")
	}
}

func TestZeroOrExpiry(t *testing.T) {
	// Zero TTL
	result := ZeroOrExpiry(0)
	if !result.IsZero() {
		t.Error("expected zero time for TTL=0")
	}

	// Negative TTL
	result = ZeroOrExpiry(-1 * time.Second)
	if !result.IsZero() {
		t.Error("expected zero time for negative TTL")
	}

	// Positive TTL
	ttl := 5 * time.Second
	result = ZeroOrExpiry(ttl)
	if result.IsZero() {
		t.Error("expected non-zero time for positive TTL")
	}
	expected := time.Now().Add(ttl)
	if result.Before(expected.Add(-100*time.Millisecond)) || result.After(expected.Add(100*time.Millisecond)) {
		t.Error("expected expiry time around now + TTL")
	}
}
