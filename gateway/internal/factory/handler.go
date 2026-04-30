package factory

import (
	"gateway/internal/handler"
	"gateway/internal/service"
)

func NewHandler(services *service.Services) *handler.Handlers {
	return &handler.Handlers{
		Health: handler.NewHealthHandler(),
		Proxy:  handler.NewProxyHandler(services.Routing),
	}
}
