package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig   `yaml:"server"`
	Gateway   GatewayConfig  `yaml:"gateway"`
	Upstreams UpstreamConfig `yaml:"upstreams"`
}

type ServerConfig struct {
	ListenAddr        string        `yaml:"listen_addr"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
}

type GatewayConfig struct {
	MaxBodyBytes          int64         `yaml:"max_body_bytes"`
	UpstreamTimeout       time.Duration `yaml:"upstream_timeout"`
	ReadyCheckTimeout     time.Duration `yaml:"ready_check_timeout"`
	ReadyCheckParallel    int           `yaml:"ready_check_parallel"`
	RequestIDHeader       string        `yaml:"request_id_header"`
	TrustForwardedHeaders bool          `yaml:"trust_forwarded_headers"`
}

type UpstreamConfig struct {
	Users    string `yaml:"users"`
	Orders   string `yaml:"orders"`
	Products string `yaml:"products"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			ListenAddr:        ":8080",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       90 * time.Second,
			ShutdownTimeout:   10 * time.Second,
			MaxHeaderBytes:    1 << 20,
		},
		Gateway: GatewayConfig{
			MaxBodyBytes:          10 << 20,
			UpstreamTimeout:       15 * time.Second,
			ReadyCheckTimeout:     2 * time.Second,
			ReadyCheckParallel:    3,
			RequestIDHeader:       "X-Request-Id",
			TrustForwardedHeaders: false,
		},
		Upstreams: UpstreamConfig{
			Users:    "http://localhost:8081",
			Orders:   "http://localhost:8082",
			Products: "http://localhost:8083",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse yaml config: %w", err)
	}

	applyEnvOverrides(&cfg)

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	for _, u := range []string{c.Upstreams.Users, c.Upstreams.Orders, c.Upstreams.Products} {
		if _, err := url.ParseRequestURI(u); err != nil {
			return fmt.Errorf("invalid upstream url %q: %w", u, err)
		}
	}
	if c.Gateway.ReadyCheckParallel <= 0 {
		return fmt.Errorf("ready_check_parallel must be > 0")
	}
	if strings.TrimSpace(c.Gateway.RequestIDHeader) == "" {
		return fmt.Errorf("request_id_header must not be empty")
	}
	return nil
}

func applyEnvOverrides(c *Config) {
	setString("GATEWAY_LISTEN_ADDR", &c.Server.ListenAddr)
	setString("USER_SERVICE_URL", &c.Upstreams.Users)
	setString("ORDER_SERVICE_URL", &c.Upstreams.Orders)
	setString("PRODUCT_SERVICE_URL", &c.Upstreams.Products)
	setString("GATEWAY_REQUEST_ID_HEADER", &c.Gateway.RequestIDHeader)

	if v := strings.TrimSpace(os.Getenv("GATEWAY_TRUST_FORWARDED_HEADERS")); v != "" {
		c.Gateway.TrustForwardedHeaders = strings.EqualFold(v, "true")
	}

	setDuration("GATEWAY_UPSTREAM_TIMEOUT", &c.Gateway.UpstreamTimeout)
	setDuration("GATEWAY_READY_CHECK_TIMEOUT", &c.Gateway.ReadyCheckTimeout)
	setDuration("GATEWAY_READ_HEADER_TIMEOUT", &c.Server.ReadHeaderTimeout)
	setDuration("GATEWAY_READ_TIMEOUT", &c.Server.ReadTimeout)
	setDuration("GATEWAY_WRITE_TIMEOUT", &c.Server.WriteTimeout)
	setDuration("GATEWAY_IDLE_TIMEOUT", &c.Server.IdleTimeout)
	setDuration("GATEWAY_SHUTDOWN_TIMEOUT", &c.Server.ShutdownTimeout)

	setInt("GATEWAY_READY_CHECK_PARALLEL", &c.Gateway.ReadyCheckParallel)
	setInt("GATEWAY_MAX_HEADER_BYTES", &c.Server.MaxHeaderBytes)
	setInt64("GATEWAY_MAX_BODY_BYTES", &c.Gateway.MaxBodyBytes)
}

func setString(key string, dst *string) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		*dst = v
	}
}

func setDuration(key string, dst *time.Duration) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return
	}
	*dst = v
}

func setInt(key string, dst *int) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return
	}
	*dst = v
}

func setInt64(key string, dst *int64) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return
	}
	*dst = v
}
