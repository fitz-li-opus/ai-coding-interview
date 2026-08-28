package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustUpstream(t *testing.T, rawURL string) *Upstream {
	t.Helper()
	u, err := newUpstream(rawURL, http.DefaultTransport, testLogger())
	if err != nil {
		t.Fatalf("newUpstream(%q): %v", rawURL, err)
	}
	return u
}

// newTestGateway wires the same handler chain main() uses, minus the access log.
func newTestGateway(timeout time.Duration, routes ...*Route) http.Handler {
	g := &Gateway{router: NewRouter(routes), timeout: timeout, logger: testLogger()}
	return withRequestInfo(recoverPanic(testLogger(), g))
}

// echoBackend records what the upstream actually received.
type received struct {
	path   string
	query  string
	method string
	body   string
	header http.Header
}

func newEchoBackend(t *testing.T, got *received) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = received{
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			method: r.Method,
			body:   string(body),
			header: r.Header.Clone(),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"echo"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func healthyRoute(t *testing.T, prefix string, strip bool, targets ...string) *Route {
	t.Helper()
	route := &Route{Prefix: prefix, StripPrefix: strip}
	for _, target := range targets {
		u := mustUpstream(t, target)
		u.healthy.Store(true)
		route.Upstreams = append(route.Upstreams, u)
	}
	return route
}

func TestProxyStripsPrefixAndPreservesQuery(t *testing.T) {
	var got received
	backend := newEchoBackend(t, &got)
	gw := newTestGateway(2*time.Second, healthyRoute(t, "/api/users", true, backend.URL))

	req := httptest.NewRequest(http.MethodGet, "/api/users/123?limit=10", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if got.path != "/123" {
		t.Errorf("upstream path = %q, want /123", got.path)
	}
	if got.query != "limit=10" {
		t.Errorf("upstream query = %q, want limit=10", got.query)
	}
	if rec.Header().Get(headerRequestID) == "" {
		t.Error("response is missing X-Request-ID")
	}
}

func TestProxyKeepsPrefixWhenStripDisabled(t *testing.T) {
	var got received
	backend := newEchoBackend(t, &got)
	gw := newTestGateway(2*time.Second, healthyRoute(t, "/api/users", false, backend.URL))

	gw.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/users/123", nil))

	if got.path != "/api/users/123" {
		t.Errorf("upstream path = %q, want /api/users/123", got.path)
	}
}

func TestProxyForwardsBodyAndTracingHeaders(t *testing.T) {
	var got received
	backend := newEchoBackend(t, &got)
	gw := newTestGateway(2*time.Second, healthyRoute(t, "/api/orders", true, backend.URL))

	req := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(`{"id":1}`))
	req.Header.Set(headerRequestID, "trace-me")
	req.RemoteAddr = "203.0.113.7:5555"
	gw.ServeHTTP(httptest.NewRecorder(), req)

	if got.method != http.MethodPost || got.body != `{"id":1}` {
		t.Errorf("upstream saw %s %q, want POST with the body", got.method, got.body)
	}
	if id := got.header.Get(headerRequestID); id != "trace-me" {
		t.Errorf("upstream X-Request-ID = %q, want trace-me", id)
	}
	if xff := got.header.Get("X-Forwarded-For"); xff != "203.0.113.7" {
		t.Errorf("upstream X-Forwarded-For = %q, want 203.0.113.7", xff)
	}
	if got.header.Get("X-Forwarded-Proto") == "" {
		t.Error("upstream is missing X-Forwarded-Proto")
	}
}

func TestUnmatchedPathReturns404(t *testing.T) {
	gw := newTestGateway(time.Second, healthyRoute(t, "/api/users", true, "http://127.0.0.1:1"))

	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if body.Path != "/nope" || body.RequestID == "" {
		t.Errorf("error body = %+v, want the original path and a request id", body)
	}
}

func TestAllUpstreamsDownReturns503(t *testing.T) {
	route := healthyRoute(t, "/api/users", true, "http://127.0.0.1:1")
	route.Upstreams[0].healthy.Store(false)
	gw := newTestGateway(time.Second, route)

	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestRefusedUpstreamReturns502(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := backend.URL
	backend.Close() // nothing is listening any more

	gw := newTestGateway(2*time.Second, healthyRoute(t, "/api/users", true, url))

	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if body.Upstream != url {
		t.Errorf("error body upstream = %q, want %q", body.Upstream, url)
	}
}

func TestSlowUpstreamReturns504(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(backend.Close)

	gw := newTestGateway(50*time.Millisecond, healthyRoute(t, "/api/users", true, backend.URL))

	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 (body %s)", rec.Code, rec.Body)
	}
}

func TestGatewayHealthReportsUpstreams(t *testing.T) {
	route := healthyRoute(t, "/api/users", true, "http://127.0.0.1:1")
	gw := newTestGateway(time.Second, route)

	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	route.Upstreams[0].healthy.Store(false)
	rec = httptest.NewRecorder()
	gw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("degraded status = %d, want 503", rec.Code)
	}

	var report healthReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("health body is not JSON: %v", err)
	}
	if report.Status != "degraded" || report.Routes[0].Upstreams[0].Healthy {
		t.Errorf("report = %+v, want degraded with a down upstream", report)
	}
}
