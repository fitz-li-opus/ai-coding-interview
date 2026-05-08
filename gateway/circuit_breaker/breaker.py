"""
輕量級 Circuit Breaker 實作（不依賴第三方套件）

狀態機：
  CLOSED  → 正常轉發請求，累計失敗次數
  OPEN    → 快速失敗，不轉發請求，等待 reset_timeout 後進入 HALF_OPEN
  HALF_OPEN → 允許一個探測請求，成功則回 CLOSED，失敗則回 OPEN
"""

import asyncio
import time
from enum import Enum

from config import CB_FAIL_MAX, CB_RESET_TIMEOUT


class CBState(Enum):
    CLOSED = "closed"
    OPEN = "open"
    HALF_OPEN = "half_open"


class CircuitBreakerError(Exception):
    """Circuit 開路時拋出"""


class CircuitBreaker:
    def __init__(self, name: str, fail_max: int = CB_FAIL_MAX, reset_timeout: float = CB_RESET_TIMEOUT):
        self.name = name
        self.fail_max = fail_max
        self.reset_timeout = reset_timeout

        self._state = CBState.CLOSED
        self._fail_counter = 0
        self._opened_at: float | None = None
        self._lock = asyncio.Lock()

    @property
    def current_state(self) -> str:
        return self._state.value

    @property
    def fail_counter(self) -> int:
        return self._fail_counter

    def _should_attempt(self) -> bool:
        """判斷目前是否允許發送請求"""
        if self._state == CBState.CLOSED:
            return True
        if self._state == CBState.OPEN:
            if self._opened_at and (time.monotonic() - self._opened_at) >= self.reset_timeout:
                self._state = CBState.HALF_OPEN
                return True
            return False
        # HALF_OPEN：允許一次探測
        return True

    def _on_success(self):
        self._fail_counter = 0
        self._state = CBState.CLOSED
        self._opened_at = None

    def _on_failure(self):
        self._fail_counter += 1
        if self._state == CBState.HALF_OPEN or self._fail_counter >= self.fail_max:
            self._state = CBState.OPEN
            self._opened_at = time.monotonic()

    async def call_async(self, func):
        """執行非同步函式，並根據結果更新 Circuit Breaker 狀態"""
        async with self._lock:
            if not self._should_attempt():
                raise CircuitBreakerError(
                    f"Circuit breaker '{self.name}' is OPEN"
                )

        try:
            result = await func()
            async with self._lock:
                self._on_success()
            return result
        except CircuitBreakerError:
            raise
        except Exception:
            async with self._lock:
                self._on_failure()
            raise


# ── 為每個後端服務建立獨立的 Circuit Breaker ──────────────────────────────
user_service_breaker = CircuitBreaker(name="user-service")
order_service_breaker = CircuitBreaker(name="order-service")
product_service_breaker = CircuitBreaker(name="product-service")

# 路由前綴 -> Circuit Breaker 對應表
BREAKERS: dict[str, CircuitBreaker] = {
    "/api/users": user_service_breaker,
    "/api/orders": order_service_breaker,
    "/api/products": product_service_breaker,
}


def get_breaker(path: str) -> CircuitBreaker | None:
    """根據請求路徑取得對應的 Circuit Breaker"""
    for prefix, breaker in BREAKERS.items():
        if path.startswith(prefix):
            return breaker
    return None
