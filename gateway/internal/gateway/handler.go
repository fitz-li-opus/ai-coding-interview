package gateway

import (
	"encoding/json"
	"net/http"

	"ai-coding-interview/gateway/internal/middleware"
	"ai-coding-interview/gateway/internal/upstream"
)

type Handler struct {
	proxy       http.Handler
	upstreams   *upstream.Manager
	upstreamIDs []string
}

type healthResponse struct {
	Status    string                 `json:"status"`
	Service   string                 `json:"service"`
	Upstreams []upstream.GroupStatus `json:"upstreams,omitempty"`
}

func NewHandler(proxyHandler http.Handler, upstreams *upstream.Manager, upstreamIDs []string) *Handler {
	return &Handler{
		proxy:       proxyHandler,
		upstreams:   upstreams,
		upstreamIDs: upstreamIDs,
	}
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/health":
		writeJSON(writer, http.StatusOK, healthResponse{Status: "healthy", Service: "api-gateway"})
	case "/ready":
		if h.upstreams.Ready(h.upstreamIDs) {
			writeJSON(writer, http.StatusOK, healthResponse{
				Status:    "ready",
				Service:   "api-gateway",
				Upstreams: h.upstreams.Status(h.upstreamIDs),
			})
			return
		}
		writeJSON(writer, http.StatusServiceUnavailable, healthResponse{
			Status:    "unavailable",
			Service:   "api-gateway",
			Upstreams: h.upstreams.Status(h.upstreamIDs),
		})
	default:
		h.proxy.ServeHTTP(writer, request)
	}
}

func Wrap(handler http.Handler, maxBodyBytes int64, loggerMiddleware func(http.Handler) http.Handler) http.Handler {
	handler = middleware.BodyLimit(maxBodyBytes, handler)
	handler = middleware.SecurityHeaders(handler)
	handler = loggerMiddleware(handler)
	return middleware.RequestID(handler)
}

func writeJSON(writer http.ResponseWriter, status int, body healthResponse) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(body); err != nil {
		panic(err)
	}
}
