import logging
import uuid
from typing import Callable

import httpx
import pytest
from fastapi.testclient import TestClient

from main import Settings, create_app


TEST_SETTINGS = Settings(
    host="127.0.0.1",
    port=8080,
    upstream_timeout_seconds=1.0,
    routes={
        "users": "http://users.test",
        "orders": "http://orders.test",
        "products": "http://products.test",
    },
)


def make_test_client(
    handler: Callable[[httpx.Request], httpx.Response]
) -> TestClient:
    transport = httpx.MockTransport(handler)
    return TestClient(create_app(TEST_SETTINGS, transport=transport))


@pytest.mark.parametrize(
    ("gateway_path", "expected_host", "expected_path"),
    [
        ("/api/users", "users.test", "/"),
        ("/api/orders/1001", "orders.test", "/1001"),
        ("/api/products/catalog/items", "products.test", "/catalog/items"),
    ],
)
def test_routes_to_expected_upstream_and_rewrites_path(
    gateway_path: str,
    expected_host: str,
    expected_path: str,
) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.host == expected_host
        assert request.url.path == expected_path
        return httpx.Response(200, json={"service": expected_host})

    with make_test_client(handler) as client:
        response = client.get(gateway_path)

    assert response.status_code == 200
    assert response.json() == {"service": expected_host}


def test_preserves_query_method_body_headers_and_request_id() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.method == "POST"
        assert request.url.path == "/42"
        assert request.url.query == b"tag=a&tag=b%20c"
        assert request.content == b'{"example":true}'
        assert request.headers["authorization"] == "Bearer test-token"
        assert request.headers["content-type"] == "application/json"
        assert request.headers["x-request-id"] == "request-123"
        return httpx.Response(
            201,
            content=b"created",
            headers={"X-Upstream": "product-service"},
        )

    with make_test_client(handler) as client:
        response = client.post(
            "/api/products/42?tag=a&tag=b%20c",
            content=b'{"example":true}',
            headers={
                "Authorization": "Bearer test-token",
                "Content-Type": "application/json",
                "X-Request-ID": "request-123",
            },
        )

    assert response.status_code == 201
    assert response.content == b"created"
    assert response.headers["x-upstream"] == "product-service"
    assert response.headers["x-request-id"] == "request-123"


def test_generated_request_id_reaches_upstream_and_response() -> None:
    observed_request_ids = []

    def handler(request: httpx.Request) -> httpx.Response:
        observed_request_ids.append(request.headers["x-request-id"])
        return httpx.Response(200, json={"ok": True})

    with make_test_client(handler) as client:
        response = client.get("/api/users")

    assert len(observed_request_ids) == 1
    uuid.UUID(observed_request_ids[0])
    assert response.headers["x-request-id"] == observed_request_ids[0]


@pytest.mark.parametrize("path", ["/api/not-configured", "/not-configured"])
def test_generates_request_id_and_includes_it_in_errors(path: str) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError("unknown routes must not reach an upstream")

    with make_test_client(handler) as client:
        response = client.get(path)

    request_id = response.headers["x-request-id"]
    uuid.UUID(request_id)
    assert response.status_code == 404
    assert response.json() == {
        "error": "route_not_found",
        "message": "No route configured for this path",
        "request_id": request_id,
    }


def test_connection_failure_returns_502() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("connection refused", request=request)

    with make_test_client(handler) as client:
        response = client.get("/api/users", headers={"X-Request-ID": "req-502"})

    assert response.status_code == 502
    assert response.json()["error"] == "bad_gateway"
    assert response.json()["request_id"] == "req-502"


def test_timeout_returns_504() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("read timed out", request=request)

    with make_test_client(handler) as client:
        response = client.get("/api/orders", headers={"X-Request-ID": "req-504"})

    assert response.status_code == 504
    assert response.json()["error"] == "gateway_timeout"
    assert response.json()["request_id"] == "req-504"


def test_health_is_independent_of_upstreams() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise AssertionError("health must not call an upstream")

    with make_test_client(handler) as client:
        response = client.get("/health")

    assert response.status_code == 200
    assert response.json() == {"status": "healthy", "service": "api-gateway"}
    uuid.UUID(response.headers["x-request-id"])


def test_unexpected_upstream_error_returns_json_500() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise RuntimeError("unexpected failure")

    with make_test_client(handler) as client:
        response = client.get("/api/users", headers={"X-Request-ID": "req-500"})

    assert response.status_code == 500
    assert response.json() == {
        "error": "internal_server_error",
        "message": "An unexpected error occurred",
        "request_id": "req-500",
    }


def test_access_log_contains_required_metadata(caplog: pytest.LogCaptureFixture) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"ok": True})

    with caplog.at_level(logging.INFO, logger="gateway"):
        with make_test_client(handler) as client:
            response = client.get(
                "/api/users",
                headers={"X-Request-ID": "request-for-log"},
            )

    assert response.status_code == 200
    access_log = next(
        record.getMessage()
        for record in caplog.records
        if "request_id=request-for-log" in record.getMessage()
    )
    assert "method=GET" in access_log
    assert "path=/api/users" in access_log
    assert "status=200" in access_log
    assert "duration_ms=" in access_log
    assert "target=users" in access_log
    assert "Authorization" not in access_log
    assert "Cookie" not in access_log


def test_http_client_closes_after_application_shutdown() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200)

    app = create_app(TEST_SETTINGS, transport=httpx.MockTransport(handler))
    with TestClient(app):
        shared_client = app.state.http_client
        assert not shared_client.is_closed

    assert shared_client.is_closed
