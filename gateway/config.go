package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config is the whole gateway configuration. Routing lives in data, not in code,
// so adding a backend never means touching a handler.
type Config struct {
	Listen      string        `json:"listen"`
	Timeouts    Timeouts      `json:"timeouts"`
	HealthCheck HealthCheck   `json:"health_check"`
	Routes      []RouteConfig `json:"routes"`
}

type Timeouts struct {
	// UpstreamMS bounds one full proxied request, dial to last byte of headers.
	UpstreamMS int `json:"upstream_ms"`
	// DialMS bounds establishing the TCP connection to a backend.
	DialMS int `json:"dial_ms"`
	// ReadHeaderMS bounds reading the request headers from a client (slowloris guard).
	ReadHeaderMS int `json:"read_header_ms"`
}

type HealthCheck struct {
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path"`
	IntervalMS int    `json:"interval_ms"`
	TimeoutMS  int    `json:"timeout_ms"`
}

type RouteConfig struct {
	Prefix string `json:"prefix"`
	// StripPrefix is a pointer so an omitted field still defaults to true.
	StripPrefix *bool    `json:"strip_prefix"`
	Targets     []string `json:"targets"`
}

func (rc RouteConfig) strip() bool { return rc.StripPrefix == nil || *rc.StripPrefix }

// Load reads, defaults and validates a config file. Unknown fields are rejected
// so a typo in the config fails at startup instead of silently doing nothing.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", path, err)
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	setDefault(&c.Listen, ":8080")
	setDefaultInt(&c.Timeouts.UpstreamMS, 5000)
	setDefaultInt(&c.Timeouts.DialMS, 2000)
	setDefaultInt(&c.Timeouts.ReadHeaderMS, 5000)
	setDefault(&c.HealthCheck.Path, "/health")
	setDefaultInt(&c.HealthCheck.IntervalMS, 5000)
	setDefaultInt(&c.HealthCheck.TimeoutMS, 2000)

	for i := range c.Routes {
		// "/api/users/" and "/api/users" must behave identically.
		p := c.Routes[i].Prefix
		if len(p) > 1 {
			c.Routes[i].Prefix = strings.TrimRight(p, "/")
		}
	}
}

func (c *Config) validate() error {
	if len(c.Routes) == 0 {
		return fmt.Errorf("no routes configured")
	}
	seen := make(map[string]bool, len(c.Routes))
	for _, rc := range c.Routes {
		if !strings.HasPrefix(rc.Prefix, "/") {
			return fmt.Errorf("route prefix %q must start with /", rc.Prefix)
		}
		if seen[rc.Prefix] {
			return fmt.Errorf("duplicate route prefix %q", rc.Prefix)
		}
		seen[rc.Prefix] = true

		if len(rc.Targets) == 0 {
			return fmt.Errorf("route %q has no targets", rc.Prefix)
		}
		for _, t := range rc.Targets {
			u, err := url.Parse(t)
			if err != nil {
				return fmt.Errorf("route %q target %q: %w", rc.Prefix, t, err)
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				return fmt.Errorf("route %q target %q: scheme must be http or https", rc.Prefix, t)
			}
			if u.Host == "" {
				return fmt.Errorf("route %q target %q: missing host", rc.Prefix, t)
			}
		}
	}
	return nil
}

func setDefault(p *string, v string) {
	if *p == "" {
		*p = v
	}
}

func setDefaultInt(p *int, v int) {
	if *p <= 0 {
		*p = v
	}
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }
