# API Gateway — Design Notes

A single-port reverse proxy in front of the three backend services, written in Go
with **zero third-party dependencies**.

```
                 ┌───────────────────────── gateway :8088 ──────────────────────────┐
client ──────►   │  requestID ─► accessLog ─► recover ─► router ─► pick ─► proxy    │ ──► user-service     :8081
                 │                              │           │                       │ ──► order-service    :8082
                 │                     longest-prefix   round-robin over             │ ──► product-service  :8083
                 │                        wins          healthy upstreams            │
                 │                                          ▲                        │
                 │                            health checker │ probes /health every 5s
                 └──────────────────────────────────────────────────────────────────┘
```

Run it:

```bash
docker compose up --build -d          # backends
cd gateway && go run . -config config.json
./demo.sh                             # end-to-end walkthrough
go test ./... -race                   # unit + httptest integration tests
```

The gateway listens on `:8088`. Port 8080 is the more conventional choice, but it
is often already taken on a developer machine (Docker Desktop and OrbStack both
publish services there), and a gateway that silently loses half its traffic to a
squatter is a bad demo. The port is config, not code: `-listen :8080` or a one
line edit to `config.json` moves it. `demo.sh` follows via `GW=http://host:port`.

---

## Decisions and trade-offs

### Routing lives in `config.json`, not in code

Adding a backend is a config edit, never a handler edit. `config.go` rejects
unknown JSON fields (`DisallowUnknownFields`) and validates prefixes and target
URLs at startup, so a typo fails loudly on boot instead of quietly 404-ing in
production.

### Longest-prefix match on a segment boundary

Routes are sorted by descending prefix length once at construction, so a linear
scan returns the most specific match. Matching is **not** a bare `HasPrefix`:
the prefix must end at a `/` or at the end of the path. Without that check,
`/api/usersfoo` would be routed to the `/api/users` service. This is the one
routing rule with a real bug hiding in it, so it has its own test table.

### Prefix stripping is configurable, defaulting to on

`/api/users/123` reaches the backend as `/123`. The task description doesn't
pin this down and both conventions are defensible (it is exactly the nginx
`proxy_pass` trailing-slash distinction), so `strip_prefix` is a per-route flag.
It is a `*bool` in the config struct so an omitted field still means `true`.

### One `http.Transport`, one `ReverseProxy` per upstream

Connection pooling and keep-alive are global (`MaxIdleConnsPerHost: 100`).
`httputil.ReverseProxy` is used rather than hand-rolled forwarding because it
already handles hop-by-hop headers, response streaming, context cancellation and
protocol upgrades correctly. The newer `Rewrite` hook is used instead of the
deprecated `Director`, which gives `SetXForwarded()` — it *appends* to
`X-Forwarded-For` rather than overwriting it, and sets `X-Forwarded-Proto` and
`X-Forwarded-Host`.

`SetURL` clears the outbound `Host`, so backends see their own hostname. The
client's original host survives in `X-Forwarded-Host`. Passing the client Host
through is the other reasonable choice — it matters as soon as a backend does
virtual-hosting or generates absolute URLs.

### Timeouts are layered, and `WriteTimeout` is deliberately unset

| Bound | Where | Why |
|---|---|---|
| `read_header_ms` | `http.Server.ReadHeaderTimeout` | slowloris guard on the inbound side |
| `dial_ms` | `net.Dialer.Timeout` | fail fast on an unreachable backend |
| `upstream_ms` | per-request `context.WithTimeout` + `Transport.ResponseHeaderTimeout` | bounds the backend call, and cancellation propagates so the backend goroutine is released too |

`http.Server.WriteTimeout` is an absolute deadline from the start of the request,
so setting it would truncate slow or streamed upstream responses. The per-request
context is the right knob.

### Status codes carry meaning

| Code | Situation |
|---|---|
| 404 | no route prefix matches — the gateway's own answer, not a backend's |
| 502 | backend refused the connection, reset it, or failed DNS |
| 503 | the whole upstream pool for that route is marked down |
| 504 | the backend accepted but did not answer in time |
| — | client disconnected: logged, no response written |

Every gateway-generated failure has one JSON shape carrying `request_id` and the
`upstream` that failed, so a caller can distinguish a gateway error from a
backend error without reading logs.

### Health checking is active, not passive

A background goroutine probes `{target}/health` every 5s and flips an
`atomic.Bool` per upstream; `Route.Pick()` skips dead instances during round-robin
and returns nil (→ 503) when the pool is empty. The first probe round runs
**synchronously before the listener opens**, so the gateway never serves its first
request into a backend it hasn't checked, and never boots believing everything is
down. Only state *transitions* are logged, so a healthy fleet stays quiet.

Request failures deliberately do **not** flip health state. Marking a backend
down from a single failed request is how one slow client takes out a healthy
node; that job belongs to a circuit breaker with a proper error-rate window,
which is listed as not-implemented below.

### Observability

One structured (`log/slog` JSON) access-log line per request with request id,
method, original path, matched route, chosen upstream, status, bytes, duration
and client IP. `X-Request-ID` is reused when the caller already sent one,
otherwise generated, and it is echoed to both the client and the backend so a
trace survives the hop.

### Graceful shutdown

`SIGINT`/`SIGTERM` → `srv.Shutdown` with a 5s drain, and the same context stops
the health checker. In-flight requests finish instead of being cut.

### Why no third-party dependencies

The repo's `.gitignore` ignores `go.sum`. Any external dependency would therefore
be missing its checksum file in the PR and `go build` would fail on the
reviewer's machine. That constraint also rules out YAML, hence `config.json` —
the standard library has a JSON parser and no YAML one. It costs nothing here:
`net/http` and `log/slog` cover every need.

---

## Not implemented, and why

Each of these is a real gateway concern that was scoped out to keep the exercise
honest about what was actually built and tested.

| Capability | Note |
|---|---|
| **Rate limiting** | Per-IP token bucket is ~40 lines. The interesting part is that it is per-instance: with several gateway replicas you need a shared counter (Redis) or you accept N× the configured limit. |
| **Circuit breaker** | Needs an error-rate window plus a half-open probe state. Pairs badly with naive retries — see below. |
| **Retries** | Only safe for idempotent methods, and only with a budget plus exponential backoff. Retrying into an already-overloaded backend is how a brownout becomes an outage. |
| **Authentication** | JWT verification middleware before routing, with the claims forwarded as headers, and per-route `public: true` opt-out. |
| **CORS** | Belongs at the gateway rather than duplicated in every backend. |
| **Metrics** | `/metrics` with request count, status distribution and a duration histogram, labelled by route and upstream — the log fields already carry everything needed. |
| **Config hot reload** | `SIGHUP` or fsnotify → rebuild the router and swap it behind an `atomic.Pointer`. Today a route change means a restart. |
| **TLS termination** | Plus HTTP/2, which `net/http` gives for free once TLS is on. |
| **Weighted / canary routing** | Weights per upstream, or header-based splitting, instead of flat round-robin. |
| **Request/response body transforms** | Deliberately out of scope; a gateway that rewrites payloads becomes a place business logic hides. |
