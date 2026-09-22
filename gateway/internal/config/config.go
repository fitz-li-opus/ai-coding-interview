package config

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultAddress           = ":8443"
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
	defaultMaxHeaderBytes    = 32 * 1024
	defaultMaxBodyBytes      = 1024 * 1024
	defaultRouteTimeout      = 3 * time.Second
	defaultHealthPath        = "/health"
	defaultHealthInterval    = 5 * time.Second
	defaultHealthTimeout     = time.Second
)

type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Upstreams []UpstreamConfig `yaml:"upstreams"`
	Routes    []RouteConfig    `yaml:"routes"`
}

type ServerConfig struct {
	Address           string        `yaml:"address"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	MaxHeaderBytes    int           `yaml:"max_header_bytes"`
	MaxBodyBytes      int64         `yaml:"max_body_bytes"`
}

type UpstreamConfig struct {
	ID        string       `yaml:"id"`
	Endpoints []string     `yaml:"endpoints"`
	Health    HealthConfig `yaml:"health"`
}

type HealthConfig struct {
	Path     string        `yaml:"path"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

type RouteConfig struct {
	ID         string          `yaml:"id"`
	Hosts      []string        `yaml:"hosts"`
	PathPrefix string          `yaml:"path_prefix"`
	Upstream   string          `yaml:"upstream"`
	Timeout    time.Duration   `yaml:"timeout"`
	RateLimit  RateLimitConfig `yaml:"rate_limit"`
}

type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()
	return Decode(file)
}

func Decode(reader io.Reader) (Config, error) {
	var cfg Config
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("decode config: multiple YAML documents are not allowed")
		}
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	applyDefaults(&cfg)
	if err := Validate(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Address == "" {
		cfg.Server.Address = defaultAddress
	}
	if cfg.Server.ReadHeaderTimeout == 0 {
		cfg.Server.ReadHeaderTimeout = defaultReadHeaderTimeout
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = defaultReadTimeout
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = defaultIdleTimeout
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = defaultShutdownTimeout
	}
	if cfg.Server.MaxHeaderBytes == 0 {
		cfg.Server.MaxHeaderBytes = defaultMaxHeaderBytes
	}
	if cfg.Server.MaxBodyBytes == 0 {
		cfg.Server.MaxBodyBytes = defaultMaxBodyBytes
	}

	for i := range cfg.Upstreams {
		health := &cfg.Upstreams[i].Health
		if health.Path == "" {
			health.Path = defaultHealthPath
		}
		if health.Interval == 0 {
			health.Interval = defaultHealthInterval
		}
		if health.Timeout == 0 {
			health.Timeout = defaultHealthTimeout
		}
	}

	for i := range cfg.Routes {
		if cfg.Routes[i].Timeout == 0 {
			cfg.Routes[i].Timeout = defaultRouteTimeout
		}
		for j := range cfg.Routes[i].Hosts {
			cfg.Routes[i].Hosts[j] = normalizeHost(cfg.Routes[i].Hosts[j])
		}
	}
}

func Validate(cfg *Config) error {
	if cfg.Server.ReadHeaderTimeout <= 0 || cfg.Server.ReadTimeout <= 0 || cfg.Server.IdleTimeout <= 0 || cfg.Server.ShutdownTimeout <= 0 {
		return errors.New("validate config: server timeouts must be positive")
	}
	if cfg.Server.MaxHeaderBytes <= 0 || cfg.Server.MaxBodyBytes <= 0 {
		return errors.New("validate config: request size limits must be positive")
	}
	if len(cfg.Upstreams) == 0 {
		return errors.New("validate config: at least one upstream is required")
	}
	if len(cfg.Routes) == 0 {
		return errors.New("validate config: at least one route is required")
	}

	upstreamIDs := make(map[string]struct{}, len(cfg.Upstreams))
	for _, upstream := range cfg.Upstreams {
		if upstream.ID == "" {
			return errors.New("validate config: upstream id is required")
		}
		if _, exists := upstreamIDs[upstream.ID]; exists {
			return fmt.Errorf("validate config: duplicate upstream id %q", upstream.ID)
		}
		upstreamIDs[upstream.ID] = struct{}{}
		if len(upstream.Endpoints) == 0 {
			return fmt.Errorf("validate config: upstream %q has no endpoints", upstream.ID)
		}
		if upstream.Health.Path == "" || !strings.HasPrefix(upstream.Health.Path, "/") {
			return fmt.Errorf("validate config: upstream %q health path must start with /", upstream.ID)
		}
		if upstream.Health.Interval <= 0 || upstream.Health.Timeout <= 0 {
			return fmt.Errorf("validate config: upstream %q health durations must be positive", upstream.ID)
		}

		seenEndpoints := make(map[string]struct{}, len(upstream.Endpoints))
		for _, endpoint := range upstream.Endpoints {
			if _, exists := seenEndpoints[endpoint]; exists {
				return fmt.Errorf("validate config: upstream %q contains duplicate endpoint %q", upstream.ID, endpoint)
			}
			seenEndpoints[endpoint] = struct{}{}
			if err := validateEndpoint(endpoint); err != nil {
				return fmt.Errorf("validate config: upstream %q endpoint %q: %w", upstream.ID, endpoint, err)
			}
		}
	}

	routeIDs := make(map[string]struct{}, len(cfg.Routes))
	matchKeys := make(map[string]string)
	for _, route := range cfg.Routes {
		if route.ID == "" {
			return errors.New("validate config: route id is required")
		}
		if _, exists := routeIDs[route.ID]; exists {
			return fmt.Errorf("validate config: duplicate route id %q", route.ID)
		}
		routeIDs[route.ID] = struct{}{}
		if _, exists := upstreamIDs[route.Upstream]; !exists {
			return fmt.Errorf("validate config: route %q references unknown upstream %q", route.ID, route.Upstream)
		}
		if route.PathPrefix == "" || !strings.HasPrefix(route.PathPrefix, "/") {
			return fmt.Errorf("validate config: route %q path_prefix must start with /", route.ID)
		}
		if route.PathPrefix != "/" && strings.HasSuffix(route.PathPrefix, "/") {
			return fmt.Errorf("validate config: route %q path_prefix must not end with /", route.ID)
		}
		if route.Timeout <= 0 {
			return fmt.Errorf("validate config: route %q timeout must be positive", route.ID)
		}
		if (route.RateLimit.RequestsPerSecond == 0) != (route.RateLimit.Burst == 0) {
			return fmt.Errorf("validate config: route %q rate limit requires both requests_per_second and burst", route.ID)
		}
		if route.RateLimit.RequestsPerSecond < 0 || route.RateLimit.Burst < 0 || math.IsNaN(route.RateLimit.RequestsPerSecond) || math.IsInf(route.RateLimit.RequestsPerSecond, 0) {
			return fmt.Errorf("validate config: route %q rate limit values must not be negative", route.ID)
		}

		hosts := route.Hosts
		if len(hosts) == 0 {
			hosts = []string{""}
		}
		seenHosts := make(map[string]struct{}, len(hosts))
		for _, host := range hosts {
			if host == "" && len(route.Hosts) > 0 {
				return fmt.Errorf("validate config: route %q contains an empty host", route.ID)
			}
			if strings.ContainsAny(host, "/ \\") || strings.Contains(host, ":") {
				return fmt.Errorf("validate config: route %q contains invalid host %q", route.ID, host)
			}
			if _, exists := seenHosts[host]; exists {
				return fmt.Errorf("validate config: route %q contains duplicate host %q", route.ID, host)
			}
			seenHosts[host] = struct{}{}
			key := host + "\x00" + route.PathPrefix
			if existing, exists := matchKeys[key]; exists {
				return fmt.Errorf("validate config: routes %q and %q have the same match", existing, route.ID)
			}
			matchKeys[key] = route.ID
		}
	}
	return nil
}

func validateEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	if parsed.Hostname() == "" {
		return errors.New("host is required")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return errors.New("userinfo, query, fragment, and escaped path are not allowed")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return errors.New("path is not allowed")
	}
	return nil
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
