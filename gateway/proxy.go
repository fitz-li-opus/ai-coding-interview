package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

type Handler struct {
	routes  []Route
	proxies map[string]*httputil.ReverseProxy
}

func newHandler(routes []Route) (*Handler, error) {
	proxies := make(map[string]*httputil.ReverseProxy)
	for _, r := range routes {
		u, err := url.Parse(r.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", r.Target, err)
		}
		proxies[r.Target] = httputil.NewSingleHostReverseProxy(u)
	}
	return &Handler{routes: routes, proxies: proxies}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

	target := h.match(r.URL.Path)
	if target == "" {
		http.NotFound(rec, r)
		log.Printf("%s %s → 404 (no route matched) %s", r.Method, r.URL.Path, time.Since(start))
		return
	}

	h.proxies[target].ServeHTTP(rec, r)
	log.Printf("%s %s → %s [%d] %s", r.Method, r.URL.Path, target, rec.status, time.Since(start))
}

// match returns the target for the longest matching prefix, or "" if none match.
func (h *Handler) match(path string) string {
	best, bestLen := "", 0
	for _, route := range h.routes {
		if strings.HasPrefix(path, route.Prefix) && len(route.Prefix) > bestLen {
			best = route.Target
			bestLen = len(route.Prefix)
		}
	}
	return best
}
