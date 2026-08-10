# Extensible API Gateway Design

## Goal

Build a Go 1.22 API Gateway that listens on one address and securely proxies
the `/api/users`, `/api/orders`, and `/api/products` route families to their
configured upstream services. The implementation must be extensible,
modular, robust, and developed in small test-driven stages.

## Scope

The gateway must:

- route `/api/users[/...]` to the user service;
- route `/api/orders[/...]` to the order service;
- route `/api/products[/...]` to the product service;
- strip the matched public prefix before forwarding;
- preserve the HTTP method, body, query string, relevant headers, upstream
  response status, headers, and body;
- return JSON errors for unmatched routes, invalid requests, upstream
  failures, and upstream timeouts;
- expose a gateway-only `GET /health` liveness endpoint;
- support graceful shutdown and structured access logging.

Authentication, rate limiting, TLS termination, service discovery, dynamic
configuration reload, upstream health probing, retries, circuit breaking,
metrics, and distributed tracing are outside this implementation. The design
must leave clear middleware or configuration extension points for them.

## Technical Approach

Use Go 1.22 and the standard library only. `net/http` provides the server and
middleware contract, while `httputil.ReverseProxy` provides response
streaming, connection reuse, forwarding behavior, and hop-by-hop header
handling. No framework or third-party dependency is needed for the required
scope.

The gateway runs on the host by default and connects to the Compose-published
ports at `localhost:8081`, `localhost:8082`, and `localhost:8083`. Adding the
gateway to Docker Compose is not part of this change.

## Module Boundaries

All files remain under `gateway/`.

| File | Responsibility |
|---|---|
| `go.mod` | Define the Go 1.22 module with no external dependencies. |
| `main.go` | Compose dependencies, configure `http.Server`, handle signals, and shut down gracefully. |
| `config.go` | Define configuration, apply defaults, parse environment values, and validate all startup input. |
| `config_test.go` | Test defaults, overrides, boundary values, and rejected configuration. |
| `route.go` | Define validated route specifications and segment-boundary matching. |
| `route_test.go` | Test route validation, exact matches, nested paths, and lookalike rejection. |
| `proxy.go` | Build one reverse proxy per route, rewrite targets and paths, and map proxy errors. |
| `proxy_test.go` | Test forwarding semantics, upstream responses, failures, timeouts, and cancellation. |
| `middleware.go` | Implement request IDs, body limits, structured access logging, and response observation. |
| `middleware_test.go` | Test request-ID validation, body limits, status capture, and optional HTTP interfaces. |
| `handler.go` | Assemble health, middleware, router, proxies, and consistent JSON responses. |
| `handler_test.go` | Exercise the complete handler through in-memory upstream servers. |
| `README.md` | Document build, configuration, routing semantics, curl demonstrations, and trade-offs. |

Each package-level unit exposes a narrow standard-library interface. The
assembled application exposes `http.Handler`, so later authentication, rate
limiting, or tracing middleware can wrap it without changing proxy internals.

## Configuration

Configuration is read once at startup:

| Environment variable | Default | Validation |
|---|---|---|
| `GATEWAY_ADDR` | `:8080` | Non-empty valid TCP listen address. |
| `USER_SERVICE_URL` | `http://localhost:8081` | Absolute `http` or `https` URL with host; no user info, query, or fragment. |
| `ORDER_SERVICE_URL` | `http://localhost:8082` | Same URL rules. |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` | Same URL rules. |
| `UPSTREAM_TIMEOUT` | `5s` | Positive duration, no greater than 60 seconds. |
| `MAX_REQUEST_BODY_BYTES` | `1048576` | Integer from 1 through 10,485,760 bytes. |

`LoadConfig(getenv func(string) string)` receives its environment lookup as a
dependency, enabling deterministic tests without process-global mutation.
Invalid configuration prevents startup and reports which field is invalid,
without logging credentials or full sensitive URLs.

## Route Model and Extensibility

Routes are declared as data rather than separate handlers:

```go
type RouteSpec struct {
    Name   string
    Prefix string
    Target *url.URL
}
```

A route is validated once during construction. Adding another service means
adding a validated `RouteSpec`; matching and proxy behavior remain unchanged.
The route table rejects duplicate prefixes, empty names, non-absolute
prefixes, trailing slashes, and invalid targets.

Matching occurs only when the path equals the prefix or begins with the
prefix followed by `/`. Thus `/api/users` and `/api/users/42` match, while
`/api/users-v2` does not.

The matched prefix is stripped:

| Incoming path | Upstream path |
|---|---|
| `/api/users` | `/` |
| `/api/users/` | `/` |
| `/api/users/42` | `/42` |

The raw query string is preserved. Ambiguous request paths containing a NUL
byte, a backslash, an encoded slash or backslash, or a dot path segment (`.`
or `..`, including encoded forms after unescaping) are rejected with `400`.
Repeated ordinary slashes are preserved because silently normalizing them
could change upstream resource identity.

All standard HTTP methods are forwarded. Method authorization is an
application policy and remains a future per-route extension rather than an
implicit gateway restriction.

## Proxy and Header Security

Upstream targets come only from validated startup configuration. No request
parameter, path component, or client header can select or override a target,
preventing the gateway from becoming an open proxy or SSRF primitive.

The proxy preserves end-to-end request headers and relies on
`ReverseProxy` to remove hop-by-hop headers. It discards client-supplied
`Forwarded` and `X-Forwarded-*` values, then constructs forwarding metadata
from the current trusted connection. This prevents spoofing proxy-chain and
original-client information.

`X-Request-ID` is accepted only when it is 1 through 128 ASCII characters
from `[A-Za-z0-9._-]`. Missing or invalid values are replaced with a locally
generated cryptographically random identifier. The selected value is sent to
the upstream and returned to the client.

Request bodies are limited to `MAX_REQUEST_BODY_BYTES`. A known
`Content-Length` above the limit is rejected before proxying with `413`. For
chunked or unknown-length bodies, the limited reader detects overflow while
the proxy reads. The gateway never logs request or response bodies.

## Request Flow

1. Validate the request target and path before routing.
2. Select or generate a safe request ID.
3. Attach access logging and response observation.
4. Serve `GET /health`, or match a route using path-segment boundaries.
5. Apply the request body limit and upstream deadline.
6. Rewrite the scheme, host, and stripped path to the route's fixed target.
7. Rebuild trusted forwarding headers and invoke the route's reusable proxy.
8. Stream the upstream response to the client.
9. Record request ID, method, path, status, duration, and remote address.

`/health` accepts only `GET`; other methods return `405` with an `Allow: GET`
header. It indicates gateway process liveness and never probes upstreams.

## Error Contract

Gateway-generated errors use `application/json` and this stable shape:

```json
{
  "error": "route_not_found",
  "message": "no route matches the requested path",
  "requestId": "..."
}
```

| Condition | Status | Code |
|---|---:|---|
| Invalid or ambiguous request path | 400 | `invalid_request` |
| Unsupported `/health` method | 405 | `method_not_allowed` |
| Request body exceeds the limit | 413 | `request_too_large` |
| No route matches | 404 | `route_not_found` |
| Upstream deadline expires | 504 | `gateway_timeout` |
| Other upstream proxy failure | 502 | `bad_gateway` |

Messages are fixed and do not contain upstream addresses, configuration,
stack traces, or operating-system errors. Internal diagnostics are logged
with the request ID.

Once an upstream response has begun, the gateway cannot replace it with a
JSON error safely; it logs the stream failure and terminates the response.

## Server Robustness

The HTTP server uses:

- `ReadHeaderTimeout`: 5 seconds;
- `ReadTimeout`: 10 seconds;
- `WriteTimeout`: 15 seconds;
- `IdleTimeout`: 60 seconds;
- `MaxHeaderBytes`: 1 MiB;
- graceful shutdown deadline: 10 seconds.

Each upstream request has the configured deadline. Cancellation propagates
from the client through the proxy. A reusable transport provides connection
pooling and uses explicit dial, TLS handshake, response-header, idle
connection, and expect-continue timeouts.

Middleware wrappers preserve supported optional `http.ResponseWriter`
interfaces, particularly `http.Flusher`, so streamed responses continue to
work. WebSocket proxying is outside the required compatibility scope.

## Test Strategy

Development proceeds in small red-green-review stages. Tests use
`httptest.Server` and never depend on locally running services.

1. **Configuration stage:** failing tests for defaults, overrides, URL
   restrictions, duration bounds, numeric bounds, and listen-address
   validation; minimal implementation; full stage review.
2. **Route stage:** failing table tests for valid routes, invalid route specs,
   exact/nested matches, lookalike paths, ambiguous paths, and prefix
   stripping; minimal implementation; full stage review.
3. **Proxy stage:** failing tests for method, body, query, headers, response
   status/body/header preservation, forwarding-header sanitization, timeout,
   connection failure, and cancellation; minimal implementation; full stage
   review.
4. **Middleware stage:** failing tests for request-ID acceptance/replacement,
   body limits, status recording, logs, and `Flusher`; minimal implementation;
   full stage review.
5. **Assembly and lifecycle stage:** failing end-to-end handler tests for all
   routes, health, 400/404/405/413/502/504 contracts, then server startup and
   shutdown wiring; full stage review.
6. **Documentation and final verification:** add runnable commands and curl
   examples, then run `gofmt`, `go test ./...`, `go test -race ./...`,
   `go vet ./...`, `go build ./...`, and `git diff --check`.

At each review gate, report the failing test evidence, implementation diff,
passing commands, security implications, and any design deviation. Do not
start the next module until the current stage is accepted.

## Repository Changes

Create the files listed in Module Boundaries and remove `gateway/.gitkeep`
after real files exist. Do not modify the root `README.md`,
`docker-compose.yml`, or anything under `services/`.

