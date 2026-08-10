package main

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

func TestGatewayRoutesToConfiguredServices(t *testing.T) {
	user := echoUpstream(t, "user-service")
	order := echoUpstream(t, "order-service")
	product := echoUpstream(t, "product-service")

	handler := mustNewHandler(t, testConfig(t, user.URL, order.URL, product.URL))
	tests := []struct {
		path     string
		wantBody string
	}{
		{path: "/api/users", wantBody: "user-service:/"},
		{path: "/api/users/42", wantBody: "user-service:/42"},
		{path: "/api/orders/1001", wantBody: "order-service:/1001"},
		{path: "/api/products/3", wantBody: "product-service:/3"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
			}
			if got := strings.TrimSpace(response.Body.String()); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestGatewayPreservesRequestAndUpstreamResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("reading upstream request: %v", err)
		}
		if request.Method != http.MethodPost {
			t.Errorf("upstream method = %q, want POST", request.Method)
		}
		if request.URL.Path != "/42" || request.URL.RawQuery != "expand=roles" {
			t.Errorf("upstream URL = %q?%s, want /42?expand=roles", request.URL.Path, request.URL.RawQuery)
		}
		if string(body) != `{"name":"Alice"}` {
			t.Errorf("upstream body = %q", body)
		}
		if request.Header.Get("X-Client-Version") != "interview" {
			t.Errorf("upstream X-Client-Version = %q", request.Header.Get("X-Client-Version"))
		}
		if request.Header.Get("X-Forwarded-For") == "spoofed" {
			t.Error("untrusted X-Forwarded-For reached upstream")
		}

		w.Header().Set("X-Upstream", "user-service")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"accepted":true}`))
	}))
	t.Cleanup(upstream.Close)

	handler := mustNewHandler(t, testConfig(t, upstream.URL, upstream.URL, upstream.URL))
	request := httptest.NewRequest(http.MethodPost, "/api/users/42?expand=roles", strings.NewReader(`{"name":"Alice"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Client-Version", "interview")
	request.Header.Set("X-Forwarded-For", "spoofed")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	if response.Header().Get("X-Upstream") != "user-service" {
		t.Errorf("X-Upstream = %q", response.Header().Get("X-Upstream"))
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"accepted":true}` {
		t.Errorf("body = %q", got)
	}
}

func TestGatewayOwnEndpointsAndErrors(t *testing.T) {
	upstream := echoUpstream(t, "service")
	cfg := testConfig(t, upstream.URL, upstream.URL, upstream.URL)
	cfg.MaxRequestBody = 4
	handler := mustNewHandler(t, cfg)

	tests := []struct {
		name       string
		request    *http.Request
		wantStatus int
		wantCode   string
	}{
		{name: "health", request: httptest.NewRequest(http.MethodGet, "/health", nil), wantStatus: http.StatusOK},
		{name: "health method", request: httptest.NewRequest(http.MethodPost, "/health", nil), wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed"},
		{name: "not found", request: httptest.NewRequest(http.MethodGet, "/api/unknown", nil), wantStatus: http.StatusNotFound, wantCode: "route_not_found"},
		{name: "route lookalike", request: httptest.NewRequest(http.MethodGet, "/api/users-v2", nil), wantStatus: http.StatusNotFound, wantCode: "route_not_found"},
		{name: "body too large", request: httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader("12345")), wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, tt.request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantCode != "" {
				var body struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatalf("error response is not JSON: %v", err)
				}
				if body.Error != tt.wantCode {
					t.Errorf("error code = %q, want %q", body.Error, tt.wantCode)
				}
			}
		})
	}
}

func TestGatewayMapsUpstreamFailures(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		cfg := testConfig(t, "http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")
		handler := mustNewHandler(t, cfg)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/users", nil))
		assertJSONError(t, response, http.StatusBadGateway, "bad_gateway")
	})

	t.Run("timeout", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(upstream.Close)
		cfg := testConfig(t, upstream.URL, upstream.URL, upstream.URL)
		cfg.UpstreamTimeout = 5 * time.Millisecond
		handler := mustNewHandler(t, cfg)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/users", nil))
		assertJSONError(t, response, http.StatusGatewayTimeout, "gateway_timeout")
	})
}

func echoUpstream(t *testing.T, service string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = w.Write([]byte(service + ":" + request.URL.Path))
	}))
	t.Cleanup(server.Close)
	return server
}

func testConfig(t *testing.T, user, order, product string) Config {
	t.Helper()
	return Config{
		Address:           ":8080",
		UserServiceURL:    mustParseURL(t, user),
		OrderServiceURL:   mustParseURL(t, order),
		ProductServiceURL: mustParseURL(t, product),
		UpstreamTimeout:   time.Second,
		MaxRequestBody:    1 << 20,
	}
}

func mustNewHandler(t *testing.T, cfg Config) http.Handler {
	t.Helper()
	handler, err := NewHandler(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func assertJSONError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, status, response.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("error response is not JSON: %v", err)
	}
	if body.Error != code {
		t.Errorf("error code = %q, want %q", body.Error, code)
	}
}
