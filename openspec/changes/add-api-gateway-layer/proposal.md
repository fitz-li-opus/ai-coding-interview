## Why

The interview scaffold provides three backend services but no single entry point for clients. An API gateway layer is needed to route client requests consistently through one port while keeping backend service details hidden.

## What Changes

- Add a gateway implementation under `gateway/` that listens on a single port, recommended `8080`.
- Implement the gateway in Go using the standard library reverse proxy support.
- Route `/api/users/**` requests to the user service at `http://localhost:8081`.
- Route `/api/orders/**` requests to the order service at `http://localhost:8082`.
- Route `/api/products/**` requests to the product service at `http://localhost:8083`.
- Constrain the critical behavior to route matching, prefix rewriting, upstream proxying, error handling, and minimal scalability properties.
- Configure upstream timeout and per-process rate limit behavior through environment variables with sensible defaults.
- Return appropriate gateway errors for unknown routes and unavailable or invalid upstream services.
- Add focused tests and a short manual `curl` verification flow for the three route families.
- Document the Go commands, route table, and production-oriented scalability trade-offs in `gateway/README.md`.

## Capabilities

### New Capabilities

- `api-gateway-routing`: Defines the gateway routing, proxying, timeout configuration, rate limiting, error handling, testing, and scalability expectations for the minimal Go implementation.

### Modified Capabilities

None.

## Impact

- Adds new source, tests, and documentation under `gateway/`.
- Does not modify the fixture services under `services/`.
- Introduces Go module files and gateway runtime commands under `gateway/`.
- Exposes a new client-facing API surface on the gateway port while relying on the existing backend service ports.
