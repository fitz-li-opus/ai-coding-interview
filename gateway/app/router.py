"""Prefix-based route matching with segment boundaries and longest-prefix-wins."""

from __future__ import annotations

from .config import Route


class Router:
    def __init__(self, routes: list[Route]):
        # Longest prefix first so more specific routes win.
        self._routes = sorted(routes, key=lambda r: len(r.prefix), reverse=True)

    def match(self, path: str) -> tuple[Route, str] | None:
        """Return (route, upstream_path) or None.

        A prefix matches only on a path-segment boundary:
        "/api/users" matches "/api/users" and "/api/users/1", not "/api/usersX".
        """
        for route in self._routes:
            p = route.prefix
            if p == "/" or path == p or path.startswith(p + "/"):
                if not route.strip_prefix or p == "/":
                    return route, path
                rest = path[len(p):]
                return route, rest or "/"
        return None
