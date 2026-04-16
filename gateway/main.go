package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"gateway/config"
	"gateway/middleware"
	"gateway/router"
)

func main() {
	cfg, err := config.Load("config/routes.yml")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	r, err := router.New(cfg.Routes)
	if err != nil {
		slog.Error("failed to create router", "error", err)
		os.Exit(1)
	}

	var handler http.Handler = r
	handler = middleware.Logging(handler)
	handler = middleware.Recovery(handler)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	slog.Info("gateway starting", "addr", addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
