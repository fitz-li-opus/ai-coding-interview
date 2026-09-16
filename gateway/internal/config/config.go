package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Duration is a time.Duration that unmarshals from either a Go duration
// string (e.g. "2s") or a number of seconds.
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` {
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		parsed, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", str, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var secs float64
	if err := json.Unmarshal(b, &secs); err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = Duration(time.Duration(secs * float64(time.Second)))
	return nil
}

type Timeout struct {
	Connect        Duration `json:"connect"`
	ResponseHeader Duration `json:"response_header"`
	Idle           Duration `json:"idle"`
}

type Retry struct {
	Max      int      `json:"max"`
	Backoff  Duration `json:"backoff"`
	Jitter   bool     `json:"jitter"`
	Statuses []int    `json:"statuses"`
}

type Health struct {
	Path               string   `json:"path"`
	Interval           Duration `json:"interval"`
	Timeout            Duration `json:"timeout"`
	HealthyThreshold   int      `json:"healthy_threshold"`
	UnhealthyThreshold int      `json:"unhealthy_threshold"`
	Ejection           Duration `json:"ejection"`
}

type Defaults struct {
	Timeout Timeout `json:"timeout"`
	Retry   Retry   `json:"retry"`
	Health  Health  `json:"health"`
}

type Upstream struct {
	URL    string `json:"url"`
	Weight int    `json:"weight"`
}

type Match struct {
	Prefix  string   `json:"prefix"`
	Methods []string `json:"methods"`
}

type Route struct {
	ID          string     `json:"id"`
	Match       Match      `json:"match"`
	Upstreams   []Upstream `json:"upstreams"`
	LB          string     `json:"lb"`
	StripPrefix string     `json:"strip_prefix"`
	Timeout     *Timeout   `json:"timeout"`
	Retry       *Retry     `json:"retry"`
	Health      *Health    `json:"health"`
	Middlewares []string   `json:"middlewares"`
}

type Config struct {
	Listen   string   `json:"listen"`
	Defaults Defaults `json:"defaults"`
	Routes   []Route  `json:"routes"`
}

func defaultConfig() Config {
	return Config{
		Listen: ":8080",
		Defaults: Defaults{
			Timeout: Timeout{
				Connect:        Duration(2 * time.Second),
				ResponseHeader: Duration(10 * time.Second),
				Idle:           Duration(60 * time.Second),
			},
			Retry: Retry{
				Max:      0,
				Backoff:  Duration(50 * time.Millisecond),
				Jitter:   true,
				Statuses: []int{502, 503, 504},
			},
			Health: Health{
				Path:               "/health",
				Interval:           Duration(5 * time.Second),
				Timeout:            Duration(2 * time.Second),
				HealthyThreshold:   2,
				UnhealthyThreshold: 3,
				Ejection:           Duration(5 * time.Second),
			},
		},
	}
}

// Load reads, defaults, env-overrides and validates the configuration file.
func Load(path string) (*Config, error) {
	cfg := defaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)
	applyEnv(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyDefaults(c *Config) {
	def := defaultConfig()

	if c.Listen == "" {
		c.Listen = def.Listen
	}
	mergeTimeout(&c.Defaults.Timeout, def.Defaults.Timeout)
	mergeRetry(&c.Defaults.Retry, def.Defaults.Retry)
	mergeHealth(&c.Defaults.Health, def.Defaults.Health)

	for i := range c.Routes {
		r := &c.Routes[i]
		if r.LB == "" {
			r.LB = "round_robin"
		}
		for j := range r.Upstreams {
			if r.Upstreams[j].Weight == 0 {
				r.Upstreams[j].Weight = 1
			}
		}
	}
}

func mergeTimeout(dst *Timeout, base Timeout) {
	if dst.Connect == 0 {
		dst.Connect = base.Connect
	}
	if dst.ResponseHeader == 0 {
		dst.ResponseHeader = base.ResponseHeader
	}
	if dst.Idle == 0 {
		dst.Idle = base.Idle
	}
}

func mergeRetry(dst *Retry, base Retry) {
	if dst.Backoff == 0 {
		dst.Backoff = base.Backoff
	}
	if len(dst.Statuses) == 0 {
		dst.Statuses = base.Statuses
	}
}

func mergeHealth(dst *Health, base Health) {
	if dst.Path == "" {
		dst.Path = base.Path
	}
	if dst.Interval == 0 {
		dst.Interval = base.Interval
	}
	if dst.Timeout == 0 {
		dst.Timeout = base.Timeout
	}
	if dst.HealthyThreshold == 0 {
		dst.HealthyThreshold = base.HealthyThreshold
	}
	if dst.UnhealthyThreshold == 0 {
		dst.UnhealthyThreshold = base.UnhealthyThreshold
	}
	if dst.Ejection == 0 {
		dst.Ejection = base.Ejection
	}
}

func envKey(id string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(id) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func applyEnv(c *Config) {
	if v := os.Getenv("GATEWAY_LISTEN"); v != "" {
		c.Listen = v
	}
	for i := range c.Routes {
		key := "GATEWAY_UPSTREAM_" + envKey(c.Routes[i].ID)
		v := os.Getenv(key)
		if v == "" {
			continue
		}
		var ups []Upstream
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			ups = append(ups, Upstream{URL: part, Weight: 1})
		}
		if len(ups) > 0 {
			c.Routes[i].Upstreams = ups
		}
	}
}

// Validate checks the configuration and returns the first problem found.
func (c *Config) Validate() error {
	if c.Listen == "" {
		return fmt.Errorf("config: listen address is empty")
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("config: no routes defined")
	}
	seen := map[string]bool{}
	for i := range c.Routes {
		r := &c.Routes[i]
		if r.ID == "" {
			return fmt.Errorf("config: route[%d] has empty id", i)
		}
		if seen[r.ID] {
			return fmt.Errorf("config: duplicate route id %q", r.ID)
		}
		seen[r.ID] = true
		if r.Match.Prefix == "" {
			return fmt.Errorf("config: route %q has empty prefix", r.ID)
		}
		if !strings.HasPrefix(r.Match.Prefix, "/") {
			return fmt.Errorf("config: route %q prefix must start with /", r.ID)
		}
		if len(r.Upstreams) == 0 {
			return fmt.Errorf("config: route %q has no upstreams", r.ID)
		}
		for j, u := range r.Upstreams {
			parsed, err := url.Parse(u.URL)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return fmt.Errorf("config: route %q upstream[%d] invalid url %q", r.ID, j, u.URL)
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return fmt.Errorf("config: route %q upstream[%d] unsupported scheme %q", r.ID, j, parsed.Scheme)
			}
			if u.Weight < 0 {
				return fmt.Errorf("config: route %q upstream[%d] negative weight", r.ID, j)
			}
		}
		switch r.LB {
		case "", "round_robin", "weighted":
		default:
			return fmt.Errorf("config: route %q unknown lb %q", r.ID, r.LB)
		}
		for _, m := range r.Match.Methods {
			if strings.TrimSpace(m) == "" {
				return fmt.Errorf("config: route %q has empty method", r.ID)
			}
		}
	}
	return nil
}

// TimeoutFor merges route-level timeout overrides over defaults.
func (c *Config) TimeoutFor(r *Route) Timeout {
	if r.Timeout == nil {
		return c.Defaults.Timeout
	}
	res := *r.Timeout
	mergeTimeout(&res, c.Defaults.Timeout)
	return res
}

// RetryFor merges route-level retry overrides over defaults.
func (c *Config) RetryFor(r *Route) Retry {
	if r.Retry == nil {
		return c.Defaults.Retry
	}
	res := *r.Retry
	mergeRetry(&res, c.Defaults.Retry)
	return res
}

// HealthFor merges route-level health overrides over defaults.
func (c *Config) HealthFor(r *Route) Health {
	if r.Health == nil {
		return c.Defaults.Health
	}
	res := *r.Health
	mergeHealth(&res, c.Defaults.Health)
	return res
}
