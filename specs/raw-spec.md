# 目標

實作 api-gateway

Implement an API Gateway in the `gateway/` directory. The gateway should listen on a single port (recommended `8080`) and forward requests to the appropriate backend service based on routing rules:

- `/api/users/**` → `http://localhost:8081`
- `/api/orders/**` → `http://localhost:8082`
- `/api/products/**` → `http://localhost:8083`


# 三個 services

| Service | Address | Description |
|---------|---------|-------------|
| user-service | `http://localhost:8081` | Returns a list of users |
| order-service | `http://localhost:8082` | Returns a list of orders |
| product-service | `http://localhost:8083` | Returns a list of products |

Each service also exposes a `/health` endpoint for health checks.


# Project Structure

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

更動只能發生在 gateway 下

# 語言選擇

Golang


