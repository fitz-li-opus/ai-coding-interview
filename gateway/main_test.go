package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gateway/internal/config"
	"gateway/internal/httpx"
	"gateway/internal/lb"
	"gateway/internal/router"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func mustInstance(t *testing.T, raw string) *lb.Instance {
	t.Helper()
	inst, err := lb.NewInstance(raw, 1)
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	return inst
}

func TestDispatcherRouting(t *testing.T) {
	route := &config.Route{
		ID:          "users",
		Match:       config.Match{Prefix: "/api/users", Methods: []string{"GET"}},
		StripPrefix: "/api",
	}
	rt := router.New([]*config.Route{route})

	var seenPath string
	bundle := &routeBundle{
		route:     route,
		instances: []*lb.Instance{mustInstance(t, "http://x")},
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenPath = r.URL.Path
			w.WriteHeader(http.StatusOK)
		}),
	}
	d := dispatcher(rt, map[string]*routeBundle{"users": bundle})

	ctx, info := httpx.WithRouteInfo(context.Background())
	req := httptest.NewRequest("GET", "/api/users/1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code got %d", rec.Code)
	}
	if seenPath != "/users/1" {
		t.Fatalf("rewritten path got %q", seenPath)
	}
	if info.ID != "users" {
		t.Fatalf("route info got %q", info.ID)
	}

	rec404 := httptest.NewRecorder()
	d.ServeHTTP(rec404, httptest.NewRequest("GET", "/nope", nil))
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("404 code got %d", rec404.Code)
	}

	rec405 := httptest.NewRecorder()
	d.ServeHTTP(rec405, httptest.NewRequest("POST", "/api/users", nil))
	if rec405.Code != http.StatusMethodNotAllowed {
		t.Fatalf("405 code got %d", rec405.Code)
	}
	if rec405.Header().Get("Allow") != "GET" {
		t.Fatalf("allow got %q", rec405.Header().Get("Allow"))
	}
}

func TestDispatcherMissingBundle(t *testing.T) {
	rt := router.New([]*config.Route{{ID: "x", Match: config.Match{Prefix: "/x"}}})
	d := dispatcher(rt, map[string]*routeBundle{})
	rec := httptest.NewRecorder()
	d.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code got %d", rec.Code)
	}
}

func TestHasHealthy(t *testing.T) {
	inst := mustInstance(t, "http://x")
	b := &routeBundle{instances: []*lb.Instance{inst}}
	if !b.hasHealthy() {
		t.Fatal("expected healthy")
	}
	inst.MarkUnhealthy()
	if b.hasHealthy() {
		t.Fatal("expected not healthy")
	}
	empty := &routeBundle{}
	if empty.hasHealthy() {
		t.Fatal("expected false for empty instances")
	}
}

func TestReadinessHandler(t *testing.T) {
	route := &config.Route{ID: "users"}
	inst := mustInstance(t, "http://x")

	ready := readinessHandler([]*config.Route{route}, map[string]*routeBundle{
		"users": {route: route, instances: []*lb.Instance{inst}},
	})
	rec := httptest.NewRecorder()
	ready(rec, httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("ready code got %d", rec.Code)
	}

	inst.MarkUnhealthy()
	notReady := readinessHandler([]*config.Route{route}, map[string]*routeBundle{
		"users": {route: route, instances: []*lb.Instance{inst}},
	})
	rec2 := httptest.NewRecorder()
	notReady(rec2, httptest.NewRequest("GET", "/readyz", nil))
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready code got %d", rec2.Code)
	}

	missing := readinessHandler([]*config.Route{route}, map[string]*routeBundle{})
	rec3 := httptest.NewRecorder()
	missing(rec3, httptest.NewRequest("GET", "/readyz", nil))
	if rec3.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing bundle code got %d", rec3.Code)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func waitFor(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

func TestRunLifecycle(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"service":"user-service"}`)
	}))
	defer up.Close()

	port := freePort(t)
	cfgPath := writeConfigFile(t, fmt.Sprintf(`{
		"listen": "127.0.0.1:%d",
		"routes": [{
			"id": "users",
			"match": {"prefix": "/api/users", "methods": ["GET"]},
			"upstreams": [{"url": %q}],
			"strip_prefix": "/api"
		}]
	}`, port, up.URL))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfgPath, testLogger()) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitFor(t, base+"/healthz")

	resp, err := http.Get(base + "/readyz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz resp=%v err=%v", resp, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	resp, err = http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("metrics err=%v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "gateway_requests_total") {
		t.Fatalf("metrics body missing counters: %s", body)
	}

	resp, err = http.Get(base + "/api/users/1")
	if err != nil {
		t.Fatalf("proxied request err=%v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "user-service") {
		t.Fatalf("proxied resp code=%d body=%s", resp.StatusCode, body)
	}

	resp, err = http.Get(base + "/nope")
	if err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("404 resp=%v err=%v", resp, err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("run did not shut down in time")
	}
}

func TestRunConfigError(t *testing.T) {
	if err := run(context.Background(), filepath.Join(t.TempDir(), "missing.json"), testLogger()); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	cfgPath := writeConfigFile(t, fmt.Sprintf(`{
		"listen": "127.0.0.1:%d",
		"routes": [{
			"id": "users",
			"match": {"prefix": "/api/users"},
			"upstreams": [{"url": "http://127.0.0.1:1"}]
		}]
	}`, port))

	if err := run(context.Background(), cfgPath, testLogger()); err == nil {
		t.Fatal("expected listen error")
	}
}
