package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultListen    = ":8080"
	defaultTimeoutMS = 3000
	shutdownGrace    = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", envOr("GATEWAY_CONFIG", "routes.json"), "path to the JSON route configuration")
	listen := flag.String("listen", envOr("GATEWAY_LISTEN", ""), "listen address; overrides the config file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, fromFile, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if !fromFile {
		slog.Warn("config file not found, using built-in defaults", "path", *configPath)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}

	router, err := NewRouter(cfg)
	if err != nil {
		return err
	}
	// Print the effective table so "file or defaults?" is never a guess.
	for _, route := range router.routes {
		slog.Info("route",
			"pattern", route.Pattern,
			"target", route.Target.String(),
			"strip_prefix", route.StripPrefix,
			"timeout", route.Timeout,
		)
	}

	srv := &http.Server{
		Addr: cfg.Listen,
		// AccessLog outermost, Recover inside it: see Recover's comment.
		Handler: Chain(root(router), AccessLog, Recover),
		// No WriteTimeout on purpose — it would cap long-lived upstream
		// responses. The per-route context deadline bounds the upstream call.
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("gateway listening", "addr", cfg.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down, draining in-flight requests")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// root reserves the gateway's own /health ahead of the route table, so no
// configured route can ever shadow it.
func root(router *Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "gateway"})
			return
		}
		router.ServeHTTP(w, r)
	})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
