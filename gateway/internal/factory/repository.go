package factory

import (
	"fmt"
	"time"

	"gateway/internal/config"
	"gateway/internal/repository"
	httpbackend "gateway/internal/repository/http"
)

type RepoFactory struct {
	Backends map[string]repository.Backend
}

// NewRepoFactory builds one Backend per route declared in config. Routes are
// keyed by Name so the service layer can resolve them when wiring routing
// rules.
func NewRepoFactory(cfg *config.Config) (*RepoFactory, error) {
	backends := make(map[string]repository.Backend, len(cfg.Routes))

	for _, route := range cfg.Routes {
		backend, err := newBackend(route.Name, route.Target, cfg.UpstreamTimeout)
		if err != nil {
			return nil, fmt.Errorf("init backend %s: %w", route.Name, err)
		}
		backends[route.Name] = backend
	}

	return &RepoFactory{Backends: backends}, nil
}

// newBackend dispatches to the per-service constructor so each microservice
// keeps a dedicated entry point — that's where future per-service
// customisation (auth headers, retry policy) will live.
func newBackend(name, target string, timeout time.Duration) (repository.Backend, error) {
	switch name {
	case "user-service":
		return httpbackend.NewUserServiceClient(target, timeout)
	case "order-service":
		return httpbackend.NewOrderServiceClient(target, timeout)
	case "product-service":
		return httpbackend.NewProductServiceClient(target, timeout)
	default:
		return nil, fmt.Errorf("unknown backend %q", name)
	}
}
