package router

import (
	"sort"
	"strings"

	"gateway/internal/config"
)

// Router resolves an incoming request path/method to a configured route.
type Router struct {
	routes []*config.Route
}

// New builds a router and orders routes by prefix length descending so the
// longest (most specific) prefix wins.
func New(routes []*config.Route) *Router {
	sorted := make([]*config.Route, len(routes))
	copy(sorted, routes)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].Match.Prefix) > len(sorted[j].Match.Prefix)
	})
	return &Router{routes: sorted}
}

// Match finds the route for method+path.
//
// It returns:
//   - route, true, nil            when a route matches and the method is allowed
//   - route, false, allowedMethods when the path matches but the method is not allowed
//   - nil, false, nil             when no route matches the path
func (r *Router) Match(method, path string) (*config.Route, bool, []string) {
	for _, route := range r.routes {
		if !hasPrefix(path, route.Match.Prefix) {
			continue
		}
		if len(route.Match.Methods) == 0 {
			return route, true, nil
		}
		for _, m := range route.Match.Methods {
			if strings.EqualFold(m, method) {
				return route, true, nil
			}
		}
		return route, false, allowedMethods(route)
	}
	return nil, false, nil
}

func allowedMethods(route *config.Route) []string {
	out := make([]string, 0, len(route.Match.Methods))
	for _, m := range route.Match.Methods {
		out = append(out, strings.ToUpper(m))
	}
	sort.Strings(out)
	return out
}

// hasPrefix reports whether path is prefix or a sub-path of prefix,
// matching on path-segment boundaries.
func hasPrefix(path, prefix string) bool {
	normalized := strings.TrimSuffix(prefix, "/")
	if normalized == "" {
		return true
	}
	if path == normalized || path == prefix {
		return true
	}
	return strings.HasPrefix(path, normalized+"/")
}

// Rewrite applies the route's strip_prefix to path.
func Rewrite(route *config.Route, path string) string {
	strip := route.StripPrefix
	if strip == "" || strip == "/" {
		return path
	}
	if path == strip {
		return "/"
	}
	if strings.HasPrefix(path, strings.TrimSuffix(strip, "/")+"/") {
		return path[len(strings.TrimSuffix(strip, "/")):]
	}
	return path
}

// Routes returns the configured routes in match order.
func (r *Router) Routes() []*config.Route { return r.routes }
