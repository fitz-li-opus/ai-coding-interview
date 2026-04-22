# API Gateway Coding Interview

## Overview

You have **40 minutes** to design and implement a lightweight API Gateway using AI tools.

You may use any AI coding tool (Claude Code / Cursor / Copilot / etc.) and any tech stack (Go / Node.js / Python / Rust / Java / etc.).

---

## Environment

This project provides **3 backend microservices** and **1 API Gateway**, ready to launch with Docker Compose:

```bash
docker compose up --build
```

Available backend services after startup:

| Service | Address | Description |
|---------|---------|-------------|
| gateway | `http://localhost:8080` | API Gateway entrypoint |
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

## Coworker Quick Start

### Prerequisites

- Docker / Docker Compose
- Go 1.22+
- Optional: `k6` for load testing

### Start Everything

Recommended:

```bash
make up
```

Equivalent Docker command:

```bash
docker compose up --build -d
```

### Stop Everything

```bash
make down
```

### View Logs

```bash
make logs
make logs-gateway
```

### Verify Gateway Connectivity

```bash
make smoke
```

This checks:

- `GET /healthz`
- `GET /readyz`
- `GET /api/users/`
- `GET /api/orders/`
- `GET /api/products/`

### Local Go Tests

```bash
make test
make test-race
```

### Hot Reload During Development

The gateway container runs with `air` and mounts the local `gateway/` directory:

- Any Go file change under `gateway/` triggers rebuild + restart automatically
- YAML config changes also trigger reload
- The container always uses the latest local source code

Follow the gateway rebuild logs with:

```bash
make logs-gateway
```

### Configuration

Gateway runtime config is managed in:

```text
gateway/config/gateway.dev.yaml
```

Current design:

- YAML is the primary config source
- Environment variables can override selected values
- Docker Compose points the gateway to `GATEWAY_CONFIG_FILE=/app/config/gateway.dev.yaml`

### Project Layout

Gateway code is now organized for maintainability and testing:

```text
gateway/
├── config/
│   └── gateway.dev.yaml
├── docs/
│   └── DEVELOPMENT.md
├── internal/
│   ├── bootstrap/   # startup and graceful shutdown
│   ├── config/      # YAML loading and env override
│   ├── di/          # dependency wiring
│   └── gateway/     # handlers, middleware, reverse proxy logic
├── tests/
│   └── k6/
└── main.go          # thin entrypoint
```

### Makefile Commands

```bash
make help        # show available commands
make up          # build and start all services
make down        # stop all services
make restart     # restart all services
make ps          # show service status
make logs        # show compose logs
make logs-gateway
make test        # run go test ./...
make test-race   # run go test -race ./...
make smoke       # curl-based connectivity checks
make k6          # run k6 script (requires local k6)
```

### k6 Load Test

`make k6` now runs `k6` inside Docker, so coworkers do not need to install `k6` locally.

```bash
make k6
```

You can also override load settings:

```bash
make k6 K6_VUS=10 K6_DURATION=15s
```

Direct Docker Compose command:

```bash
docker compose --profile tools run --rm k6
```

This script simulates repeated requests to:

- `/healthz`
- `/readyz`
- `/api/users/`
- `/api/orders/`
- `/api/products/`

It also sends a spoofed `X-Forwarded-For` header to validate the gateway's forwarded-header handling behavior.

---

## Your Task

Implement an API Gateway in the `gateway/` directory. The gateway should listen on a single port (recommended `8080`) and forward requests to the appropriate backend service based on routing rules:

- `/api/users/**` → `http://localhost:8081`
- `/api/orders/**` → `http://localhost:8082`
- `/api/products/**` → `http://localhost:8083`

This is the **minimum requirement**. Think about what a production-grade API Gateway should look like — any additional capabilities you design and implement will be considered as bonus points.

---

## Deliverables

- Working, runnable code
- Demonstrate with curl that requests are correctly routed through the gateway
- Be prepared to explain your design decisions and trade-offs

---

## Suggested Time Allocation

| Phase | Time | Activity |
|-------|------|----------|
| Understand requirements & design | ~8 min | Read this document, plan your architecture, align with the interviewer |
| Implement with AI | ~22 min | Code your solution in the `gateway/` directory |
| Demo & Q&A | ~10 min | curl demo + interviewer questions |

---

## Important Notes

- **Do NOT modify** any code under the `services/` directory
- All your code should be written in the `gateway/` directory
- Submit your code as a Pull Request when finished
- The interviewer will observe your screen throughout — the focus is on **how you collaborate with AI**, not just the final result

---

## Project Structure

```
.
├── README.md              ← You are here
├── Makefile               ← Common commands for coworkers
├── docker-compose.yml     ← One-click local environment startup
├── services/
│   ├── user-service/      ← User service (:8081)
│   ├── order-service/     ← Order service (:8082)
│   └── product-service/   ← Product service (:8083)
└── gateway/               ← API Gateway implementation
```
