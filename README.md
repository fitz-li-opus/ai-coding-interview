# API Gateway Coding Interview

## Overview

This repository contains a lightweight API Gateway coding-interview project.

The gateway implementation lives in `gateway/` and is built with Bun, TypeScript, and Elysia. It exposes one public HTTP entry point and forwards requests to three backend microservices.

---

## Environment

This project provides **3 backend microservices**, ready to launch with Docker Compose:

```bash
docker compose up --build
```

Available backend services after startup:

| Service | Address | Description |
|---------|---------|-------------|
| user-service | `http://localhost:8081` | Returns a list of users |
| order-service | `http://localhost:8082` | Returns a list of orders |
| product-service | `http://localhost:8083` | Returns a list of products |

Verify the services are running:

```bash
curl http://localhost:8081/
curl http://localhost:8082/
curl http://localhost:8083/
```

Each service also exposes a `/health` endpoint for health checks.

---

## API Gateway

The gateway listens on `http://localhost:8080` by default and forwards requests based on path prefix:

- `/api/users/**` → `http://localhost:8081`
- `/api/orders/**` → `http://localhost:8082`
- `/api/products/**` → `http://localhost:8083`

It also exposes:

- `/health` → gateway health and configured upstream list

Implemented gateway features:

- Elysia-based routing
- native `fetch` proxying
- method, query string, body, and header forwarding
- `X-Request-ID` propagation/generation
- upstream timeout handling
- `404` for unknown routes
- `502` for upstream failures
- Bun tests, TypeScript type checking, oxlint, and oxfmt

---

## Run

Start backend services from the repository root:

```bash
docker compose up --build
```

In another terminal, start the gateway:

```bash
cd gateway
bun install
bun run dev
```

Demo requests:

```bash
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products
curl http://localhost:8080/health
```

## Configuration

| Env var | Default |
|---------|---------|
| `PORT` | `8080` |
| `REQUEST_TIMEOUT_MS` | `5000` |
| `USER_SERVICE_URL` | `http://localhost:8081` |
| `ORDER_SERVICE_URL` | `http://localhost:8082` |
| `PRODUCT_SERVICE_URL` | `http://localhost:8083` |

Example:

```bash
PORT=3000 REQUEST_TIMEOUT_MS=2000 bun run start
```

## Development

Run all gateway checks:

```bash
cd gateway
bun run check
```

Individual commands:

```bash
bun run lint
bun run format:check
bun run typecheck
bun test
```

## Important Notes

- **Do NOT modify** any code under the `services/` directory
- Gateway code is under the `gateway/` directory
- Submit your code as a Pull Request when finished
- The interviewer will observe your screen throughout — the focus is on **how you collaborate with AI**, not just the final result

---

## Project Structure

```
.
├── README.md              ← You are here
├── docker-compose.yml     ← One-click backend startup
├── services/
│   ├── user-service/      ← User service (:8081)
│   ├── order-service/     ← Order service (:8082)
│   └── product-service/   ← Product service (:8083)
└── gateway/               ← Bun + Elysia API Gateway
    ├── src/               ← Gateway source
    ├── test/              ← Bun tests
    ├── package.json       ← Scripts and dependencies
    └── README.md          ← Gateway-specific notes
```
