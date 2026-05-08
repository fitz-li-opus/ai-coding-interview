import httpx
from fastapi import HTTPException, Request, status
from fastapi.responses import Response

from circuit_breaker.breaker import CircuitBreakerError, get_breaker
from config import PROXY_TIMEOUT, ROUTE_MAP

# 共用的非同步 HTTP client（連線池複用）
_client: httpx.AsyncClient | None = None


async def get_http_client() -> httpx.AsyncClient:
    global _client
    if _client is None or _client.is_closed:
        _client = httpx.AsyncClient(timeout=PROXY_TIMEOUT)
    return _client


async def close_http_client():
    global _client
    if _client and not _client.is_closed:
        await _client.aclose()


def resolve_upstream(path: str) -> str | None:
    """根據請求路徑找到對應的後端 base URL"""
    for prefix, upstream in ROUTE_MAP.items():
        if path.startswith(prefix):
            return upstream
    return None


async def proxy_request(request: Request) -> Response:
    """
    核心反向代理邏輯：
    1. 解析路由 -> 找到後端 URL
    2. 透過 Circuit Breaker 保護
    3. 轉發請求（保留 method、headers、body、query string）
    4. 回傳後端回應
    """
    path = request.url.path
    upstream_base = resolve_upstream(path)

    if upstream_base is None:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail=f"No route found for path: {path}",
        )

    # 組合完整的後端 URL（保留 query string）
    query = request.url.query
    upstream_url = f"{upstream_base}{path}"
    if query:
        upstream_url = f"{upstream_url}?{query}"

    # 過濾不應轉發的 hop-by-hop headers
    HOP_BY_HOP = {
        "host", "connection", "keep-alive", "proxy-authenticate",
        "proxy-authorization", "te", "trailers", "transfer-encoding", "upgrade",
    }
    forward_headers = {
        k: v for k, v in request.headers.items()
        if k.lower() not in HOP_BY_HOP
    }

    # 加入 X-Forwarded-For
    client_ip = request.client.host if request.client else "unknown"
    forward_headers["X-Forwarded-For"] = client_ip
    forward_headers["X-Forwarded-Host"] = request.headers.get("host", "")

    body = await request.body()

    # 取得對應的 Circuit Breaker
    breaker = get_breaker(path)
    client = await get_http_client()

    async def do_request():
        response = await client.request(
            method=request.method,
            url=upstream_url,
            headers=forward_headers,
            content=body,
        )
        return response

    try:
        if breaker:
            upstream_response = await breaker.call_async(do_request)
        else:
            upstream_response = await do_request()

    except CircuitBreakerError:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="Service temporarily unavailable (circuit open)",
        )
    except httpx.TimeoutException:
        raise HTTPException(
            status_code=status.HTTP_504_GATEWAY_TIMEOUT,
            detail="Upstream service timed out",
        )
    except httpx.RequestError as exc:
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail=f"Error connecting to upstream: {exc}",
        )

    # 過濾回應中的 hop-by-hop headers
    response_headers = {
        k: v for k, v in upstream_response.headers.items()
        if k.lower() not in HOP_BY_HOP
    }

    return Response(
        content=upstream_response.content,
        status_code=upstream_response.status_code,
        headers=response_headers,
        media_type=upstream_response.headers.get("content-type"),
    )
