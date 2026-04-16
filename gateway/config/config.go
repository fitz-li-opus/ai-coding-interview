package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server         ServerConfig         `yaml:"server"`
	Routes         []RouteConfig        `yaml:"routes"`
	Auth           AuthConfig           `yaml:"auth"`
	RateLimit      RateLimitConfig      `yaml:"rate_limit"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
	HealthCheck    HealthCheckConfig    `yaml:"health_check"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type RouteConfig struct {
	Prefix      string `yaml:"prefix"`
	Upstream    string `yaml:"upstream"`
	StripPrefix bool   `yaml:"strip_prefix"`
}

type AuthConfig struct {
	Enabled bool     `yaml:"enabled"`
	APIKeys []string `yaml:"api_keys"`
}

type RateLimitConfig struct {
	Enabled bool    `yaml:"enabled"`
	Rate    float64 `yaml:"rate"`
	Burst   int     `yaml:"burst"`
}

type CircuitBreakerConfig struct {
	Enabled          bool `yaml:"enabled"`
	FailureThreshold int  `yaml:"failure_threshold"`
	SuccessThreshold int  `yaml:"success_threshold"`
	TimeoutSeconds   int  `yaml:"timeout_seconds"`
}

type HealthCheckConfig struct {
	IntervalSeconds int `yaml:"interval_seconds"`
	TimeoutSeconds  int `yaml:"timeout_seconds"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
