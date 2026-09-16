package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gateway/internal/config"
	"gateway/internal/health"
	"gateway/internal/httpx"
	"gateway/internal/lb"
	"gateway/internal/metrics"
	"gateway/internal/middleware"
	"gateway/internal/proxy"
	"gateway/internal/router"
)

type routeBundle struct {
	route     *config.Route
	handler   http.Handler
	instances []*lb.Instance
}

func (b *routeBundle) hasHealthy() bool {
	for _, inst := range b.instances {
		if inst.Healthy() {
			return true
		}
	}
	return false
}

func main() {
	configPath := flag.String("config", "config/routes.json", "path to the gateway config file")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *configPath, logger); err != nil {
		logger.Error("gateway exited with error", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string, logger *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	m := metrics.New()

	var (
		probers []*health.Active
		bundles = map[string]*routeBundle{}
		routes  = make([]*config.Route, 0, len(cfg.Routes))
	)

	for i := range cfg.Routes {
		route := &cfg.Routes[i]
		timeout := cfg.TimeoutFor(route)
		retry := cfg.RetryFor(route)
		healthCfg := cfg.HealthFor(route)

		instances := make([]*lb.Instance, 0, len(route.Upstreams))
		for _, u := range route.Upstreams {
			inst, err := lb.NewInstance(u.URL, u.Weight)
			if err != nil {
				return err
			}
			instances = append(instances, inst)
		}

		balancer, err := lb.New(route.LB)
		if err != nil {
			return err
		}

		passive := health.NewPassive(healthCfg.UnhealthyThreshold, healthCfg.Ejection.D())
		handler := proxy.New(route, instances, balancer, passive, timeout, retry, m, logger)

		prober := health.NewActive(
			healthCfg.Path,
			healthCfg.Interval.D(),
			healthCfg.Timeout.D(),
			healthCfg.HealthyThreshold,
			healthCfg.UnhealthyThreshold,
			logger,
		)
		prober.Start(instances)
		probers = append(probers, prober)

		bundles[route.ID] = &routeBundle{route: route, handler: handler, instances: instances}
		routes = append(routes, route)

		logger.Info("route configured",
			"route", route.ID,
			"prefix", route.Match.Prefix,
			"lb", route.LB,
			"upstreams", len(instances),
		)
	}

	rt := router.New(routes)

	registry := middleware.NewRegistry()
	registry.Register("request_id", func() middleware.Middleware { return middleware.RequestID })
	registry.Register("recovery", func() middleware.Middleware { return middleware.Recovery(logger) })
	registry.Register("logging", func() middleware.Middleware { return middleware.Logging(logger) })
	registry.Register("metrics", func() middleware.Middleware { return middleware.Observe(m) })

	for _, b := range bundles {
		mws, err := registry.Build(b.route.Middlewares)
		if err != nil {
			return err
		}
		if len(mws) > 0 {
			b.handler = middleware.New(mws...).Then(b.handler)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", readinessHandler(routes, bundles))
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.Render(w)
	})
	mux.Handle("/", dispatcher(rt, bundles))

	chain := middleware.New(
		middleware.RequestID,
		middleware.Recovery(logger),
		middleware.Logging(logger),
		middleware.Observe(m),
	)
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           chain.Then(mux),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       cfg.Defaults.Timeout.Idle.D(),
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gateway listening", "addr", cfg.Listen, "routes", len(cfg.Routes))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	for _, p := range probers {
		p.Stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("gateway stopped")
	return nil
}

func dispatcher(rt *router.Router, bundles map[string]*routeBundle) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, methodOK, allowed := rt.Match(r.Method, r.URL.Path)
		if route == nil {
			httpx.WriteError(w, http.StatusNotFound, "no route matched")
			return
		}
		if !methodOK {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if info := httpx.RouteInfoFrom(r.Context()); info != nil {
			info.ID = route.ID
		}
		bundle, ok := bundles[route.ID]
		if !ok {
			httpx.WriteError(w, http.StatusInternalServerError, "route handler missing")
			return
		}
		r.URL.Path = router.Rewrite(route, r.URL.Path)
		bundle.handler.ServeHTTP(w, r)
	})
}

func readinessHandler(routes []*config.Route, bundles map[string]*routeBundle) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ready := true
		for _, route := range routes {
			b, ok := bundles[route.ID]
			if !ok {
				ready = false
				break
			}
			if !b.hasHealthy() {
				ready = false
				break
			}
		}
		if !ready {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
