"""API Gateway entrypoint: `uvicorn app.main:app --port 8080`."""

from __future__ import annotations

import logging
from contextlib import asynccontextmanager

import httpx
from fastapi import FastAPI, Request

from .config import Config, load_config
from .middleware import access_log_middleware
from .proxy import error_response, forward
from .ratelimit import TokenBucketLimiter
from .router import Router

logging.basicConfig(level=logging.INFO, format="%(message)s")

PROXY_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"]


def create_app(
    config: Config | None = None,
    transport: httpx.AsyncBaseTransport | None = None,
) -> FastAPI:
    config = config or load_config()
    router = Router(config.routes)
    limiters = {
        r.name: TokenBucketLimiter(r.rate_limit.rate, r.rate_limit.burst)
        for r in config.routes
        if r.rate_limit
    }

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        t = config.timeouts
        # One shared client => connection pooling / keep-alive to upstreams.
        app.state.client = httpx.AsyncClient(
            timeout=httpx.Timeout(connect=t.connect, read=t.read, write=t.write, pool=t.pool),
            limits=httpx.Limits(max_connections=200, max_keepalive_connections=50),
            follow_redirects=False,  # pass redirects through to the caller
            transport=transport,
        )
        try:
            yield
        finally:
            await app.state.client.aclose()

    app = FastAPI(title="API Gateway", lifespan=lifespan, docs_url=None, redoc_url=None)
    app.middleware("http")(access_log_middleware)

    @app.get("/healthz")
    async def healthz():
        return {"status": "ok"}

    @app.api_route("/{path:path}", methods=PROXY_METHODS)
    async def proxy(request: Request, path: str):
        request_id = request.state.request_id
        # Match on the raw (still percent-encoded) path so it is forwarded unchanged.
        raw_path = request.scope.get("raw_path") or request.url.path.encode()
        matched = router.match(raw_path.decode("latin-1"))
        if matched is None:
            return error_response(404, "no route matched", request_id)
        route, upstream_path = matched
        request.state.route = route.name

        rl_headers: dict[str, str] = {}
        limiter = limiters.get(route.name)
        if limiter is not None:
            # Key on the TCP peer, not X-Forwarded-For: XFF is client-controlled
            # unless we sit behind a trusted proxy.
            client_ip = request.client.host if request.client else "unknown"
            decision = limiter.allow(client_ip)
            rl_headers = {
                "RateLimit-Limit": str(decision.limit),
                "RateLimit-Remaining": str(decision.remaining),
            }
            if not decision.allowed:
                return error_response(
                    429,
                    "rate limit exceeded",
                    request_id,
                    headers={**rl_headers, "Retry-After": str(decision.retry_after)},
                )

        response = await forward(
            app.state.client, request, route, upstream_path.encode("latin-1"), request_id
        )
        response.headers.update(rl_headers)
        return response

    return app


app = create_app()
