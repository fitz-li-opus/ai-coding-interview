package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGatewayAddress  = ":8080"
	defaultUserServiceURL  = "http://localhost:8081"
	defaultOrderServiceURL = "http://localhost:8082"
	defaultProductURL      = "http://localhost:8083"
	defaultUpstreamTimeout = 5 * time.Second
	defaultMaxRequestBody  = int64(1 << 20)
	maxUpstreamTimeout     = 60 * time.Second
	maxRequestBodyLimit    = int64(10 << 20)
)

type Config struct {
	Address           string
	UserServiceURL    *url.URL
	OrderServiceURL   *url.URL
	ProductServiceURL *url.URL
	UpstreamTimeout   time.Duration
	MaxRequestBody    int64
}

func LoadConfig(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("environment lookup is required")
	}

	address := valueOrDefault(getenv("GATEWAY_ADDR"), defaultGatewayAddress)
	if err := validateListenAddress(address); err != nil {
		return Config{}, fmt.Errorf("GATEWAY_ADDR: %w", err)
	}

	userServiceURL, err := parseURL(valueOrDefault(getenv("USER_SERVICE_URL"), defaultUserServiceURL))
	if err != nil {
		return Config{}, fmt.Errorf("USER_SERVICE_URL: %w", err)
	}
	orderServiceURL, err := parseURL(valueOrDefault(getenv("ORDER_SERVICE_URL"), defaultOrderServiceURL))
	if err != nil {
		return Config{}, fmt.Errorf("ORDER_SERVICE_URL: %w", err)
	}
	productServiceURL, err := parseURL(valueOrDefault(getenv("PRODUCT_SERVICE_URL"), defaultProductURL))
	if err != nil {
		return Config{}, fmt.Errorf("PRODUCT_SERVICE_URL: %w", err)
	}

	upstreamTimeout := defaultUpstreamTimeout
	if value := getenv("UPSTREAM_TIMEOUT"); value != "" {
		upstreamTimeout, err = time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("UPSTREAM_TIMEOUT: invalid duration")
		}
	}
	if upstreamTimeout <= 0 || upstreamTimeout > maxUpstreamTimeout {
		return Config{}, fmt.Errorf("UPSTREAM_TIMEOUT: must be greater than zero and at most %s", maxUpstreamTimeout)
	}

	maxRequestBody := defaultMaxRequestBody
	if value := getenv("MAX_REQUEST_BODY_BYTES"); value != "" {
		maxRequestBody, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_REQUEST_BODY_BYTES: must be an integer")
		}
	}
	if maxRequestBody <= 0 || maxRequestBody > maxRequestBodyLimit {
		return Config{}, fmt.Errorf("MAX_REQUEST_BODY_BYTES: must be between 1 and %d", maxRequestBodyLimit)
	}

	return Config{
		Address:           address,
		UserServiceURL:    userServiceURL,
		OrderServiceURL:   orderServiceURL,
		ProductServiceURL: productServiceURL,
		UpstreamTimeout:   upstreamTimeout,
		MaxRequestBody:    maxRequestBody,
	}, nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("credentials are not allowed")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, fmt.Errorf("query is not allowed")
	}
	if parsed.Fragment != "" {
		return nil, fmt.Errorf("fragment is not allowed")
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return nil, fmt.Errorf("port is required after colon")
	}
	if portText := parsed.Port(); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("port must be between 1 and 65535")
		}
	}
	return parsed, nil
}

func validateListenAddress(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("must include a valid host and port")
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("host contains whitespace")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}
