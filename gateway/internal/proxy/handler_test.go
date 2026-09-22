package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-coding-interview/gateway/internal/config"
	"ai-coding-interview/gateway/internal/gateway"
	"ai-coding-interview/gateway/internal/middleware"
	"ai-coding-interview/gateway/internal/router"
	"ai-coding-interview/gateway/internal/upstream"
)

type backendRequest struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	Query        string `json:"query"`
	Body         string `json:"body"`
	RequestID    string `json:"request_id"`
	ForwardedFor string `json:"forwarded_for"`
}

func TestProxyPreservesRequestAndRewritesPathAndHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			panic(err)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(writer).Encode(backendRequest{
			Method:       request.Method,
			Path:         request.URL.Path,
			Query:        request.URL.RawQuery,
			Body:         string(body),
			RequestID:    request.Header.Get("X-Request-ID"),
			ForwardedFor: request.Header.Get("X-Forwarded-For"),
		}); err != nil {
			panic(err)
		}
	}))
	defer backend.Close()

	handler, cancel := testGateway(t, backend.URL, 100, 100, time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodPut, "https://localhost/api/users/42?active=1", strings.NewReader("payload"))
	request.RemoteAddr = "192.0.2.10:4321"
	request.Header.Set("X-Request-ID", "request-1")
	request.Header.Set("X-Forwarded-For", "203.0.113.99")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") != "request-1" {
		t.Fatalf("response request id = %q", response.Header().Get("X-Request-ID"))
	}

	var received backendRequest
	if err := json.NewDecoder(response.Body).Decode(&received); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if received.Method != http.MethodPut || received.Path != "/42" || received.Query != "active=1" || received.Body != "payload" {
		t.Fatalf("backend request = %#v", received)
	}
	if received.RequestID != "request-1" {
		t.Fatalf("backend request id = %q", received.RequestID)
	}
	if received.ForwardedFor != "192.0.2.10" {
		t.Fatalf("forwarded for = %q", received.ForwardedFor)
	}
}

func TestProxyErrorsAndRateLimit(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	handler, cancel := testGateway(t, backend.URL, 1, 1, time.Second)
	defer cancel()

	tests := []struct {
		name   string
		path   string
		body   string
		length int64
		want   int
	}{
		{name: "not found", path: "/unknown", want: http.StatusNotFound},
		{name: "body too large", path: "/api/users", body: "12345", length: 5, want: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://localhost"+test.path, strings.NewReader(test.body))
			request.RemoteAddr = "192.0.2.20:1234"
			if test.length > 0 {
				request.ContentLength = test.length
			}
			response := httptest.NewRecorder()
			if test.name == "body too large" {
				limited := middleware.BodyLimit(4, handler)
				limited.ServeHTTP(response, request)
			} else {
				handler.ServeHTTP(response, request)
			}
			if response.Code != test.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}

	chunkedRequest := httptest.NewRequest(http.MethodPost, "https://localhost/api/users", strings.NewReader(strings.Repeat("x", 1024*1024+1)))
	chunkedRequest.ContentLength = -1
	chunkedRequest.RemoteAddr = "192.0.2.21:1234"
	chunkedResponse := httptest.NewRecorder()
	handler.ServeHTTP(chunkedResponse, chunkedRequest)
	if chunkedResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked body status = %d, body = %s", chunkedResponse.Code, chunkedResponse.Body.String())
	}

	first := httptest.NewRequest(http.MethodGet, "https://localhost/api/users", nil)
	first.RemoteAddr = "192.0.2.30:1234"
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, first)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first status = %d", firstResponse.Code)
	}
	second := httptest.NewRequest(http.MethodGet, "https://localhost/api/users", nil)
	second.RemoteAddr = first.RemoteAddr
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, second)
	if secondResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d", secondResponse.Code)
	}
}

func TestUnavailableConnectionFailureAndTimeout(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		handler, cancel := testGateway(t, "http://127.0.0.1:1", 100, 100, 50*time.Millisecond)
		defer cancel()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://localhost/api/users", nil))
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("connection failure", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusOK)
		}))
		handler, cancel := testGateway(t, backend.URL, 100, 100, time.Second)
		defer cancel()
		backend.Close()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://localhost/api/users", nil))
		if response.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("timeout", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/health" {
				writer.WriteHeader(http.StatusOK)
				return
			}
			time.Sleep(100 * time.Millisecond)
			writer.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()
		handler, cancel := testGateway(t, backend.URL, 100, 100, 10*time.Millisecond)
		defer cancel()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://localhost/api/users", nil))
		if response.Code != http.StatusGatewayTimeout {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})
}

func TestHealthAndReadiness(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()
	handler, cancel := testGateway(t, backend.URL, 100, 100, time.Second)
	defer cancel()
	for _, path := range []string{"/health", "/ready"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://localhost"+path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, response.Code, response.Body.String())
		}
	}
}

func testGateway(t *testing.T, backendURL string, requestsPerSecond float64, burst int, routeTimeout time.Duration) (http.Handler, context.CancelFunc) {
	t.Helper()
	upstreamManager, err := upstream.New([]config.UpstreamConfig{{
		ID:        "users",
		Endpoints: []string{backendURL},
		Health: config.HealthConfig{
			Path:     "/health",
			Interval: time.Hour,
			Timeout:  50 * time.Millisecond,
		},
	}}, &http.Transport{Proxy: nil})
	if err != nil {
		t.Fatalf("upstream.New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	upstreamManager.Start(ctx)
	routes := []config.RouteConfig{{
		ID:         "users",
		PathPrefix: "/api/users",
		Upstream:   "users",
		Timeout:    routeTimeout,
		RateLimit: config.RateLimitConfig{
			RequestsPerSecond: requestsPerSecond,
			Burst:             burst,
		},
	}}
	proxyHandler := NewHandler(router.New(routes), upstreamManager, middleware.NewLimiterStore(), &http.Transport{Proxy: nil})
	root := gateway.NewHandler(proxyHandler, upstreamManager, []string{"users"})
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	return gateway.Wrap(root, 1024*1024, func(next http.Handler) http.Handler {
		return middleware.AccessLog(logger, next)
	}), cancel
}
