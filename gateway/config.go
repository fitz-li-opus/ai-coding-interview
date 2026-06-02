package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	defaultUserServiceURL    = "http://localhost:8081"
	defaultOrderServiceURL   = "http://localhost:8082"
	defaultProductServiceURL = "http://localhost:8083"
	defaultUpstreamTimeoutMS = "2000"
	defaultRateLimitRequests = "100"
	defaultRateLimitWindowMS = "60000"
)

type Config struct {
	Port              string
	UserServiceURL    *url.URL
	OrderServiceURL   *url.URL
	ProductServiceURL *url.URL
	UpstreamTimeout   time.Duration
	RateLimitRequests int
	RateLimitWindow   time.Duration
}

func LoadConfig() (Config, error) {
	userServiceURL, err := parseHTTPURL("USER_SERVICE_URL", envOrDefault("USER_SERVICE_URL", defaultUserServiceURL))
	if err != nil {
		return Config{}, err
	}
	orderServiceURL, err := parseHTTPURL("ORDER_SERVICE_URL", envOrDefault("ORDER_SERVICE_URL", defaultOrderServiceURL))
	if err != nil {
		return Config{}, err
	}
	productServiceURL, err := parseHTTPURL("PRODUCT_SERVICE_URL", envOrDefault("PRODUCT_SERVICE_URL", defaultProductServiceURL))
	if err != nil {
		return Config{}, err
	}
	upstreamTimeoutMS, err := parsePositiveInt("UPSTREAM_TIMEOUT_MS", envOrDefault("UPSTREAM_TIMEOUT_MS", defaultUpstreamTimeoutMS))
	if err != nil {
		return Config{}, err
	}
	rateLimitRequests, err := parsePositiveInt("RATE_LIMIT_REQUESTS", envOrDefault("RATE_LIMIT_REQUESTS", defaultRateLimitRequests))
	if err != nil {
		return Config{}, err
	}
	rateLimitWindowMS, err := parsePositiveInt("RATE_LIMIT_WINDOW_MS", envOrDefault("RATE_LIMIT_WINDOW_MS", defaultRateLimitWindowMS))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:              envOrDefault("PORT", defaultPort),
		UserServiceURL:    userServiceURL,
		OrderServiceURL:   orderServiceURL,
		ProductServiceURL: productServiceURL,
		UpstreamTimeout:   time.Duration(upstreamTimeoutMS) * time.Millisecond,
		RateLimitRequests: rateLimitRequests,
		RateLimitWindow:   time.Duration(rateLimitWindowMS) * time.Millisecond,
	}, nil
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func parseHTTPURL(name, value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid URL: %w", name, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%s must use http or https", name)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("%s must include a host", name)
	}
	return parsed, nil
}

func parsePositiveInt(name, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return parsed, nil
}
