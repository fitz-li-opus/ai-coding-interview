## ADDED Requirements

### Requirement: Gateway listens on a single client-facing port
The Go gateway SHALL expose one HTTP server for client requests and SHALL default to port `8080`.

#### Scenario: Gateway process is reachable
- **WHEN** a client sends a request to the gateway server
- **THEN** the gateway handles the request without requiring direct access to backend service ports

### Requirement: Gateway routes user API requests
The gateway SHALL forward requests matching `/api/users` or `/api/users/**` to the user service at the configured user upstream URL, defaulting to `http://localhost:8081`.

#### Scenario: User route forwards to user service
- **WHEN** a client sends `GET /api/users/`
- **THEN** the gateway forwards the request to `http://localhost:8081/`

#### Scenario: User nested route preserves suffix
- **WHEN** a client sends `GET /api/users/health`
- **THEN** the gateway forwards the request to `http://localhost:8081/health`

### Requirement: Gateway routes order API requests
The gateway SHALL forward requests matching `/api/orders` or `/api/orders/**` to the order service at the configured order upstream URL, defaulting to `http://localhost:8082`.

#### Scenario: Order route forwards to order service
- **WHEN** a client sends `GET /api/orders/`
- **THEN** the gateway forwards the request to `http://localhost:8082/`

#### Scenario: Order nested route preserves suffix
- **WHEN** a client sends `GET /api/orders/health`
- **THEN** the gateway forwards the request to `http://localhost:8082/health`

### Requirement: Gateway routes product API requests
The gateway SHALL forward requests matching `/api/products` or `/api/products/**` to the product service at the configured product upstream URL, defaulting to `http://localhost:8083`.

#### Scenario: Product route forwards to product service
- **WHEN** a client sends `GET /api/products/`
- **THEN** the gateway forwards the request to `http://localhost:8083/`

#### Scenario: Product nested route preserves suffix
- **WHEN** a client sends `GET /api/products/health`
- **THEN** the gateway forwards the request to `http://localhost:8083/health`

### Requirement: Gateway preserves request details
The gateway SHALL preserve the HTTP method, rewritten path suffix, query string, and request body when forwarding requests to an upstream service.

#### Scenario: Query string is preserved
- **WHEN** a client sends `GET /api/users/?active=true`
- **THEN** the gateway forwards the request to `http://localhost:8081/?active=true`

#### Scenario: Request method and body are preserved
- **WHEN** a client sends `POST /api/orders/submit` with a request body
- **THEN** the gateway forwards a `POST` request with that body to `http://localhost:8082/submit`

### Requirement: Gateway handles unsupported routes
The gateway SHALL return `404 Not Found` for requests that do not match a configured route or local gateway endpoint.

#### Scenario: Unknown API route returns not found
- **WHEN** a client sends `GET /api/inventory/`
- **THEN** the gateway returns `404 Not Found`

#### Scenario: Near-miss route returns not found
- **WHEN** a client sends `GET /api/users-extra`
- **THEN** the gateway returns `404 Not Found`

### Requirement: Gateway handles upstream failures
The gateway SHALL use bounded upstream timeouts and SHALL return a gateway error when a configured upstream cannot be reached.

#### Scenario: Unavailable upstream returns gateway error
- **WHEN** a client sends a request for a configured route and the target upstream is unavailable
- **THEN** the gateway returns a `5xx` response instead of hanging indefinitely

#### Scenario: Timeout environment config changes upstream timeout
- **WHEN** the gateway starts with `UPSTREAM_TIMEOUT_MS` set to a valid positive integer
- **THEN** the gateway uses that value as the upstream request timeout in milliseconds

#### Scenario: Invalid upstream configuration fails setup
- **WHEN** the gateway is configured with an invalid upstream URL
- **THEN** gateway setup fails before serving requests

#### Scenario: Invalid timeout configuration fails setup
- **WHEN** the gateway starts with `UPSTREAM_TIMEOUT_MS` set to an invalid value
- **THEN** gateway setup fails before serving requests

### Requirement: Gateway supports configurable local rate limiting
The gateway SHALL support a process-local rate limit configured by `RATE_LIMIT_REQUESTS` and `RATE_LIMIT_WINDOW_MS`, and SHALL return `429 Too Many Requests` when the configured limit is exceeded.

#### Scenario: Default rate limit permits normal demo traffic
- **WHEN** rate limit environment variables are absent
- **THEN** the gateway uses default rate limit settings that allow the documented curl verification flow

#### Scenario: Rate limit environment config changes limit
- **WHEN** the gateway starts with valid positive integer values for `RATE_LIMIT_REQUESTS` and `RATE_LIMIT_WINDOW_MS`
- **THEN** the gateway enforces that request count for each configured time window

#### Scenario: Request over limit is rejected
- **WHEN** a client sends more requests than allowed by the configured rate limit window
- **THEN** the gateway returns `429 Too Many Requests` for requests over the limit

#### Scenario: Rate limit resets after window
- **WHEN** a client exceeds the configured limit and then waits for the configured window to elapse
- **THEN** the gateway accepts new requests in the next window

#### Scenario: Invalid rate limit configuration fails setup
- **WHEN** the gateway starts with invalid `RATE_LIMIT_REQUESTS` or `RATE_LIMIT_WINDOW_MS` values
- **THEN** gateway setup fails before serving requests

### Requirement: Gateway avoids readiness endpoints
The gateway SHALL NOT expose a readiness endpoint or aggregate backend readiness status in the minimal implementation.

#### Scenario: Readiness endpoint is not available
- **WHEN** a client sends `GET /ready` to the gateway
- **THEN** the gateway returns `404 Not Found`

### Requirement: Gateway supports minimal scalability constraints
The gateway SHALL keep routing state static, in-memory, and stateless per process so multiple gateway instances can be run independently with equivalent configuration.

#### Scenario: Gateway has no request-affinity state
- **WHEN** two gateway instances use the same route configuration
- **THEN** either instance can route a matching request without shared mutable state

#### Scenario: Rate limit is process-local
- **WHEN** two gateway instances run with the same rate limit configuration
- **THEN** each instance enforces its own local limit without requiring shared state

### Requirement: Gateway includes implementation documentation
The gateway SHALL include documentation for running, testing, manually verifying the gateway routes, and explaining scalability considerations.

#### Scenario: README includes curl verification
- **WHEN** a developer reads the gateway documentation
- **THEN** it includes commands to verify `/api/users/**`, `/api/orders/**`, and `/api/products/**` routing through the gateway
