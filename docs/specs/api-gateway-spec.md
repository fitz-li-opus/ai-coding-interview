# API Gateway SPEC

## Problem Statement

We need to build a lightweight API Gateway for the interview project.

The gateway will be the single entry point for clients. It listens on one port, recommended `8080`, then forwards requests to one of the existing backend services based on the request path.

Required routes:

- `/api/users/**` -> user-service
- `/api/orders/**` -> order-service
- `/api/products/**` -> product-service

The goal is not to build a full commercial gateway. The goal is to deliver a runnable, explainable solution within the interview time, while showing awareness of what a production-grade gateway would need.

## Proposed Solution

Use Node.js for the gateway implementation.

Use Express plus a lightweight proxy middleware. This keeps the implementation short and focused on gateway behavior instead of low-level request forwarding details.

The intended demo mode is to run the three backend services with Docker Compose and run the gateway locally from the `gateway/` directory. In that mode, the gateway uses `localhost:8081`, `localhost:8082`, and `localhost:8083` as default backend targets.

### Must-have

- Start a gateway service on port `8080` by default.
- Route requests by path prefix:
  - `/api/users/**` to user-service
  - `/api/orders/**` to order-service
  - `/api/products/**` to product-service
- Forward paths as-is. For example, `/api/users/123` is forwarded as `/api/users/123`.
- When using Express proxy middleware, confirm it does not accidentally strip `/api/users`, `/api/orders`, or `/api/products` from the forwarded path. The backend response `path` field should still show the original API path during curl demo.
- Match only the intended prefixes. For example, `/api/users` and `/api/users/123` match user-service, but `/api/usersx` does not.
- Return JSON `404` for unsupported routes.
- Return JSON `502` when the matched backend service is unavailable.

### Bonus

Bonus work should start only after the minimum curl demo is already working. Prefer logging and timeout handling first; add API key auth and rate limiting if time remains.

- Request logging: method, path, target service, response status, and duration.
- Timeout handling: avoid requests hanging forever when a backend does not respond.
- Unified JSON error responses.
- API key authentication:
  - Use the `x-api-key` request header.
  - Compare it with a configured environment variable.
  - If no API key is configured, authentication is disabled for easy local demo.
  - Keep `/health` public if the gateway exposes its own health endpoint.
- Fixed-window rate limiting:
  - Use in-memory storage.
  - Limit requests by client IP.
  - Keep it simple and demo-friendly, not distributed.
  - Allow the limit and time window to be configured by environment variables.
  - In local demo mode, do not trust client-provided `X-Forwarded-For`. Only use trusted proxy headers if the gateway is explicitly deployed behind a trusted reverse proxy.

## Technical Constraints

- Do not modify files under `services/`.
- Put gateway implementation under `gateway/`.
- Use Node.js.
- Use lightweight dependencies only.
- Backend target URLs should be configurable by environment variables with local defaults:
  - user-service default: `http://localhost:8081`
  - order-service default: `http://localhost:8082`
  - product-service default: `http://localhost:8083`
- If the gateway is later containerized into the same Docker Compose network, backend URLs must be changed to service names such as `http://user-service:8080`.
- The rate limiter is single-process and in-memory.
- API key auth is gateway-level request protection, not a full user identity system.
- Installing Node dependencies is required before running the gateway.

## Non-goals

- No changes to the existing backend mock services.
- No service discovery.
- No config center.
- No Redis or database.
- No circuit breaker.
- No JWT or OAuth.
- No full monitoring platform.
- No Kubernetes, production deployment, or CI/CD work.
- No complex path rewriting.
- No gateway containerization in the first implementation unless the interviewer asks for it.

## Success Criteria

### Minimum demo acceptance

The minimum solution is successful when:

- The gateway starts locally on `localhost:8080`.
- `curl http://localhost:8080/api/users` reaches user-service.
- `curl http://localhost:8080/api/orders` reaches order-service.
- `curl http://localhost:8080/api/products` reaches product-service.
- The backend response confirms paths are forwarded as-is.
- Unsupported routes return JSON `404`.
- Unavailable backend services return JSON `502`.

### Optional bonus acceptance

If bonus features are implemented, they are successful when:

- If API key auth is configured, missing or wrong keys are rejected.
- If rate limit is exceeded, the gateway returns a rate-limit error.
- Logs show enough information to explain where each request was routed.
