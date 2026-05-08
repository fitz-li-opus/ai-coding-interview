from slowapi import Limiter
from slowapi.util import get_remote_address

from config import RATE_LIMIT

# 以 IP 為 key 進行限流
limiter = Limiter(key_func=get_remote_address, default_limits=[RATE_LIMIT])
