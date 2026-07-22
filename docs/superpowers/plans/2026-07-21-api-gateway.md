# API Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an API gateway in Go that routes requests to backend microservices with rate limiting, caching, and load balancing.

**Architecture:** Single HTTP server using standard library. Middleware chain: Rate Limiter → Cache → Router → Load Balancer → Reverse Proxy. Each component is a separate file with clear interfaces.

**Tech Stack:** Go 1.21+, standard library only (net/http, net/http/httputil, sync, sync/atomic)

---

## File Structure

```
gateway/
├── main.go           # Entry point, HTTP server, middleware chain
├── router.go         # Route matching: path prefix → service name
├── loadbalancer.go   # Round-robin backend selection
├── proxy.go          # Reverse proxy using httputil.ReverseProxy
├── ratelimit.go      # Per-IP rate limiting with sliding window
├── cache.go          # In-memory GET response cache
├── router_test.go    # Router tests
├── loadbalancer_test.go
├── ratelimit_test.go
└── cache_test.go
```

---

## Task 1: Initialize Go Module

**Files:**
- Create: `gateway/go.mod`

- [ ] **Step 1: Create go.mod**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway
go mod init gateway
```

Expected output: `go: creating new go.mod: module gateway`

- [ ] **Step 2: Verify go.mod exists**

```bash
cat /Users/linguo/Desktop/mycode/ai-coding-interview/gateway/go.mod
```

Expected:
```
module gateway

go 1.21
```

- [ ] **Step 3: Commit**

```bash
git add gateway/go.mod
git commit -m "feat(gateway): initialize Go module"
```

---

## Task 2: Implement Router

**Files:**
- Create: `gateway/router.go`
- Create: `gateway/router_test.go`

- [ ] **Step 1: Write the failing test**

Create `gateway/router_test.go`:

```go
package main

import (
	"testing"
)

func TestRouterMatch(t *testing.T) {
	router := NewRouter()
	router.AddRoute("/api/users", "user-service")
	router.AddRoute("/api/orders", "order-service")
	router.AddRoute("/api/products", "product-service")

	tests := []struct {
		path        string
		wantService string
		wantFound   bool
	}{
		{"/api/users", "user-service", true},
		{"/api/users/123", "user-service", true},
		{"/api/users/profile/settings", "user-service", true},
		{"/api/orders", "order-service", true},
		{"/api/orders/456", "order-service", true},
		{"/api/products", "product-service", true},
		{"/api/unknown", "", false},
		{"/health", "", false},
		{"/", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			service, found := router.Match(tt.path)
			if found != tt.wantFound {
				t.Errorf("Match(%q) found = %v, want %v", tt.path, found, tt.wantFound)
			}
			if service != tt.wantService {
				t.Errorf("Match(%q) service = %q, want %q", tt.path, service, tt.wantService)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestRouterMatch -v
```

Expected: FAIL with "undefined: NewRouter"

- [ ] **Step 3: Write minimal implementation**

Create `gateway/router.go`:

```go
package main

import (
	"strings"
)

// Router matches URL paths to service names.
type Router struct {
	routes map[string]string // prefix -> service name
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{
		routes: make(map[string]string),
	}
}

// AddRoute registers a path prefix to a service.
func (r *Router) AddRoute(prefix, service string) {
	r.routes[prefix] = service
}

// Match finds the service for a given path.
// Returns the service name and true if found, empty string and false otherwise.
func (r *Router) Match(path string) (string, bool) {
	// Find longest matching prefix
	var matchedPrefix string
	var matchedService string

	for prefix, service := range r.routes {
		if strings.HasPrefix(path, prefix) {
			if len(prefix) > len(matchedPrefix) {
				matchedPrefix = prefix
				matchedService = service
			}
		}
	}

	if matchedPrefix == "" {
		return "", false
	}
	return matchedService, true
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestRouterMatch -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add gateway/router.go gateway/router_test.go
git commit -m "feat(gateway): implement router with prefix matching"
```

---

## Task 3: Implement Load Balancer

**Files:**
- Create: `gateway/loadbalancer.go`
- Create: `gateway/loadbalancer_test.go`

- [ ] **Step 1: Write the failing test**

Create `gateway/loadbalancer_test.go`:

```go
package main

import (
	"testing"
)

func TestLoadBalancerRoundRobin(t *testing.T) {
	lb := NewLoadBalancer()
	lb.AddBackends("user-service", []string{
		"http://localhost:8081",
		"http://localhost:8181",
		"http://localhost:8281",
	})

	// Round-robin should cycle through backends
	results := make([]string, 6)
	for i := 0; i < 6; i++ {
		results[i] = lb.Next("user-service")
	}

	expected := []string{
		"http://localhost:8081",
		"http://localhost:8181",
		"http://localhost:8281",
		"http://localhost:8081",
		"http://localhost:8181",
		"http://localhost:8281",
	}

	for i, want := range expected {
		if results[i] != want {
			t.Errorf("Next() call %d = %q, want %q", i, results[i], want)
		}
	}
}

func TestLoadBalancerSingleBackend(t *testing.T) {
	lb := NewLoadBalancer()
	lb.AddBackends("order-service", []string{"http://localhost:8082"})

	for i := 0; i < 3; i++ {
		got := lb.Next("order-service")
		if got != "http://localhost:8082" {
			t.Errorf("Next() = %q, want http://localhost:8082", got)
		}
	}
}

func TestLoadBalancerUnknownService(t *testing.T) {
	lb := NewLoadBalancer()
	got := lb.Next("unknown-service")
	if got != "" {
		t.Errorf("Next(unknown) = %q, want empty string", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestLoadBalancer -v
```

Expected: FAIL with "undefined: NewLoadBalancer"

- [ ] **Step 3: Write minimal implementation**

Create `gateway/loadbalancer.go`:

```go
package main

import (
	"sync/atomic"
)

// LoadBalancer distributes requests across backends using round-robin.
type LoadBalancer struct {
	backends map[string][]string  // service -> backend URLs
	counters map[string]*uint64   // service -> atomic counter
}

// NewLoadBalancer creates a new LoadBalancer.
func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		backends: make(map[string][]string),
		counters: make(map[string]*uint64),
	}
}

// AddBackends registers backend URLs for a service.
func (lb *LoadBalancer) AddBackends(service string, urls []string) {
	lb.backends[service] = urls
	var counter uint64
	lb.counters[service] = &counter
}

// Next returns the next backend URL for a service using round-robin.
// Returns empty string if service is not found.
func (lb *LoadBalancer) Next(service string) string {
	backends, ok := lb.backends[service]
	if !ok || len(backends) == 0 {
		return ""
	}

	counter := lb.counters[service]
	index := atomic.AddUint64(counter, 1) - 1
	return backends[index%uint64(len(backends))]
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestLoadBalancer -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add gateway/loadbalancer.go gateway/loadbalancer_test.go
git commit -m "feat(gateway): implement round-robin load balancer"
```

---

## Task 4: Implement Rate Limiter

**Files:**
- Create: `gateway/ratelimit.go`
- Create: `gateway/ratelimit_test.go`

- [ ] **Step 1: Write the failing test**

Create `gateway/ratelimit_test.go`:

```go
package main

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(5, time.Minute)

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied
	if rl.Allow("192.168.1.1") {
		t.Error("Request 6 should be denied")
	}
}

func TestRateLimiterDifferentIPs(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// Each IP has its own limit
	if !rl.Allow("192.168.1.1") {
		t.Error("IP1 request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("IP1 request 2 should be allowed")
	}
	if rl.Allow("192.168.1.1") {
		t.Error("IP1 request 3 should be denied")
	}

	// Different IP should have fresh limit
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 request 2 should be allowed")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)

	if !rl.Allow("192.168.1.1") {
		t.Error("Request 1 should be allowed")
	}
	if !rl.Allow("192.168.1.1") {
		t.Error("Request 2 should be allowed")
	}
	if rl.Allow("192.168.1.1") {
		t.Error("Request 3 should be denied")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow("192.168.1.1") {
		t.Error("Request after window expiry should be allowed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestRateLimiter -v
```

Expected: FAIL with "undefined: NewRateLimiter"

- [ ] **Step 3: Write minimal implementation**

Create `gateway/ratelimit.go`:

```go
package main

import (
	"sync"
	"time"
)

// RateLimiter limits requests per IP using a sliding window.
type RateLimiter struct {
	requests map[string][]time.Time
	limit    int
	window   time.Duration
	mu       sync.Mutex
}

// NewRateLimiter creates a new RateLimiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a request from the given IP is allowed.
// Returns true if allowed, false if rate limited.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Get existing requests and filter out expired ones
	requests := rl.requests[ip]
	var valid []time.Time
	for _, t := range requests {
		if t.After(windowStart) {
			valid = append(valid, t)
		}
	}

	// Check if under limit
	if len(valid) >= rl.limit {
		rl.requests[ip] = valid
		return false
	}

	// Allow request and record it
	rl.requests[ip] = append(valid, now)
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestRateLimiter -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add gateway/ratelimit.go gateway/ratelimit_test.go
git commit -m "feat(gateway): implement sliding window rate limiter"
```

---

## Task 5: Implement Cache

**Files:**
- Create: `gateway/cache.go`
- Create: `gateway/cache_test.go`

- [ ] **Step 1: Write the failing test**

Create `gateway/cache_test.go`:

```go
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache(time.Minute)

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	cache.Set("/api/users", 200, headers, []byte(`{"users":[]}`))

	entry, found := cache.Get("/api/users")
	if !found {
		t.Fatal("Expected cache hit")
	}
	if entry.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", entry.StatusCode)
	}
	if string(entry.Body) != `{"users":[]}` {
		t.Errorf("Body = %q, want %q", entry.Body, `{"users":[]}`)
	}
	if entry.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", entry.Headers.Get("Content-Type"))
	}
}

func TestCacheMiss(t *testing.T) {
	cache := NewCache(time.Minute)

	_, found := cache.Get("/api/unknown")
	if found {
		t.Error("Expected cache miss for unknown key")
	}
}

func TestCacheExpiry(t *testing.T) {
	cache := NewCache(50 * time.Millisecond)

	cache.Set("/api/users", 200, http.Header{}, []byte("data"))

	// Should be found immediately
	_, found := cache.Get("/api/users")
	if !found {
		t.Error("Expected cache hit before expiry")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	_, found = cache.Get("/api/users")
	if found {
		t.Error("Expected cache miss after expiry")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestCache -v
```

Expected: FAIL with "undefined: NewCache"

- [ ] **Step 3: Write minimal implementation**

Create `gateway/cache.go`:

```go
package main

import (
	"net/http"
	"sync"
	"time"
)

// CacheEntry stores a cached response.
type CacheEntry struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	expiry     time.Time
}

// Cache stores HTTP responses with TTL expiration.
type Cache struct {
	entries map[string]CacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

// NewCache creates a new Cache with the given TTL.
func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]CacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a cached entry by key.
// Returns the entry and true if found and not expired, nil and false otherwise.
func (c *Cache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiry) {
		return nil, false
	}

	return &entry, true
}

// Set stores a response in the cache.
func (c *Cache) Set(key string, statusCode int, headers http.Header, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clone headers to avoid mutation
	clonedHeaders := headers.Clone()

	c.entries[key] = CacheEntry{
		StatusCode: statusCode,
		Headers:    clonedHeaders,
		Body:       body,
		expiry:     time.Now().Add(c.ttl),
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go test -run TestCache -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add gateway/cache.go gateway/cache_test.go
git commit -m "feat(gateway): implement in-memory response cache with TTL"
```

---

## Task 6: Implement Reverse Proxy

**Files:**
- Create: `gateway/proxy.go`

- [ ] **Step 1: Write the implementation**

Create `gateway/proxy.go`:

```go
package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// ProxyHandler handles reverse proxying to backend services.
type ProxyHandler struct {
	router   *Router
	lb       *LoadBalancer
	cache    *Cache
	limiter  *RateLimiter
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(router *Router, lb *LoadBalancer, cache *Cache, limiter *RateLimiter) *ProxyHandler {
	return &ProxyHandler{
		router:  router,
		lb:      lb,
		cache:   cache,
		limiter: limiter,
	}
}

// responseRecorder captures the response for caching.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

// ServeHTTP handles incoming requests.
func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get client IP
	clientIP := r.RemoteAddr

	// Rate limiting
	if !p.limiter.Allow(clientIP) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Check cache for GET requests
	cacheKey := r.URL.Path + "?" + r.URL.RawQuery
	if r.Method == http.MethodGet {
		if entry, found := p.cache.Get(cacheKey); found {
			// Cache hit
			for key, values := range entry.Headers {
				for _, v := range values {
					w.Header().Add(key, v)
				}
			}
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(entry.StatusCode)
			w.Write(entry.Body)
			return
		}
	}

	// Route matching
	service, found := p.router.Match(r.URL.Path)
	if !found {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Load balancing
	backendURL := p.lb.Next(service)
	if backendURL == "" {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Parse backend URL
	target, err := url.Parse(backendURL)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Modify the request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	// For GET requests, capture response for caching
	if r.Method == http.MethodGet {
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		proxy.ModifyResponse = func(resp *http.Response) error {
			// Read the body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			resp.Body.Close()

			// Cache the response
			p.cache.Set(cacheKey, resp.StatusCode, resp.Header, body)

			// Put the body back
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}

		w.Header().Set("X-Cache", "MISS")
		proxy.ServeHTTP(recorder, r)
	} else {
		proxy.ServeHTTP(w, r)
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go build -o /dev/null .
```

Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add gateway/proxy.go
git commit -m "feat(gateway): implement reverse proxy with caching support"
```

---

## Task 7: Implement Main Entry Point

**Files:**
- Create: `gateway/main.go`

- [ ] **Step 1: Write the implementation**

Create `gateway/main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func main() {
	// Configuration from environment
	port := getEnvInt("GATEWAY_PORT", 8080)
	rateLimit := getEnvInt("RATE_LIMIT", 100)
	cacheTTL := getEnvInt("CACHE_TTL", 60)

	// Initialize components
	router := NewRouter()
	router.AddRoute("/api/users", "user-service")
	router.AddRoute("/api/orders", "order-service")
	router.AddRoute("/api/products", "product-service")

	lb := NewLoadBalancer()
	lb.AddBackends("user-service", []string{"http://localhost:8081"})
	lb.AddBackends("order-service", []string{"http://localhost:8082"})
	lb.AddBackends("product-service", []string{"http://localhost:8083"})

	cache := NewCache(time.Duration(cacheTTL) * time.Second)
	limiter := NewRateLimiter(rateLimit, time.Minute)

	// Create handler
	handler := NewProxyHandler(router, lb, cache, limiter)

	// Start server
	addr := ":" + strconv.Itoa(port)
	log.Printf("API Gateway listening on %s", addr)
	log.Printf("Rate limit: %d requests/minute, Cache TTL: %ds", rateLimit, cacheTTL)
	log.Fatal(http.ListenAndServe(addr, handler))
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go build -o gateway .
```

Expected: Creates `gateway` executable

- [ ] **Step 3: Commit**

```bash
git add gateway/main.go
git commit -m "feat(gateway): add main entry point with configuration"
```

---

## Task 8: Integration Testing

**Files:**
- None (manual testing)

- [ ] **Step 1: Start backend services**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview && docker compose up --build -d
```

Wait for services to be healthy.

- [ ] **Step 2: Start gateway**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview/gateway && go run *.go
```

Expected output:
```
API Gateway listening on :8080
Rate limit: 100 requests/minute, Cache TTL: 60s
```

- [ ] **Step 3: Test basic routing**

In a new terminal:

```bash
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products
```

Expected: JSON responses from each service

- [ ] **Step 4: Test 404 for unknown routes**

```bash
curl -i http://localhost:8080/api/unknown
```

Expected: HTTP 404 Not Found

- [ ] **Step 5: Test caching**

```bash
curl -i http://localhost:8080/api/users 2>&1 | grep X-Cache
curl -i http://localhost:8080/api/users 2>&1 | grep X-Cache
```

Expected:
- First request: `X-Cache: MISS`
- Second request: `X-Cache: HIT`

- [ ] **Step 6: Test rate limiting**

```bash
for i in {1..105}; do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/users; done | tail -10
```

Expected: Should see some `429` responses after exceeding 100 requests

- [ ] **Step 7: Clean up**

```bash
cd /Users/linguo/Desktop/mycode/ai-coding-interview && docker compose down
```

- [ ] **Step 8: Final commit**

```bash
git add -A
git commit -m "feat(gateway): complete API gateway implementation"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Initialize Go module | `go.mod` |
| 2 | Implement Router | `router.go`, `router_test.go` |
| 3 | Implement Load Balancer | `loadbalancer.go`, `loadbalancer_test.go` |
| 4 | Implement Rate Limiter | `ratelimit.go`, `ratelimit_test.go` |
| 5 | Implement Cache | `cache.go`, `cache_test.go` |
| 6 | Implement Reverse Proxy | `proxy.go` |
| 7 | Implement Main | `main.go` |
| 8 | Integration Testing | Manual verification |
