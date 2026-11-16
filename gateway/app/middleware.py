"""Request ID propagation and structured (JSON) access logging."""

from __future__ import annotations

import json
import logging
import re
import time
import uuid

from starlette.requests import Request

access_logger = logging.getLogger("gateway.access")

# Accept client-supplied IDs only if they look sane (avoid log/header injection).
_VALID_REQUEST_ID = re.compile(r"^[A-Za-z0-9._\-]{1,128}$")


def get_request_id(request: Request) -> str:
    incoming = request.headers.get("x-request-id", "")
    return incoming if _VALID_REQUEST_ID.match(incoming) else uuid.uuid4().hex


async def access_log_middleware(request: Request, call_next):
    request.state.request_id = get_request_id(request)
    request.state.route = None
    start = time.perf_counter()
    status = 500
    try:
        response = await call_next(request)
        status = response.status_code
        return response
    finally:
        # Latency is time-to-response-headers; body streaming happens afterwards.
        access_logger.info(
            json.dumps(
                {
                    "request_id": request.state.request_id,
                    "method": request.method,
                    "path": request.url.path,
                    "query": request.url.query,
                    "route": request.state.route,
                    "status": status,
                    "latency_ms": round((time.perf_counter() - start) * 1000, 2),
                    "client": request.client.host if request.client else None,
                }
            )
        )
