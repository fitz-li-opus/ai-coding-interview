package router

import (
	"net/http"

	"gateway/internal/handler"
	"gateway/internal/middleware"
)

func Setup(h *handler.Handlers, m *middleware.Middlewares) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", h.Health.Check)
	mux.Handle("/", h.Proxy)

	return m.Logger(mux)
}
