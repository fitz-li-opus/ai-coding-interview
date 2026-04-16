package router

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"gateway/config"
	"gateway/proxy"
)

type Router struct {
	routes []route
}

type route struct {
	prefix string
	proxy  *proxy.ReverseProxy
}

func New(routes []config.RouteConfig) (*Router, error) {
	r := &Router{}

	for _, rc := range routes {
		p, err := proxy.New(rc.Upstream, rc.Prefix, rc.StripPrefix)
		if err != nil {
			return nil, err
		}
		r.routes = append(r.routes, route{
			prefix: rc.Prefix,
			proxy:  p,
		})
	}

	// Sort by prefix length descending for longest match first
	sort.Slice(r.routes, func(i, j int) bool {
		return len(r.routes[i].prefix) > len(r.routes[j].prefix)
	})

	return r, nil
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, rt := range r.routes {
		if strings.HasPrefix(req.URL.Path, rt.prefix) {
			rt.proxy.ServeHTTP(w, req)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]any{
		"error":  "not found",
		"status": http.StatusNotFound,
	})
}
