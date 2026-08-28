package main

import "testing"

func TestRouterMatchesLongestPrefixOnSegmentBoundary(t *testing.T) {
	router := NewRouter([]*Route{
		{Prefix: "/api"},
		{Prefix: "/api/users"},
		{Prefix: "/api/orders"},
		{Prefix: "/api/products"},
	})

	tests := []struct {
		path string
		want string // "" means no route
	}{
		{"/api/users", "/api/users"},
		{"/api/users/", "/api/users"},
		{"/api/users/123", "/api/users"},
		{"/api/users/123/orders", "/api/users"},
		{"/api/orders/1001", "/api/orders"},
		{"/api/products", "/api/products"},
		{"/api", "/api"},
		{"/api/unknown", "/api"},
		// The boundary check: a longer path segment must not match a shorter route.
		{"/api/usersfoo", "/api"},
		{"/apifoo", ""},
		{"/", ""},
		{"/health", ""},
	}

	for _, tc := range tests {
		got := ""
		if route := router.Match(tc.path); route != nil {
			got = route.Prefix
		}
		if got != tc.want {
			t.Errorf("Match(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestRouterCatchAllPrefix(t *testing.T) {
	router := NewRouter([]*Route{{Prefix: "/"}, {Prefix: "/api/users"}})

	if got := router.Match("/api/users/1").Prefix; got != "/api/users" {
		t.Errorf("specific route should win, got %q", got)
	}
	if got := router.Match("/anything").Prefix; got != "/" {
		t.Errorf("catch-all should match, got %q", got)
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct{ path, prefix, want string }{
		{"/api/users/123", "/api/users", "/123"},
		{"/api/users/", "/api/users", "/"},
		{"/api/users", "/api/users", "/"},
		{"/api/users/1/orders", "/api/users", "/1/orders"},
		{"/anything", "/", "/anything"},
	}
	for _, tc := range tests {
		if got := stripPrefix(tc.path, tc.prefix); got != tc.want {
			t.Errorf("stripPrefix(%q, %q) = %q, want %q", tc.path, tc.prefix, got, tc.want)
		}
	}
}

func TestRoutePickSkipsUnhealthyAndRoundRobins(t *testing.T) {
	route := &Route{Prefix: "/api/users", Upstreams: []*Upstream{
		mustUpstream(t, "http://a.invalid"),
		mustUpstream(t, "http://b.invalid"),
		mustUpstream(t, "http://c.invalid"),
	}}
	for _, u := range route.Upstreams {
		u.healthy.Store(true)
	}

	seen := map[string]int{}
	for i := 0; i < 6; i++ {
		seen[route.Pick().URL.Host]++
	}
	for host, n := range seen {
		if n != 2 {
			t.Errorf("round robin uneven: %s got %d of 6", host, n)
		}
	}

	route.Upstreams[0].healthy.Store(false)
	route.Upstreams[2].healthy.Store(false)
	for i := 0; i < 5; i++ {
		if got := route.Pick().URL.Host; got != "b.invalid" {
			t.Fatalf("expected only healthy upstream b.invalid, got %s", got)
		}
	}

	route.Upstreams[1].healthy.Store(false)
	if got := route.Pick(); got != nil {
		t.Errorf("expected nil when the whole pool is down, got %v", got.URL)
	}
}
