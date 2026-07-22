package main

import (
	"bytes"
	"io"
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

// responseRecorder captures the response for caching.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

// ServeHTTP handles incoming requests.
func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get client IP
	clientIP := r.RemoteAddr

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

	// Modify the request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}

	// For GET requests, capture response for caching
	if r.Method == http.MethodGet {
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

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
		proxy.ServeHTTP(recorder, r)
	} else {
		proxy.ServeHTTP(w, r)
	}
}
