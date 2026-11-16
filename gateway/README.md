# API Gateway

A lightweight reverse-proxy gateway built with **FastAPI + httpx**. It listens on `:8080` and routes by path prefix:

| Prefix | Upstream | Upstream receives |
|---|---|---|
| `/api/users/**` | `http://localhost:8081` | path with `/api/users` stripped |
| `/api/orders/**` | `http://localhost:8082` | path with `/api/orders` stripped |
| `/api/products/**` | `http://localhost:8083` | path with `/api/products` stripped |

## Run

```bash
# from repo root: start the backends
docker compose up --build -d

cd gateway
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
pytest -q
uvicorn app.main:app --port 8080 --no-access-log   # gateway emits its own JSON access log
```

## Demo

```bash
curl -s localhost:8080/api/users                        # service=user-service, path="/"
curl -s 'localhost:8080/api/orders/1001?x=1'             # path="/1001", query preserved
curl -s localhost:8080/api/products
curl -si localhost:8080/api/usersX                      # 404 {"error":"no route matched"}
curl -si -H 'X-Request-ID: demo-123' localhost:8080/api/users   # echoed back
docker compose stop order-service
curl -si localhost:8080/api/orders                      # 502 {"error":"upstream unavailable"}
curl -s localhost:8080/healthz                          # gateway liveness
for i in $(seq 12); do curl -s -o /dev/null -w '%{http_code} ' localhost:8080/api/products; done
                                                        # 200 x10 then 429 (burst=10)
```

## Features

- **Config-driven routing** (`routes.yaml`); each upstream overridable via `UPSTREAM_<NAME>_URL` (e.g. `UPSTREAM_USERS_URL=http://user-service:8080` inside compose). Config path overridable via `GATEWAY_CONFIG`.
- **Segment-boundary, longest-prefix matching**: `/api/usersX` does not match `/api/users`.
- **Prefix stripping** (per route `strip_prefix`), query string and percent-encoding forwarded unchanged.
- **Correct proxy headers**: hop-by-hop headers (incl. those named in `Connection:`) removed both directions; `Host` rewritten to the upstream; `X-Forwarded-For/-Proto/-Host` added; repeated response headers (e.g. `Set-Cookie`) preserved.
- **Streaming** request and response bodies (`aiter_raw`, so compressed responses pass through untouched). Requests without a body are not sent as chunked.
- **Timeouts** (connect 2s / read 10s by default) and **error mapping**: no route → 404, connection failure → 502, timeout → 504, always JSON `{error, request_id}`.
- **Rate limiting** (token bucket, in-memory, per route, keyed by client IP): optional `rate_limit: {rate, burst}` per route. Over limit → `429` with `Retry-After`; every limited response carries `RateLimit-Limit` / `RateLimit-Remaining`. Idle buckets are bounded by LRU eviction (10k keys) to resist IP spraying. Keyed on the TCP peer, not `X-Forwarded-For`, because XFF is client-controlled.
- **Request ID**: reuse a sane incoming `X-Request-ID` (validated against header/log injection) or generate one; forwarded upstream and returned to the client.
- **Structured JSON access log**: request_id, method, path, query, route, status, latency_ms, client.
- **Shared connection pool** (one `httpx.AsyncClient` per process) and **graceful shutdown** (uvicorn drains in-flight requests on SIGTERM; lifespan closes the pool).

## Layout

```
app/config.py      routes.yaml + env overrides -> Config/Route
app/router.py      prefix matching + stripping
app/ratelimit.py   token bucket limiter
app/proxy.py       header handling, streaming forward, 502/504 mapping
app/middleware.py  request id + JSON access log
app/main.py        app factory, lifespan, /healthz, catch-all proxy route
tests/             router/limiter unit tests + end-to-end tests via httpx.MockTransport
```

## Trade-offs

- **Python vs Go**: Go's `httputil.ReverseProxy` is more battle-tested and faster; FastAPI + httpx was chosen for iteration speed. The async stack still handles concurrent I/O well.
- **No retries**: retrying non-idempotent requests (POST) is unsafe; a real version would retry idempotent methods only, with backoff and a circuit breaker.
- **Latency in the access log** is time-to-headers; body streaming time is not included.
- **Mid-stream upstream failures** (after headers are sent) can only abort the connection, not change the status code.
- **Static config**: changes need a restart.
- **Rate limit state is per process**: with N workers/instances the effective limit is N×. A global limit needs a shared store (Redis + Lua script for atomic refill/take), with a fail-open policy if Redis is down. Behind a load balancer, key on a trusted-proxy-derived client IP instead of the peer address.
- **Token bucket vs windows**: token bucket allows controlled bursts and has no fixed-window boundary spike; memory is O(active clients).

## Next steps

Aggregated `/health` with active upstream checks, distributed rate limiting (Redis) keyed by API key/user, auth (API key / JWT), load balancing across multiple upstream instances, retries + circuit breaker, Prometheus metrics, config hot reload.
