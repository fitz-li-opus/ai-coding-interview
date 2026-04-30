package factory

import (
	"fmt"

	"gateway/internal/config"
	"gateway/internal/service"
)

func NewService(cfg *config.Config, repos *RepoFactory) (*service.Services, error) {
	routes := make([]service.Route, 0, len(cfg.Routes))
	for _, r := range cfg.Routes {
		backend, ok := repos.Backends[r.Name]
		if !ok {
			return nil, fmt.Errorf("no backend registered for %s", r.Name)
		}
		routes = append(routes, service.Route{
			PathPrefix: r.PathPrefix,
			Backend:    backend,
		})
	}

	return &service.Services{
		Routing: service.NewRoutingService(routes),
	}, nil
}
