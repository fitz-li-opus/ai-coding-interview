## Why

Two production concerns remain after the first three changes:

1. The gateway has no protection against a single client exhausting upstream capacity — one misbehaving caller can degrade every tenant behind the gateway.
2. The gateway's `/health` endpoint reports only its own process state. It returns `healthy` even when all three upstreams are down, which misleads load balancers and on-call.

## What Changes

- **Rate limiting**: per-IP fixed-window limiter (default 100 req / min, configurable via `RATE_LIMIT_RPM` and `RATE_LIMIT_WINDOW_MS`). Exceeding the limit returns `429 Too Many Requests` with the standard `Retry-After` header and the gateway error envelope. Limiter is in-memory (single instance); note in design that a multi-replica deploy needs Redis.
- **Health aggregation**: `/health` probes each configured upstream's `/health` in parallel with a short timeout (500 ms). Response:
  ```json
  {
    "status": "healthy" | "degraded" | "unhealthy",
    "upstreams": {
      "user-service":   { "status": "healthy", "latency_ms": 3 },
      "order-service":  { "status": "unhealthy", "error": "ECONNREFUSED" }
    }
  }
  ```
  - `healthy` = all upstreams healthy
  - `degraded` = at least one healthy, at least one unhealthy
  - `unhealthy` = all upstreams unhealthy
  HTTP status: `200` for healthy/degraded, `503` for unhealthy. This lets k8s readiness probes behave correctly (a degraded gateway still serves the healthy routes).
- Cache aggregated health result for 1 s to avoid hammering upstreams when probes are frequent.

## Capabilities

### New Capabilities
- `rate-limiting`: per-client request throttling at the gateway layer.
- `health-aggregation`: gateway `/health` reports aggregated upstream health, not just its own.

### Modified Capabilities
<!-- none -->

## Impact

- `gateway/index.js`: add rate-limit middleware before proxy middlewares; replace the current `/health` handler.
- `gateway/package.json`: add `express-rate-limit`. Health probe uses built-in `fetch` (Node 20+), no new dep.
- Depends on `route-config` change to discover the upstream list; without it, health aggregation has to duplicate the route literal.
- Single-instance limiter is a known limitation — flag in design.md for a follow-up change (distributed limiter with Redis).
