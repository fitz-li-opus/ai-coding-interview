# API Gateway Design Spec

## Overview

Implement a lightweight API gateway in Go that routes incoming requests to three backend microservices based on URL path prefixes.

## Requirements

- Gateway listens on port 8080
- Route requests based on path prefix:
  - `/api/users/**` → `http://localhost:8081`
  - `/api/orders/**` → `http://localhost:8082`
  - `/api/products/**` → `http://localhost:8083`
- Return 404 for unmatched routes

## Technical Approach

**Language**: Go (standard library only, no external dependencies)

**Core Component**: `net/http/httputil.ReverseProxy`

## Architecture

```
Client Request → Gateway (:8080) → Route Matching → Reverse Proxy → Backend Service
```

## Route Table

| Path Prefix      | Target Service           |
|------------------|--------------------------|
| `/api/users`     | `http://localhost:8081`  |
| `/api/orders`    | `http://localhost:8082`  |
| `/api/products`  | `http://localhost:8083`  |

## Request Flow

1. Client sends request to gateway (e.g., `GET /api/users/123`)
2. Gateway matches path prefix (`/api/users`) to target (`http://localhost:8081`)
3. Gateway rewrites path: `/api/users/123` → `/123`
4. Gateway forwards request via ReverseProxy
5. Backend response returned to client

## File Structure

```
gateway/
└── main.go    # Single file implementation (~50 lines)
```

## Implementation Details

### Route Matching

- Iterate through route table
- Match longest prefix first (if needed)
- Use `strings.HasPrefix()` for matching

### Path Rewriting

- Strip the matched prefix from the original path
- Example: `/api/users/profile` → `/profile`

### Error Handling

- Unmatched routes: return HTTP 404
- Backend errors: propagate as-is (ReverseProxy default behavior)

## Testing

Verify with curl commands:

```bash
# Start backend services
docker compose up --build

# Start gateway
go run gateway/main.go

# Test routing
curl http://localhost:8080/api/users
curl http://localhost:8080/api/orders
curl http://localhost:8080/api/products

# Test 404
curl http://localhost:8080/api/unknown
```

## Out of Scope

- Rate limiting
- Authentication
- Caching
- Load balancing
- Health checks
