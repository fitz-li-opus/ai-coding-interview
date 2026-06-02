package main

import (
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	clearConfigEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Port != defaultPort {
		t.Fatalf("Port = %q, want %q", cfg.Port, defaultPort)
	}
	if cfg.UserServiceURL.String() != defaultUserServiceURL {
		t.Fatalf("UserServiceURL = %q, want %q", cfg.UserServiceURL.String(), defaultUserServiceURL)
	}
	if cfg.OrderServiceURL.String() != defaultOrderServiceURL {
		t.Fatalf("OrderServiceURL = %q, want %q", cfg.OrderServiceURL.String(), defaultOrderServiceURL)
	}
	if cfg.ProductServiceURL.String() != defaultProductServiceURL {
		t.Fatalf("ProductServiceURL = %q, want %q", cfg.ProductServiceURL.String(), defaultProductServiceURL)
	}
	if cfg.UpstreamTimeout != 2*time.Second {
		t.Fatalf("UpstreamTimeout = %s, want 2s", cfg.UpstreamTimeout)
	}
	if cfg.RateLimitRequests != 100 {
		t.Fatalf("RateLimitRequests = %d, want 100", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Fatalf("RateLimitWindow = %s, want 1m", cfg.RateLimitWindow)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("USER_SERVICE_URL", "http://users.internal:9001")
	t.Setenv("ORDER_SERVICE_URL", "http://orders.internal:9002")
	t.Setenv("PRODUCT_SERVICE_URL", "http://products.internal:9003")
	t.Setenv("UPSTREAM_TIMEOUT_MS", "150")
	t.Setenv("RATE_LIMIT_REQUESTS", "7")
	t.Setenv("RATE_LIMIT_WINDOW_MS", "250")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.UserServiceURL.String() != "http://users.internal:9001" {
		t.Fatalf("UserServiceURL = %q", cfg.UserServiceURL.String())
	}
	if cfg.OrderServiceURL.String() != "http://orders.internal:9002" {
		t.Fatalf("OrderServiceURL = %q", cfg.OrderServiceURL.String())
	}
	if cfg.ProductServiceURL.String() != "http://products.internal:9003" {
		t.Fatalf("ProductServiceURL = %q", cfg.ProductServiceURL.String())
	}
	if cfg.UpstreamTimeout != 150*time.Millisecond {
		t.Fatalf("UpstreamTimeout = %s, want 150ms", cfg.UpstreamTimeout)
	}
	if cfg.RateLimitRequests != 7 {
		t.Fatalf("RateLimitRequests = %d, want 7", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow != 250*time.Millisecond {
		t.Fatalf("RateLimitWindow = %s, want 250ms", cfg.RateLimitWindow)
	}
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		env   string
		value string
	}{
		{name: "invalid user upstream URL", env: "USER_SERVICE_URL", value: "ftp://users.internal"},
		{name: "invalid timeout", env: "UPSTREAM_TIMEOUT_MS", value: "soon"},
		{name: "zero timeout", env: "UPSTREAM_TIMEOUT_MS", value: "0"},
		{name: "invalid rate limit requests", env: "RATE_LIMIT_REQUESTS", value: "many"},
		{name: "zero rate limit requests", env: "RATE_LIMIT_REQUESTS", value: "0"},
		{name: "invalid rate limit window", env: "RATE_LIMIT_WINDOW_MS", value: "later"},
		{name: "zero rate limit window", env: "RATE_LIMIT_WINDOW_MS", value: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv(tt.env, tt.value)

			if _, err := LoadConfig(); err == nil {
				t.Fatal("LoadConfig() error = nil, want error")
			}
		})
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"PORT",
		"USER_SERVICE_URL",
		"ORDER_SERVICE_URL",
		"PRODUCT_SERVICE_URL",
		"UPSTREAM_TIMEOUT_MS",
		"RATE_LIMIT_REQUESTS",
		"RATE_LIMIT_WINDOW_MS",
	} {
		t.Setenv(name, "")
	}
}
