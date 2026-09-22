package router

import (
	"net"
	"net/http"
	"strings"

	"ai-coding-interview/gateway/internal/config"
)

type Router struct {
	routes []config.RouteConfig
}

func New(routes []config.RouteConfig) *Router {
	return &Router{routes: routes}
}

func (r *Router) Match(request *http.Request) *config.RouteConfig {
	host := requestHost(request.Host)
	path := request.URL.Path
	var selected *config.RouteConfig
	selectedHostSpecific := false
	selectedPrefixLength := -1

	for i := range r.routes {
		route := &r.routes[i]
		hostSpecific, hostMatches := matchHost(route.Hosts, host)
		if !hostMatches || !matchPath(route.PathPrefix, path) {
			continue
		}
		prefixLength := len(route.PathPrefix)
		if selected == nil || (hostSpecific && !selectedHostSpecific) || (hostSpecific == selectedHostSpecific && prefixLength > selectedPrefixLength) {
			selected = route
			selectedHostSpecific = hostSpecific
			selectedPrefixLength = prefixLength
		}
	}
	return selected
}

func StripPrefix(path, prefix string) string {
	if prefix == "/" {
		return path
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

func matchHost(hosts []string, requestHost string) (bool, bool) {
	if len(hosts) == 0 {
		return false, true
	}
	for _, host := range hosts {
		if host == requestHost {
			return true, true
		}
	}
	return true, false
}

func matchPath(prefix, path string) bool {
	if prefix == "/" {
		return strings.HasPrefix(path, "/")
	}
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

func requestHost(value string) string {
	host := value
	if parsed, _, err := net.SplitHostPort(value); err == nil {
		host = parsed
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	return strings.TrimSuffix(strings.ToLower(host), ".")
}
