# Gateway Design

## Decision

- Demo: Node.js 24, standard library only.
- Production: do not build a full gateway. Use Envoy/Kong/APISIX and keep this
  implementation as the interview reference.
- Data plane is stateless and can run multiple replicas.
- Keep the first version small: route, proxy, timeout, tracing, concurrency
  limit, and passive circuit breaking.

## Request Path

```text
client
  -> gateway :8080
  -> request id / trace propagation
  -> route match
  -> per-route concurrency limit
  -> upstream selection
  -> circuit breaker
  -> reverse proxy
  -> user-service :8081 / order-service :8082 / product-service :8083
```

Forward these unchanged:

- method, path, query string, body, headers
- response status, headers, body
- W3C `traceparent` and `tracestate`
- generate `x-request-id` when absent

Keep the `/api/{service}` prefix. The supplied mock backends accept any normal
path, so prefix rewriting is not part of the acceptance criteria.

## Routes

```js
{
  "/api/users": {
    targets: ["http://127.0.0.1:8081"],
    timeoutMs: 300,
    maxInFlight: 200
  },
  "/api/orders": {
    targets: ["http://127.0.0.1:8082"],
    timeoutMs: 500,
    maxInFlight: 200
  },
  "/api/products": {
    targets: ["http://127.0.0.1:8083"],
    timeoutMs: 300,
    maxInFlight: 200
  }
}
```

Production routes should support multiple targets, health state, zone labels,
atomic config reload, and route-specific policy.

## Failure Policy

| Condition | Gateway behavior |
| --- | --- |
| Backend connect error | Fail fast, count one breaker failure, return `503` |
| Request/header timeout | Abort the upstream request, return `504` |
| Slow backend | Bound by route timeout and `maxInFlight`; shed overload, never queue without limit |
| Safe transient failure | Retry only idempotent requests, at most once, on another target |
| `4xx` response | Pass through; do not count as backend failure |
| `5xx` response | Pass through; count only `502/503/504` and transport failures |
| Partial response | Never retry after bytes are sent to the client |
| All targets unavailable | Return structured `503`, short `Retry-After`, no cascading retry loop |

Failure isolation is per route and per target. A failed user-service must not
block order-service or product-service.

## Circuit Breaker

Use a small in-memory breaker per target:

- rolling 10-second window
- open after at least 20 requests and at least 50% failures
- stay open for 5 seconds
- allow 2 half-open probes
- count timeouts, connect errors, `502`, `503`, and `504`

Reject with `503` while open. Keep breaker state local to each gateway
instance; distributed consistency is not worth the extra failure dependency.

## Tracing and Logs

- Accept or create a W3C-compatible trace context.
- Send `traceparent`, `tracestate`, and `x-request-id` upstream.
- Log one structured line per request:
  `trace_id`, `route`, `target`, `method`, `path`, `status`, `duration_ms`,
  `attempt`, `error_type`.
- Export production metrics through OpenTelemetry:
  RPS, p50/p95/p99, error ratio, in-flight requests, connection-pool
  saturation, retry count, breaker state.

Do not put request or response bodies in logs.

## Concurrency

- Node handles requests asynchronously; use keep-alive agents with bounded
  sockets per target.
- Apply one global and one per-route in-flight limit.
- Reject overload immediately with `503`; queuing merely moves the outage.
- Respect the client cancellation signal.
- Stream proxy traffic instead of buffering whole bodies.
- Drain connections and stop accepting traffic during graceful shutdown.

## Redundancy

- Run at least two gateway replicas behind a layer-4 load balancer.
- Configure at least two backend replicas per service, preferably across zones.
- Select targets by least outstanding requests or round robin.
- Remove failed targets through passive health signals; active probes are optional.
- Use service discovery or DNS for target changes.
- Keep retry budget and circuit breakers to prevent retry storms.
- Disable request hedging by default because it adds load during latency spikes.

The current Compose file has one instance per backend. It can demonstrate
isolation and fail-fast behavior, but not true backend failover.

## Production Boundary

Use the custom implementation only to demonstrate the mechanics. In production,
the equivalent responsibilities belong to:

- Envoy/Kong/APISIX: routing, load balancing, retries, breakers, rate limits
- service discovery: target health and topology
- OpenTelemetry Collector: traces, metrics, logs
- orchestrator/NLB: gateway replication, health checks, rolling deploys
- config service: versioned, validated, atomic policy rollout

Do not add a shared Redis or database to the request path unless a specific
feature requires distributed state.

## Interview Scope

Implement:

1. Prefix routing and transparent proxying.
2. Per-route timeout and structured `502/503/504` errors.
3. Request ID and trace-context propagation.
4. Per-route concurrency limit.
5. Passive circuit breaker.
6. Multi-target selection when more than one target is configured.

Explain but do not implement:

- OpenTelemetry exporter
- distributed rate limiting
- authentication and request transformation
- multi-region failover
- dynamic control plane

## Acceptance

```text
/api/users/**    -> user-service data
/api/orders/**   -> order-service data
/api/products/** -> product-service data
```

Additional checks:

- Stop one backend: only its route returns a fast `503`; other routes stay `200`.
- Add latency above the route timeout: the route returns `504`.
- Send high concurrency: in-flight work stays bounded.
- Inspect logs: one trace ID correlates the request and upstream attempt.
