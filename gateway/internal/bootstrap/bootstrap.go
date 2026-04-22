package bootstrap

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"ai-coding-interview-gateway/internal/config"
	"ai-coding-interview-gateway/internal/di"
)

func Run() error {
	configFile := os.Getenv("GATEWAY_CONFIG_FILE")
	if configFile == "" {
		configFile = "config/gateway.dev.yaml"
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return err
	}

	container, err := di.New(cfg, log.Default())
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("gateway listening on %s", cfg.Server.ListenAddr)
		errCh <- container.Server.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("shutdown signal: %s", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	return container.Server.Shutdown(ctx)
}
