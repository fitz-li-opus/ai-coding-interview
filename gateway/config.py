from typing import Dict

# 路由規則：前綴 -> 後端 base URL
ROUTE_MAP: Dict[str, str] = {
    "/api/users": "http://localhost:8081",
    "/api/orders": "http://localhost:8082",
    "/api/products": "http://localhost:8083",
}

# JWT 設定
JWT_SECRET_KEY = "super-secret-key-change-in-production"
JWT_ALGORITHM = "HS256"
JWT_EXPIRE_MINUTES = 30

# Rate Limiting
RATE_LIMIT = "60/minute"  # 每個 IP 每分鐘最多 60 次請求

# HTTP Client 逾時設定（秒）
PROXY_TIMEOUT = 5.0

# Circuit Breaker 設定
CB_FAIL_MAX = 5        # 連續失敗 5 次後開路
CB_RESET_TIMEOUT = 30  # 30 秒後嘗試半開

# Gateway 監聽 Port
GATEWAY_PORT = 8080
