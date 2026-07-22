package main

import (
	"strings"
)

// Router matches URL paths to service names.
type Router struct {
	routes map[string]string // prefix -> service name
}

// NewRouter creates a new Router.
func NewRouter() *Router {
	return &Router{
		routes: make(map[string]string),
	}
}

// AddRoute registers a path prefix to a service.
func (r *Router) AddRoute(prefix, service string) {
	r.routes[prefix] = service
}

// Match finds the service for a given path.
// Returns the service name and true if found, empty string and false otherwise.
func (r *Router) Match(path string) (string, bool) {
	// Find longest matching prefix
	var matchedPrefix string
	var matchedService string

	for prefix, service := range r.routes {
		if strings.HasPrefix(path, prefix) {
			if len(prefix) > len(matchedPrefix) {
				matchedPrefix = prefix
				matchedService = service
			}
		}
	}

	if matchedPrefix == "" {
		return "", false
	}
	return matchedService, true
}
