package service

import (
	"net/http"
	"strings"

	"gateway/internal/repository"
)

type Route struct {
	PathPrefix string
	Backend    repository.Backend
}

type RoutingService struct {
	routes []Route
}

func NewRoutingService(routes []Route) *RoutingService {
	return &RoutingService{routes: routes}
}

// Forward picks the backend whose PathPrefix matches r.URL.Path and proxies to
// it. Returns false if no route matches so the caller can decide on the
// response shape (e.g. 404).
func (s *RoutingService) Forward(w http.ResponseWriter, r *http.Request) bool {
	backend, ok := s.lookup(r.URL.Path)
	if !ok {
		return false
	}
	backend.ServeHTTP(w, r)
	return true
}

func (s *RoutingService) lookup(path string) (repository.Backend, bool) {
	for _, route := range s.routes {
		if matchPrefix(path, route.PathPrefix) {
			return route.Backend, true
		}
	}
	return nil, false
}

// matchPrefix returns true when path equals prefix or extends it on a path
// boundary (so "/api/users" or "/api/users/123" matches "/api/users", but
// "/api/usersx" does not).
func matchPrefix(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if len(path) == len(prefix) {
		return true
	}
	return path[len(prefix)] == '/'
}
