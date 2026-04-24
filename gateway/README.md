# Java API Gateway (JDK 21, DDD-style)

## What is implemented (minimum requirement)

- Single gateway port: `8080` (configurable by `GATEWAY_PORT`)
- Route forwarding:
  - `/api/users/**` -> `http://localhost:8081`
  - `/api/orders/**` -> `http://localhost:8082`
  - `/api/products/**` -> `http://localhost:8083`
- Path rewrite rule: **do not strip prefix**, upstream receives original path.
- Gateway health check: `GET /health`
- Simple auth: request header `X-API-Key` must equal `GATEWAY_API_KEY` (default `interview-key`)

## DDD layering used

- `domain`: routing rule model and route resolver
- `application`: gateway orchestration service and forwarding contract
- `infrastructure`: Java `HttpClient` forwarding implementation
- `interfaces`: incoming HTTP handler

## Run

1. Start backend services in project root:

```bash
docker compose up --build
```

2. Start gateway (PowerShell, no Maven required):

```bash
cd gateway
powershell -ExecutionPolicy Bypass -File .\run.ps1
```

Alternative (if Maven is available):

```bash
cd gateway
mvn clean compile
mvn exec:java
```

## Verify by curl

```bash
curl http://localhost:8080/health
curl -H "X-API-Key: interview-key" http://localhost:8080/api/users/
curl -H "X-API-Key: interview-key" http://localhost:8080/api/orders/
curl -H "X-API-Key: interview-key" http://localhost:8080/api/products/
```

## TODO (production hardening roadmap)

### P0 - correctness and security baseline

- TODO: Fix route matching boundary bug (`/api/users2` should NOT match `/api/users`)
- TODO: Enforce max request body/header size and reject oversized payloads (`413` / `431`)
- TODO: Replace static API key auth with JWT/OIDC validation and per-route authorization
- TODO: Remove weak default secret and fail fast when auth config is missing
- TODO: Complete request/response header sanitization for hop-by-hop and proxy-only headers

### P1 - delivery quality gates (CI/CD + tests + regression control)

- TODO: Add CI workflow (`mvn verify`) with required status checks before merge
- TODO: Add unit tests for route resolver, auth checks, error mapping, and header filtering
- TODO: Add integration tests against docker-compose upstream services
- TODO: Add coverage gate (JaCoCo) and enforce minimum threshold
- TODO: Add performance regression gate (k6/Gatling) with latency/error-rate thresholds
- TODO: Add security gates (SCA/SAST/secret scan) in pull-request pipeline

### P2 - reliability, observability, and HA operations

- TODO: Add structured JSON logging and end-to-end request ID propagation
- TODO: Add metrics and tracing (Prometheus/OpenTelemetry) with dashboards and alerts
- TODO: Add resilience controls: rate limiting, circuit breaker, retry with backoff/jitter
- TODO: Split `/health/live` and `/health/ready` and wire readiness to dependencies
- TODO: Externalize route config with schema validation and environment profiles
- TODO: Add graceful shutdown and deployment strategy notes (rolling/canary + rollback)
- TODO: add team collaboration guide (`docs/04-collaboration.md`)
- TODO: add quick onboarding docs (`docs/01-quick-start.md`, `docs/02-architecture.md`, `docs/03-dev-guide.md`)
- TODO: add PR template with required test steps
- TODO: define branch naming and code review checklist
- TODO: create task board and split work by DDD layers
- TODO: add environment layering config (`local` / `test` / `prod`) with clear variable strategy
- TODO: split gateway config files by environment and support startup profile selection
- TODO: ensure local development environment can run end-to-end without Docker dependency
- TODO: add dedicated local development documentation (setup, startup, verification, troubleshooting)
