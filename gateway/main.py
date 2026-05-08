from contextlib import asynccontextmanager

import httpx
from fastapi import Depends, FastAPI, HTTPException, Request, status
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from fastapi.security import OAuth2PasswordRequestForm
from slowapi import _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.middleware import SlowAPIMiddleware

from circuit_breaker.breaker import BREAKERS
from config import GATEWAY_PORT, ROUTE_MAP
from middleware.auth import authenticate_user, create_access_token, jwt_auth_middleware
from middleware.logging import logging_middleware
from middleware.rate_limit import limiter
from proxy.reverse_proxy import close_http_client, proxy_request


@asynccontextmanager
async def lifespan(app: FastAPI):
    """應用程式生命週期管理：啟動 / 關閉時的初始化與清理"""
    print("🚀 API Gateway starting on port", GATEWAY_PORT)
    yield
    await close_http_client()
    print("🛑 API Gateway shut down")


app = FastAPI(
    title="API Gateway",
    description="Production-grade API Gateway with routing, auth, rate limiting, circuit breaker",
    version="1.0.0",
    lifespan=lifespan,
)

# ── Middleware 掛載順序（由外到內執行）──────────────────────────────────────
# 1. CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# 2. Rate Limiting
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)
app.add_middleware(SlowAPIMiddleware)

# 3. Request Logging
app.middleware("http")(logging_middleware)

# 4. JWT Auth
app.middleware("http")(jwt_auth_middleware)


# ── Auth 端點 ──────────────────────────────────────────────────────────────
@app.post("/auth/token", tags=["Auth"], summary="取得 JWT Token")
async def login(form_data: OAuth2PasswordRequestForm = Depends()):
    """
    使用帳號密碼換取 JWT Token。

    測試帳號：
    - admin / admin123
    - user  / user123
    """
    user = authenticate_user(form_data.username, form_data.password)
    if not user:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Incorrect username or password",
            headers={"WWW-Authenticate": "Bearer"},
        )
    token = create_access_token(
        data={"sub": user["username"], "role": user["role"]}
    )
    return {"access_token": token, "token_type": "bearer"}


# ── Health Check 端點 ──────────────────────────────────────────────────────
@app.get("/health", tags=["Health"], summary="Gateway 及後端服務健康狀態")
async def health_check():
    """
    同時探測 Gateway 本身與三個後端服務的健康狀態。
    """
    results = {"gateway": "healthy", "services": {}}

    async with httpx.AsyncClient(timeout=3.0) as client:
        for prefix, upstream in ROUTE_MAP.items():
            service_name = prefix.removeprefix("/api/")
            try:
                resp = await client.get(f"{upstream}/health")
                results["services"][service_name] = (
                    "healthy" if resp.status_code == 200 else f"unhealthy ({resp.status_code})"
                )
            except Exception as exc:
                results["services"][service_name] = f"unreachable ({exc})"

    return results


# ── Circuit Breaker 狀態端點 ───────────────────────────────────────────────
@app.get("/admin/circuit-breakers", tags=["Admin"], summary="查看 Circuit Breaker 狀態")
async def circuit_breaker_status(request: Request):
    """查看各後端服務的 Circuit Breaker 開關狀態（需 admin 角色）"""
    if getattr(request.state, "role", None) != "admin":
        raise HTTPException(
            status_code=status.HTTP_403_FORBIDDEN,
            detail="Admin role required",
        )
    return {
        prefix: {
            "state": breaker.current_state,
            "fail_counter": breaker.fail_counter,
        }
        for prefix, breaker in BREAKERS.items()
    }


# ── 反向代理：捕捉所有 /api/** 請求 ──────────────────────────────────────
@app.api_route(
    "/api/{full_path:path}",
    methods=["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"],
    tags=["Proxy"],
    summary="反向代理至後端微服務",
)
@limiter.limit("60/minute")
async def gateway_proxy(request: Request, full_path: str):
    """
    路由規則：
    - /api/users/**    → user-service    (http://localhost:8081)
    - /api/orders/**   → order-service   (http://localhost:8082)
    - /api/products/** → product-service (http://localhost:8083)
    """
    return await proxy_request(request)


# ── 根路徑 ────────────────────────────────────────────────────────────────
@app.get("/", tags=["Info"])
async def root():
    return {
        "service": "api-gateway",
        "version": "1.0.0",
        "routes": list(ROUTE_MAP.keys()),
        "docs": "/docs",
    }


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("main:app", host="0.0.0.0", port=GATEWAY_PORT, reload=True)
