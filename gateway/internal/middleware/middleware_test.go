package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gateway/internal/httpx"
	"gateway/internal/metrics"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestChainOrder(t *testing.T) {
	var order []string
	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "before-"+name)
				next.ServeHTTP(w, r)
				order = append(order, "after-"+name)
			})
		}
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})
	New(mark("a"), mark("b")).Then(final).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

	want := []string{"before-a", "before-b", "handler", "after-b", "after-a"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("order got %v want %v", order, want)
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register("a", func() Middleware {
		return func(next http.Handler) http.Handler { return next }
	})
	mws, err := r.Build([]string{"a"})
	if err != nil || len(mws) != 1 {
		t.Fatalf("build got %v err=%v", mws, err)
	}
	if _, err := r.Build([]string{"missing"}); err == nil {
		t.Fatal("expected unknown middleware error")
	}
	names := r.Names()
	if len(names) != 1 || names[0] != "a" {
		t.Fatalf("names got %v", names)
	}
}

func TestNewRequestID(t *testing.T) {
	id := NewRequestID()
	if len(id) != 32 {
		t.Fatalf("id length got %d", len(id))
	}
	if NewRequestID() == id {
		t.Fatal("expected unique ids")
	}
}

func TestRequestIDGenerated(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = httpx.RequestIDFrom(r.Context())
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if seen == "" {
		t.Fatal("expected generated request id in context")
	}
	if rec.Header().Get("X-Request-ID") != seen {
		t.Fatalf("header/context mismatch: %q vs %q", rec.Header().Get("X-Request-ID"), seen)
	}
}

func TestRequestIDPreserved(t *testing.T) {
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") != "abc" {
		t.Fatalf("expected preserved id, got %q", rec.Header().Get("X-Request-ID"))
	}
}

func TestRecovery(t *testing.T) {
	h := Recovery(testLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("body got %s", rec.Body.String())
	}

	ok := Recovery(testLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec2 := httptest.NewRecorder()
	ok.ServeHTTP(rec2, httptest.NewRequest("GET", "/", nil))
	if rec2.Code != http.StatusNoContent {
		t.Fatalf("code got %d", rec2.Code)
	}

	nilLogger := Recovery(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	nilLogger.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestLogging(t *testing.T) {
	h := Logging(testLogger())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("code got %d", rec.Code)
	}

	nilLogger := Logging(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	nilLogger.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
}

func TestObserve(t *testing.T) {
	m := metrics.New()
	h := Observe(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if info := httpx.RouteInfoFrom(r.Context()); info != nil {
			info.ID = "users"
		}
		w.WriteHeader(http.StatusCreated)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/users", nil))

	out := m.String()
	if !strings.Contains(out, `gateway_requests_total{route="users",method="GET",code=201} 1`) {
		t.Fatalf("metrics missing route: %s", out)
	}
}
