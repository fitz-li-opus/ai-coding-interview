import httpx
import pytest
from fastapi.testclient import TestClient

from app.config import Config, RateLimit, Route, load_config
from app.main import create_app
from app.ratelimit import TokenBucketLimiter


class FakeClock:
    def __init__(self):
        self.t = 1000.0

    def __call__(self):
        return self.t


# ---------- bucket algorithm ----------

def test_burst_then_reject():
    lim = TokenBucketLimiter(rate=1, burst=3, clock=FakeClock())
    results = [lim.allow("ip").allowed for _ in range(4)]
    assert results == [True, True, True, False]


def test_remaining_counts_down():
    lim = TokenBucketLimiter(rate=1, burst=3, clock=FakeClock())
    assert [lim.allow("ip").remaining for _ in range(3)] == [2, 1, 0]


def test_refill_over_time():
    clock = FakeClock()
    lim = TokenBucketLimiter(rate=2, burst=2, clock=clock)
    assert lim.allow("ip").allowed and lim.allow("ip").allowed
    assert not lim.allow("ip").allowed
    clock.t += 0.5  # 0.5s * 2 tokens/s = 1 token
    assert lim.allow("ip").allowed
    assert not lim.allow("ip").allowed


def test_refill_capped_at_burst():
    clock = FakeClock()
    lim = TokenBucketLimiter(rate=10, burst=2, clock=clock)
    lim.allow("ip")
    clock.t += 3600  # long idle must not accumulate more than burst
    assert [lim.allow("ip").allowed for _ in range(3)] == [True, True, False]


def test_retry_after():
    clock = FakeClock()
    lim = TokenBucketLimiter(rate=0.5, burst=1, clock=clock)  # 1 token every 2s
    lim.allow("ip")
    d = lim.allow("ip")
    assert (d.allowed, d.retry_after) == (False, 2)
    clock.t += 1.5
    assert lim.allow("ip").retry_after == 1  # ceil(0.25 / 0.5)


def test_keys_are_independent():
    lim = TokenBucketLimiter(rate=1, burst=1, clock=FakeClock())
    assert lim.allow("a").allowed
    assert not lim.allow("a").allowed
    assert lim.allow("b").allowed


def test_lru_eviction_bounds_memory():
    lim = TokenBucketLimiter(rate=1, burst=1, max_keys=2, clock=FakeClock())
    for k in ["a", "b", "c"]:
        lim.allow(k)
    assert list(lim._buckets) == ["b", "c"]
    assert lim.allow("a").allowed  # evicted => fresh (lenient) bucket


@pytest.mark.parametrize("rate, burst", [(0, 1), (-1, 1), (1, 0)])
def test_invalid_params(rate, burst):
    with pytest.raises(ValueError):
        TokenBucketLimiter(rate=rate, burst=burst)
    with pytest.raises(ValueError):
        RateLimit(rate=rate, burst=burst)


def test_config_loads_rate_limit():
    users = next(r for r in load_config().routes if r.name == "users")
    assert users.rate_limit == RateLimit(rate=5.0, burst=10)


# ---------- through the gateway ----------

class _AsyncBody(httpx.AsyncByteStream):
    async def __aiter__(self):
        yield b"ok"


def make_client(routes) -> TestClient:
    transport = httpx.MockTransport(lambda req: httpx.Response(200, stream=_AsyncBody()))
    return TestClient(create_app(Config(routes=routes), transport=transport))


def test_gateway_returns_429_with_headers():
    routes = [Route("users", "/api/users", "http://u", rate_limit=RateLimit(rate=0.01, burst=2))]
    with make_client(routes) as c:
        r1, r2, r3 = (c.get("/api/users") for _ in range(3))
    assert (r1.status_code, r2.status_code, r3.status_code) == (200, 200, 429)
    assert r1.headers["ratelimit-limit"] == "2"
    assert (r1.headers["ratelimit-remaining"], r2.headers["ratelimit-remaining"]) == ("1", "0")
    assert r3.json()["error"] == "rate limit exceeded"
    assert r3.headers["retry-after"] == "100"
    assert r3.headers["x-request-id"] == r3.json()["request_id"]


def test_limits_are_per_route():
    rl = RateLimit(rate=0.01, burst=1)
    routes = [
        Route("users", "/api/users", "http://u", rate_limit=rl),
        Route("orders", "/api/orders", "http://o", rate_limit=rl),
    ]
    with make_client(routes) as c:
        assert c.get("/api/users").status_code == 200
        assert c.get("/api/users").status_code == 429
        assert c.get("/api/orders").status_code == 200


def test_unlimited_route_and_healthz_not_limited():
    routes = [Route("users", "/api/users", "http://u")]
    with make_client(routes) as c:
        assert all(c.get("/api/users").status_code == 200 for _ in range(20))
        assert "ratelimit-limit" not in c.get("/api/users").headers
        assert all(c.get("/healthz").status_code == 200 for _ in range(20))
