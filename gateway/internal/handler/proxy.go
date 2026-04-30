package handler

import (
	"net/http"

	"gateway/internal/service"
)

type ProxyHandler struct {
	routing *service.RoutingService
}

func NewProxyHandler(routing *service.RoutingService) *ProxyHandler {
	return &ProxyHandler{routing: routing}
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.routing.Forward(w, r) {
		http.NotFound(w, r)
	}
}
