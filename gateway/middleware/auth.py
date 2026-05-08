from datetime import datetime, timedelta
from typing import Optional

import bcrypt
from fastapi import HTTPException, Request, status
from fastapi.responses import JSONResponse
from jose import JWTError, jwt

from config import JWT_ALGORITHM, JWT_EXPIRE_MINUTES, JWT_SECRET_KEY


def _hash_password(plain: str) -> bytes:
    return bcrypt.hashpw(plain.encode(), bcrypt.gensalt())


def verify_password(plain_password: str, hashed_password: bytes) -> bool:
    return bcrypt.checkpw(plain_password.encode(), hashed_password)


# 模擬用戶資料庫（實際應用應連接真實 DB）
FAKE_USERS = {
    "admin": {
        "username": "admin",
        "hashed_password": _hash_password("admin123"),
        "role": "admin",
    },
    "user": {
        "username": "user",
        "hashed_password": _hash_password("user123"),
        "role": "user",
    },
}

# 不需要驗證的路徑白名單
AUTH_WHITELIST = {"/health", "/auth/token", "/docs", "/openapi.json", "/redoc", "/"}


def authenticate_user(username: str, password: str) -> Optional[dict]:
    user = FAKE_USERS.get(username)
    if not user:
        return None
    if not verify_password(password, user["hashed_password"]):
        return None
    return user


def create_access_token(data: dict) -> str:
    to_encode = data.copy()
    expire = datetime.utcnow() + timedelta(minutes=JWT_EXPIRE_MINUTES)
    to_encode.update({"exp": expire})
    return jwt.encode(to_encode, JWT_SECRET_KEY, algorithm=JWT_ALGORITHM)


def decode_token(token: str) -> dict:
    try:
        payload = jwt.decode(token, JWT_SECRET_KEY, algorithms=[JWT_ALGORITHM])
        return payload
    except JWTError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid or expired token",
            headers={"WWW-Authenticate": "Bearer"},
        )


async def jwt_auth_middleware(request: Request, call_next):
    """JWT 驗證 middleware，白名單路徑跳過驗證"""
    path = request.url.path

    # 白名單路徑直接放行
    if path in AUTH_WHITELIST:
        return await call_next(request)

    # 取得 Authorization header
    auth_header = request.headers.get("Authorization")
    if not auth_header or not auth_header.startswith("Bearer "):
        return JSONResponse(
            status_code=status.HTTP_401_UNAUTHORIZED,
            content={"detail": "Missing or invalid Authorization header"},
            headers={"WWW-Authenticate": "Bearer"},
        )

    token = auth_header.split(" ", 1)[1]
    try:
        payload = jwt.decode(token, JWT_SECRET_KEY, algorithms=[JWT_ALGORITHM])
    except JWTError:
        return JSONResponse(
            status_code=status.HTTP_401_UNAUTHORIZED,
            content={"detail": "Invalid or expired token"},
            headers={"WWW-Authenticate": "Bearer"},
        )

    # 將用戶資訊注入 request state，供後續使用
    request.state.user = payload.get("sub")
    request.state.role = payload.get("role")

    return await call_next(request)
