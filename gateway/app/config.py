"""Load routing config from YAML, with per-upstream env var overrides."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path

import yaml

DEFAULT_CONFIG_PATH = Path(__file__).resolve().parent.parent / "routes.yaml"


@dataclass(frozen=True)
class RateLimit:
    rate: float  # tokens refilled per second
    burst: int  # bucket capacity

    def __post_init__(self):
        if self.rate <= 0 or self.burst < 1:
            raise ValueError(f"rate_limit needs rate > 0 and burst >= 1, got {self}")


@dataclass(frozen=True)
class Route:
    name: str
    prefix: str
    upstream: str
    strip_prefix: bool = True
    rate_limit: RateLimit | None = None


@dataclass(frozen=True)
class Timeouts:
    connect: float = 2.0
    read: float = 10.0
    write: float = 10.0
    pool: float = 2.0


@dataclass(frozen=True)
class Config:
    routes: list[Route]
    timeouts: Timeouts = field(default_factory=Timeouts)


def _normalize_prefix(prefix: str) -> str:
    if not prefix.startswith("/"):
        raise ValueError(f"route prefix must start with '/': {prefix!r}")
    return prefix.rstrip("/") or "/"


def load_config(path: str | os.PathLike | None = None) -> Config:
    path = Path(path or os.environ.get("GATEWAY_CONFIG", DEFAULT_CONFIG_PATH))
    raw = yaml.safe_load(path.read_text()) or {}

    routes: list[Route] = []
    for item in raw.get("routes", []):
        name = item["name"]
        env_key = f"UPSTREAM_{name.upper()}_URL"
        upstream = os.environ.get(env_key, item["upstream"]).rstrip("/")
        rl = item.get("rate_limit")
        routes.append(
            Route(
                name=name,
                prefix=_normalize_prefix(item["prefix"]),
                upstream=upstream,
                strip_prefix=bool(item.get("strip_prefix", True)),
                rate_limit=RateLimit(rate=float(rl["rate"]), burst=int(rl["burst"])) if rl else None,
            )
        )
    if not routes:
        raise ValueError(f"no routes defined in {path}")

    timeouts = Timeouts(**(raw.get("timeouts") or {}))
    return Config(routes=routes, timeouts=timeouts)
