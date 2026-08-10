package main

import (
	"errors"
	"net/url"
	"testing"
)

func TestNewRouteTableAcceptsValidRoutes(t *testing.T) {
	routes := []RouteSpec{
		{Name: "user-service", Prefix: "/api/users", Target: mustParseURL(t, "http://localhost:8081")},
		{Name: "order-service", Prefix: "/api/orders", Target: mustParseURL(t, "http://localhost:8082")},
		{Name: "product-service", Prefix: "/api/products", Target: mustParseURL(t, "http://localhost:8083")},
	}

	if _, err := NewRouteTable(routes); err != nil {
		t.Fatalf("NewRouteTable() error = %v", err)
	}
}

func TestNewRouteTableRejectsInvalidRoutes(t *testing.T) {
	valid := RouteSpec{
		Name:   "user-service",
		Prefix: "/api/users",
		Target: mustParseURL(t, "http://localhost:8081"),
	}

	tests := []struct {
		name   string
		routes []RouteSpec
	}{
		{name: "empty route table", routes: nil},
		{name: "empty name", routes: []RouteSpec{{Prefix: valid.Prefix, Target: valid.Target}}},
		{name: "blank name", routes: []RouteSpec{{Name: "  ", Prefix: valid.Prefix, Target: valid.Target}}},
		{name: "name with surrounding whitespace", routes: []RouteSpec{{Name: " user-service", Prefix: valid.Prefix, Target: valid.Target}}},
		{name: "empty prefix", routes: []RouteSpec{{Name: valid.Name, Target: valid.Target}}},
		{name: "relative prefix", routes: []RouteSpec{{Name: valid.Name, Prefix: "api/users", Target: valid.Target}}},
		{name: "root prefix", routes: []RouteSpec{{Name: valid.Name, Prefix: "/", Target: valid.Target}}},
		{name: "trailing slash", routes: []RouteSpec{{Name: valid.Name, Prefix: "/api/users/", Target: valid.Target}}},
		{name: "query in prefix", routes: []RouteSpec{{Name: valid.Name, Prefix: "/api/users?admin=true", Target: valid.Target}}},
		{name: "fragment in prefix", routes: []RouteSpec{{Name: valid.Name, Prefix: "/api/users#admin", Target: valid.Target}}},
		{name: "dot segment in prefix", routes: []RouteSpec{{Name: valid.Name, Prefix: "/api/../users", Target: valid.Target}}},
		{name: "encoded slash in prefix", routes: []RouteSpec{{Name: valid.Name, Prefix: "/api%2fusers", Target: valid.Target}}},
		{name: "nil target", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix}}},
		{name: "relative target", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: mustParseURL(t, "/users")}}},
		{name: "unsupported target scheme", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: mustParseURL(t, "file://localhost/users")}}},
		{name: "target credentials", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: mustParseURL(t, "http://admin:secret@localhost:8081")}}},
		{name: "empty target query", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: mustParseURL(t, "http://localhost:8081?")}}},
		{name: "dot segment in target path", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: mustParseURL(t, "http://localhost:8081/base/../admin")}}},
		{name: "encoded slash in target path", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: mustParseURL(t, "http://localhost:8081/base%2fadmin")}}},
		{name: "inconsistent target raw path", routes: []RouteSpec{{Name: valid.Name, Prefix: valid.Prefix, Target: &url.URL{Scheme: "http", Host: "localhost:8081", Path: "/safe", RawPath: "/different"}}}},
		{
			name: "duplicate name",
			routes: []RouteSpec{
				valid,
				{Name: valid.Name, Prefix: "/api/accounts", Target: mustParseURL(t, "http://localhost:8084")},
			},
		},
		{
			name: "duplicate prefix",
			routes: []RouteSpec{
				valid,
				{Name: "account-service", Prefix: valid.Prefix, Target: mustParseURL(t, "http://localhost:8084")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRouteTable(tt.routes)
			if err == nil {
				t.Fatal("NewRouteTable() error = nil, want validation error")
			}
		})
	}
}

func TestNewRouteTableCopiesRouteConfiguration(t *testing.T) {
	target := mustParseURL(t, "http://localhost:8081")
	routes := []RouteSpec{{Name: "user-service", Prefix: "/api/users", Target: target}}

	table, err := NewRouteTable(routes)
	if err != nil {
		t.Fatalf("NewRouteTable() error = %v", err)
	}

	routes[0].Name = "attacker-service"
	routes[0].Prefix = "/api/attacker"
	target.Host = "attacker.internal"

	got, _, matched := table.Match("/api/users")
	if !matched {
		t.Fatal("Match(/api/users) = false after caller mutation, want true")
	}
	if got.Name != "user-service" {
		t.Errorf("matched route name = %q, want %q", got.Name, "user-service")
	}
	if got.Target.Host != "localhost:8081" {
		t.Errorf("matched target host = %q, want %q", got.Target.Host, "localhost:8081")
	}

	got.Target.Host = "second-attacker.internal"
	again, _, matched := table.Match("/api/users")
	if !matched || again.Target.Host != "localhost:8081" {
		t.Errorf("route table exposed mutable target; second match = %#v, matched = %v", again, matched)
	}
}

func TestRouteTableMatchesPathBoundariesAndStripsPrefix(t *testing.T) {
	table, err := NewRouteTable([]RouteSpec{
		{Name: "user-service", Prefix: "/api/users", Target: mustParseURL(t, "http://localhost:8081")},
		{Name: "admin-service", Prefix: "/api/users/admin", Target: mustParseURL(t, "http://localhost:8084")},
		{Name: "order-service", Prefix: "/api/orders", Target: mustParseURL(t, "http://localhost:8082")},
	})
	if err != nil {
		t.Fatalf("NewRouteTable() error = %v", err)
	}

	tests := []struct {
		name         string
		path         string
		wantRoute    string
		wantUpstream string
		wantMatch    bool
	}{
		{name: "exact", path: "/api/users", wantRoute: "user-service", wantUpstream: "/", wantMatch: true},
		{name: "trailing slash", path: "/api/users/", wantRoute: "user-service", wantUpstream: "/", wantMatch: true},
		{name: "nested", path: "/api/users/42", wantRoute: "user-service", wantUpstream: "/42", wantMatch: true},
		{name: "repeated slash preserved", path: "/api/users//42", wantRoute: "user-service", wantUpstream: "//42", wantMatch: true},
		{name: "longest prefix wins", path: "/api/users/admin/audit", wantRoute: "admin-service", wantUpstream: "/audit", wantMatch: true},
		{name: "lookalike", path: "/api/users-v2", wantMatch: false},
		{name: "suffix without slash", path: "/api/usersXYZ", wantMatch: false},
		{name: "partial prefix", path: "/api/user", wantMatch: false},
		{name: "root", path: "/", wantMatch: false},
		{name: "unknown", path: "/api/products", wantMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRoute, gotUpstream, gotMatch := table.Match(tt.path)
			if gotMatch != tt.wantMatch {
				t.Fatalf("Match(%q) matched = %v, want %v", tt.path, gotMatch, tt.wantMatch)
			}
			if !tt.wantMatch {
				if gotRoute != (RouteSpec{}) || gotUpstream != "" {
					t.Errorf("unmatched result = (%#v, %q), want zero values", gotRoute, gotUpstream)
				}
				return
			}
			if gotRoute.Name != tt.wantRoute {
				t.Errorf("matched route = %q, want %q", gotRoute.Name, tt.wantRoute)
			}
			if gotUpstream != tt.wantUpstream {
				t.Errorf("upstream path = %q, want %q", gotUpstream, tt.wantUpstream)
			}
		})
	}
}

func TestNilRouteTableDoesNotMatch(t *testing.T) {
	var table *RouteTable
	gotRoute, gotUpstream, matched := table.Match("/api/users")
	if matched || gotRoute != (RouteSpec{}) || gotUpstream != "" {
		t.Errorf("nil table Match() = (%#v, %q, %v), want zero values", gotRoute, gotUpstream, matched)
	}
}

func TestValidateRequestPathAcceptsUnambiguousPaths(t *testing.T) {
	tests := []*url.URL{
		{Path: "/api/users"},
		{Path: "/api/users//42"},
		{Path: "/api/users/alice smith", RawPath: "/api/users/alice%20smith"},
		{Path: "/api/users/张三"},
	}

	for _, requestURL := range tests {
		t.Run(requestURL.EscapedPath(), func(t *testing.T) {
			if err := ValidateRequestPath(requestURL); err != nil {
				t.Errorf("ValidateRequestPath(%q) error = %v", requestURL.EscapedPath(), err)
			}
		})
	}
}

func TestValidateRequestPathRejectsAmbiguousPaths(t *testing.T) {
	tests := []struct {
		name       string
		requestURL *url.URL
	}{
		{name: "nil URL", requestURL: nil},
		{name: "relative path", requestURL: &url.URL{Path: "api/users"}},
		{name: "absolute form", requestURL: &url.URL{Scheme: "http", Host: "attacker.internal", Path: "/api/users"}},
		{name: "opaque URL", requestURL: &url.URL{Opaque: "//attacker.internal/api/users"}},
		{name: "NUL", requestURL: &url.URL{Path: "/api/users/a\x00b"}},
		{name: "backslash", requestURL: &url.URL{Path: `/api/users/a\b`}},
		{name: "encoded slash", requestURL: &url.URL{Path: "/api/users/a/b", RawPath: "/api/users/a%2Fb"}},
		{name: "encoded backslash", requestURL: &url.URL{Path: `/api/users/a\b`, RawPath: "/api/users/a%5Cb"}},
		{name: "dot segment", requestURL: &url.URL{Path: "/api/users/./42"}},
		{name: "parent segment", requestURL: &url.URL{Path: "/api/users/../admin"}},
		{name: "encoded parent segment", requestURL: &url.URL{Path: "/api/users/../admin", RawPath: "/api/users/%2e%2e/admin"}},
		{name: "double encoded parent segment", requestURL: &url.URL{Path: "/api/users/%2e%2e/admin", RawPath: "/api/users/%252e%252e/admin"}},
		{name: "double encoded slash", requestURL: &url.URL{Path: "/api/users/a%2fb", RawPath: "/api/users/a%252fb"}},
		{name: "inconsistent raw path", requestURL: &url.URL{Path: "/api/users/safe", RawPath: "/api/users/%2F"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequestPath(tt.requestURL)
			var invalid *InvalidRequestError
			if !errors.As(err, &invalid) {
				t.Fatalf("ValidateRequestPath() error = %T %v, want *InvalidRequestError", err, err)
			}
		})
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return parsed
}
