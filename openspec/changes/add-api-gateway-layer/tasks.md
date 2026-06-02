## 1. Gateway Setup

- [x] 1.1 Create a Go module and initial project files under `gateway/`
- [x] 1.2 Add a runnable entry point that starts an HTTP server on port `8080` by default
- [x] 1.3 Add configuration for the gateway port and upstream URLs with localhost defaults for users, orders, and products
- [x] 1.4 Add `UPSTREAM_TIMEOUT_MS`, `RATE_LIMIT_REQUESTS`, and `RATE_LIMIT_WINDOW_MS` environment configuration with sensible defaults
- [x] 1.5 Validate upstream URL, timeout, and rate limit configuration during gateway setup

## 2. Routing and Proxying

- [x] 2.1 Define an explicit route table for `/api/users/**`, `/api/orders/**`, and `/api/products/**`
- [x] 2.2 Implement strict route matching so near-miss paths such as `/api/users-extra` do not match
- [x] 2.3 Implement prefix rewriting so `/api/<resource>/...` forwards to the upstream path suffix
- [x] 2.4 Use `net/http/httputil.ReverseProxy` to preserve request method, query string, request body, and appropriate headers when forwarding
- [x] 2.5 Return `404 Not Found` for unmatched gateway routes, including readiness paths such as `/ready`
- [x] 2.6 Apply the configured upstream timeout and return a `5xx` gateway response when an upstream cannot be reached in time
- [x] 2.7 Implement a process-local fixed-window rate limiter that returns `429 Too Many Requests` over the configured limit
- [x] 2.8 Keep gateway routing state static and request handling stateless for horizontal scalability

## 3. Automated Tests

- [x] 3.1 Add tests for each route family forwarding to the correct upstream
- [x] 3.2 Add tests for strict route matching, including near-miss paths returning `404`
- [x] 3.3 Add tests for path suffix rewriting, no-trailing-slash behavior, trailing slash behavior, and query string preservation
- [x] 3.4 Add tests that request method and body are preserved for proxied requests
- [x] 3.5 Add tests for unmatched route `404` behavior and `/ready` returning `404`
- [x] 3.6 Add tests for default and environment-provided timeout and rate limit configuration
- [x] 3.7 Add tests for invalid upstream, timeout, and rate limit configuration
- [x] 3.8 Add tests for unavailable upstream failure handling
- [x] 3.9 Add tests for rate limit allow, reject, and reset behavior

## 4. Documentation and Scalability Notes

- [x] 4.1 Create `gateway/README.md` with Go setup, run, and test commands
- [x] 4.2 Document the route table and manual curl verification commands for users, orders, and products
- [x] 4.3 Document core minimal functionality: single entry point, route matching, prefix rewriting, request preservation, and gateway errors
- [x] 4.4 Document that the minimal gateway does not expose readiness or aggregate backend readiness
- [x] 4.5 Document timeout and rate limit environment variables with defaults
- [x] 4.6 Document scalability considerations: stateless horizontal scaling, configurable upstreams, timeouts, process-local versus distributed rate limits, load balancing, service discovery, observability, retries, authentication, and caching

## 5. Verification

- [x] 5.1 Run the gateway automated test suite
- [x] 5.2 Start backend services with `docker compose up --build`
- [x] 5.3 Start the gateway on port `8080`
- [x] 5.4 Verify `/api/users/`, `/api/orders/`, and `/api/products/` route through the gateway with curl
- [x] 5.5 Verify representative nested health routes proxied to backends, such as `/api/users/health`, `/api/orders/health`, and `/api/products/health`
- [x] 5.6 Verify an unmatched route and `/ready` return `404 Not Found`
- [x] 5.7 Verify a low configured rate limit returns `429 Too Many Requests` after the allowed request count
