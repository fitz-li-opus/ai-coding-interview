package main

import (
	"net/url"
	"testing"
)

func TestMatchPrefix(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
		want   bool
	}{
		{"exact", "/api/users", "/api/users", true},
		{"trailing slash", "/api/users/", "/api/users", true},
		{"sub path", "/api/users/123", "/api/users", true},
		{"nested sub path", "/api/users/123/orders", "/api/users", true},
		{"segment boundary", "/api/usersecret", "/api/users", false},
		{"shorter than prefix", "/api/user", "/api/users", false},
		{"different subtree", "/api/orders/1", "/api/users", false},
		{"catch-all prefix", "/anything", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchPrefix(tt.path, tt.prefix); got != tt.want {
				t.Errorf("matchPrefix(%q, %q) = %v, want %v", tt.path, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
		wantErr bool
	}{
		{"trailing wildcard", "/api/users/**", "/api/users", false},
		{"no wildcard", "/api/users", "/api/users", false},
		{"trailing slash", "/api/users/", "/api/users", false},
		{"root catch-all", "/**", "", false},
		{"missing leading slash", "api/users/**", "", true},
		{"mid-path wildcard", "/api/*/users", "", true},
		{"single star suffix", "/api/users/*", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePattern(tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePattern(%q) error = %v, wantErr %v", tt.pattern, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parsePattern(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestRouterMatchPrefersLongestPrefix(t *testing.T) {
	cfg := &Config{
		Listen:           ":8080",
		DefaultTimeoutMS: 1000,
		Routes: []RouteConfig{
			// Deliberately configured shortest-first: the router must not rely
			// on config order to pick the more specific route.
			{Path: "/api/users/**", Target: "http://localhost:8081"},
			{Path: "/api/users/admin/**", Target: "http://localhost:9090"},
		},
	}
	router, err := NewRouter(cfg)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	tests := []struct {
		path       string
		wantPrefix string // "" means no match expected
	}{
		{"/api/users/1", "/api/users"},
		{"/api/users/admin", "/api/users/admin"},
		{"/api/users/admin/tokens", "/api/users/admin"},
		{"/api/usersecret", ""},
		{"/api/orders/1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			route := router.Match(tt.path)
			if tt.wantPrefix == "" {
				if route != nil {
					t.Fatalf("Match(%q) = %q, want no match", tt.path, route.Prefix)
				}
				return
			}
			if route == nil {
				t.Fatalf("Match(%q) = nil, want %q", tt.path, tt.wantPrefix)
			}
			if route.Prefix != tt.wantPrefix {
				t.Errorf("Match(%q) = %q, want %q", tt.path, route.Prefix, tt.wantPrefix)
			}
		})
	}
}

func TestRouterRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{"no routes", &Config{Routes: nil}},
		{"bad scheme", &Config{Routes: []RouteConfig{{Path: "/a/**", Target: "ftp://host"}}}},
		{"missing host", &Config{Routes: []RouteConfig{{Path: "/a/**", Target: "http://"}}}},
		{"duplicate prefix", &Config{Routes: []RouteConfig{
			{Path: "/a/**", Target: "http://localhost:1"},
			{Path: "/a", Target: "http://localhost:2"},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewRouter(tt.cfg); err == nil {
				t.Errorf("NewRouter(%s) = nil error, want error", tt.name)
			}
		})
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		name     string
		rawURL   string
		prefix   string
		wantPath string // URL.EscapedPath() after stripping
	}{
		{"exact match becomes root", "http://x/api/users", "/api/users", "/"},
		{"trailing slash becomes root", "http://x/api/users/", "/api/users", "/"},
		{"sub path", "http://x/api/users/123", "/api/users", "/123"},
		{"query untouched", "http://x/api/users/123?a=b", "/api/users", "/123"},
		{"encoding preserved", "http://x/api/users/a%2Fb", "/api/users", "/a%2Fb"},
		{"empty prefix is a no-op", "http://x/api/users", "", "/api/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("url.Parse: %v", err)
			}
			stripPrefix(u, tt.prefix)
			if got := u.EscapedPath(); got != tt.wantPath {
				t.Errorf("stripPrefix(%q, %q) path = %q, want %q", tt.rawURL, tt.prefix, got, tt.wantPath)
			}
		})
	}
}
