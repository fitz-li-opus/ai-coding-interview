# Repository Guidelines

## Project Structure & Module Organization

This repository is an API gateway interview scaffold. The provided backend fixtures live under `services/` and should not be modified:

- `services/user-service/` serves mock users on host port `8081`.
- `services/order-service/` serves mock orders on host port `8082`.
- `services/product-service/` serves mock products on host port `8083`.

Each service contains a small Go `main.go` and its own `Dockerfile`. `docker-compose.yml` builds and runs the three services. Implement all gateway code in `gateway/`, which is intentionally empty at the start of the exercise.

## Build, Test, and Development Commands

- `docker compose up --build` builds and starts the three backend services.
- `curl http://localhost:8081/`, `curl http://localhost:8082/`, and `curl http://localhost:8083/` verify the services are reachable.
- `curl http://localhost:8081/health` checks a service health endpoint.

Gateway commands depend on the stack chosen inside `gateway/`. Prefer documenting any new commands in `gateway/README.md` or the root `README.md`; for example, `go test ./...`, `npm test`, or `python -m pytest`.

## Coding Style & Naming Conventions

For existing Go services, keep standard Go formatting with tabs via `gofmt`. Use clear package-level names and small HTTP handlers, matching the current service style. For new gateway code, follow the conventions of the chosen language and include formatter or linter configuration when practical. Use descriptive names for routes and upstreams, such as `users`, `orders`, `products`, and `upstreamURL`.

## Testing Guidelines

There is no existing automated test suite. Add focused tests under `gateway/` for routing behavior, path rewriting, error handling, and health checks. Name tests according to the selected framework, such as `*_test.go` for Go or `*.test.ts` for TypeScript. Always include a short manual verification flow with `curl`, especially for:

- `/api/users/**` to `http://localhost:8081`
- `/api/orders/**` to `http://localhost:8082`
- `/api/products/**` to `http://localhost:8083`

## Commit & Pull Request Guidelines

Recent commits use short, imperative messages, for example `Simplify requirements to only basic routing`. Keep commit subjects concise and action-oriented. Pull requests should describe the gateway design, list test commands or curl demos, and call out any bonus features such as retries, logging, rate limiting, or configuration. Include screenshots only if a UI is added.

## Agent-Specific Instructions

Do not change files under `services/` unless explicitly asked. Keep implementation work scoped to `gateway/`, and avoid adding routing hints outside the submitted gateway solution.
