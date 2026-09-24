# Gateway Implementation Plan

## Deliverable

Implement a dependency-free Node.js 24 gateway in `gateway/`.

```text
gateway/
  package.json
  server.js
  gateway.js
  breaker.js
  test/
    gateway.test.js
    acceptance.test.js
```

Run:

```text
node server.js
```

Acceptance:

```text
npm test
npm run test:acceptance
```

## Public Contract

The gateway listens on `PORT`, default `8080`.

| Path prefix | Target default |
| --- | --- |
| `/api/users` | `http://127.0.0.1:8081` |
| `/api/orders` | `http://127.0.0.1:8082` |
| `/api/products` | `http://127.0.0.1:8083` |

Environment overrides:

```text
PORT
USER_SERVICE_URL
ORDER_SERVICE_URL
PRODUCT_SERVICE_URL
```

Proxy behavior:

- Match prefixes by path segment, not raw string prefix.
- Keep path and query unchanged.
- Forward method, body, and end-to-end headers.
- Return upstream status, headers, and body unchanged on success.
- Remove hop-by-hop headers in both directions.
- Add or preserve `x-request-id`.
- Preserve a valid incoming W3C `traceparent`; create one when absent.
- Return the request ID on the response.

Error shape:

```json
{
  "error": {
    "code": "upstream_timeout",
    "message": "upstream timed out",
    "requestId": "..."
  }
}
```

Status mapping:

| Status | Code | Condition |
| --- | --- | --- |
| `404` | `route_not_found` | No route matches |
| `502` | `bad_gateway` | Proxy fails before an upstream response and no retry succeeds |
| `503` | `overloaded` | Global or route in-flight limit reached |
| `503` | `upstream_unavailable` | No target, all breakers open, or connect failure |
| `504` | `upstream_timeout` | Route deadline expires before response headers |

## Configuration

```js
const config = {
  port: 8080,
  globalMaxInFlight: 1000,
  routes: [
    {
      prefix: "/api/users",
      targets: ["http://127.0.0.1:8081"],
      timeoutMs: 300,
      maxInFlight: 200,
      maxSockets: 50,
      retry: {
        maxAttempts: 2,
        backoffMs: 25
      },
      breaker: {
        windowMs: 10_000,
        minRequests: 20,
        failureRatio: 0.5,
        openMs: 5_000,
        halfOpenProbes: 2
      }
    }
  ]
};
```

`server.js` parses environment variables and calls
`createGateway(config)`. `createGateway` returns an unstarted `http.Server`.
Tests inject fake targets and short timeouts through the same function.

## Request State

For each request:

1. Parse URL and match the longest route prefix.
2. Return `404` if no route matches.
3. Generate request and trace IDs if absent.
4. Acquire global and route in-flight slots or return `503`.
5. Run at most `maxAttempts`.
6. Select an allowed target by least outstanding requests.
7. Skip targets already attempted in this request.
8. Start the route deadline; include upstream connect and response-header wait.
9. Write the upstream response and stream its body.
10. Record the breaker outcome exactly once per attempt.
11. Release route and global slots in `finally`.
12. Emit one structured log line.

The route deadline ends when upstream response headers arrive. Slow response
bodies are then streamed and governed by client cancellation and socket
timeouts.

## Retry Rules

Retry only when all conditions are true:

- method is `GET`, `HEAD`, or `OPTIONS`
- request has no body
- another untried target exists
- failure is connect error, route timeout, or upstream `502/503/504`
- fewer than `maxAttempts` attempts have been made

Never retry:

- `POST`, `PUT`, `PATCH`, or `DELETE`
- after response headers or bytes reach the client
- a request with a body
- upstream `4xx` or `500`
- the same target twice in one request

Use a short bounded backoff. Do not add hedging.

## Circuit Breaker

One breaker exists per route target.

Closed state:

- store outcomes in a rolling `windowMs` window
- require `minRequests`
- open when failure ratio is at least `failureRatio`

Open state:

- reject immediately for `openMs`
- after that, allow at most `halfOpenProbes`

Half-open state:

- any failure opens the breaker again
- successful probes close it after reaching `halfOpenProbes`
- admitted but unfinished probes count toward the probe limit

Failure classification:

- failure: timeout, connect error, `502`, `503`, `504`
- success: all other completed responses, including `4xx`
- ignored: local overload and locally generated `503`

Inject `now()` into the breaker so tests can advance time without sleeping.

## Concurrency

- Node's request handler is asynchronous; no request worker per connection.
- One global counter and one counter per route.
- Reject immediately when either counter is at its limit.
- Use one keep-alive `http.Agent` per target with bounded `maxSockets`.
- Abort the upstream request when the client disconnects.
- Do not buffer request or response bodies.
- On `SIGTERM`, stop accepting connections, wait for in-flight requests, and
  exit after a bounded grace period.

## Logging

Use `JSON.stringify` on one object per request:

```json
{
  "time": "2026-09-24T00:00:00.000Z",
  "level": "info",
  "request_id": "...",
  "trace_id": "...",
  "route": "/api/users",
  "target": "http://127.0.0.1:8081",
  "method": "GET",
  "path": "/api/users/",
  "status": 200,
  "duration_ms": 12,
  "attempt": 1,
  "error_type": null
}
```

Inject the logger in tests. Do not log bodies or authorization headers.

## Test Layers

### Unit and Integration

`test/gateway.test.js` starts fake upstreams and the gateway on ephemeral
ports. It must prove:

1. Each API prefix reaches the expected fake backend.
2. Similar but invalid prefixes return `404`.
3. Method, path, query, body, and end-to-end headers are preserved.
4. Upstream status, headers, and body are preserved.
5. Hop-by-hop headers are removed.
6. Missing trace context is generated and returned.
7. Valid incoming trace context is preserved.
8. A slow upstream returns `504` inside the configured deadline.
9. A refused backend returns `503` quickly.
10. A failed route does not affect a healthy route.
11. The breaker opens after the configured failures.
12. An open breaker avoids the target.
13. The breaker recovers after half-open probes.
14. A `4xx` response does not count as a breaker failure.
15. A `500` response is passed through and is not retried.
16. An idempotent request retries a different target.
17. A non-idempotent request never retries.
18. Route overload returns `503` immediately.
19. A request is admitted again after an in-flight slot is released.
20. Logs contain one correlated record with route, target, status, and attempt.

Use configurable fake clocks and short timeouts. Tests must finish without
sleeping for production durations.

### Acceptance

`test/acceptance.test.js` runs against the real Compose backends:

```text
docker compose up --build -d
npm run test:acceptance
```

It starts the gateway on an ephemeral port and asserts:

1. `/api/users/` returns `200`, `service=user-service`, and 3 records.
2. `/api/orders/` returns `200`, `service=order-service`, and 3 records.
3. `/api/products/` returns `200`, `service=product-service`, and 4 records.
4. The response `x-request-id` is present.
5. An unknown route returns `404`.

This test fails when Docker or any real backend is unavailable. It is the final
acceptance gate.

## Implementation Order

1. `package.json` and the empty server lifecycle.
2. Route matching and transparent proxying.
3. Request ID and trace propagation.
4. Timeout, cancellation, and error mapping.
5. In-flight limits and bounded agents.
6. Multi-target selection, retry rules, and breaker.
7. Unit and integration tests.
8. Real-backend acceptance test.

## Acceptance Gate

Implementation is complete only when all of these pass:

```text
npm test
docker compose up --build -d
npm run test:acceptance
```

No manual curl, screenshot, or visual inspection is part of acceptance.
