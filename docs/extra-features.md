# Feature: Load Balancing & Rate Limiting

## Overview

You are tasked with designing and implementing a lightweight API Gateway that can handle a high volume of requests.

The API Gateway should be able to:

- Distribute incoming requests across multiple backend services
- Limit the number of requests per second to each backend service

---

## Your Task

Implement the API Gateway in the `gateway/` directory.

---

## Implemented MVP

- Routing by path prefix: `/api/users/**`, `/api/orders/**`, `/api/products/**`.
- Request timeout with `504` response.
- Upstream failure handling with `502` response.
- Request ID propagation.
- In-memory fixed-window rate limiting per client IP and route.
- Structured JSON access logs.

## TODO Features

### 1. Load Balancing

Support multiple upstream URLs per route and distribute requests across them.

Suggested design:

- Add env vars such as `USER_SERVICE_URLS=http://localhost:8081,http://localhost:8181`.
- Keep existing single-url env vars as fallback.
- Use round-robin selection per service.
- Add tests proving repeated requests alternate upstreams.

Business value:

- Improves availability and throughput as traffic grows.
- Allows horizontal scaling without changing client URLs.

### 2. Health-Aware Upstream Selection

Avoid routing traffic to failing upstreams.

Suggested design:

- Track upstream failures in memory.
- Mark an upstream unhealthy after repeated `502` or timeout failures.
- Skip unhealthy upstreams for a short cooldown period.
- Add `/health` detail showing upstream state.

Business value:

- Reduces customer-visible errors.
- Gives operators faster signal during incidents.

### 3. Distributed Rate Limiting

Move rate limit counters out of process.

Suggested design:

- Use Redis or another shared store.
- Key by customer/API key when auth exists; fallback to client IP.
- Keep local in-memory limiter only for single-instance development.

Business value:

- Enforces limits correctly across multiple gateway instances.
- Supports paid-plan quotas and abuse protection.

### 4. Authentication and API Keys

Identify callers before forwarding requests.

Suggested design:

- Accept `Authorization: Bearer <token>` or `x-api-key`.
- Validate token/API key in middleware.
- Attach caller identity to logs and rate-limit keys.

Business value:

- Enables customer-specific access control.
- Supports usage analytics, billing, and revocation.

### 5. Metrics and Tracing

Expose operational signals for dashboards and alerts.

Suggested design:

- Add `/metrics` endpoint for request count, latency, status codes, and upstream errors.
- Emit OpenTelemetry traces with request ID and upstream target.
- Track metrics by route and upstream.

Business value:

- Speeds up incident detection and debugging.
- Helps capacity planning and customer support.

### 6. Retry and Circuit Breaker

Fail gracefully when upstreams are unstable.

Suggested design:

- Retry only idempotent methods such as `GET` and `HEAD`.
- Use small retry counts with backoff.
- Open circuit after repeated upstream failures.

Business value:

- Reduces transient failures.
- Prevents cascading outages during backend incidents.

## Not MVP Yet

- Response caching: useful, but correctness depends on data freshness and cache keys.
- Admin API/dynamic config: useful later, but too large for 30-minute implementation.
- WAF/schema validation: valuable security layer, but requires route schemas and policy decisions.
