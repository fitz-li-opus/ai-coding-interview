package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyRewriteAndForwardedHeaderSanitization(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path":      r.URL.Path,
			"xff":       r.Header.Get("X-Forwarded-For"),
			"requestId": r.Header.Get("X-Request-Id"),
		})
	}))
	defer backend.Close()

	handler, err := NewHandler(Options{
		UserURL:               backend.URL,
		OrderURL:              backend.URL,
		ProductURL:            backend.URL,
		MaxBodyBytes:          1 << 20,
		UpstreamTimeout:       2 * time.Second,
		ReadyCheckTimeout:     1 * time.Second,
		ReadyCheckParallel:    2,
		RequestIDHeader:       "X-Request-Id",
		TrustForwardedHeaders: false,
	}, log.Default())
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users/profile", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-Request-Id", "req-123")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["path"] != "/profile" {
		t.Fatalf("path = %v, want /profile", body["path"])
	}

	gotXFF, _ := body["xff"].(string)
	if strings.Contains(gotXFF, "1.2.3.4") {
		t.Fatalf("xff trusted spoofed value unexpectedly: %q", gotXFF)
	}

	if body["requestId"] != "req-123" {
		t.Fatalf("request id = %v, want req-123", body["requestId"])
	}
}

func TestReadyzNotReadyWhenUpstreamDown(t *testing.T) {
	okBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer okBackend.Close()

	downBackend := "http://127.0.0.1:65535"

	handler, err := NewHandler(Options{
		UserURL:               okBackend.URL,
		OrderURL:              downBackend,
		ProductURL:            okBackend.URL,
		MaxBodyBytes:          1 << 20,
		UpstreamTimeout:       1 * time.Second,
		ReadyCheckTimeout:     400 * time.Millisecond,
		ReadyCheckParallel:    3,
		RequestIDHeader:       "X-Request-Id",
		TrustForwardedHeaders: false,
	}, log.Default())
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestBodyTooLarge(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	handler, err := NewHandler(Options{
		UserURL:               backend.URL,
		OrderURL:              backend.URL,
		ProductURL:            backend.URL,
		MaxBodyBytes:          8,
		UpstreamTimeout:       1 * time.Second,
		ReadyCheckTimeout:     1 * time.Second,
		ReadyCheckParallel:    2,
		RequestIDHeader:       "X-Request-Id",
		TrustForwardedHeaders: false,
	}, log.Default())
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/users/upload", strings.NewReader("0123456789abcdef"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}
