# Roadmap

Where this gateway stands, why it is built the way it is, and what comes next.
[README.md](README.md) documents how it works; this file documents the decisions
behind it.

## Status

| Phase | Scope | State |
|---|---|---|
| **1 — MVP** | Routing, middleware pipeline, error mapping, timeouts, observability | **Done** |
| **2 — Nice to have** | Rate limiting, API-key auth, circuit breaking | Not started |
| **3 — Extra** | Load balancing, active health checks, hot reload, metrics | Not started |

732 lines of Go across 6 files, plus tests. No external dependencies: `go.mod`
has no `require` block, so the gateway builds and runs with no network access.

## Phase 1 — what shipped

- **Routing** — JSON route table, longest-prefix wins, segment-aware matching,
  per-route prefix stripping.
- **Middleware pipeline** — stdlib `func(http.Handler) http.Handler` signature,
  global chain plus a per-route chain, composed once at config load.
- **Error mapping** — 404 / 500 / 502 / 504 into a gateway-specific JSON envelope.
- **Timeouts** — three layers: read-header, transport dial/TLS, per-route deadline.
- **Observability** — `X-Request-Id` generated or propagated, one access-log line
  per request, effective route table printed at startup.
- **Lifecycle** — graceful shutdown draining in-flight requests.
- **Verification** — `demo.sh` end to end, table-driven tests on route matching.

The middleware pipeline is the load-bearing piece. Everything in Phase 2 is meant
to arrive as an entry in a per-route middleware list, with the routing core
untouched — that is the property Phase 1 was shaped to make possible.

## Key decisions

Each of these was a fork in the road, not a default. The last column is what the
choice costs.

| Decision | Chosen | Alternative | Why | Given up |
|---|---|---|---|---|
| Language / proxy core | Go + `httputil.ReverseProxy` | Node + `http-proxy`; hand-rolled `http.Client` loop | stdlib already handles hop-by-hop header stripping, body streaming, and connection pooling — all of which a hand-rolled copy loop gets wrong | Nothing material |
| Config format | JSON | YAML | YAML needs `gopkg.in/yaml.v3`, which forfeits the zero-dependency, works-offline property | Slightly less pleasant to hand-edit; no comments |
| Config source | File with built-in fallback | Hardcoded table; env vars only | Externalized routing is a real gateway feature, and the fallback keeps `go run .` working with no setup | A silent fallback could mask a misconfigured deploy — mitigated by always printing the effective route table |
| Path rewriting | Per-route `strip_prefix`, default true | Always strip; always pass through | Both are defensible in production (Kong strips, nginx depends on how you write `location`); making it explicit in config says it was a decision | One more field to document |
| Wildcards | Trailing `/**` only | Full glob engine; plain `prefix` field | Matches the shape a reader expects from a route table, at ~10 lines. Cross-segment glob is a genuine trap | No mid-path wildcards; rejected loudly at startup rather than silently |
| Pipeline shape | Global chain + per-route chain, stdlib signature | Custom `Before`/`After` plugin interface | Anything in the `net/http` ecosystem plugs in without an adapter | Less "plugin-like" than a bespoke interface with lifecycle hooks |
| Chain assembly | At config load | Per request | Hot path is a table scan plus one `ServeHTTP`; no per-request closure allocation | Route changes require a restart (see Phase 3 hot reload) |
| Middleware order | `AccessLog` outside `Recover` | The conventional `Recover` outermost | A panic still produces exactly one access-log line, with status 500, instead of vanishing | A panic inside `AccessLog` itself is unguarded — it is ten lines and does not panic |
| Error body | Own envelope + `request_id` | Mimic the backend response shape; RFC 7807 | A caller can tell a gateway failure from a backend failure | Not a registered standard media type |
| Timeouts | Three layers, no `WriteTimeout` | A single `Server.WriteTimeout` | `WriteTimeout` would cap long-lived upstream responses; the per-route deadline is the correct bound | More knobs |
| Logging | `log/slog` `TextHandler` | `JSONHandler`; a logging library | Readable on screen during a live demo; swapping to JSON is one constructor call | Not machine-parseable as shipped |
| Deployment | `go run` on the host | Inside docker-compose | Fastest iteration; no rebuild per change | No container artifact yet — see below |
| Tests | Table-driven on route matching, parsing, config validation | Full HTTP integration suite; none | Covers the places where a plausible-looking implementation is quietly wrong, at low cost | No test exercises the proxy end to end — `demo.sh` covers that manually |

## Phase 1 — known gaps

- **No Dockerfile or compose entry.** The gateway runs on the host today. Adding
  it means a Dockerfile plus a second config whose targets are service names
  (`http://user-service:8080`) rather than `localhost:8081-8083`, since the host
  port mappings do not exist inside the compose network.
- **No retry on a dead pooled connection.** When a backend stops, an idle pooled
  connection can yield `EOF` and surface as a 502 rather than being retried on a
  fresh connection. Correct for non-idempotent requests, unnecessarily strict for
  a `GET`. See Phase 2.
- **No automated end-to-end test.** `demo.sh` is a manual walkthrough, not a test
  that fails a build.
- **`X-Forwarded-For` is replaced, not appended.** Correct when the gateway is the
  outermost hop, wrong behind another proxy. Needs a trusted-proxy list to fix
  properly.

## Phase 2 — nice to have

The point of this phase is less the features themselves than proving the Phase 1
abstraction holds: each item should be a new middleware plus a config field, with
no change to `router.go` or `proxy.go`.

1. **Config surface for middlewares.** Add `"middlewares": [...]` to a route.
   Deliberately *not* added in Phase 1 — introducing it here, alongside the first
   real consumer, is what demonstrates the pipeline was extensible rather than
   merely claimed to be.
2. **Rate limiting.** Token bucket per client key, `429` with `Retry-After`.
   Open question: what is the key — client IP, `X-API-Key`, or a route-level
   global bucket? IP alone is wrong behind a proxy, which ties back to the
   `X-Forwarded-For` gap above.
3. **API-key auth.** `X-API-Key` against a configured set, `401` on failure.
   Keys in the config file are fine for a demo and wrong for production; the
   config would need to reference an env var or a secret store.
4. **Circuit breaking.** Count consecutive upstream failures per route, open the
   circuit, fail fast with `503`, half-open after a cooldown. This is the one item
   that needs state shared across requests, so it is also the first real test of
   whether the middleware signature is sufficient or needs per-route instances.
5. **Retry for idempotent methods.** `GET`/`HEAD`/`PUT`/`DELETE` only, and only
   when the body is absent or replayable — a consumed body cannot be replayed
   without buffering it, which trades away streaming. Closes the `EOF` gap above.

Rough order of value: 1 → 2 → 4 → 5 → 3.

## Phase 3 — extra

- **Load balancing.** `Route.Target` becomes a slice with a round-robin or
  least-connections picker. Untestable in this environment: each service runs a
  single instance. The data model change is small; the interesting part is how it
  interacts with circuit breaking and health checks.
- **Active health checks.** The backends already expose `/health`. A background
  prober would eject unhealthy targets from the pool — only meaningful once there
  is more than one target per route.
- **Config hot reload.** Watch the config file or accept `SIGHUP`, rebuild the
  route table, and swap it in atomically. Straightforward because handlers are
  already assembled as an immutable table at load time.
- **Metrics.** Request count, latency histogram, and status distribution per
  route. A Prometheus endpoint would mean the first external dependency, so an
  expvar-based version may be the better trade here.
- **Request / response rewriting.** Header injection and body transformation as
  middlewares.

## Explicitly out of scope

Not planned, and each for a reason rather than for lack of time:

- **TLS termination** — normally handled by the layer in front of the gateway.
- **WebSocket / SSE proxying** — `ReverseProxy` supports both, but neither backend
  speaks them, so there would be nothing to verify against.
- **Response caching** — needs a cache-key and invalidation design that this
  workload does not justify.
- **Service discovery** — a static route table is the right fit for three fixed
  services.
- **An admin API** — config-file plus reload covers the same ground with less
  surface area.
