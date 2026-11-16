"""Reverse-proxy a Starlette request to an upstream with httpx (streaming both ways)."""

from __future__ import annotations

import logging

import httpx
from starlette.background import BackgroundTask
from starlette.requests import Request
from starlette.responses import JSONResponse, Response, StreamingResponse

from .config import Route

logger = logging.getLogger("gateway")

# RFC 7230 §6.1 hop-by-hop headers: meaningful for one connection only, never forwarded.
HOP_BY_HOP = {
    b"connection",
    b"keep-alive",
    b"proxy-authenticate",
    b"proxy-authorization",
    b"te",
    b"trailer",
    b"transfer-encoding",
    b"upgrade",
}


def _strip_hop_by_hop(raw: list[tuple[bytes, bytes]]) -> list[tuple[bytes, bytes]]:
    drop = set(HOP_BY_HOP)
    # Headers named in `Connection:` are also hop-by-hop.
    for k, v in raw:
        if k.lower() == b"connection":
            drop.update(t.strip().lower() for t in v.split(b",") if t.strip())
    return [(k, v) for k, v in raw if k.lower() not in drop]


def error_response(
    status: int, message: str, request_id: str, headers: dict[str, str] | None = None
) -> JSONResponse:
    return JSONResponse(
        {"error": message, "request_id": request_id},
        status_code=status,
        headers={"X-Request-ID": request_id, **(headers or {})},
    )


def _build_upstream_headers(request: Request, request_id: str) -> list[tuple[bytes, bytes]]:
    headers = [
        (k, v)
        for k, v in _strip_hop_by_hop(request.headers.raw)
        # Host is set by httpx from the upstream URL; X-Request-ID is re-added below.
        if k.lower() not in (b"host", b"x-request-id")
    ]
    client_ip = request.client.host if request.client else "unknown"
    prior_xff = request.headers.get("x-forwarded-for")
    xff = f"{prior_xff}, {client_ip}" if prior_xff else client_ip
    headers += [
        (b"x-forwarded-for", xff.encode("latin-1")),
        (b"x-forwarded-proto", request.url.scheme.encode("latin-1")),
        (b"x-forwarded-host", request.headers.get("host", "").encode("latin-1")),
        (b"x-request-id", request_id.encode("latin-1")),
    ]
    return headers


def _has_body(request: Request) -> bool:
    # Only stream a body when the client actually sent one; otherwise httpx would
    # send `Transfer-Encoding: chunked` on e.g. GET requests.
    return "content-length" in request.headers or "transfer-encoding" in request.headers


async def forward(
    client: httpx.AsyncClient,
    request: Request,
    route: Route,
    upstream_path: bytes,
    request_id: str,
) -> Response:
    query = request.scope.get("query_string", b"")
    raw_path = upstream_path + (b"?" + query if query else b"")
    url = httpx.URL(route.upstream).copy_with(raw_path=raw_path)

    upstream_req = client.build_request(
        request.method,
        url,
        headers=_build_upstream_headers(request, request_id),
        content=request.stream() if _has_body(request) else None,
    )

    try:
        upstream_resp = await client.send(upstream_req, stream=True)
    except httpx.TimeoutException as exc:
        logger.warning("upstream timeout route=%s url=%s err=%r", route.name, url, exc)
        return error_response(504, "upstream timeout", request_id)
    except httpx.TransportError as exc:
        logger.warning("upstream unavailable route=%s url=%s err=%r", route.name, url, exc)
        return error_response(502, "upstream unavailable", request_id)

    response = StreamingResponse(
        upstream_resp.aiter_raw(),  # raw bytes: keep Content-Encoding/Length consistent
        status_code=upstream_resp.status_code,
        background=BackgroundTask(upstream_resp.aclose),
    )
    # Keep raw header list so repeated headers (e.g. Set-Cookie) survive.
    # Drop upstream Date: uvicorn adds its own and Date must appear only once.
    response.raw_headers = [
        (k, v)
        for k, v in _strip_hop_by_hop(upstream_resp.headers.raw)
        if k.lower() not in (b"x-request-id", b"date")
    ] + [(b"x-request-id", request_id.encode("latin-1"))]
    return response
