package main

import (
	"net/url"
	"strings"
)

type Route struct {
	Name        string
	Prefix      string
	UpstreamURL *url.URL
}

func routesFromConfig(cfg Config) []Route {
	return []Route{
		{Name: "users", Prefix: "/api/users", UpstreamURL: cfg.UserServiceURL},
		{Name: "orders", Prefix: "/api/orders", UpstreamURL: cfg.OrderServiceURL},
		{Name: "products", Prefix: "/api/products", UpstreamURL: cfg.ProductServiceURL},
	}
}

func matchRoute(routes []Route, path string) (Route, bool) {
	for _, route := range routes {
		if path == route.Prefix || strings.HasPrefix(path, route.Prefix+"/") {
			return route, true
		}
	}
	return Route{}, false
}

func rewritePath(route Route, path string) string {
	suffix := strings.TrimPrefix(path, route.Prefix)
	if suffix == "" {
		return "/"
	}
	return suffix
}
