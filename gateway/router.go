package main

import (
	"sort"
	"strings"
)

// Router resolves a request path to a route using longest-prefix-wins.
type Router struct {
	routes []*Route
}

// NewRouter sorts routes by descending prefix length so the first match found
// by a linear scan is also the most specific one.
func NewRouter(routes []*Route) *Router {
	sorted := make([]*Route, len(routes))
	copy(sorted, routes)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Prefix) > len(sorted[j].Prefix)
	})
	return &Router{routes: sorted}
}

func (r *Router) Routes() []*Route { return r.routes }

func (r *Router) Match(path string) *Route {
	for _, rt := range r.routes {
		if matchPrefix(path, rt.Prefix) {
			return rt
		}
	}
	return nil
}

// matchPrefix requires the prefix to end on a path-segment boundary, so
// /api/usersfoo does not match the /api/users route.
func matchPrefix(path, prefix string) bool {
	if prefix == "/" {
		return strings.HasPrefix(path, "/")
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	return len(path) == len(prefix) || path[len(prefix)] == '/'
}

// stripPrefix removes the route prefix and always leaves an absolute path,
// so /api/users maps to / rather than the empty string.
func stripPrefix(path, prefix string) string {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || rest[0] != '/' {
		return "/" + rest
	}
	return rest
}
