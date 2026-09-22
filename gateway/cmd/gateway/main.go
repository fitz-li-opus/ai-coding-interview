package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-coding-interview/gateway/internal/config"
	"ai-coding-interview/gateway/internal/gateway"
	"ai-coding-interview/gateway/internal/middleware"
	proxyhandler "ai-coding-interview/gateway/internal/proxy"
	"ai-coding-interview/gateway/internal/router"
	"ai-coding-interview/gateway/internal/upstream"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway terminated", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configPath, certificatePath, privateKeyPath, err := parseFlags(os.Args[1:])
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	certificate, err := tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	proxyTransport := newTransport()
	healthTransport := newTransport()
	upstreamManager, err := upstream.New(cfg.Upstreams, healthTransport)
	if err != nil {
		return err
	}
	upstreamManager.Start(ctx)

	limiterStore := middleware.NewLimiterStore()
	limiterStore.Start(ctx)
	proxyHandler := proxyhandler.NewHandler(router.New(cfg.Routes), upstreamManager, limiterStore, proxyTransport)
	rootHandler := gateway.NewHandler(proxyHandler, upstreamManager, referencedUpstreams(cfg.Routes))
	handler := gateway.Wrap(rootHandler, cfg.Server.MaxBodyBytes, func(next http.Handler) http.Handler {
		return middleware.AccessLog(logger, next)
	})

	server := &http.Server{
		Addr:              cfg.Server.Address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
		},
	}

	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("gateway listening", "address", cfg.Server.Address)
		serveErrors <- server.ListenAndServeTLS("", "")
	}()

	select {
	case serveErr := <-serveErrors:
		stop()
		proxyTransport.CloseIdleConnections()
		healthTransport.CloseIdleConnections()
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTPS: %w", serveErr)
	case <-ctx.Done():
		logger.Info("gateway shutting down")
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownContext)
	var closeErr error
	if shutdownErr != nil {
		closeErr = server.Close()
	}
	serveErr := <-serveErrors
	proxyTransport.CloseIdleConnections()
	healthTransport.CloseIdleConnections()
	if closeErr != nil {
		return fmt.Errorf("close gateway after shutdown timeout: %w", closeErr)
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown gateway: %w", shutdownErr)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTPS: %w", serveErr)
	}
	return nil
}

func parseFlags(arguments []string) (string, string, string, error) {
	flags := flag.NewFlagSet("gateway", flag.ContinueOnError)
	configPath := flags.String("config", "", "path to the YAML configuration")
	certificatePath := flags.String("tls-cert", "", "path to the TLS certificate")
	privateKeyPath := flags.String("tls-key", "", "path to the TLS private key")
	if err := flags.Parse(arguments); err != nil {
		return "", "", "", err
	}
	if flags.NArg() != 0 {
		return "", "", "", fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *configPath == "" || *certificatePath == "" || *privateKeyPath == "" {
		return "", "", "", errors.New("--config, --tls-cert, and --tls-key are required")
	}
	return *configPath, *certificatePath, *privateKeyPath, nil
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   2 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}

func referencedUpstreams(routes []config.RouteConfig) []string {
	seen := make(map[string]struct{}, len(routes))
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		if _, exists := seen[route.Upstream]; exists {
			continue
		}
		seen[route.Upstream] = struct{}{}
		ids = append(ids, route.Upstream)
	}
	return ids
}
