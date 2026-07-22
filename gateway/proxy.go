package main

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// ProxyHandler handles reverse proxying to backend services.
type ProxyHandler struct {
	router  *Router
	lb      *LoadBalancer
	cache   *Cache
	limiter *RateLimiter
}

// NewProxyHandler creates a new ProxyHandler.
func NewProxyHandler(router *Router, lb *LoadBalancer, cache *Cache, limiter *RateLimiter) *ProxyHandler {
	return &ProxyHandler{
		router:  router,
		lb:      lb,
		cache:   cache,
		limiter: limiter,
	}
}

// getClientIP extracts the client IP from the request.
// It checks X-Forwarded-For header first (for proxy scenarios),
// then falls back to RemoteAddr.
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := len(xff); idx > 0 {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Fall back to RemoteAddr, strip port
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ServeHTTP handles incoming requests.
func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get client IP
	clientIP := getClientIP(r)

	// Rate limiting
	if !p.limiter.Allow(clientIP) {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Check cache for GET requests
	cacheKey := r.URL.Path + "?" + r.URL.RawQuery
	if r.Method == http.MethodGet {
		if entry, found := p.cache.Get(cacheKey); found {
			// Cache hit
			for key, values := range entry.Headers {
				for _, v := range values {
					w.Header().Add(key, v)
				}
			}
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(entry.StatusCode)
			w.Write(entry.Body)
			return
		}
	}

	// Route matching
	service, found := p.router.Match(r.URL.Path)
	if !found {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	// Load balancing
	backendURL := p.lb.Next(service)
	if backendURL == "" {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Parse backend URL
	target, err := url.Parse(backendURL)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Modify the request to add X-Forwarded headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host

		// Add X-Forwarded headers
		if prior := req.Header.Get("X-Forwarded-For"); prior != "" {
			req.Header.Set("X-Forwarded-For", prior+", "+clientIP)
		} else {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
		req.Header.Set("X-Forwarded-Host", r.Host)
		if r.TLS != nil {
			req.Header.Set("X-Forwarded-Proto", "https")
		} else {
			req.Header.Set("X-Forwarded-Proto", "http")
		}
	}

	// For GET requests, capture response for caching
	if r.Method == http.MethodGet {
		proxy.ModifyResponse = func(resp *http.Response) error {
			// Read the body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			resp.Body.Close()

			// Cache the response
			p.cache.Set(cacheKey, resp.StatusCode, resp.Header, body)

			// Put the body back
			resp.Body = io.NopCloser(bytes.NewReader(body))
			return nil
		}

		w.Header().Set("X-Cache", "MISS")
	}

	proxy.ServeHTTP(w, r)
}
