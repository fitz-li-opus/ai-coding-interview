package router

import (
	"net/http/httptest"
	"testing"

	"ai-coding-interview/gateway/internal/config"
)

func TestMatchPrecedenceAndPathBoundary(t *testing.T) {
	routeMatcher := New([]config.RouteConfig{
		{ID: "fallback", PathPrefix: "/api", Upstream: "fallback"},
		{ID: "users-path", PathPrefix: "/api/users", Upstream: "users"},
		{ID: "users-host", Hosts: []string{"users.internal.test"}, PathPrefix: "/", Upstream: "users"},
	})

	tests := []struct {
		name string
		host string
		path string
		want string
	}{
		{name: "host wins", host: "Users.Internal.Test:8443", path: "/api/orders", want: "users-host"},
		{name: "longest prefix", host: "localhost", path: "/api/users/123", want: "users-path"},
		{name: "fallback", host: "localhost", path: "/api/orders", want: "fallback"},
		{name: "segment boundary", host: "localhost", path: "/api/users2", want: "fallback"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "https://"+test.host+test.path, nil)
			request.Host = test.host
			matched := routeMatcher.Match(request)
			if matched == nil || matched.ID != test.want {
				t.Fatalf("Match() = %#v, want %q", matched, test.want)
			}
		})
	}
}

func TestStripPrefix(t *testing.T) {
	tests := map[string]string{
		"/api/users":     "/",
		"/api/users/123": "/123",
		"/":              "/",
	}
	for input, want := range tests {
		prefix := "/api/users"
		if input == "/" {
			prefix = "/"
		}
		if got := StripPrefix(input, prefix); got != want {
			t.Fatalf("StripPrefix(%q, %q) = %q, want %q", input, prefix, got, want)
		}
	}
}
