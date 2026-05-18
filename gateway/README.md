# API Gateway

A production-grade API Gateway built with Bun that routes requests to backend microservices.

## Features

### Core Features
- **Request Routing**: Routes requests based on path prefixes to appropriate backend services
- **Load Balancing**: Round-robin load balancing across multiple backend instances
- **Health Checks**: Gateway health endpoint at `/health` with metrics and circuit breaker status

### Reliability & Performance
- **Circuit Breaker**: Automatically fails fast when backend services are unhealthy
- **Request Timeout**: Configurable timeout to prevent hanging requests (default: 30s)
- **Request ID Tracing**: Unique `X-Request-ID` header for distributed tracing
- **Response Time Tracking**: `X-Response-Time` header shows request latency

### Security & Rate Limiting
- **Rate Limiting**: Per-IP rate limiting to prevent abuse (default: 100 req/min)
- **CORS Support**: Configurable CORS headers for cross-origin requests
- **Header Filtering**: Removes hop-by-hop headers for proper proxying

### Monitoring
- **Metrics Collection**: Tracks total requests, success/failure rates, and average latency
- **Request Logging**: Detailed logs with timestamps and request IDs
- **Circuit Breaker Monitoring**: Real-time circuit breaker state in health endpoint

## Installation

```bash
cd gateway
bun install
```

## Running the Gateway

```bash
bun start
```

The gateway will start on `http://localhost:8080`

## Testing

### Run Unit Tests

Unit tests verify the gateway's core functionality without requiring backend services:

```bash
bun test index.test.ts
```

### Run Integration Tests

Integration tests require both the gateway and backend services to be running:

```bash
# Terminal 1: Start backend services
docker compose up --build

# Terminal 2: Start the gateway
cd gateway
bun start

# Terminal 3: Run integration tests
cd gateway
bun test integration.test.ts
```

### Run All Tests

```bash
bun test
```

### Watch Mode

Run tests in watch mode for development:

```bash
bun test --watch
```

### Load Testing

Test the gateway's performance under load:

```bash
# Small load test (100 requests, 10 concurrent)
bun run load-test:small

# Medium load test (1000 requests, 50 concurrent)
bun run load-test:medium

# Large load test (5000 requests, 100 concurrent)
bun run load-test:large

# Custom load test
bun run load-test.ts [total_requests] [concurrency]
```

For detailed testing documentation, see [TESTING.md](./TESTING.md).

## Configuration

The gateway can be configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Gateway listening port |
| `REQUEST_TIMEOUT` | `30000` | Request timeout in milliseconds |
| `RATE_LIMIT_WINDOW` | `60000` | Rate limit window in milliseconds |
| `RATE_LIMIT_MAX_REQUESTS` | `100` | Max requests per window |
| `CORS_ENABLED` | `true` | Enable CORS headers |
| `USER_SERVICE_URLS` | `http://localhost:8081` | Comma-separated user service URLs |
| `ORDER_SERVICE_URLS` | `http://localhost:8082` | Comma-separated order service URLs |
| `PRODUCT_SERVICE_URLS` | `http://localhost:8083` | Comma-separated product service URLs |

Example with custom configuration:

```bash
PORT=3000 REQUEST_TIMEOUT=10000 RATE_LIMIT_MAX_REQUESTS=50 bun start
```

## Routing Configuration

| Path Prefix | Backend Service | Default Port |
|-------------|----------------|--------------|
| `/api/users/**` | user-service | 8081 |
| `/api/orders/**` | order-service | 8082 |
| `/api/products/**` | product-service | 8083 |

## Usage Examples

### Check Gateway Health
```bash
curl http://localhost:8080/health
```

Response includes metrics and circuit breaker status:
```json
{
  "status": "healthy",
  "service": "api-gateway",
  "timestamp": "2026-05-18T10:30:00.000Z",
  "metrics": {
    "totalRequests": 150,
    "successfulRequests": 145,
    "failedRequests": 5,
    "averageLatency": 45
  },
  "circuitBreakers": [
    {
      "service": "http://localhost:8081",
      "state": "closed",
      "failures": 0
    }
  ]
}
```

### Access User Service
```bash
curl http://localhost:8080/api/users/
```

### Access Order Service
```bash
curl http://localhost:8080/api/orders/
```

### Access Product Service
```bash
curl http://localhost:8080/api/products/
```

### Check Backend Service Health
```bash
curl http://localhost:8080/api/users/health
curl http://localhost:8080/api/orders/health
curl http://localhost:8080/api/products/health
```

### Test with Request ID Tracing
```bash
curl -v http://localhost:8080/api/users/
```

Look for `X-Request-ID` and `X-Response-Time` headers in the response.

## Architecture

### Request Flow
1. Client sends request to gateway
2. Gateway generates unique request ID
3. Rate limiting check (per client IP)
4. Route matching based on path prefix
5. Circuit breaker check for target service
6. Load balancer selects backend instance (round-robin)
7. Request forwarded with filtered headers and tracing info
8. Response returned with CORS headers and metrics

### Circuit Breaker
- **Threshold**: 5 consecutive failures
- **Timeout**: 30 seconds
- **States**: 
  - `closed`: Normal operation
  - `open`: Service unavailable, fail fast
  - `half-open`: Testing if service recovered

### Rate Limiting
- Per-IP address tracking
- Sliding window algorithm
- Returns `429 Too Many Requests` when exceeded
- Includes `X-RateLimit-*` headers

### Load Balancing
- Round-robin algorithm
- Supports multiple backend instances per service
- Configure via comma-separated URLs in environment variables

## Error Handling

| Status Code | Description |
|-------------|-------------|
| `404 Not Found` | No route matches the request path |
| `429 Too Many Requests` | Rate limit exceeded |
| `503 Service Unavailable` | Backend service unreachable or circuit breaker open |
| `504 Gateway Timeout` | Request exceeded timeout threshold |

## Headers

### Request Headers Added
- `X-Request-ID`: Unique request identifier
- `X-Forwarded-For`: Client IP address
- `X-Forwarded-Proto`: Original protocol (http/https)
- `X-Forwarded-Host`: Original host

### Response Headers Added
- `X-Request-ID`: Same as request for tracing
- `X-Response-Time`: Request latency in milliseconds
- `Access-Control-Allow-*`: CORS headers (if enabled)

## Production Considerations

### Implemented
✅ Request timeout handling  
✅ Circuit breaker pattern  
✅ Rate limiting  
✅ Load balancing  
✅ Request ID tracing  
✅ Metrics collection  
✅ CORS support  
✅ Header filtering  
✅ Error handling  

### Future Enhancements
- Authentication/Authorization middleware
- Request/response transformation hooks
- Advanced load balancing (weighted, least-connections)
- Persistent metrics storage
- WebSocket support
- SSL/TLS termination
