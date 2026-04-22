## Why

If an upstream service hangs or is unreachable, `http-proxy-middleware` either waits on the default Node socket timeout (~2 min) or surfaces a raw `ECONNREFUSED` / `ECONNRESET` as an unhandled response. Both are unacceptable in production: clients experience indefinite latency, and error responses are inconsistent (sometimes empty, sometimes HTML). The gateway must own upstream timeouts and return a uniform JSON error envelope.

## What Changes

- Set a bounded upstream timeout (default 5000 ms, configurable via `UPSTREAM_TIMEOUT_MS`) on every proxied request.
- Map upstream failure modes to stable HTTP statuses:
  - connection refused / DNS failure → `502 Bad Gateway`
  - timeout → `504 Gateway Timeout`
  - upstream returns 5xx → pass through status, but wrap body in our error envelope if body is empty
- Return a uniform error envelope `{ "error": { "code": "<string>", "message": "<string>", "request_id": "<uuid>" } }` for all gateway-originated failures.
- Catch `proxyReq`/`error` events in `http-proxy-middleware` so no unhandled error reaches the default Node error handler.

## Capabilities

### New Capabilities
- `resilience`: upstream timeout enforcement and uniform gateway error responses.

### Modified Capabilities
<!-- none -->

## Impact

- `gateway/index.js`: pass `proxyTimeout` and `timeout` options to `createProxyMiddleware`, add an `on.error` handler that writes the error envelope.
- Depends on `observability` change for `request_id` in the envelope — if that change has not landed, fall back to generating a fresh UUID per error.
- No new dependencies.
- Clients now receive deterministic `502`/`504` with JSON body; docs/tests should assert on this.
