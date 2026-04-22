## 1. Configuration

- [x] 1.1 Read `UPSTREAM_TIMEOUT_MS` from `process.env` with a default of 5000 and coerce to a positive integer
- [x] 1.2 Log the resolved timeout value at startup so ops can verify the active setting

## 2. Proxy wiring

- [x] 2.1 Pass `timeout` and `proxyTimeout` options (both set to the resolved timeout) to `createProxyMiddleware`
- [x] 2.2 Add an `on.error` handler on each proxied route

## 3. Error envelope

- [x] 3.1 Implement a `sendErrorEnvelope(res, status, code, message, requestId)` helper that writes `{ error: { code, message, request_id } }` as JSON
- [x] 3.2 Guard the helper against `res.headersSent` — destroy the response if headers already went out
- [x] 3.3 Fall back to `crypto.randomUUID()` when `req.requestId` is missing so `request_id` is never empty

## 4. Error mapping

- [x] 4.1 Map `ECONNREFUSED`, `ENOTFOUND`, `EAI_AGAIN`, `EHOSTUNREACH`, `ENETUNREACH` to `502` / `BAD_GATEWAY`
- [x] 4.2 Map `ETIMEDOUT`, `ECONNABORTED` to `504` / `GATEWAY_TIMEOUT`
- [x] 4.3 Treat any other error code as `502` / `BAD_GATEWAY` (fail-closed default)
- [x] 4.4 Log the original `err.code` and `err.message` at `error` level alongside `request_id` for observability
