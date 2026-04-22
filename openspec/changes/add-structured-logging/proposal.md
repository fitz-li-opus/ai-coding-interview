## Why

The gateway currently logs via `http-proxy-middleware`'s default console format, which is human-oriented and lacks a request identifier. Without structured fields and a request-id propagated to upstream services, we cannot correlate a client request to logs across gateway + 3 backends, which is essential for debugging in production.

## What Changes

- Emit one structured JSON log line per request at the gateway, including: `ts`, `level`, `request_id`, `method`, `path`, `upstream`, `status`, `duration_ms`, `remote_addr`.
- Generate a UUID v4 `X-Request-Id` per inbound request. If the client already sent one, reuse it.
- Inject `X-Request-Id` into the upstream request headers and echo it back in the response headers so clients can quote it in bug reports.
- Replace the default `logger: console` in `http-proxy-middleware` with the structured logger so proxy-level events (errors, retries) share the same format.

## Capabilities

### New Capabilities
- `observability`: structured logging and request-id propagation across gateway and upstream.

### Modified Capabilities
<!-- none -->

## Impact

- `gateway/index.js`: add request-id middleware (runs before proxy middlewares), add logger helper, wire per-request log on `res.on('finish')`.
- `gateway/package.json`: no new runtime deps required (UUID via `crypto.randomUUID()`, JSON via `JSON.stringify`). If we choose `pino` for speed, add it here.
- No API contract change for clients; only an added response header `X-Request-Id`.
- Backends see a new inbound header `X-Request-Id`; they are not required to honor it yet, but may later.
