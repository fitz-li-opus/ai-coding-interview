package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// HealthChecker actively probes every upstream so a dead backend is taken out
// of rotation before a real request hits it, instead of after.
type HealthChecker struct {
	routes   []*Route
	path     string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
}

func newHealthChecker(cfg *Config, routes []*Route, logger *slog.Logger) *HealthChecker {
	return &HealthChecker{
		routes:   routes,
		path:     cfg.HealthCheck.Path,
		interval: ms(cfg.HealthCheck.IntervalMS),
		client:   &http.Client{Timeout: ms(cfg.HealthCheck.TimeoutMS)},
		logger:   logger,
	}
}

// Run probes on a ticker until ctx is cancelled. Callers run ProbeAll once
// synchronously first, so the gateway never starts up thinking everything is
// down (or, worse, serves the first request into a dead backend).
func (h *HealthChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.ProbeAll(ctx)
		}
	}
}

func (h *HealthChecker) ProbeAll(ctx context.Context) {
	var wg sync.WaitGroup
	for _, rt := range h.routes {
		for _, u := range rt.Upstreams {
			wg.Add(1)
			go func(u *Upstream) {
				defer wg.Done()
				h.probe(ctx, u)
			}(u)
		}
	}
	wg.Wait()
}

func (h *HealthChecker) probe(ctx context.Context, u *Upstream) {
	target := u.URL.JoinPath(h.path).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return
	}

	resp, err := h.client.Do(req)
	healthy := err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300
	if resp != nil {
		// Drain a bounded amount so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		_ = resp.Body.Close()
	}

	// Only log transitions; a per-tick log on a healthy fleet is just noise.
	if !u.setHealthy(healthy) {
		return
	}
	if healthy {
		h.logger.Info("upstream is up", "upstream", u.URL.String())
	} else {
		h.logger.Warn("upstream is down", "upstream", u.URL.String(), "err", err)
	}
}
