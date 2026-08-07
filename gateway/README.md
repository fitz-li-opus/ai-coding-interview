# API Gateway

A reverse-proxy gateway on `:8080`, written against the Go standard library only —
`go.mod` has no `require` block, so it builds and runs with no network access.

This file covers how it works. [ROADMAP.md](ROADMAP.md) covers what is done, the
trade-offs behind each decision, and what comes next.

## Run

```bash
cd gateway
go run .                       # reads ./routes.json
go run . -config /path/to.json # explicit config
go run . -listen :9000         # override the listen address
```

`GATEWAY_CONFIG` and `GATEWAY_LISTEN` work as env equivalents of the two flags.

The three backends must be up first: `docker compose up --build -d` from the repo root.

## Verify

```bash
curl http://localhost:8080/api/users/1        # -> user-service, backend sees /1
curl http://localhost:8080/api/orders/1001    # -> order-service, backend sees /1001
curl http://localhost:8080/api/products/      # -> product-service, backend sees /
curl http://localhost:8080/health             # -> the gateway itself
curl -i http://localhost:8080/api/nope        # -> 404 no_route
```

The backends echo the path they received, so the `path` field in the response is
the proof that prefix stripping happened.

`./demo.sh` walks all of it end to end, including the failure paths: routing and
prefix stripping, the reserved `/health`, both 404 cases, request-id propagation,
502 (it stops `order-service` and starts it again), and 504 (a throwaway gateway
instance pointed at an upstream that never answers). It restores everything on
exit, including on Ctrl-C.

## Configuration

```json
{
  "listen": ":8080",
  "default_timeout_ms": 3000,
  "routes": [
    {
      "path": "/api/users/**",
      "target": "http://localhost:8081",
      "strip_prefix": true,
      "timeout_ms": 5000
    }
  ]
}
```

| Field | Default | Notes |
|---|---|---|
| `path` | required | `**` is supported **only** as a trailing `/**` |
| `target` | required | must be `http`/`https` with a host |
| `strip_prefix` | `true` | omitted means strip; set `false` to forward the path unchanged |
| `timeout_ms` | `default_timeout_ms` | per-route upstream deadline |

JSON rather than YAML so the gateway keeps zero external dependencies. Unknown keys
are rejected at startup, so a typo fails loudly instead of being silently ignored.

If the config file is missing the gateway logs a `WARN` and falls back to built-in
defaults matching the three services. It always prints the effective route table at
startup, so "file or defaults?" is never a guess.

## Design

**Request path.** `AccessLog → Recover → /health → router → per-route chain → ReverseProxy`

**Middleware pipeline.** Middlewares use the stdlib `func(http.Handler) http.Handler`
signature, so anything from the wider `net/http` ecosystem plugs in without an
adapter. `Chain` composes them. Per-route handlers are assembled **once at config
load time**, not per request, so the hot path is a table scan plus one `ServeHTTP`.

`Recover` sits *inside* `AccessLog` rather than outside. That ordering is what makes
a panic still produce exactly one access-log line, with status 500, instead of
vanishing between the two.

**Routing.** Longest matching prefix wins, independent of config order. Matching is
on segment boundaries, so `/api/usersecret` does *not* match the prefix `/api/users`.
Matching uses `URL.EscapedPath()`, not `URL.Path`: `/api%2Fusers/1` decodes to
`/api/users/1`, but the client never wrote a real separator there, so it must not be
routed as though it had. Prefix stripping preserves the client's original
percent-encoding of everything after the prefix.

**Timeouts** are layered, because any single one leaves a hole:

| Layer | Value | Guards against |
|---|---|---|
| `Server.ReadHeaderTimeout` | 5s | Slowloris |
| `Transport` dial / TLS | 2s / 5s | unreachable upstream hanging a goroutine |
| per-route `context` deadline | 3s default | a slow upstream holding the request open |

`Server.WriteTimeout` is deliberately unset — it would cap long-lived upstream
responses. The per-route deadline is what bounds an upstream call.

**One shared `http.Transport`** for the whole gateway. Connection pooling lives on
the Transport, so building one per route — let alone per request — would defeat
keep-alive. `MaxIdleConnsPerHost` is raised to 100; the stdlib default of 2 is far
too low for a gateway.

**Error mapping.** The gateway's error envelope is shaped differently from the
backends' responses on purpose, so a caller can tell which hop failed.

| Condition | Status | `error` |
|---|---|---|
| no route matches | 404 | `no_route` |
| upstream unreachable | 502 | `bad_gateway` |
| upstream deadline exceeded | 504 | `gateway_timeout` |
| panic in the gateway | 500 | `internal_error` |
| client disconnected | — | nothing written; logged only |

**Request ID.** `X-Request-Id` is taken from the client or generated, then propagated
upstream *and* returned on every response, including errors. The id in an error body
greps straight to its access-log line.

**Forwarded headers.** `SetXForwarded` *replaces* any inbound `X-Forwarded-For`
rather than appending to it: the gateway is the trust boundary here, so a
client-supplied chain must not be believed. Behind a real load balancer this would
need to become an append with a configured trusted-proxy list.

**Graceful shutdown.** SIGINT/SIGTERM drains in-flight requests with a 10s budget.

## Scope

Deliberately not implemented, in rough priority order:

- **Rate limiting, API-key auth, circuit breaking** — all of these are "add one
  entry to a per-route middleware list"; the pipeline exists for exactly this.
- **Load balancing** — `Route.Target` would become a slice. Untestable here: each
  service has a single instance.
- **Active upstream health checks** — the backends expose `/health`; a background
  prober could eject unhealthy targets.
- **Retries** — only safe for idempotent methods, and a consumed request body cannot
  be replayed without buffering it, which trades away streaming.
- **Config hot reload, TLS, response caching, metrics.**

## Tests

```bash
go test ./...
```

Covers the parts where a plausible-looking implementation is quietly wrong: segment
boundary matching, longest-prefix selection, pattern parsing and its rejections,
config validation, and encoding-preserving prefix stripping.
