package di

import (
	"log"
	"net/http"

	"ai-coding-interview-gateway/internal/config"
	"ai-coding-interview-gateway/internal/gateway"
)

type Container struct {
	Config config.Config
	Logger *log.Logger
	Server *http.Server
}

func New(cfg config.Config, logger *log.Logger) (*Container, error) {
	handler, err := gateway.NewHandler(gateway.Options{
		UserURL:               cfg.Upstreams.Users,
		OrderURL:              cfg.Upstreams.Orders,
		ProductURL:            cfg.Upstreams.Products,
		MaxBodyBytes:          cfg.Gateway.MaxBodyBytes,
		UpstreamTimeout:       cfg.Gateway.UpstreamTimeout,
		ReadyCheckTimeout:     cfg.Gateway.ReadyCheckTimeout,
		ReadyCheckParallel:    cfg.Gateway.ReadyCheckParallel,
		RequestIDHeader:       cfg.Gateway.RequestIDHeader,
		TrustForwardedHeaders: cfg.Gateway.TrustForwardedHeaders,
	}, logger)
	if err != nil {
		return nil, err
	}

	return &Container{
		Config: cfg,
		Logger: logger,
		Server: &http.Server{
			Addr:              cfg.Server.ListenAddr,
			Handler:           handler,
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
			ReadTimeout:       cfg.Server.ReadTimeout,
			WriteTimeout:      cfg.Server.WriteTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
			MaxHeaderBytes:    cfg.Server.MaxHeaderBytes,
		},
	}, nil
}
