package main

import (
	"context"
	"net/http"
	"net/http/httputil"
	"time"
)

const defaultPort = "8080"

type Gateway struct {
	routes          []Route
	upstreamTimeout time.Duration
	rateLimiter     *FixedWindowRateLimiter
}

func newServer(cfg Config) http.Handler {
	return &Gateway{
		routes:          routesFromConfig(cfg),
		upstreamTimeout: cfg.UpstreamTimeout,
		rateLimiter:     NewFixedWindowRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow),
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, ok := matchRoute(g.routes, r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !g.rateLimiter.Allow() {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	originalPath := r.URL.Path
	ctx, cancel := context.WithTimeout(r.Context(), g.upstreamTimeout)
	defer cancel()

	proxy := httputil.NewSingleHostReverseProxy(route.UpstreamURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = rewritePath(route, originalPath)
		req.URL.RawPath = ""
		req.Host = route.UpstreamURL.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
}
