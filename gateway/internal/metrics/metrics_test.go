package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestMetricsRender(t *testing.T) {
	m := New()
	m.ObserveRequest("users", "GET", 200, 5*time.Millisecond)
	m.ObserveRequest("users", "GET", 200, 5*time.Millisecond)
	m.ObserveRequest("orders", "POST", 502, time.Millisecond)
	m.IncRetry("users")
	m.SetUpstreamHealth("users", "http://a", true)
	m.SetUpstreamHealth("users", "http://b", false)
	m.SetUpstreamHealth("orders", "http://c", true)

	out := m.String()
	wants := []string{
		`gateway_requests_total{route="users",method="GET",code=200} 2`,
		`gateway_requests_total{route="orders",method="POST",code=502} 1`,
		`gateway_request_duration_seconds_count{route="users"} 2`,
		`gateway_upstream_retries_total{route="users"} 1`,
		`gateway_upstream_healthy{route="users",upstream="http://a"} 1`,
		`gateway_upstream_healthy{route="users",upstream="http://b"} 0`,
		`gateway_upstream_healthy{route="orders",upstream="http://c"} 1`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Fatalf("output missing %q\n%s", w, out)
		}
	}
}

func TestMetricsEmpty(t *testing.T) {
	m := New()
	out := m.String()
	if !strings.Contains(out, "gateway_requests_total") {
		t.Fatalf("expected header lines, got %s", out)
	}
}
