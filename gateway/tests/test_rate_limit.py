"""
Rate Limit 測試：驗證超過 60/minute 限制後會回傳 HTTP 429
"""

import asyncio
import httpx
import pytest


GATEWAY_URL = "http://localhost:8080"
# 使用不需要 JWT 的端點（/health 不在 jwt_auth_middleware 保護範圍內）
# 但 rate limit 只掛在 /api/** 路由上，所以用 /api/products/
TARGET_URL = f"{GATEWAY_URL}/api/products/"


@pytest.mark.asyncio
async def test_rate_limit_returns_429():
    """
    連續發送 70 個請求，預期第 61 個（含）之後收到 429 Too Many Requests。
    """
    total_requests = 70
    status_codes = []

    # 先取得 JWT token（/api/** 需要認證）
    async with httpx.AsyncClient() as client:
        token_resp = await client.post(
            f"{GATEWAY_URL}/auth/token",
            data={"username": "user", "password": "user123"},
        )
        assert token_resp.status_code == 200, f"取得 token 失敗：{token_resp.text}"
        token = token_resp.json()["access_token"]
        headers = {"Authorization": f"Bearer {token}"}

        for i in range(total_requests):
            resp = await client.get(TARGET_URL, headers=headers)
            status_codes.append(resp.status_code)
            print(f"  Request {i+1:>3}: {resp.status_code}")

    success_count = status_codes.count(200)
    rate_limited_count = status_codes.count(429)

    print(f"\n結果統計：")
    print(f"  200 OK           : {success_count}")
    print(f"  429 Too Many Req : {rate_limited_count}")
    print(f"  其他             : {total_requests - success_count - rate_limited_count}")

    assert rate_limited_count > 0, (
        f"預期至少 1 個 429，但全部都是 {status_codes}"
    )
    # 前 60 個應該都是 200（或後端服務回傳的狀態碼）
    assert success_count >= 60, (
        f"預期至少 60 個成功請求，實際只有 {success_count}"
    )
    print(f"\n✅ Rate limit 正常運作：第 {success_count + 1} 個請求開始被限流")


@pytest.mark.asyncio
async def test_rate_limit_concurrent_429():
    """
    同時發送 80 個並發請求，驗證超過限制的請求會收到 429。
    """
    total_requests = 80

    async with httpx.AsyncClient() as client:
        # 取得 token
        token_resp = await client.post(
            f"{GATEWAY_URL}/auth/token",
            data={"username": "user", "password": "user123"},
        )
        assert token_resp.status_code == 200
        token = token_resp.json()["access_token"]
        headers = {"Authorization": f"Bearer {token}"}

        async def send_request(idx: int):
            resp = await client.get(TARGET_URL, headers=headers)
            return idx, resp.status_code

        tasks = [send_request(i) for i in range(total_requests)]
        results = await asyncio.gather(*tasks)

    status_codes = [code for _, code in results]
    rate_limited = [idx for idx, code in results if code == 429]

    print(f"\n並發測試結果：")
    print(f"  總請求數         : {total_requests}")
    print(f"  200 OK           : {status_codes.count(200)}")
    print(f"  429 Too Many Req : {len(rate_limited)}")
    print(f"  被限流的請求編號 : {sorted(rate_limited)}")

    assert len(rate_limited) > 0, (
        f"並發 {total_requests} 個請求，預期有 429，但全部狀態碼：{set(status_codes)}"
    )
    print(f"\n✅ 並發 rate limit 正常：{len(rate_limited)} 個請求被限流")
