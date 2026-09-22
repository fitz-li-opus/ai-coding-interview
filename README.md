# API Gateway Coding Interview

## Overview

You have **40 minutes** to design and implement a lightweight API Gateway using AI tools.

You may use any AI coding tool (Claude Code / Cursor / Copilot / etc.) and any tech stack (Go / Node.js / Python / Rust / Java / etc.).

---

## Environment

This project provides **3 backend microservices**, ready to launch with Docker Compose:

```bash
./gateway/scripts/generate-dev-cert.sh
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
├── docker-compose.yml     ← One-click backend startup
├── services/
│   ├── user-service/      ← User service (:8081)
│   ├── order-service/     ← Order service (:8082)
│   └── product-service/   ← Product service (:8083)
└── gateway/               ← Your working directory (currently empty)
```
