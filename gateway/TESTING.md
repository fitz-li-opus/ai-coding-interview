# Testing Guide

This document describes the testing strategy and how to run tests for the API Gateway.

## Test Types

### 1. Unit Tests (`index.test.ts`)

Unit tests verify the gateway's core functionality in isolation. They test:

- Health check endpoint
- CORS support
- Request routing logic
- Request ID generation and tracing
- Response time tracking
- Rate limiting
- Error handling
- HTTP method support
- Metrics collection
- Load balancing logic
- Header forwarding

**Prerequisites**: None (tests run independently)

**Run unit tests**:
```bash
bun test index.test.ts
```

### 2. Integration Tests (`integration.test.ts`)

Integration tests verify the gateway works correctly with real backend services. They test:

- End-to-end request flow
- Backend health check proxying
- Response consistency with direct backend calls
- Query parameter forwarding
- Request body forwarding
- Concurrent request handling
- Performance overhead
- Circuit breaker integration
- Metrics accuracy
- Error scenarios
- Header propagation

**Prerequisites**: 
- Backend services must be running (`docker compose up`)
- Gateway must be running (`bun start`)

**Run integration tests**:
```bash
# Terminal 1: Start backend services
docker compose up --build

# Terminal 2: Start gateway
cd gateway
bun start

# Terminal 3: Run tests
cd gateway
bun test integration.test.ts
```

### 3. Load Tests (`load-test.ts`)

Load tests measure the gateway's performance under various load conditions.

**Prerequisites**: 
- Backend services must be running
- Gateway must be running

**Run load tests**:

```bash
# Small load (100 requests, 10 concurrent)
bun run load-test:small

# Medium load (1000 requests, 50 concurrent)
bun run load-test:medium

# Large load (5000 requests, 100 concurrent)
bun run load-test:large

# Custom load
bun run load-test.ts [total_requests] [concurrency]
```

**Example output**:
```
📊 Results:
   Total Requests:        1000
   Successful:            950 (95.00%)
   Failed:                0 (0.00%)
   Rate Limited:          50 (5.00%)

⏱️  Performance:
   Total Duration:        12500ms
   Average Latency:       45.23ms
   Min Latency:           12ms
   Max Latency:           234ms
   Requests/Second:       80.00

📈 Status Codes:
   200: 950 (95.00%)
   429: 50 (5.00%)
```

## Running All Tests

```bash
# Run all tests (unit + integration)
bun test

# Run tests in watch mode
bun test --watch

# Run specific test file
bun test index.test.ts
```

## Test Coverage

### What's Tested

✅ Request routing and path matching  
✅ Load balancing (round-robin)  
✅ Circuit breaker pattern  
✅ Rate limiting  
✅ Request timeout handling  
✅ CORS support  
✅ Request ID tracing  
✅ Response time tracking  
✅ Metrics collection  
✅ Error handling  
✅ Header filtering and forwarding  
✅ Query parameter forwarding  
✅ Request body forwarding  
✅ HTTP method support (GET, POST, PUT, DELETE)  
✅ Concurrent request handling  
✅ Performance overhead  

### What's Not Tested

❌ SSL/TLS termination (not implemented)  
❌ Authentication/Authorization (not implemented)  
❌ WebSocket proxying (not implemented)  
❌ Request/response transformation (not implemented)  

## Writing New Tests

### Unit Test Example

```typescript
import { describe, test, expect } from "bun:test";

describe("Feature Name", () => {
  test("should do something", async () => {
    const response = await fetch("http://localhost:8080/api/users/");
    expect(response.status).toBe(200);
  });
});
```

### Integration Test Example

```typescript
import { describe, test, expect, beforeAll } from "bun:test";

describe("Integration Test", () => {
  beforeAll(async () => {
    // Verify services are running
    await fetch("http://localhost:8081");
  });

  test("should proxy correctly", async () => {
    const response = await fetch("http://localhost:8080/api/users/");
    expect(response.status).toBe(200);
  });
});
```

## Continuous Integration

For CI/CD pipelines, use this workflow:

```bash
# 1. Start backend services
docker compose up -d

# 2. Wait for services to be ready
sleep 5

# 3. Start gateway in background
cd gateway
bun start &
GATEWAY_PID=$!

# 4. Wait for gateway to be ready
sleep 2

# 5. Run tests
bun test

# 6. Cleanup
kill $GATEWAY_PID
docker compose down
```

## Troubleshooting

### Tests Failing with Connection Errors

**Problem**: `fetch failed` or `ECONNREFUSED` errors

**Solution**: 
1. Verify backend services are running: `docker compose ps`
2. Verify gateway is running: `curl http://localhost:8080/health`
3. Check port conflicts: `lsof -i :8080` (macOS/Linux) or `netstat -ano | findstr :8080` (Windows)

### Rate Limit Tests Failing

**Problem**: Rate limit tests don't trigger 429 responses

**Solution**: 
1. Check rate limit configuration in environment variables
2. Ensure no other processes are making requests to the gateway
3. Wait for rate limit window to reset (default: 60 seconds)

### Integration Tests Timing Out

**Problem**: Tests take too long or timeout

**Solution**:
1. Increase test timeout: `test("name", async () => { ... }, 30000)`
2. Check backend service performance
3. Reduce number of concurrent requests in tests

### Load Tests Show Poor Performance

**Problem**: Low requests/second or high latency

**Solution**:
1. Check if backend services are under load
2. Verify network conditions
3. Reduce concurrency level
4. Check system resources (CPU, memory)

## Performance Benchmarks

Expected performance on a typical development machine:

| Metric | Expected Value |
|--------|---------------|
| Average Latency | < 50ms |
| Requests/Second | > 500 |
| Success Rate | > 95% |
| Gateway Overhead | < 10ms |

Actual performance depends on:
- Backend service response time
- Network conditions
- System resources
- Concurrent load

## Best Practices

1. **Run unit tests frequently** during development
2. **Run integration tests** before committing changes
3. **Run load tests** before deploying to production
4. **Monitor metrics** during load tests to identify bottlenecks
5. **Test edge cases** like timeouts, errors, and rate limits
6. **Keep tests isolated** - each test should be independent
7. **Use meaningful test names** that describe what's being tested
8. **Clean up resources** after tests (close connections, stop servers)
