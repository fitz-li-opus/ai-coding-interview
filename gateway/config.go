package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Config is the on-disk gateway configuration; see routes.json.
type Config struct {
	Listen           string        `json:"listen"`
	DefaultTimeoutMS int           `json:"default_timeout_ms"`
	Routes           []RouteConfig `json:"routes"`
}

// RouteConfig is a single routing rule.
//
// StripPrefix is a pointer so that an omitted field can be told apart from an
// explicit false. Omitted means "strip", which is the documented default.
type RouteConfig struct {
	Path        string `json:"path"`
	Target      string `json:"target"`
	StripPrefix *bool  `json:"strip_prefix"`
	TimeoutMS   int    `json:"timeout_ms"` // 0 inherits default_timeout_ms
}

func (rc RouteConfig) stripPrefix() bool {
	return rc.StripPrefix == nil || *rc.StripPrefix
}

// LoadConfig reads path. A missing file is not an error: the caller gets the
// built-in defaults and fromFile=false, so `go run .` works with no setup. The
// caller is expected to log which of the two it got.
func LoadConfig(path string) (cfg *Config, fromFile bool, err error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultConfig(), false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	cfg = &Config{}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// A typo in a config key should fail loudly at startup rather than be
	// silently ignored and misbehave under load.
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.Listen == "" {
		cfg.Listen = defaultListen
	}
	if cfg.DefaultTimeoutMS <= 0 {
		cfg.DefaultTimeoutMS = defaultTimeoutMS
	}
	return cfg, true, nil
}

func defaultConfig() *Config {
	return &Config{
		Listen:           defaultListen,
		DefaultTimeoutMS: defaultTimeoutMS,
		Routes: []RouteConfig{
			{Path: "/api/users/**", Target: "http://localhost:8081"},
			{Path: "/api/orders/**", Target: "http://localhost:8082"},
			{Path: "/api/products/**", Target: "http://localhost:8083"},
		},
	}
}
