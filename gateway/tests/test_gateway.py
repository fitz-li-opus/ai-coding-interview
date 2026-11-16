import json

import httpx
import pytest
from fastapi.testclient import TestClient

from app.config import Config, Route, load_config
from app.main import create_app
from app.router import Router

ROUTES = [
    Route("users", "/api/users", "http://users.local"),
    Route("orders", "/api/orders", "http://orders.local"),
    Route("products", "/api/products", "http://products.local"),
]


# ---------- router ----------

@pytest.mark.parametrize(
    "path, route, upstream_path",
    [
        ("/api/users", "users", "/"),
        ("/api/users/", "users", "/"),
        ("/api/users/1", "users", "/1"),
        ("/api/orders/1001/items", "orders", "/1001/items"),
        ("/api/products", "products", "/"),
    ],
)
def test_router_matches_and_strips(path, route, upstream_path):
    r, p = Router(ROUTES).match(path)
    assert (r.name, p) == (route, upstream_path)


@pytest.mark.parametrize("path", ["/api/usersX", "/api", "/", "/users", "/api/user"])
def test_router_rejects_non_boundary(path):
    assert Router(ROUTES).match(path) is None


def test_router_longest_prefix_wins():
    routes = ROUTES + [Route("admin", "/api/users/admin", "http://admin.local")]
    r, p = Router(routes).match("/api/users/admin/x")
    assert (r.name, p) == ("admin", "/x")


def test_router_no_strip():
    r, p = Router([Route("u", "/api/users", "http://u", strip_prefix=False)]).match("/api/users/1")
    assert p == "/api/users/1"


def test_load_config_env_override(monkeypatch):
    monkeypatch.setenv("UPSTREAM_USERS_URL", "http://user-service:8080/")
    cfg = load_config()
    users = next(r for r in cfg.routes if r.name == "users")
    assert users.upstream == "http://user-service:8080"
    assert cfg.timeouts.connect == 2.0


# ---------- proxy (end to end through the ASGI app) ----------

class _AsyncBody(httpx.AsyncByteStream):
    def __init__(self, data: bytes):
        self._data = data

    async def __aiter__(self):
        yield self._data


def make_client(handler) -> TestClient:
    # httpx.Response(content=...) is pre-read; real transports return an unread
    # stream. Re-wrap so the gateway's streaming path is exercised realistically.
    def streaming_handler(request: httpx.Request) -> httpx.Response:
        resp = handler(request)
        return httpx.Response(
            resp.status_code, headers=resp.headers.raw, stream=_AsyncBody(resp.content)
        )

    app = create_app(Config(routes=ROUTES), transport=httpx.MockTransport(streaming_handler))
    return TestClient(app)


def echo(request: httpx.Request) -> httpx.Response:
    return httpx.Response(
        200,
        json={
            "host": request.url.host,
            "raw_path": request.url.raw_path.decode(),
            "method": request.method,
            "headers": dict(request.headers),
            "body": request.content.decode(),
        },
        headers=[
            ("Set-Cookie", "a=1"),
            ("Set-Cookie", "b=2"),
            ("Connection", "close"),
            ("Date", "Mon, 01 Jan 2024 00:00:00 GMT"),
        ],
    )


def test_routes_to_correct_upstream_with_query():
    with make_client(echo) as c:
        r = c.get("/api/orders/1001?x=1&y=%2F")
    body = r.json()
    assert r.status_code == 200
    assert body["host"] == "orders.local"
    assert body["raw_path"] == "/1001?x=1&y=%2F"


def test_forwards_body_and_method():
    with make_client(echo) as c:
        r = c.post("/api/users", json={"name": "Dan"})
    body = r.json()
    assert body["method"] == "POST"
    assert json.loads(body["body"]) == {"name": "Dan"}


def test_get_without_body_is_not_chunked():
    with make_client(echo) as c:
        h = c.get("/api/users").json()["headers"]
    assert "transfer-encoding" not in h


def test_headers_forwarded_and_hop_by_hop_stripped():
    with make_client(echo) as c:
        r = c.get(
            "/api/users",
            headers={
                "X-Request-ID": "abc-123",
                "X-Forwarded-For": "1.2.3.4",
                "Connection": "keep-alive, X-Secret",
                "X-Secret": "s",
                "Authorization": "Bearer t",
            },
        )
    h = r.json()["headers"]
    assert h["x-request-id"] == "abc-123"
    assert h["x-forwarded-for"].startswith("1.2.3.4, ")
    assert h["host"] == "users.local"
    assert h["authorization"] == "Bearer t"
    assert "x-secret" not in h
    # response side
    assert r.headers["x-request-id"] == "abc-123"
    assert r.headers.get_list("set-cookie") == ["a=1", "b=2"]
    assert r.headers.get("connection") != "close"
    assert "2024" not in r.headers.get("date", "")  # upstream Date dropped


def test_generates_request_id_when_missing_or_invalid():
    with make_client(echo) as c:
        r1 = c.get("/api/users")
        r2 = c.get("/api/users", headers={"X-Request-ID": "bad id\r\nx"})
    assert len(r1.headers["x-request-id"]) == 32
    assert r2.headers["x-request-id"] != "bad id\r\nx"


def test_unmatched_route_404():
    with make_client(echo) as c:
        r = c.get("/api/usersX")
    assert r.status_code == 404
    assert r.json()["error"] == "no route matched"
    assert r.json()["request_id"] == r.headers["x-request-id"]


def test_upstream_connect_error_502():
    def boom(request):
        raise httpx.ConnectError("refused", request=request)

    with make_client(boom) as c:
        r = c.get("/api/users")
    assert r.status_code == 502
    assert r.json()["error"] == "upstream unavailable"


def test_upstream_timeout_504():
    def slow(request):
        raise httpx.ReadTimeout("slow", request=request)

    with make_client(slow) as c:
        r = c.get("/api/users")
    assert r.status_code == 504
    assert r.json()["error"] == "upstream timeout"


def test_upstream_status_passthrough():
    with make_client(lambda req: httpx.Response(418, text="teapot")) as c:
        r = c.get("/api/products/x")
    assert (r.status_code, r.text) == (418, "teapot")


def test_healthz():
    with make_client(echo) as c:
        assert c.get("/healthz").json() == {"status": "ok"}
