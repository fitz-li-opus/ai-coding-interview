package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "path to the gateway config file")
	listenOverride := flag.String("listen", "", "override the listen address from the config")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := Load(*configPath)
	if err != nil {
		logger.Error("cannot load config", "err", err)
		os.Exit(1)
	}
	if *listenOverride != "" {
		cfg.Listen = *listenOverride
	}

	transport := newTransport(cfg.Timeouts)
	routes, err := BuildRoutes(cfg, transport, logger)
	if err != nil {
		logger.Error("cannot build routes", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.HealthCheck.Enabled {
		checker := newHealthChecker(cfg, routes, logger)
		checker.ProbeAll(ctx) // first round is synchronous: know the truth before serving
		go checker.Run(ctx)
	}

	gateway := &Gateway{
		router:  NewRouter(routes),
		timeout: ms(cfg.Timeouts.UpstreamMS),
		logger:  logger,
	}
	handler := withRequestInfo(accessLog(logger, recoverPanic(logger, gateway)))

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: ms(cfg.Timeouts.ReadHeaderMS),
		IdleTimeout:       60 * time.Second,
		// No WriteTimeout on purpose: it is an absolute deadline that would cut
		// off slow or streaming upstream responses. The per-request context
		// timeout in Gateway.ServeHTTP bounds the backend call instead.
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	// Bind before announcing anything. ListenAndServe inside a goroutine would
	// let the process log "listening" and exit 0 on a port clash, which is the
	// worst way to fail: a supervisor sees a healthy start and no service.
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		logger.Error("cannot bind listen address", "addr", cfg.Listen, "err", err)
		os.Exit(1)
	}

	for _, rt := range routes {
		logger.Info("route registered", "prefix", rt.Prefix, "strip_prefix", rt.StripPrefix, "targets", len(rt.Upstreams))
	}
	logger.Info("gateway listening", "addr", listener.Addr().String())

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}
	stop()
	logger.Info("shutting down, draining in-flight requests")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	logger.Info("bye")
}
