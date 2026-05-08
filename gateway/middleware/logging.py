import logging
import time

from fastapi import Request

# 設定 logger
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S",
)
logger = logging.getLogger("api-gateway")


async def logging_middleware(request: Request, call_next):
    """記錄每個請求的 method、path、status code 與耗時"""
    start_time = time.perf_counter()

    # 取得 client IP
    client_ip = request.client.host if request.client else "unknown"

    response = await call_next(request)

    elapsed_ms = (time.perf_counter() - start_time) * 1000

    logger.info(
        "%s %s %s -> %d (%.2fms)",
        client_ip,
        request.method,
        request.url.path,
        response.status_code,
        elapsed_ms,
    )

    # 在 response header 加上耗時資訊
    response.headers["X-Process-Time-Ms"] = f"{elapsed_ms:.2f}"

    return response
