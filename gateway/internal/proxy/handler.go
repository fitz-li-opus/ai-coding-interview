package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"

	"ai-coding-interview/gateway/internal/config"
	"ai-coding-interview/gateway/internal/middleware"
	"ai-coding-interview/gateway/internal/router"
	"ai-coding-interview/gateway/internal/upstream"
)

type selectionKey struct{}

type selection struct {
	route    *config.RouteConfig
	endpoint *upstream.Endpoint
}

type Handler struct {
	router    *router.Router
	upstreams *upstream.Manager
	limiters  *middleware.LimiterStore
	proxy     *httputil.ReverseProxy
}

func NewHandler(routeMatcher *router.Router, upstreams *upstream.Manager, limiters *middleware.LimiterStore, transport http.RoundTripper) *Handler {
	handler := &Handler{
		router:    routeMatcher,
		upstreams: upstreams,
		limiters:  limiters,
	}
	handler.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite:   handler.rewrite,
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			handler.handleProxyError(writer, request, err)
		},
	}
	return handler
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	matchedRoute := h.router.Match(request)
	if matchedRoute == nil {
		middleware.WriteError(writer, request, http.StatusNotFound, "route_not_found", "no route matched the request")
		return
	}
	state := middleware.State(request)
	if state != nil {
		state.Route = matchedRoute.ID
	}

	clientIP := middleware.ClientIP(request)
	if !h.limiters.Allow(matchedRoute.ID, clientIP, matchedRoute.RateLimit.RequestsPerSecond, matchedRoute.RateLimit.Burst) {
		writer.Header().Set("Retry-After", "1")
		middleware.WriteError(writer, request, http.StatusTooManyRequests, "rate_limited", "request rate limit exceeded")
		return
	}

	endpoint, ok := h.upstreams.Pick(matchedRoute.Upstream)
	if !ok {
		middleware.WriteError(writer, request, http.StatusServiceUnavailable, "upstream_unavailable", "no healthy upstream endpoint")
		return
	}
	if state != nil {
		state.Upstream = endpoint.URL.Host
	}

	ctx, cancel := context.WithTimeout(request.Context(), matchedRoute.Timeout)
	defer cancel()
	ctx = context.WithValue(ctx, selectionKey{}, selection{route: matchedRoute, endpoint: endpoint})

	proxyRequest := request.Clone(ctx)
	proxyURL := *request.URL
	proxyURL.Path = router.StripPrefix(request.URL.Path, matchedRoute.PathPrefix)
	proxyURL.RawPath = ""
	proxyRequest.URL = &proxyURL
	h.proxy.ServeHTTP(writer, proxyRequest)
}

func (h *Handler) rewrite(request *httputil.ProxyRequest) {
	selected, ok := request.In.Context().Value(selectionKey{}).(selection)
	if !ok {
		panic("proxy selection is missing")
	}
	request.SetURL(selected.endpoint.URL)
	request.Out.Host = selected.endpoint.URL.Host
	request.SetXForwarded()
	if state := middleware.State(request.In); state != nil {
		request.Out.Header.Set("X-Request-ID", state.RequestID)
	}
}

func (h *Handler) handleProxyError(writer http.ResponseWriter, request *http.Request, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		middleware.WriteError(writer, request, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the configured limit")
		return
	}

	selected, _ := request.Context().Value(selectionKey{}).(selection)
	if request.Context().Err() == nil && selected.endpoint != nil {
		h.upstreams.MarkUnhealthy(selected.route.Upstream, selected.endpoint)
	}
	if errors.Is(request.Context().Err(), context.DeadlineExceeded) {
		middleware.WriteError(writer, request, http.StatusGatewayTimeout, "gateway_timeout", "upstream request timed out")
		return
	}
	middleware.WriteError(writer, request, http.StatusBadGateway, "upstream_error", "upstream request failed")
}
