"""Lightweight API Gateway for the interview backend services."""

import logging
import os
import time
import uuid
from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import Dict, Iterable, List, Optional, Tuple

import httpx
import uvicorn
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse, Response
from starlette.exceptions import HTTPException as StarletteHTTPException


logger = logging.getLogger("gateway")
logger.setLevel(logging.INFO)

SUPPORTED_METHODS = [
    "GET",
    "POST",
    "PUT",
    "PATCH",
    "DELETE",
    "OPTIONS",
    "HEAD",
]

# RFC 9110 section 7.6.1: these headers describe a single transport hop and
# must not be forwarded by a proxy.
HOP_BY_HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
}


@dataclass(frozen=True)
class Settings:
    """Runtime configuration loaded from environment variables."""

    host: str
    port: int
    upstream_timeout_seconds: float
    routes: Dict[str, str]

    @classmethod
    def from_environment(cls) -> "Settings":
        return cls(
            host=os.getenv("GATEWAY_HOST", "0.0.0.0"),
            port=_positive_int_from_env("GATEWAY_PORT", 8080),
            upstream_timeout_seconds=_positive_float_from_env(
                "UPSTREAM_TIMEOUT_SECONDS", 5.0
            ),
            routes={
                "users": os.getenv(
                    "USER_SERVICE_URL", "http://localhost:8081"
                ).rstrip("/"),
                "orders": os.getenv(
                    "ORDER_SERVICE_URL", "http://localhost:8082"
                ).rstrip("/"),
                "products": os.getenv(
                    "PRODUCT_SERVICE_URL", "http://localhost:8083"
                ).rstrip("/"),
            },
        )


def _positive_int_from_env(name: str, default: int) -> int:
    raw_value = os.getenv(name, str(default))
    try:
        value = int(raw_value)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if value <= 0:
        raise ValueError(f"{name} must be greater than zero")
    return value


def _positive_float_from_env(name: str, default: float) -> float:
    raw_value = os.getenv(name, str(default))
    try:
        value = float(raw_value)
    except ValueError as exc:
        raise ValueError(f"{name} must be a number") from exc
    if value <= 0:
        raise ValueError(f"{name} must be greater than zero")
    return value


def _connection_header_names(headers: Iterable[Tuple[bytes, bytes]]) -> set:
    """Return additional hop-by-hop header names listed by Connection."""

    names = set()
    for raw_name, raw_value in headers:
        if raw_name.decode("latin-1").lower() != "connection":
            continue
        names.update(
            token.strip().lower()
            for token in raw_value.decode("latin-1").split(",")
            if token.strip()
        )
    return names


def _request_headers(request: Request) -> List[Tuple[str, str]]:
    raw_headers = request.headers.raw
    excluded = HOP_BY_HOP_HEADERS | _connection_header_names(raw_headers)
    # HTTPX constructs the correct Host and Content-Length values for the
    # selected upstream and forwarded body. X-Request-ID is added from request
    # state so generated IDs and client-supplied IDs follow the same path.
    excluded.update({"host", "content-length", "x-request-id"})

    headers = [
        (name.decode("latin-1"), value.decode("latin-1"))
        for name, value in raw_headers
        if name.decode("latin-1").lower() not in excluded
    ]
    headers.append(("X-Request-ID", request.state.request_id))
    return headers


def _response_headers(response: httpx.Response) -> List[Tuple[bytes, bytes]]:
    connection_names = {
        token.strip().lower()
        for token in response.headers.get("connection", "").split(",")
        if token.strip()
    }
    excluded = HOP_BY_HOP_HEADERS | connection_names

    return [
        (name.encode("latin-1"), value.encode("latin-1"))
        for name, value in response.headers.multi_items()
        if name.lower() not in excluded
    ]


def _error(
    status_code: int,
    error: str,
    message: str,
    request_id: Optional[str] = None,
) -> JSONResponse:
    content = {"error": error, "message": message}
    if request_id is not None:
        content["request_id"] = request_id
    return JSONResponse(
        status_code=status_code,
        content=content,
    )


def create_app(
    settings: Optional[Settings] = None,
    transport: Optional[httpx.AsyncBaseTransport] = None,
) -> FastAPI:
    settings = settings or Settings.from_environment()

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        app.state.http_client = httpx.AsyncClient(
            timeout=httpx.Timeout(settings.upstream_timeout_seconds),
            limits=httpx.Limits(
                max_connections=100,
                max_keepalive_connections=20,
            ),
            follow_redirects=False,
            transport=transport,
        )
        try:
            yield
        finally:
            await app.state.http_client.aclose()

    app = FastAPI(
        title="API Gateway",
        docs_url=None,
        redoc_url=None,
        openapi_url=None,
        lifespan=lifespan,
    )
    app.state.settings = settings

    @app.middleware("http")
    async def request_context(request: Request, call_next):
        started_at = time.perf_counter()
        request_id = request.headers.get("X-Request-ID", "").strip()
        if not request_id:
            request_id = str(uuid.uuid4())

        request.state.request_id = request_id
        request.state.target_service = "unmatched"

        try:
            response = await call_next(request)
        except Exception as exc:
            logger.error(
                "Unhandled gateway error",
                exc_info=(type(exc), exc, exc.__traceback__),
            )
            response = _error(
                500,
                "internal_server_error",
                "An unexpected error occurred",
                request_id,
            )

        response.headers["X-Request-ID"] = request_id
        duration_ms = (time.perf_counter() - started_at) * 1000
        logger.info(
            "request_id=%s method=%s path=%s status=%s duration_ms=%.2f target=%s",
            request_id,
            request.method,
            request.url.path,
            response.status_code,
            duration_ms,
            request.state.target_service,
        )
        return response

    @app.exception_handler(StarletteHTTPException)
    async def http_exception_handler(
        request: Request, exc: StarletteHTTPException
    ) -> JSONResponse:
        if exc.status_code == 404:
            return _error(
                404,
                "route_not_found",
                "No route configured for this path",
                request.state.request_id,
            )
        return _error(
            exc.status_code,
            "http_error",
            str(exc.detail),
            request.state.request_id,
        )

    @app.exception_handler(Exception)
    async def unexpected_exception_handler(
        request: Request, exc: Exception
    ) -> JSONResponse:
        logger.error(
            "Unhandled gateway error",
            exc_info=(type(exc), exc, exc.__traceback__),
        )
        return _error(
            500,
            "internal_server_error",
            "An unexpected error occurred",
            request.state.request_id,
        )

    @app.get("/health", include_in_schema=False)
    async def health(request: Request) -> Dict[str, str]:
        request.state.target_service = "gateway"
        return {"status": "healthy", "service": "api-gateway"}

    async def forward_request(
        request: Request, service: str, path: str = ""
    ) -> Response:
        upstream_base_url = settings.routes.get(service)
        if upstream_base_url is None:
            return _error(
                404,
                "route_not_found",
                "No route configured for this path",
                request.state.request_id,
            )

        request.state.target_service = service

        rewritten_path = f"/{path}" if path else "/"
        upstream_url = httpx.URL(
            f"{upstream_base_url}{rewritten_path}"
        ).copy_with(query=request.scope.get("query_string", b""))

        try:
            upstream_request = request.app.state.http_client.build_request(
                method=request.method,
                url=upstream_url,
                headers=_request_headers(request),
                content=await request.body(),
            )
            upstream_response = await request.app.state.http_client.send(
                upstream_request,
                stream=True,
            )
            try:
                # Buffering is intentional for the P0 implementation. Streaming
                # large responses remains an explicitly documented P2 feature.
                if upstream_response.is_stream_consumed:
                    response_body = upstream_response.content
                else:
                    response_body = b"".join(
                        [chunk async for chunk in upstream_response.aiter_raw()]
                    )
            finally:
                await upstream_response.aclose()
        except httpx.TimeoutException:
            return _error(
                504,
                "gateway_timeout",
                "Upstream service timed out",
                request.state.request_id,
            )
        except httpx.RequestError:
            return _error(
                502,
                "bad_gateway",
                "Upstream service is unavailable",
                request.state.request_id,
            )

        downstream_response = Response(
            content=response_body,
            status_code=upstream_response.status_code,
        )
        safe_upstream_headers = _response_headers(upstream_response)
        if any(
            name.lower() == b"content-length"
            for name, _ in safe_upstream_headers
        ):
            # HEAD responses can legitimately describe the size of the
            # corresponding GET body even though their own body is empty.
            downstream_response.raw_headers = [
                (name, value)
                for name, value in downstream_response.raw_headers
                if name.lower() != b"content-length"
            ]
        # Append all safe upstream headers, preserving repeated headers.
        downstream_response.raw_headers.extend(safe_upstream_headers)
        return downstream_response

    app.add_api_route(
        "/api/{service}",
        forward_request,
        methods=SUPPORTED_METHODS,
    )
    app.add_api_route(
        "/api/{service}/{path:path}",
        forward_request,
        methods=SUPPORTED_METHODS,
    )

    return app


app = create_app()


if __name__ == "__main__":
    runtime_settings = app.state.settings
    uvicorn.run(
        app,
        host=runtime_settings.host,
        port=runtime_settings.port,
    )
