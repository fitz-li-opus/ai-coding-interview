## Context

The repository provides three fixture backend services and asks candidates to implement a lightweight API gateway in `gateway/`. The gateway is the only new runtime component and must leave `services/` unchanged. For the minimal implementation, the critical behavior is constrained to route matching, prefix rewriting, error handling, and scalability properties that keep the gateway simple but explainable.

## Goals / Non-Goals

**Goals:**

- Implement the gateway in Go using `net/http` and `net/http/httputil.ReverseProxy`.
- Provide a single client-facing HTTP entry point, recommended on port `8080`.
- Match only the required route families: `/api/users/**`, `/api/orders/**`, and `/api/products/**`.
- Rewrite each matched route to the upstream path suffix and proxy the request to the correct fixture service.
- Preserve method, query string, body, and appropriate headers through the proxy.
- Return clear errors for unmatched routes, invalid upstream configuration, and unavailable upstream services.
- Configure upstream timeout and per-process rate limiting through environment variables with defaults suitable for local demos.
- Keep the gateway stateless and configurable enough to scale horizontally later.
- Add focused automated tests and a concise manual `curl` demo.

**Non-Goals:**

- Do not modify or extend the mock backend services.
- Do not expose a gateway readiness endpoint or aggregate backend readiness state.
- Do not implement authentication, authorization, TLS termination, request transformation, caching, service discovery, dynamic config reload, or distributed tracing for the minimal solution.
- Do not implement distributed rate limiting; the minimal rate limiter is process-local and intended as a simple overload guard.
- Do not build a full production gateway platform; document scalability considerations instead of overbuilding them for the interview scaffold.

## Decisions

- Use Go standard library proxy support.
  - Rationale: `httputil.ReverseProxy` directly supports the required proxy behavior, keeps dependencies low, and fits the existing Go service style.
  - Alternative considered: Node.js or Python proxy libraries. They would work, but add a second runtime style to a repository whose fixtures already use Go.

- Use an explicit route table for upstream mapping.
  - Rationale: The required routes are small and fixed, so a route table is easier to test and explain than dynamic discovery.
  - Alternative considered: Separate hard-coded handlers for each route. That is simple but duplicates matching and proxy setup logic.

- Treat route matching as a strict prefix contract.
  - Rationale: `/api/users` and `/api/users/...` should match the user route, while similar paths such as `/api/users-extra` must not accidentally match.
  - Alternative considered: Loose string prefix matching. That is shorter but creates ambiguous routing behavior.

- Strip the `/api/<resource>` prefix before forwarding to the upstream service.
  - Rationale: The fixture services expose their resources at `/` and `/health`, so `/api/users/` should reach `http://localhost:8081/`, and `/api/users/health` should reach `http://localhost:8081/health`.
  - Alternative considered: Preserve the full path. That would send paths the fixtures do not define and would make the required curl demos fail unless backend services changed.

- Keep upstream URLs configurable with sensible localhost defaults.
  - Rationale: Defaults support local interview demos, while environment configuration allows Docker or future deployment changes without code edits.
  - Alternative considered: Hard-code all upstream URLs. This is acceptable for the smallest exercise but weakens portability and scalability.

- Use bounded upstream timeouts and `ReverseProxy.ErrorHandler`.
  - Rationale: Gateway requests must not hang indefinitely when an upstream is unavailable, and proxy failures should become predictable `5xx` responses.
  - Alternative considered: Rely on default transport behavior. That is simpler but makes failure behavior harder to test and explain.

- Read timeout and rate limit settings from environment variables.
  - Rationale: `UPSTREAM_TIMEOUT_MS`, `RATE_LIMIT_REQUESTS`, and `RATE_LIMIT_WINDOW_MS` make operational behavior tunable without code changes while keeping the implementation small.
  - Alternative considered: Hard-code timeout and rate limit values. That is simpler but makes scalability and failure-mode tuning harder to demonstrate.

- Use a process-local fixed-window rate limiter.
  - Rationale: A local limiter is enough for the minimal implementation, requires no external dependency, and protects each gateway instance from bursts.
  - Alternative considered: Distributed rate limiting through Redis or another shared store. That is more production-ready but out of scope for this interview scaffold.

## Test Plan

- Route matching tests:
  - Verify `/api/users` and `/api/users/...` route to the user upstream.
  - Verify `/api/orders` and `/api/orders/...` route to the order upstream.
  - Verify `/api/products` and `/api/products/...` route to the product upstream.
  - Verify near-miss paths such as `/api/users-extra` return `404 Not Found`.

- Prefix rewriting tests:
  - Verify `/api/users/` forwards as `/`.
  - Verify nested paths such as `/api/orders/health` forward as `/health`.
  - Verify query strings are preserved after rewriting.

- Proxy preservation tests:
  - Verify HTTP method and request body reach the stub upstream unchanged.
  - Verify appropriate request headers are available to the upstream while hop-by-hop behavior is left to the proxy transport.

- Error handling tests:
  - Verify unmatched routes return `404 Not Found`.
  - Verify invalid upstream configuration fails early during gateway setup.
  - Verify unavailable upstreams return a bounded `5xx` response.
  - Verify invalid timeout and rate limit environment values fail setup with a clear configuration error.

- Configuration tests:
  - Verify default timeout and rate limit settings are applied when environment variables are absent.
  - Verify `UPSTREAM_TIMEOUT_MS` changes the upstream timeout budget.
  - Verify `RATE_LIMIT_REQUESTS` and `RATE_LIMIT_WINDOW_MS` change the local rate limit behavior.

- Rate limiting tests:
  - Verify requests within the configured limit are proxied normally.
  - Verify requests beyond the configured limit return `429 Too Many Requests`.
  - Verify the limit resets after the configured window.

- Manual verification:
  - Start fixture services with `docker compose up --build`.
  - Start the Go gateway on `8080`.
  - Use curl to verify `/api/users/`, `/api/orders/`, `/api/products/`, and representative nested routes such as `/api/users/health`.

## Risks / Trade-offs

- Prefix rewriting is easy to get wrong -> Cover root path, no-trailing-slash path, nested path, near-miss path, and query string scenarios in tests.
- Upstream services are unavailable or slow -> Configure timeouts and return a clear gateway error rather than hanging client requests.
- Rate limiting is per process only -> Document that horizontal deployments need either per-instance limits with capacity planning or a shared limiter for global quotas.
- Header forwarding can leak or corrupt hop-by-hop metadata -> Use `ReverseProxy` and avoid custom connection-level header forwarding.
- Single gateway process can become a bottleneck -> Keep the implementation stateless so multiple gateway instances can run behind a load balancer.
- Static route configuration does not adapt to changing service instances -> Treat service discovery, load balancing, and dynamic config reload as future scalability extensions.
- Retries can amplify traffic during outages -> Do not add retries in the minimal implementation; document bounded retry policy as a future production consideration.

## Migration Plan

1. Add the Go gateway implementation, tests, and README under `gateway/`.
2. Run `go test ./...` from `gateway/`.
3. Start backend services with `docker compose up --build`.
4. Start the gateway on port `8080`.
5. Verify routed requests with curl for users, orders, products, and nested health routes proxied to the fixture services.

Rollback is deleting or ignoring the new `gateway/` implementation; existing backend services remain unchanged.

## Open Questions

None for the minimal implementation.
