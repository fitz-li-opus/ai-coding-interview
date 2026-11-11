package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// echoUpstream stands in for a backend: it reports its name and what it received.
func echoUpstream(t *testing.T, name string) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "%s %s %s?%s body=%s rid=%s", name, r.Method, r.URL.Path, r.URL.RawQuery, body, r.Header.Get("X-Request-ID"))
	}))
	t.Cleanup(s.Close)
	return s.URL
}

func newTestGateway(t *testing.T, routes map[string]string, timeout time.Duration) http.Handler {
	t.Helper()
	gw, err := newGateway(routes, timeout)
	if err != nil {
		t.Fatal(err)
	}
	return gw
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestRouting(t *testing.T) {
	gw := newTestGateway(t, map[string]string{
		"/api/users":    echoUpstream(t, "users"),
		"/api/orders":   echoUpstream(t, "orders"),
		"/api/products": echoUpstream(t, "products"),
	}, time.Second)

	tests := []struct {
		method, target, body string
		wantStatus           int
		wantBody             string
	}{
		{"GET", "/api/users", "", 200, "users GET /?"},
		{"GET", "/api/users/", "", 200, "users GET /?"},
		{"GET", "/api/users/1?x=1", "", 200, "users GET /1?x=1"},
		{"POST", "/api/orders/1001", `{"qty":2}`, 200, `orders POST /1001? body={"qty":2}`},
		{"DELETE", "/api/products/4/stock", "", 200, "products DELETE /4/stock?"},
		{"GET", "/api/usersX", "", 404, `"error"`},
		{"GET", "/api", "", 404, `"error"`},
		{"GET", "/", "", 404, `"error"`},
		{"GET", "/api/users/../orders", "", 301, "/api/orders"}, // cleaned before routing, never sent to users
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := serve(gw, httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body)))
			if rec.Code != tt.wantStatus || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf("got %d %q, want %d containing %q", rec.Code, rec.Body.String(), tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestUpstreamErrors(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	down.Close() // nothing listens there any more
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	gw := newTestGateway(t, map[string]string{"/api/down": down.URL, "/api/slow": slow.URL}, 50*time.Millisecond)

	for target, want := range map[string]int{"/api/down": http.StatusBadGateway, "/api/slow": http.StatusGatewayTimeout} {
		rec := serve(gw, httptest.NewRequest("GET", target, nil))
		if rec.Code != want || !strings.Contains(rec.Body.String(), `"error"`) {
			t.Errorf("%s: got %d %q, want %d with a JSON error", target, rec.Code, rec.Body.String(), want)
		}
	}
}

func TestStalledBodyIsCutOff(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	stalled := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush() // headers and some body are out, then the upstream hangs
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(stalled.Close)
	gw := httptest.NewServer(newTestGateway(t, map[string]string{"/api/stalled": stalled.URL}, 100*time.Millisecond))
	t.Cleanup(gw.Close)

	start := time.Now()
	resp, err := http.Get(gw.URL + "/api/stalled")
	if err == nil {
		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("a stalled upstream body should be cut off after the timeout; err=%v after %s", err, time.Since(start))
	}
	gw.Close() // waits for the handler, so its access-log line is written
	if !strings.Contains(logs.String(), "aborted=true") {
		t.Fatalf("a cut-off response must not be logged as a plain success, got %q", logs.String())
	}
}

func TestRequestIDCannotForgeLogFields(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	gw := newTestGateway(t, map[string]string{}, time.Second)

	req := httptest.NewRequest("GET", "/nope", nil)
	req.Header.Set("X-Request-ID", "x status=200 path=/admin")
	serve(gw, req)
	if !strings.Contains(buf.String(), `id="x status=200 path=/admin"`) {
		t.Fatalf("a client-supplied id must be quoted in the log, got %q", buf.String())
	}
}

func TestRequestID(t *testing.T) {
	gw := newTestGateway(t, map[string]string{"/api/users": echoUpstream(t, "users")}, time.Second)

	rec := serve(gw, httptest.NewRequest("GET", "/api/users", nil))
	id := rec.Header().Get("X-Request-ID")
	if id == "" || !strings.Contains(rec.Body.String(), "rid="+id) {
		t.Fatalf("generated id %q should be returned and forwarded upstream, body %q", id, rec.Body.String())
	}

	req := httptest.NewRequest("GET", "/api/users", nil)
	req.Header.Set("X-Request-ID", "abc123")
	rec = serve(gw, req)
	if got := rec.Header().Get("X-Request-ID"); got != "abc123" || !strings.Contains(rec.Body.String(), "rid=abc123") {
		t.Fatalf("client id should be kept: header %q, body %q", got, rec.Body.String())
	}
}

func TestHealthAndConfig(t *testing.T) {
	gw := newTestGateway(t, map[string]string{}, time.Second)
	if rec := serve(gw, httptest.NewRequest("GET", "/health", nil)); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"healthy"`) {
		t.Fatalf("gateway health: got %d %q", rec.Code, rec.Body.String())
	}
	if _, err := newGateway(map[string]string{"/api/users": "localhost:8081"}, time.Second); err == nil {
		t.Fatal("an upstream without scheme should be rejected at startup")
	}
}
