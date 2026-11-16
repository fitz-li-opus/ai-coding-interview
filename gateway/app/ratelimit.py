"""In-memory token-bucket rate limiter.

Each key (e.g. client IP) gets a bucket holding up to `burst` tokens, refilled at
`rate` tokens/second. A request costs one token; an empty bucket means 429.

State lives in process memory: with several workers/instances each one counts
separately. A shared store (e.g. Redis + Lua) would be needed for a global limit.
"""

from __future__ import annotations

import math
import time
from collections import OrderedDict
from dataclasses import dataclass
from typing import Callable


@dataclass
class Decision:
    allowed: bool
    limit: int
    remaining: int
    retry_after: int  # whole seconds until one token is available (0 if allowed)


class TokenBucketLimiter:
    def __init__(
        self,
        rate: float,
        burst: int,
        max_keys: int = 10_000,
        clock: Callable[[], float] = time.monotonic,
    ):
        if rate <= 0 or burst < 1:
            raise ValueError(f"invalid rate limit: rate={rate}, burst={burst}")
        self.rate = rate
        self.burst = burst
        self.max_keys = max_keys
        self._clock = clock
        # key -> (tokens, last_refill). Ordered for LRU eviction.
        self._buckets: OrderedDict[str, tuple[float, float]] = OrderedDict()

    def allow(self, key: str) -> Decision:
        # Single-threaded asyncio: no await in here, so no lock is needed.
        now = self._clock()
        tokens, last = self._buckets.get(key, (float(self.burst), now))
        tokens = min(self.burst, tokens + (now - last) * self.rate)

        if tokens >= 1:
            tokens -= 1
            allowed, retry_after = True, 0
        else:
            allowed = False
            retry_after = math.ceil((1 - tokens) / self.rate)

        self._buckets[key] = (tokens, now)
        self._buckets.move_to_end(key)
        # Bound memory against key spraying (many distinct IPs). Evicting the
        # least-recently-used bucket only resets it to full, i.e. errs lenient.
        while len(self._buckets) > self.max_keys:
            self._buckets.popitem(last=False)

        return Decision(allowed, self.burst, int(tokens), retry_after)
