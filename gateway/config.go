package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

type routeConfig struct {
	Name        string
	PathPrefix  string
	UpstreamURL string
}

type config struct {
	ListenAddress        string
	Routes               []routeConfig
	MaxRequestBodyBytes  int64
	ReadHeaderTimeout    time.Duration
	ReadTimeout          time.Duration
	WriteTimeout         time.Duration
	IdleTimeout          time.Duration
	UpstreamTimeout      time.Duration
	ShutdownTimeout      time.Duration
}

func loadConfig() (config, error) {
	maxBodyBytes, err := envInt64("MAX_REQUEST_BODY_BYTES", 1<<20)
	if err != nil {
		return config{}, err
	}

	cfg := config{
		ListenAddress:       envOrDefault("GATEWAY_ADDRESS", ":8080"),
		MaxRequestBodyBytes: maxBodyBytes,
		ReadHeaderTimeout:   5 * time.Second,
		ReadTimeout:         15 * time.Second,
		WriteTimeout:        30 * time.Second,
		IdleTimeout:         60 * time.Second,
		UpstreamTimeout:     10 * time.Second,
		ShutdownTimeout:     10 * time.Second,
		Routes: []routeConfig{
			{Name: "users", PathPrefix: "/api/users", UpstreamURL: envOrDefault("USER_SERVICE_URL", "http://localhost:8081")},
			{Name: "orders", PathPrefix: "/api/orders", UpstreamURL: envOrDefault("ORDER_SERVICE_URL", "http://localhost:8082")},
			{Name: "products", PathPrefix: "/api/products", UpstreamURL: envOrDefault("PRODUCT_SERVICE_URL", "http://localhost:8083")},
		},
	}

	if err := cfg.validate(); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func (cfg config) validate() error {
	if cfg.ListenAddress == "" {
		return fmt.Errorf("gateway address must not be empty")
	}
	if cfg.MaxRequestBodyBytes <= 0 {
		return fmt.Errorf("max request body bytes must be positive")
	}

	for _, route := range cfg.Routes {
		upstream, err := url.ParseRequestURI(route.UpstreamURL)
		if err != nil {
			return fmt.Errorf("parse %s upstream URL: %w", route.Name, err)
		}
		if (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" {
			return fmt.Errorf("%s upstream URL must use http or https and include a host", route.Name)
		}
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
