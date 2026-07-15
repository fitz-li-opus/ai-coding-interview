package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type contextKey string

const routeNameKey contextKey = "route-name"

type route struct {
	name       string
	pathPrefix string
	upstream   *url.URL
	handler    http.Handler
}

type gateway struct {
	routes      []route
	healthClient *http.Client
	logger      *slog.Logger
}

func newGateway(cfg config, logger *slog.Logger) (*gateway, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: cfg.UpstreamTimeout,
	}

	g := &gateway{
		healthClient: &http.Client{Timeout: min(cfg.UpstreamTimeout, 2*time.Second)},
		logger:       logger,
	}

	for _, item := range cfg.Routes {
		target, err := url.Parse(item.UpstreamURL)
		if err != nil {
			return nil, fmt.Errorf("parse %s upstream URL: %w", item.Name, err)
		}
		g.routes = append(g.routes, route{
			name:       item.Name,
			pathPrefix: item.PathPrefix,
			upstream:   target,
			handler:    newReverseProxy(target, transport, logger),
		})
	}
	return g, nil
}

func newReverseProxy(target *url.URL, transport http.RoundTripper, logger *slog.Logger) http.Handler {
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(target)
			request.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status := http.StatusBadGateway
			code := "bad_gateway"

			var bodyTooLarge *http.MaxBytesError
			var networkError net.Error
			switch {
			case errors.As(err, &bodyTooLarge):
				status = http.StatusRequestEntityTooLarge
				code = "request_too_large"
			case errors.As(err, &networkError) && networkError.Timeout():
				status = http.StatusGatewayTimeout
				code = "gateway_timeout"
			}

			logger.Warn("upstream request failed", "request_id", requestIDFromContext(r.Context()), "error", err)
			writeJSONError(w, r, status, code)
		},
	}
}

func (g *gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
		return
	case "/readyz":
		g.serveReadiness(w, r)
		return
	}

	for _, route := range g.routes {
		if pathMatches(r.URL.Path, route.pathPrefix) {
			*r = *r.WithContext(context.WithValue(r.Context(), routeNameKey, route.name))
			route.handler.ServeHTTP(w, r)
			return
		}
	}

	writeJSONError(w, r, http.StatusNotFound, "route_not_found")
}

func pathMatches(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func (g *gateway) serveReadiness(w http.ResponseWriter, r *http.Request) {
	type result struct {
		name string
		err  error
	}

	results := make(chan result, len(g.routes))
	var wg sync.WaitGroup
	for _, currentRoute := range g.routes {
		wg.Add(1)
		go func(currentRoute route) {
			defer wg.Done()
			healthURL := currentRoute.upstream.ResolveReference(&url.URL{Path: "/health"})
			request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, healthURL.String(), nil)
			if err != nil {
				results <- result{name: currentRoute.name, err: err}
				return
			}
			response, err := g.healthClient.Do(request)
			if err == nil {
				response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					err = fmt.Errorf("unexpected health status %d", response.StatusCode)
				}
			}
			results <- result{name: currentRoute.name, err: err}
		}(currentRoute)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	failed := make([]string, 0)
	for check := range results {
		if check.err != nil {
			failed = append(failed, check.name)
			g.logger.Warn("upstream health check failed", "service", check.name, "error", check.err)
		}
	}
	if len(failed) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unready", "unhealthy_services": failed})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
