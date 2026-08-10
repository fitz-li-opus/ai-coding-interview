package main

import (
	"strings"
	"testing"
	"time"
)

func environment(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestLoadConfigUsesDefaults(t *testing.T) {
	cfg, err := LoadConfig(environment(nil))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Address != ":8080" {
		t.Errorf("Address = %q, want %q", cfg.Address, ":8080")
	}
	if got := cfg.UserServiceURL.String(); got != "http://localhost:8081" {
		t.Errorf("UserServiceURL = %q, want %q", got, "http://localhost:8081")
	}
	if got := cfg.OrderServiceURL.String(); got != "http://localhost:8082" {
		t.Errorf("OrderServiceURL = %q, want %q", got, "http://localhost:8082")
	}
	if got := cfg.ProductServiceURL.String(); got != "http://localhost:8083" {
		t.Errorf("ProductServiceURL = %q, want %q", got, "http://localhost:8083")
	}
	if cfg.UpstreamTimeout != 5*time.Second {
		t.Errorf("UpstreamTimeout = %s, want %s", cfg.UpstreamTimeout, 5*time.Second)
	}
	if cfg.MaxRequestBody != 1<<20 {
		t.Errorf("MaxRequestBody = %d, want %d", cfg.MaxRequestBody, 1<<20)
	}
}

func TestLoadConfigUsesEnvironmentOverrides(t *testing.T) {
	cfg, err := LoadConfig(environment(map[string]string{
		"GATEWAY_ADDR":           "127.0.0.1:9090",
		"USER_SERVICE_URL":       "https://users.internal:9443",
		"ORDER_SERVICE_URL":      "http://orders.internal:9080",
		"PRODUCT_SERVICE_URL":    "https://products.internal",
		"UPSTREAM_TIMEOUT":       "2500ms",
		"MAX_REQUEST_BODY_BYTES": "2048",
	}))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Address != "127.0.0.1:9090" {
		t.Errorf("Address = %q, want %q", cfg.Address, "127.0.0.1:9090")
	}
	if got := cfg.UserServiceURL.String(); got != "https://users.internal:9443" {
		t.Errorf("UserServiceURL = %q, want %q", got, "https://users.internal:9443")
	}
	if got := cfg.OrderServiceURL.String(); got != "http://orders.internal:9080" {
		t.Errorf("OrderServiceURL = %q, want %q", got, "http://orders.internal:9080")
	}
	if got := cfg.ProductServiceURL.String(); got != "https://products.internal" {
		t.Errorf("ProductServiceURL = %q, want %q", got, "https://products.internal")
	}
	if cfg.UpstreamTimeout != 2500*time.Millisecond {
		t.Errorf("UpstreamTimeout = %s, want %s", cfg.UpstreamTimeout, 2500*time.Millisecond)
	}
	if cfg.MaxRequestBody != 2048 {
		t.Errorf("MaxRequestBody = %d, want %d", cfg.MaxRequestBody, 2048)
	}
}

func TestLoadConfigRejectsInvalidListenAddresses(t *testing.T) {
	tests := []string{
		"localhost",
		"localhost:not-a-port",
		"localhost:0",
		"localhost:65536",
		"::1:8080",
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			_, err := LoadConfig(environment(map[string]string{"GATEWAY_ADDR": value}))
			assertConfigErrorContains(t, err, "GATEWAY_ADDR")
		})
	}
}

func TestLoadConfigRejectsUnsafeUpstreamURLs(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "relative", value: "/users"},
		{name: "missing host", value: "http:///users"},
		{name: "unsupported scheme", value: "file://localhost/tmp/users"},
		{name: "credentials", value: "http://admin:secret@localhost:8081"},
		{name: "query", value: "http://localhost:8081?admin=true"},
		{name: "empty query", value: "http://localhost:8081?"},
		{name: "fragment", value: "http://localhost:8081#internal"},
		{name: "zero port", value: "http://localhost:0"},
		{name: "empty port", value: "http://localhost:"},
		{name: "out of range port", value: "http://localhost:65536"},
	}

	for _, serviceKey := range []string{
		"USER_SERVICE_URL",
		"ORDER_SERVICE_URL",
		"PRODUCT_SERVICE_URL",
	} {
		for _, tt := range tests {
			t.Run(serviceKey+"/"+tt.name, func(t *testing.T) {
				_, err := LoadConfig(environment(map[string]string{serviceKey: tt.value}))
				assertConfigErrorContains(t, err, serviceKey)
			})
		}
	}
}

func TestLoadConfigRejectsInvalidUpstreamTimeouts(t *testing.T) {
	for _, value := range []string{"invalid", "0s", "-1s", "61s"} {
		t.Run(value, func(t *testing.T) {
			_, err := LoadConfig(environment(map[string]string{"UPSTREAM_TIMEOUT": value}))
			assertConfigErrorContains(t, err, "UPSTREAM_TIMEOUT")
		})
	}
}

func TestLoadConfigRejectsInvalidRequestBodyLimits(t *testing.T) {
	for _, value := range []string{"invalid", "0", "-1", "10485761"} {
		t.Run(value, func(t *testing.T) {
			_, err := LoadConfig(environment(map[string]string{"MAX_REQUEST_BODY_BYTES": value}))
			assertConfigErrorContains(t, err, "MAX_REQUEST_BODY_BYTES")
		})
	}
}

func TestLoadConfigRejectsNilEnvironmentLookup(t *testing.T) {
	_, err := LoadConfig(nil)
	assertConfigErrorContains(t, err, "environment lookup")
}

func assertConfigErrorContains(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("LoadConfig() error = nil, want error containing %q", field)
	}
	if !strings.Contains(err.Error(), field) {
		t.Errorf("LoadConfig() error = %q, want field %q", err, field)
	}
}
