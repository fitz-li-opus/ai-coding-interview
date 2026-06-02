package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestRoutesForwardToCorrectUpstream(t *testing.T) {
	upstreams := map[string]*httptest.Server{}
	for name := range map[string]struct{}{"users": {}, "orders": {}, "products": {}} {
		name := name
		upstreams[name] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(name))
		}))
		defer upstreams[name].Close()
	}

	server := httptest.NewServer(newServer(testConfig(t, upstreams["users"].URL, upstreams["orders"].URL, upstreams["products"].URL)))
	defer server.Close()

	tests := []struct {
		path string
		want string
	}{
		{path: "/api/users/", want: "users"},
		{path: "/api/orders/", want: "orders"},
		{path: "/api/products/", want: "products"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != tt.want {
				t.Fatalf("body = %q, want %q", string(body), tt.want)
			}
		})
	}
}

func TestStrictRouteMatchingRejectsNearMisses(t *testing.T) {
	var upstreamHits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamHits, 1)
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer upstream.Close()

	server := httptest.NewServer(newServer(testConfig(t, upstream.URL, upstream.URL, upstream.URL)))
	defer server.Close()

	for _, path := range []string{"/api/users-extra", "/api/ordersx", "/api/products-extra"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(server.URL + path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
			}
		})
	}

	if got := atomic.LoadInt32(&upstreamHits); got != 0 {
		t.Fatalf("upstream hits = %d, want 0", got)
	}
}

func TestPathRewritingAndQueryPreservation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RequestURI()))
	}))
	defer upstream.Close()

	server := httptest.NewServer(newServer(testConfig(t, upstream.URL, upstream.URL, upstream.URL)))
	defer server.Close()

	tests := []struct {
		path string
		want string
	}{
		{path: "/api/users", want: "/"},
		{path: "/api/users/", want: "/"},
		{path: "/api/orders/health", want: "/health"},
		{path: "/api/products/search?q=tea&active=true", want: "/search?q=tea&active=true"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get(server.URL + tt.path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != tt.want {
				t.Fatalf("upstream request URI = %q, want %q", string(body), tt.want)
			}
		})
	}
}

func TestProxyPreservesMethodAndBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
		}
		_, _ = w.Write([]byte(r.Method + " " + string(body)))
	}))
	defer upstream.Close()

	server := httptest.NewServer(newServer(testConfig(t, upstream.URL, upstream.URL, upstream.URL)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/orders/submit", "application/json", bytes.NewBufferString(`{"id":42}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != `POST {"id":42}` {
		t.Fatalf("method/body = %q, want %q", string(body), `POST {"id":42}`)
	}
}

func TestUnmatchedRoutesAndReadyReturnNotFound(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer upstream.Close()

	server := httptest.NewServer(newServer(testConfig(t, upstream.URL, upstream.URL, upstream.URL)))
	defer server.Close()

	for _, path := range []string{"/api/inventory/", "/ready"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(server.URL + path)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
			}
		})
	}
}

func TestUnavailableUpstreamReturnsGatewayError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("closed"))
	}))
	upstreamURL := upstream.URL
	upstream.Close()

	server := httptest.NewServer(newServer(testConfig(t, upstreamURL, upstreamURL, upstreamURL)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/users/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadGateway)
	}
}

func TestGatewayRateLimitRejectsRequestsOverLimit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	cfg := testConfig(t, upstream.URL, upstream.URL, upstream.URL)
	cfg.RateLimitRequests = 1
	cfg.RateLimitWindow = time.Minute
	server := httptest.NewServer(newServer(cfg))
	defer server.Close()

	first, err := http.Get(server.URL + "/api/users/")
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first status = %d, want %d", first.StatusCode, http.StatusOK)
	}

	second, err := http.Get(server.URL + "/api/users/")
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.StatusCode, http.StatusTooManyRequests)
	}
}

func TestFixedWindowRateLimiterResetsAfterWindow(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	limiter := NewFixedWindowRateLimiter(1, time.Second)
	limiter.now = func() time.Time { return now }
	limiter.windowStart = now

	if !limiter.Allow() {
		t.Fatal("first request rejected, want allowed")
	}
	if limiter.Allow() {
		t.Fatal("second request allowed, want rejected")
	}

	now = now.Add(time.Second)
	if !limiter.Allow() {
		t.Fatal("request after window rejected, want allowed")
	}
}

func testConfig(t *testing.T, userURL, orderURL, productURL string) Config {
	t.Helper()
	return Config{
		Port:              defaultPort,
		UserServiceURL:    mustParseURL(t, userURL),
		OrderServiceURL:   mustParseURL(t, orderURL),
		ProductServiceURL: mustParseURL(t, productURL),
		UpstreamTimeout:   500 * time.Millisecond,
		RateLimitRequests: 1000,
		RateLimitWindow:   time.Minute,
	}
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	return parsed
}
