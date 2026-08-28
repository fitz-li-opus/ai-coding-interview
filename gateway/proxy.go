package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

// newTransport is shared by every upstream so connection pooling and keep-alive
// are global rather than per-backend.
func newTransport(t Timeouts) *http.Transport {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{
		Timeout:   ms(t.DialMS),
		KeepAlive: 30 * time.Second,
	}).DialContext
	tr.MaxIdleConns = 200
	tr.MaxIdleConnsPerHost = 100
	tr.ResponseHeaderTimeout = ms(t.UpstreamMS)
	return tr
}

func newUpstream(raw string, tr http.RoundTripper, logger *slog.Logger) (*Upstream, error) {
	target, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	u := &Upstream{URL: target}
	u.proxy = &httputil.ReverseProxy{
		Transport: tr,
		Rewrite: func(pr *httputil.ProxyRequest) {
			// SetURL also clears Out.Host, so the backend sees its own host
			// rather than the client's. X-Forwarded-Host carries the original.
			pr.SetURL(target)
			pr.SetXForwarded()
			if info := infoFrom(pr.In.Context()); info != nil {
				pr.Out.Header.Set(headerRequestID, info.id)
			}
		},
		ErrorHandler: upstreamErrorHandler(u, logger),
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	// Optimistic until the first probe, so a gateway started with health checks
	// disabled still serves traffic.
	u.healthy.Store(true)
	return u, nil
}

// upstreamErrorHandler turns transport failures into meaningful status codes:
// 504 when we gave up waiting, 502 when the backend refused or broke, and
// nothing at all when the client is the one who walked away.
func upstreamErrorHandler(u *Upstream, logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		switch {
		case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
			logger.Warn("upstream timed out", "upstream", u.URL.String(), "err", err)
			writeError(w, r, http.StatusGatewayTimeout, "upstream timed out", u.URL.String())
		case errors.Is(err, context.Canceled):
			logger.Warn("client canceled request", "upstream", u.URL.String())
		default:
			logger.Warn("upstream request failed", "upstream", u.URL.String(), "err", err)
			writeError(w, r, http.StatusBadGateway, "upstream request failed", u.URL.String())
		}
	}
}

func BuildRoutes(cfg *Config, tr http.RoundTripper, logger *slog.Logger) ([]*Route, error) {
	routes := make([]*Route, 0, len(cfg.Routes))
	for _, rc := range cfg.Routes {
		rt := &Route{Prefix: rc.Prefix, StripPrefix: rc.strip()}
		for _, target := range rc.Targets {
			u, err := newUpstream(target, tr, logger)
			if err != nil {
				return nil, fmt.Errorf("route %s: %w", rc.Prefix, err)
			}
			rt.Upstreams = append(rt.Upstreams, u)
		}
		routes = append(routes, rt)
	}
	return routes, nil
}

// Gateway is the request path: match a route, pick a live upstream, proxy.
type Gateway struct {
	router  *Router
	timeout time.Duration
	logger  *slog.Logger
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The gateway's own liveness endpoint. In production this would sit on a
	// separate admin port rather than share the data plane.
	if r.URL.Path == "/health" {
		g.serveHealth(w, r)
		return
	}

	route := g.router.Match(r.URL.Path)
	if route == nil {
		writeError(w, r, http.StatusNotFound, "no route matches this path", "")
		return
	}

	upstream := route.Pick()
	if upstream == nil {
		writeError(w, r, http.StatusServiceUnavailable, "no healthy upstream for "+route.Prefix, "")
		return
	}

	if info := infoFrom(r.Context()); info != nil {
		info.route = route.Prefix
		info.upstream = upstream.URL.String()
	}

	ctx, cancel := context.WithTimeout(r.Context(), g.timeout)
	defer cancel()

	out := r.Clone(ctx)
	if route.StripPrefix {
		out.URL.Path = stripPrefix(out.URL.Path, route.Prefix)
		if out.URL.RawPath != "" {
			out.URL.RawPath = stripPrefix(out.URL.RawPath, route.Prefix)
		}
	}
	upstream.proxy.ServeHTTP(w, out)
}

type healthReport struct {
	Status string        `json:"status"`
	Routes []routeHealth `json:"routes"`
}

type routeHealth struct {
	Prefix    string           `json:"prefix"`
	Upstreams []upstreamHealth `json:"upstreams"`
}

type upstreamHealth struct {
	Target  string `json:"target"`
	Healthy bool   `json:"healthy"`
}

// serveHealth reports the live view of the upstream pool. It answers 503 when
// any route has lost its whole pool, so an orchestrator can act on it.
func (g *Gateway) serveHealth(w http.ResponseWriter, r *http.Request) {
	report := healthReport{Status: "ok"}
	for _, rt := range g.router.Routes() {
		rh := routeHealth{Prefix: rt.Prefix}
		anyUp := false
		for _, u := range rt.Upstreams {
			up := u.Healthy()
			anyUp = anyUp || up
			rh.Upstreams = append(rh.Upstreams, upstreamHealth{Target: u.URL.String(), Healthy: up})
		}
		if !anyUp {
			report.Status = "degraded"
		}
		report.Routes = append(report.Routes, rh)
	}

	w.Header().Set("Content-Type", "application/json")
	if report.Status != "ok" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(report)
}
