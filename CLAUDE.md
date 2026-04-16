# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Purpose

This is a **40-minute coding interview scaffold**. The candidate must implement an API Gateway in the `gateway/` directory (currently empty) that proxies requests to 3 pre-built backend microservices.

## Starting the Backend Services

```bash
docker compose up --build
```

Services after startup:
- `http://localhost:8081` — user-service
- `http://localhost:8082` — order-service
- `http://localhost:8083` — product-service

Each service exposes `/` (returns JSON with service name, path, method, timestamp, and data) and `/health`.

## Gateway Task

Implement the gateway in `gateway/` to listen on `:8080` with these routing rules:

- `/api/users/**` → `http://localhost:8081`
- `/api/orders/**` → `http://localhost:8082`
- `/api/products/**` → `http://localhost:8083`

The gateway can be in any language/framework. Use `docker compose up` or run directly.

## Constraints

- Do **not** modify anything under `services/`
- All implementation goes in `gateway/`

## Backend Service Behavior

All three services respond to any path (catch-all `/` handler) and return a uniform JSON envelope:
```json
{
  "service": "<service-name>",
  "path": "<original path>",
  "method": "GET",
  "timestamp": "...",
  "data": [...]
}
```
The services run on internal port `8080`, mapped to `8081/8082/8083` on the host via Docker.
