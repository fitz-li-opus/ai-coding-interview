# Gateway — TODO

Tracks follow-up work not covered by the current implementation.
Grouped by priority for a production rollout.

---

## P0 — ship-blocking for production

- [ ] **Graceful shutdown on SIGTERM / SIGINT**
      Currently `server.stop()` is never called — in-flight requests are killed
      when the container terminates. Register signal handlers, call
      `server.stop(false)` (stop accepting new, drain in-flight), then exit.

- [ ] **Streaming-safe timeout**
      `AbortSignal.timeout(UPSTREAM_TIMEOUT_MS)` aborts after a total wall-clock
      deadline. This **breaks SSE, long polling, large downloads/uploads**.
      Split into *connect timeout* (time to first byte) and *idle timeout*
      (no progress for N seconds), or opt out per route.

- [ ] **Request body size limit**
      No cap today — a client can stream an unbounded body. Enforce a max
      (e.g. `MAX_BODY_BYTES`, 10 MiB default) and return `413 Payload Too Large`.

- [ ] **X-Forwarded-For trust boundary**
      We blindly append the peer IP to any incoming XFF. If the gateway is
      internet-facing (not behind a trusted LB), attackers can spoof the
      chain. Add `TRUSTED_PROXIES` config: clobber XFF when the peer is
      untrusted, otherwise append.

---

## P1 — strongly recommended before prod traffic

- [ ] **Structured access logs (JSON)**
      Include: timestamp, request-id, method, path, route, upstream, status,
      latency_ms, bytes_in/out, client_ip, user_agent. Plain `console.log`
      strings are painful to query in a log pipeline.

- [ ] **Request ID / correlation ID**
      Generate `X-Request-Id` if absent; forward to upstream; echo on response.
      Propagate `traceparent` (W3C trace context) if present.

- [ ] **Metrics endpoint**
      Expose `/metrics` (Prometheus) with request count, latency histogram,
      upstream error count per route.

- [ ] **Readiness vs liveness split**
      `/health` today is liveness. Add `/ready` that probes each upstream's
      `/health` (with a short cache) so the orchestrator doesn't route traffic
      before upstreams are up.

- [ ] **Retry on idempotent methods**
      Retry `GET/HEAD/OPTIONS` once on connect-refused / 5xx with jittered
      backoff; never retry non-idempotent methods unless `Idempotency-Key` is
      present.

---

## P2 — feature work as demand appears

- [ ] **WebSocket / HTTP upgrade passthrough**
      Fetch-based proxy can't forward `Upgrade: websocket`. Handle via
      `server.upgrade()` with a matching client `WebSocket` to upstream.

- [ ] **Rate limiting**
      Token bucket keyed on client IP (or API key) with a shared store
      (Redis / in-memory for single instance).

- [ ] **AuthN / AuthZ**
      JWT / API-key validation before proxying; strip internal auth headers
      so they can't be spoofed from outside.

- [ ] **Circuit breaker**
      Per-upstream open/half-open/closed state machine — fail fast when an
      upstream is down instead of piling on timeouts.

- [ ] **Load balancing across multiple upstream instances**
      Today each route points to a single URL. Support a list + strategy
      (round-robin, least-connections).

- [ ] **Config file / hot reload**
      Move the route table out of source to `routes.yml` or similar; reload
      on SIGHUP. (Today's hardcoded table is fine for 3 services, brittle at 30.)

- [ ] **Response caching**
      For cacheable `GET` with appropriate `Cache-Control`, short TTL cache
      to reduce upstream load.

---

## Verified / intentionally out of scope

- TLS termination — assumed handled by an upstream LB / ingress.
- HTTP/2, HTTP/3 — terminate at LB, gateway speaks HTTP/1.1 internally.
- Encoded slash handling (`%2F`) — WHATWG URL does not decode, upstream
  sees raw escapes; matches nginx default behavior.
