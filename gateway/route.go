package main

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type RouteSpec struct {
	Name   string
	Prefix string
	Target *url.URL
}

type RouteTable struct {
	routes []RouteSpec
}

type InvalidRequestError struct {
	Reason string
}

func (err *InvalidRequestError) Error() string {
	return "invalid request path: " + err.Reason
}

func NewRouteTable(routes []RouteSpec) (*RouteTable, error) {
	if len(routes) == 0 {
		return nil, fmt.Errorf("route table must contain at least one route")
	}

	copied := make([]RouteSpec, 0, len(routes))
	names := make(map[string]struct{}, len(routes))
	prefixes := make(map[string]struct{}, len(routes))

	for index, route := range routes {
		trimmedName := strings.TrimSpace(route.Name)
		if trimmedName == "" {
			return nil, fmt.Errorf("route %d: name is required", index)
		}
		if trimmedName != route.Name {
			return nil, fmt.Errorf("route %d: name must not have surrounding whitespace", index)
		}
		if err := validateRoutePrefix(route.Prefix); err != nil {
			return nil, fmt.Errorf("route %q prefix: %w", route.Name, err)
		}
		if route.Target == nil {
			return nil, fmt.Errorf("route %q target is required", route.Name)
		}
		targetPath := route.Target.Path
		if targetPath == "" {
			targetPath = "/"
		}
		if err := ValidateRequestPath(&url.URL{Path: targetPath, RawPath: route.Target.RawPath}); err != nil {
			return nil, fmt.Errorf("route %q target path: %w", route.Name, err)
		}
		target, err := parseURL(route.Target.String())
		if err != nil {
			return nil, fmt.Errorf("route %q target: %w", route.Name, err)
		}
		if _, duplicate := names[route.Name]; duplicate {
			return nil, fmt.Errorf("duplicate route name %q", route.Name)
		}
		if _, duplicate := prefixes[route.Prefix]; duplicate {
			return nil, fmt.Errorf("duplicate route prefix %q", route.Prefix)
		}

		names[route.Name] = struct{}{}
		prefixes[route.Prefix] = struct{}{}
		copied = append(copied, RouteSpec{
			Name:   route.Name,
			Prefix: route.Prefix,
			Target: target,
		})
	}

	sort.SliceStable(copied, func(i, j int) bool {
		return len(copied[i].Prefix) > len(copied[j].Prefix)
	})

	return &RouteTable{routes: copied}, nil
}

func (table *RouteTable) Match(path string) (RouteSpec, string, bool) {
	if table == nil {
		return RouteSpec{}, "", false
	}
	for _, route := range table.routes {
		if path == route.Prefix {
			return cloneRoute(route), "/", true
		}
		if strings.HasPrefix(path, route.Prefix+"/") {
			return cloneRoute(route), path[len(route.Prefix):], true
		}
	}
	return RouteSpec{}, "", false
}

func ValidateRequestPath(requestURL *url.URL) error {
	if requestURL == nil {
		return invalidRequest("URL is required")
	}
	if requestURL.Scheme != "" || requestURL.Host != "" || requestURL.Opaque != "" {
		return invalidRequest("absolute or opaque request targets are not allowed")
	}
	if requestURL.Path == "" || requestURL.Path[0] != '/' {
		return invalidRequest("path must be absolute")
	}
	if requestURL.RawPath != "" {
		decoded, err := url.PathUnescape(requestURL.RawPath)
		if err != nil || decoded != requestURL.Path {
			return invalidRequest("raw path is inconsistent")
		}
	}

	candidate := requestURL.EscapedPath()
	const maxDecodeDepth = 8
	for depth := 0; depth < maxDecodeDepth; depth++ {
		lower := strings.ToLower(candidate)
		if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
			return invalidRequest("encoded path separators are not allowed")
		}

		decoded, err := url.PathUnescape(candidate)
		if err != nil {
			return invalidRequest("path encoding is invalid")
		}
		if strings.IndexByte(decoded, 0) >= 0 || strings.Contains(decoded, `\`) {
			return invalidRequest("unsafe path characters are not allowed")
		}
		if containsDotSegment(decoded) {
			return invalidRequest("dot path segments are not allowed")
		}
		if decoded == candidate {
			return nil
		}
		candidate = decoded
	}

	return invalidRequest("path encoding is too deeply nested")
}

func validateRoutePrefix(prefix string) error {
	if prefix == "" || prefix[0] != '/' {
		return fmt.Errorf("must be an absolute path")
	}
	if prefix == "/" || strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("must not end with a slash")
	}
	if strings.ContainsAny(prefix, "?#") {
		return fmt.Errorf("must not contain a query or fragment")
	}
	if strings.Contains(prefix, "%") {
		return fmt.Errorf("must not contain percent encoding")
	}
	if err := ValidateRequestPath(&url.URL{Path: prefix}); err != nil {
		return err
	}
	return nil
}

func cloneRoute(route RouteSpec) RouteSpec {
	cloned := route
	if route.Target != nil {
		target := *route.Target
		cloned.Target = &target
	}
	return cloned
}

func containsDotSegment(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func invalidRequest(reason string) error {
	return &InvalidRequestError{Reason: reason}
}
