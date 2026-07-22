# API Gateway Design Spec

## Overview

Implement an API gateway in Go that routes incoming requests to three backend microservices, with rate limiting, caching, and load balancing support.

## Requirements

### Core
- Gateway listens on port 8080
- Route requests based on path prefix (preserve original path):
  - `/api/users/**` → user-service backends
  - `/api/orders/**` → order-service backends
  - `/api/products/**` → product-service backends
- Return 404 for unmatched routes

### Additional Features
- **Rate Limiting**: Limit request frequency per client IP
- **Caching**: Cache GET responses to reduce backend load
- **Load Balancing**: Distribute requests across multiple backend instances

## Technical Approach

**Language**: Go (standard library only)

## Architecture

```
Client Request
      ↓
  Gateway (:8080)
      ↓
  Rate Limiter (per IP)
      ↓
  Cache Check (GET only)
      ↓ (cache miss)
  Route Matching
      ↓
  Load Balancer (round-robin)
      ↓
  Reverse Proxy → Backend Service
      ↓
  Cache Store (GET only)
      ↓
  Response to Client
```

## Route Table

| Path Prefix      | Target Backends                    |
|------------------|------------------------------------|
| `/api/users`     | `http://localhost:8081` (可扩展)   |
| `/api/orders`    | `http://localhost:8082` (可扩展)   |
| `/api/products`  | `http://localhost:8083` (可扩展)   |

## Feature Details

### 1. Rate Limiting

- **Algorithm**: Token bucket (简化版：固定窗口计数)
- **Scope**: Per client IP
- **Default**: 100 requests per minute
- **Response**: HTTP 429 Too Many Requests when exceeded

```go
type RateLimiter struct {
    requests map[string][]time.Time  // IP -> request timestamps
    limit    int                      // max requests
    window   time.Duration            // time window
    mu       sync.Mutex
}
```

### 2. Caching

- **Scope**: GET requests only
- **Key**: Request path + query string
- **TTL**: 60 seconds (configurable)
- **Storage**: In-memory map
- **Headers**: Add `X-Cache: HIT` or `X-Cache: MISS`

```go
type Cache struct {
    entries map[string]CacheEntry
    ttl     time.Duration
    mu      sync.RWMutex
}

type CacheEntry struct {
    body       []byte
    headers    http.Header
    statusCode int
    expiry     time.Time
}
```

### 3. Load Balancing

- **Algorithm**: Round-robin
- **Scope**: Per service (each service can have multiple backends)
- **State**: Atomic counter per service

```go
type LoadBalancer struct {
    backends map[string][]string  // service -> backend URLs
    counters map[string]*uint64   // service -> counter
}
```

## File Structure

```
gateway/
├── main.go           # Entry point, HTTP server
├── router.go         # Route matching logic
├── proxy.go          # Reverse proxy implementation
├── ratelimit.go      # Rate limiter
├── cache.go          # Response cache
└── loadbalancer.go   # Load balancer
```

## Request Flow Example

```
GET /api/users/123
      ↓
Rate Limiter: Check IP 192.168.1.1 → OK (50/100 requests used)
      ↓
Cache: Check "/api/users/123" → MISS
      ↓
Router: Match "/api/users" → user-service
      ↓
Load Balancer: Round-robin → http://localhost:8081
      ↓
Proxy: Forward to http://localhost:8081/api/users/123
      ↓
Cache: Store response (TTL=60s)
      ↓
Response: 200 OK + X-Cache: MISS
```

## Testing

```bash
# Start backend services
docker compose up --build

# Start gateway
go run gateway/*.go

# Test basic routing
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products

# Test caching (second request should be faster)
curl -i http://localhost:8080/api/users  # X-Cache: MISS
curl -i http://localhost:8080/api/users  # X-Cache: HIT

# Test rate limiting (run many requests quickly)
for i in {1..110}; do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/users; done

# Test 404
curl http://localhost:8080/api/unknown
```

## Configuration

可通过环境变量配置：

| Variable | Default | Description |
|----------|---------|-------------|
| `GATEWAY_PORT` | 8080 | Gateway listen port |
| `RATE_LIMIT` | 100 | Requests per minute per IP |
| `CACHE_TTL` | 60 | Cache TTL in seconds |
