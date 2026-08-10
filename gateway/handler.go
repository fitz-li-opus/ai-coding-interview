package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"time"
)

type gatewayHandler struct {
	routes          *RouteTable
	proxies         map[string]*httputil.ReverseProxy
	upstreamTimeout time.Duration
	maxRequestBody  int64
}

func NewHandler(cfg Config, logger *log.Logger) (http.Handler, error) {
	if cfg.UpstreamTimeout <= 0 || cfg.UpstreamTimeout > maxUpstreamTimeout {
		return nil, fmt.Errorf("upstream timeout must be greater than zero and at most %s", maxUpstreamTimeout)
	}
	if cfg.MaxRequestBody <= 0 || cfg.MaxRequestBody > maxRequestBodyLimit {
		return nil, fmt.Errorf("maximum request body must be between 1 and %d bytes", maxRequestBodyLimit)
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	routes := []RouteSpec{
		{Name: "user-service", Prefix: "/api/users", Target: cfg.UserServiceURL},
		{Name: "order-service", Prefix: "/api/orders", Target: cfg.OrderServiceURL},
		{Name: "product-service", Prefix: "/api/products", Target: cfg.ProductServiceURL},
	}
	table, err := NewRouteTable(routes)
	if err != nil {
		return nil, fmt.Errorf("building route table: %w", err)
	}

	proxies := make(map[string]*httputil.ReverseProxy, len(routes))
	for _, route := range routes {
		proxies[route.Name] = newRouteProxy(route, logger)
	}

	return &gatewayHandler{
		routes:          table,
		proxies:         proxies,
		upstreamTimeout: cfg.UpstreamTimeout,
		maxRequestBody:  cfg.MaxRequestBody,
	}, nil
}

func (gateway *gatewayHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if err := ValidateRequestPath(request.URL); err != nil {
		writeJSONError(writer, http.StatusBadRequest, "invalid_request", "request path is invalid")
		return
	}

	if request.URL.Path == "/health" {
		gateway.serveHealth(writer, request)
		return
	}

	route, upstreamPath, matched := gateway.routes.Match(request.URL.Path)
	if !matched {
		writeJSONError(writer, http.StatusNotFound, "route_not_found", "no route matches the requested path")
		return
	}
	if request.ContentLength > gateway.maxRequestBody {
		writeJSONError(writer, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the configured limit")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), gateway.upstreamTimeout)
	defer cancel()
	proxiedRequest := request.Clone(ctx)
	proxiedRequest.URL.Path = upstreamPath
	proxiedRequest.URL.RawPath = ""
	proxiedRequest.Body = http.MaxBytesReader(writer, request.Body, gateway.maxRequestBody)

	gateway.proxies[route.Name].ServeHTTP(writer, proxiedRequest)
}

func (gateway *gatewayHandler) serveHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeJSONError(writer, http.StatusMethodNotAllowed, "method_not_allowed", "health endpoint only supports GET")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{
		"status":  "healthy",
		"service": "api-gateway",
	})
}

func writeJSONError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]string{
		"error":   code,
		"message": message,
	})
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
