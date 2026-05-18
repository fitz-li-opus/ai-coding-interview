# Quick Start Guide

Get the API Gateway up and running in 5 minutes.

## Prerequisites

- [Bun](https://bun.sh/) installed
- [Docker](https://www.docker.com/) and Docker Compose installed

## Step 1: Start Backend Services

```bash
# From the project root directory
docker compose up --build
```

Wait for all services to start. You should see:
```
✔ user-service running on :8081
✔ order-service running on :8082
✔ product-service running on :8083
```

Verify services are running:
```bash
curl http://localhost:8081/
curl http://localhost:8082/
curl http://localhost:8083/
```

## Step 2: Install Gateway Dependencies

```bash
cd gateway
bun install
```

## Step 3: Start the Gateway

```bash
bun start
```

You should see:
```
🚀 API Gateway running on http://localhost:8080

Configuration:
  Request Timeout: 30000ms
  Rate Limit: 100 requests per 60s
  CORS Enabled: true
  Circuit Breaker Threshold: 5 failures

Configured routes:
  /api/users -> http://localhost:8081
  /api/orders -> http://localhost:8082
  /api/products -> http://localhost:8083

Health check: http://localhost:8080/health
```

## Step 4: Test the Gateway

### Check Gateway Health

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "service": "api-gateway",
  "timestamp": "2026-05-18T10:30:00.000Z",
  "metrics": {
    "totalRequests": 0,
    "successfulRequests": 0,
    "failedRequests": 0,
    "averageLatency": 0
  },
  "circuitBreakers": []
}
```

### Test User Service

```bash
curl http://localhost:8080/api/users/
```

### Test Order Service

```bash
curl http://localhost:8080/api/orders/
```

### Test Product Service

```bash
curl http://localhost:8080/api/products/
```

### Test with Verbose Output

```bash
curl -v http://localhost:8080/api/users/
```

Look for these headers:
- `X-Request-ID`: Unique request identifier
- `X-Response-Time`: Request latency
- `Access-Control-Allow-Origin`: CORS header

## Step 5: Run Tests (Optional)

### Unit Tests

```bash
bun test index.test.ts
```

### Integration Tests

Make sure backend services and gateway are running, then:

```bash
bun test integration.test.ts
```

### Load Test

```bash
bun run load-test:small
```

## Common Commands

```bash
# Start gateway in development mode (auto-reload)
bun run dev

# Run all tests
bun test

# Run tests in watch mode
bun test --watch

# Run load test
bun run load-test:medium

# Check gateway health
curl http://localhost:8080/health
```

## Configuration

Create a `.env` file in the `gateway/` directory to customize settings:

```bash
# .env
PORT=8080
REQUEST_TIMEOUT=30000
RATE_LIMIT_WINDOW=60000
RATE_LIMIT_MAX_REQUESTS=100
CORS_ENABLED=true
USER_SERVICE_URLS=http://localhost:8081
ORDER_SERVICE_URLS=http://localhost:8082
PRODUCT_SERVICE_URLS=http://localhost:8083
```

Then restart the gateway:

```bash
bun start
```

## Troubleshooting

### Gateway won't start

**Error**: `Address already in use`

**Solution**: Another process is using port 8080. Either:
1. Stop the other process
2. Change the port: `PORT=3000 bun start`

### Backend services not responding

**Error**: `Service unavailable` or `ECONNREFUSED`

**Solution**: 
1. Check if Docker containers are running: `docker compose ps`
2. Restart services: `docker compose restart`
3. Check logs: `docker compose logs`

### Tests failing

**Error**: Connection errors in tests

**Solution**:
1. Make sure backend services are running
2. Make sure gateway is running
3. Wait a few seconds for services to be ready

### Rate limit errors

**Error**: `429 Too Many Requests`

**Solution**: You've exceeded the rate limit. Either:
1. Wait 60 seconds for the window to reset
2. Increase the limit: `RATE_LIMIT_MAX_REQUESTS=1000 bun start`

## Next Steps

- Read the [README.md](./README.md) for detailed feature documentation
- Check [TESTING.md](./TESTING.md) for comprehensive testing guide
- Review [REVIEW.md](./REVIEW.md) for implementation details
- Explore the code in `index.ts` to understand the implementation

## Architecture Overview

```
┌─────────┐
│ Client  │
└────┬────┘
     │
     ▼
┌─────────────────────────────────────┐
│      API Gateway (:8080)            │
│                                     │
│  • Request Routing                  │
│  • Load Balancing                   │
│  • Rate Limiting                    │
│  • Circuit Breaker                  │
│  • Request Tracing                  │
│  • Metrics Collection               │
└─────────────────────────────────────┘
     │
     ├──────────┬──────────┬──────────┐
     ▼          ▼          ▼          ▼
┌─────────┐ ┌─────────┐ ┌─────────┐
│  User   │ │ Order   │ │ Product │
│ Service │ │ Service │ │ Service │
│  :8081  │ │  :8082  │ │  :8083  │
└─────────┘ └─────────┘ └─────────┘
```

## Key Features

✅ **Request Routing** - Path-based routing to backend services  
✅ **Load Balancing** - Round-robin across multiple backends  
✅ **Circuit Breaker** - Automatic failure detection and recovery  
✅ **Rate Limiting** - Per-IP rate limiting  
✅ **Request Tracing** - Unique ID for each request  
✅ **Metrics** - Real-time performance metrics  
✅ **CORS** - Full CORS support  
✅ **Timeout** - Configurable request timeout  
✅ **Health Checks** - Gateway and backend health monitoring  

## Support

For issues or questions:
1. Check the [README.md](./README.md) for detailed documentation
2. Review [TESTING.md](./TESTING.md) for testing help
3. Check [REVIEW.md](./REVIEW.md) for implementation details

Happy coding! 🚀
