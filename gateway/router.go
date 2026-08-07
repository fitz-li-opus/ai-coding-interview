package main

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Route is a compiled RouteConfig. handler is assembled once, at config load
// time, so the request hot path is a table scan plus one ServeHTTP call — no
// per-request middleware composition.
type Route struct {
	Pattern     string // as written in the config, e.g. "/api/users/**"
	Prefix      string // literal match prefix, e.g. "/api/users"
	Target      *url.URL
	StripPrefix bool
	Timeout     time.Duration

	handler http.Handler
}

// Router dispatches requests to routes by longest matching prefix.
type Router struct {
	routes []*Route
}

func NewRouter(cfg *Config) (*Router, error) {
	if len(cfg.Routes) == 0 {
		return nil, fmt.Errorf("no routes configured")
	}

	rt := &Router{}
	seen := make(map[string]bool, len(cfg.Routes))
	for i, rc := range cfg.Routes {
		prefix, err := parsePattern(rc.Path)
		if err != nil {
			return nil, fmt.Errorf("routes[%d]: %w", i, err)
		}
		if seen[prefix] {
			return nil, fmt.Errorf("routes[%d]: duplicate path %q", i, rc.Path)
		}
		seen[prefix] = true

		target, err := parseTarget(rc.Target)
		if err != nil {
			return nil, fmt.Errorf("routes[%d]: %w", i, err)
		}

		timeout := time.Duration(cfg.DefaultTimeoutMS) * time.Millisecond
		if rc.TimeoutMS > 0 {
			timeout = time.Duration(rc.TimeoutMS) * time.Millisecond
		}

		route := &Route{
			Pattern:     rc.Path,
			Prefix:      prefix,
			Target:      target,
			StripPrefix: rc.stripPrefix(),
			Timeout:     timeout,
		}
		route.handler = newRouteHandler(route)
		rt.routes = append(rt.routes, route)
	}

	// Longest prefix wins, so "/api/users/admin" beats "/api/users" no matter
	// which order they appear in the config file.
	slices.SortStableFunc(rt.routes, func(a, b *Route) int {
		return len(b.Prefix) - len(a.Prefix)
	})
	return rt, nil
}

// ServeHTTP matches on EscapedPath rather than Path: "/api%2Fusers/1" decodes
// to "/api/users/1" but the client did not write a real separator there, so it
// must not be routed as though it had.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route := rt.Match(r.URL.EscapedPath())
	if route == nil {
		writeError(w, r, http.StatusNotFound, "no_route", "no route matches "+r.URL.Path)
		return
	}
	route.handler.ServeHTTP(w, r)
}

func (rt *Router) Match(escapedPath string) *Route {
	for _, route := range rt.routes {
		if matchPrefix(escapedPath, route.Prefix) {
			return route
		}
	}
	return nil
}

// matchPrefix matches on path segment boundaries. Plain strings.HasPrefix would
// route "/api/usersecret" to the user service.
func matchPrefix(path, prefix string) bool {
	if prefix == "" {
		return true // "/**" is a catch-all
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := path[len(prefix):]
	return rest == "" || rest[0] == '/'
}

// parsePattern converts a config pattern into a literal match prefix. "**" is
// supported only as a trailing "/**" segment; a full glob engine is out of
// scope, and anything else is rejected at startup instead of silently ignored.
func parsePattern(pattern string) (string, error) {
	if !strings.HasPrefix(pattern, "/") {
		return "", fmt.Errorf("path %q must start with /", pattern)
	}
	prefix, _ := strings.CutSuffix(pattern, "/**")
	if strings.Contains(prefix, "*") {
		return "", fmt.Errorf("path %q: * is only supported as a trailing /**", pattern)
	}
	return strings.TrimSuffix(prefix, "/"), nil
}

func parseTarget(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("target %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("target %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("target %q: missing host", raw)
	}
	return u, nil
}
