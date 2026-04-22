## Context

`gateway/index.js` wires `http-proxy-middleware` (v3) with only `target`, `changeOrigin`, `pathRewrite`, `logger`, and an `on.proxyReq` hook. No `timeout`/`proxyTimeout` is set, and no `on.error` handler exists. When an upstream is unreachable (`ECONNREFUSED`, DNS failure) or hangs, `http-proxy-middleware` either waits on the default Node socket timeout (~2 min) or writes an `ERR_*` to the default Node handler — so clients see either indefinite latency or an empty/HTML response. Request IDs are already set by the logging middleware added in `add-structured-logging` (`req.requestId` + `X-Request-Id` header), so the error envelope can reuse them.

## Goals / Non-Goals

**Goals:**
- Bounded, configurable upstream timeout (default 5000 ms).
- Deterministic JSON error envelope for every gateway-originated failure, keyed by `request_id`.
- Stable status codes per failure class: `502` for connection/DNS failure, `504` for timeout.
- No unhandled errors escape to the default Node handler.

**Non-Goals:**
- Retries or circuit breaking (out of scope for this change).
- Per-route timeout overrides (can be added later once config is file-driven).
- Reshaping upstream 5xx bodies when the upstream did return a body — we only fill an envelope when the body is empty.

## Decisions

**1. Use `http-proxy-middleware` built-in `timeout` + `proxyTimeout` options rather than a custom `AbortController`.**

`http-proxy-middleware` v3 forwards both to the underlying `http-proxy`: `timeout` bounds the inbound socket, `proxyTimeout` bounds the upstream request. Together they surface an `ECONNRESET`/`ETIMEDOUT` through the `on.error` event, which is where the envelope is written. A custom abort layer would duplicate this and risk double-responding. Alternative considered: wrap each route in a `setTimeout` + `res.destroy()` — rejected because it races with proxy internals and leaks sockets.

**2. Map errors by `err.code`, not by `err.message`.**

Node surfaces a stable `code` on socket errors (`ECONNREFUSED`, `ENOTFOUND`, `EAI_AGAIN`, `ECONNRESET`, `ETIMEDOUT`, `ECONNABORTED`). Messages are locale-/version-sensitive. The mapping:
- `ECONNREFUSED`, `ENOTFOUND`, `EAI_AGAIN`, `EHOSTUNREACH`, `ENETUNREACH` → `502` / `code: "BAD_GATEWAY"`
- `ETIMEDOUT`, `ECONNABORTED` (from `proxyTimeout`) → `504` / `code: "GATEWAY_TIMEOUT"`
- Anything else → `502` / `code: "BAD_GATEWAY"` (fail-closed default)

**3. Envelope shape matches the proposal exactly.**

```
{ "error": { "code": "<UPPER_SNAKE>", "message": "<human readable>", "request_id": "<uuid>" } }
```

`request_id` is read from `req.requestId` (set by the existing logging middleware). If absent (defense-in-depth), generate a fresh `crypto.randomUUID()` so the field is never empty. `Content-Type: application/json` is set explicitly because `res.writeHead` is used (the proxy may have already begun writing headers via `res`).

**4. Guard against double-write.**

`http-proxy-middleware`'s `on.error` fires after `res.writeHead` may already have been called by the proxy. Check `res.headersSent` before writing the envelope; if headers are already out, call `res.destroy(err)` so the client sees a clean socket close and the access log still records the status. This prevents `ERR_HTTP_HEADERS_SENT` crashes.

## Risks / Trade-offs

- **[Risk]** A slow-but-eventually-succeeding upstream (>5s) now fails with `504` where it previously succeeded. → **Mitigation:** timeout is env-configurable via `UPSTREAM_TIMEOUT_MS`; ops can raise it per environment.
- **[Risk]** Upstream 5xx with empty body is rewritten into our envelope, potentially hiding that the upstream ran. → **Mitigation:** only fill envelope when body is empty; `X-Request-Id` is still propagated so correlation is preserved. (This is deferred — see Open Questions.)
- **[Risk]** `on.error` handler itself throws. → **Mitigation:** wrap body-write in try/catch and fall through to `res.destroy(err)`.

## Migration Plan

Backwards-compatible for healthy paths. No config change required; `UPSTREAM_TIMEOUT_MS` defaults to 5000. Rollback = revert the commit; no data migration. Tests should assert `502`/`504` status + envelope shape against a stubbed unreachable upstream.

## Open Questions

- Should we also rewrite empty upstream 5xx into the envelope? The proposal suggests yes; this design defers it to keep the change minimal and avoids buffering upstream response bodies. Can be added in a follow-up once we decide on a buffering strategy.
