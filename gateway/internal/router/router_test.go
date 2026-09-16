package router

import (
	"testing"

	"gateway/internal/config"
)

func route(id, prefix string, methods ...string) *config.Route {
	return &config.Route{ID: id, Match: config.Match{Prefix: prefix, Methods: methods}}
}

func TestNewSortsByPrefixLength(t *testing.T) {
	r := New([]*config.Route{
		route("short", "/api"),
		route("long", "/api/users"),
	})
	got := r.Routes()
	if got[0].ID != "long" || got[1].ID != "short" {
		t.Fatalf("unexpected order: %s, %s", got[0].ID, got[1].ID)
	}
}

func TestMatchLongestPrefix(t *testing.T) {
	r := New([]*config.Route{
		route("api", "/api"),
		route("users", "/api/users"),
	})
	got, ok, _ := r.Match("GET", "/api/users/1")
	if !ok || got.ID != "users" {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

func TestMatchMethodAllowedCaseInsensitive(t *testing.T) {
	r := New([]*config.Route{route("users", "/api/users", "GET", "post")})
	got, ok, _ := r.Match("POST", "/api/users")
	if !ok || got.ID != "users" {
		t.Fatalf("got %v ok=%v", got, ok)
	}
}

func TestMatchMethodNotAllowed(t *testing.T) {
	r := New([]*config.Route{route("users", "/api/users", "post", "get")})
	got, ok, allowed := r.Match("DELETE", "/api/users")
	if got == nil || ok {
		t.Fatalf("expected method not allowed, got %v ok=%v", got, ok)
	}
	if len(allowed) != 2 || allowed[0] != "GET" || allowed[1] != "POST" {
		t.Fatalf("allowed got %v", allowed)
	}
}

func TestMatchNoRoute(t *testing.T) {
	r := New([]*config.Route{route("users", "/api/users")})
	got, ok, allowed := r.Match("GET", "/nope")
	if got != nil || ok || allowed != nil {
		t.Fatalf("expected no match, got %v ok=%v allowed=%v", got, ok, allowed)
	}
}

func TestMatchNoMethodsAllowsAll(t *testing.T) {
	r := New([]*config.Route{route("users", "/api/users")})
	if _, ok, _ := r.Match("PATCH", "/api/users"); !ok {
		t.Fatal("expected allow all")
	}
}

func TestHasPrefixBoundaries(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		want   bool
	}{
		{"/api/users", "/api/users", true},
		{"/api/users/1", "/api/users", true},
		{"/api/usersx", "/api/users", false},
		{"/anything", "/", true},
		{"/anything", "", true},
		{"/api/users", "/api/users/", true},
		{"/other", "/api", false},
	}
	for _, tc := range tests {
		if got := hasPrefix(tc.path, tc.prefix); got != tc.want {
			t.Fatalf("hasPrefix(%q,%q) got %v want %v", tc.path, tc.prefix, got, tc.want)
		}
	}
}

func TestRewrite(t *testing.T) {
	tests := []struct {
		name  string
		strip string
		path  string
		want  string
	}{
		{name: "no strip", strip: "", path: "/api/users/1", want: "/api/users/1"},
		{name: "root strip", strip: "/", path: "/api/users", want: "/api/users"},
		{name: "exact strip", strip: "/api", path: "/api", want: "/"},
		{name: "sub path", strip: "/api", path: "/api/users/1", want: "/users/1"},
		{name: "trailing slash strip", strip: "/api/", path: "/api/users", want: "/users"},
		{name: "no prefix match", strip: "/api", path: "/other", want: "/other"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Rewrite(&config.Route{StripPrefix: tc.strip}, tc.path)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
